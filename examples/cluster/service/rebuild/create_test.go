package rebuild_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	// The application registers its models, services, jobs, leader work and
	// locks through the init of these packages, exactly as main.go imports
	// them.
	_ "cluster/configx"
	_ "cluster/cronjob"
	_ "cluster/leader"
	_ "cluster/lock"
	_ "cluster/middleware"
	"cluster/model"
	_ "cluster/module"
	"cluster/router"
	_ "cluster/service"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Routes:   router.Init,
	})
}

// TestCreateRunsOnceAtATime proves the lock behind the action: of two
// rebuilds requested at once, one runs and the other is refused with 409
// right away, and once the first is done the next request runs again. Every
// run leaves one lock event behind.
func TestCreateRunsOnceAtATime(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, postErr := cli.Post[model.RebuildRsp]("/api/rebuilds", model.RebuildReq{Seconds: 2})
			results <- postErr
		})
	}
	wg.Wait()
	close(results)

	ran, refused := 0, 0
	for err := range results {
		var refusal *client.Error
		switch {
		case err == nil:
			ran++
		case errors.As(err, &refusal) && refusal.StatusCode == http.StatusConflict:
			refused++
		default:
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, ran, "exactly one of two concurrent rebuilds runs")
	require.Equal(t, 1, refused, "the other is refused at once with 409")

	rsp, err := cli.Post[model.RebuildRsp]("/api/rebuilds", model.RebuildReq{Seconds: 1})
	require.NoError(t, err, "the lock is free again once the rebuild returned")
	require.Equal(t, 1, rsp.Seconds)

	runs := 0
	require.NoError(t, database.Database[*model.Event](context.Background()).WithQuery(&model.Event{Kind: "lock"}).Count(&runs))
	require.Equal(t, 2, runs, "every run under the lock leaves one event")
}
