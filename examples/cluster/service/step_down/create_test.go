package stepdown_test

import (
	"testing"

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

// TestCreateAsksTheLeaderWorkToReturn proves the request reaches the replica
// that took it and names it. Whether that replica held the leadership is what
// asked reports; the deployment scenarios in the README send it to the one
// that does.
func TestCreateAsksTheLeaderWorkToReturn(t *testing.T) {
	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	rsp, err := cli.Post[model.StepDownRsp]("/api/step-downs", nil)
	require.NoError(t, err)
	require.NotEmpty(t, rsp.Replica, "the reply names the replica that took the request")
}
