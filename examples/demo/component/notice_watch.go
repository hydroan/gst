package component

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/logger"
	gstredis "github.com/hydroan/gst/redis"
	"go.uber.org/zap"
)

// noticeChannel is the redis channel the notices arrive on.
const noticeChannel = "demo:notices"

// watchNotices consumes the notices published on a redis channel for as
// long as the process runs: the shape of a component, a loop that returns
// when ctx ends. Returning earlier, with an error or without, fails the
// process, which is what a client that cannot be built or a subscription
// that closed deserve: the process has lost its feed. Every replica
// subscribes for itself, so a notice reaches all of them; redis-cli PUBLISH
// demo:notices hello shows one arriving in the log.
func watchNotices(ctx context.Context) error {
	client, err := gstredis.Client()
	if err != nil {
		return err
	}
	subscription := client.Subscribe(ctx, noticeChannel)
	defer subscription.Close()
	notices := subscription.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case notice, ok := <-notices:
			if !ok {
				return errors.New("the notice subscription closed")
			}
			logger.App.Infoz("notice received", zap.String("channel", notice.Channel), zap.String("payload", notice.Payload))
		}
	}
}
