package cached_test

import (
	"testing"

	// The application registers its models, services, jobs, leader work and
	// locks through the init of these packages, exactly as main.go imports
	// them.
	_ "cluster/component"
	_ "cluster/configx"
	_ "cluster/cronjob"
	_ "cluster/leader"
	_ "cluster/lock"
	_ "cluster/middleware"
	"cluster/model"
	_ "cluster/module"
	"cluster/router"
	_ "cluster/service"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBMySQL,
		Kafka:    true,
		Routes:   router.Init,
	})
}

// TestCreateCachesTheEntry proves the write path of the replicated cache: the
// entry lands in the store of the replica that answered, which is what its
// peers then catch up with. The propagation between replicas is what the
// deployment scenarios in the README check; one process can only show that
// the entry was cached at all.
func TestCreateCachesTheEntry(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	rsp, err := cli.Post[model.CachedRsp](t.Context(), "/api/caches", model.CachedReq{Key: "created-key", Value: "created-value"})
	require.NoError(t, err)
	require.Equal(t, "created-key", rsp.Key)
	require.NotEmpty(t, rsp.Replica, "the reply names the replica that cached the entry")

	read, err := cli.Get[model.CachedRsp](t.Context(), "/api/caches/created-key")
	require.NoError(t, err)
	require.True(t, read.Found, "the entry is in the store of the replica that wrote it")
	require.Equal(t, "created-value", read.Value)
}
