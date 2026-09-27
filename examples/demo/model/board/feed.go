package board

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Feed is the streaming half of the gRPC example, served over gRPC alone: a
// Stream action carries a stream of messages on one side of the call or on
// both, which HTTP cannot, so a model declaring one needs GRPC(). WatchFeed
// answers one request with a stream of events, UploadFeed takes a stream of
// events and answers once, ChatFeed streams both ways. A Stream action names
// its rpc with Service("name") and always has service code, in service/board/feed;
// the router registers nothing for it.
type Feed struct {
	model.Empty
}

// FeedWatchReq names the topic to watch.
type FeedWatchReq struct {
	Topic string `json:"topic" pb:"1"`
}

// FeedEvent is one event of a feed.
type FeedEvent struct {
	Seq  int64  `json:"seq" pb:"1"`
	Body string `json:"body" pb:"2"`
}

// FeedUploadRsp counts the events a client streamed in.
type FeedUploadRsp struct {
	Accepted int64 `json:"accepted" pb:"1"`
}

func (Feed) Design() {
	GRPC()

	Route("board/feeds/watch", func() {
		Stream(func() {
			Public()
			Service("watch")
			Payload[*FeedWatchReq]()
			StreamingResult[*FeedEvent]()
		})
	})
	Route("board/feeds/upload", func() {
		Stream(func() {
			Service("upload")
			StreamingPayload[*FeedEvent]()
			Result[*FeedUploadRsp]()
		})
	})
	Route("board/feeds/chat", func() {
		Stream(func() {
			Service("chat")
			StreamingPayload[*FeedEvent]()
			StreamingResult[*FeedEvent]()
		})
	})
}
