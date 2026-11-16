package notice_test

import (
	"net/http"
	"testing"

	"cluster/helper"
	"cluster/internal/testsupport"
	"cluster/model"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/testutil/iamtest"
	"github.com/stretchr/testify/require"
)

// TestCreate covers POST /api/notices, served by Creator in create.go: the
// notice is published and the reply names the replica that published it. Who
// receives it is what TestWatch covers.
func TestCreate(t *testing.T) {
	t.Run("publishes the notice", func(t *testing.T) {
		cli := iamtest.Login(t, testsupport.Root).Client

		rsp, err := cli.Post[model.NoticeCreateRsp](t.Context(), "/api/notices", model.NoticeReq{Text: "published"})
		require.NoError(t, err)
		require.Equal(t, "published", rsp.Text)
		require.Equal(t, helper.Replica(), rsp.Replica, "the reply names the replica that published the notice")
	})

	t.Run("without a text", func(t *testing.T) {
		cli := iamtest.Login(t, testsupport.Root).Client

		_, err := cli.Post[model.NoticeCreateRsp](t.Context(), "/api/notices", model.NoticeReq{})
		testutil.RequireError(t, err, http.StatusBadRequest, "a text is required")
	})

	t.Run("without a session", func(t *testing.T) {
		cli, err := client.New(testutil.BaseURL())
		require.NoError(t, err)

		_, err = cli.Post[model.NoticeCreateRsp](t.Context(), "/api/notices", model.NoticeReq{Text: "anonymous"})
		testutil.RequireError(t, err, http.StatusUnauthorized)
	})
}
