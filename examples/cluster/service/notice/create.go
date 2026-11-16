package notice

import (
	"encoding/json"
	"net/http"

	"cluster/helper"
	"cluster/model"

	"github.com/hydroan/gst"
	gstredis "github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/service"
)

// channel is the redis channel a notice travels on, from the replica that
// publishes it to the connections watching on every replica.
const channel = "cluster:notices"

// notice is what travels on the channel.
type notice struct {
	Text string `json:"text"`
	From string `json:"from"` // the replica that published it
}

type Creator struct {
	service.Base[*model.Notice, *model.NoticeReq, *model.NoticeCreateRsp]
}

// Create publishes the notice on the redis channel every watching connection
// is subscribed to, so it reaches the clients of every replica and not only
// those of the replica that took the request. The reply names that replica
// and says how many connections redis handed the notice to.
func (c *Creator) Create(ctx *gst.ServiceContext, req *model.NoticeReq) (*model.NoticeCreateRsp, error) {
	if req.Text == "" {
		return nil, gst.NewError(http.StatusBadRequest, "a text is required")
	}
	client, err := gstredis.Client()
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "the notice was not published", err)
	}
	published, err := json.Marshal(notice{Text: req.Text, From: helper.Replica()})
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "the notice was not published", err)
	}
	watchers, err := client.Publish(ctx, channel, published).Result()
	if err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "the notice was not published", err)
	}
	return &model.NoticeCreateRsp{Replica: helper.Replica(), Text: req.Text, Watchers: watchers}, nil
}
