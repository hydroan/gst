package board

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Feed is the stream of events of the board: a client watches the events of
// a topic, uploads a run of events, or exchanges events both ways. It is the
// streaming half of the gRPC example. A Stream action carries a stream of
// messages on one side of the call or on both; declared on a model with
// GRPC() it is a streaming rpc as well as the WebSocket connection it is
// over HTTP, both served by one piece of service code. WatchFeed answers one
// request with a stream of events, UploadFeed takes a stream of events and
// answers once, ChatFeed streams both ways. A Stream action names its
// service with Service("name") and always has service code, in
// service/board/feed.
type Feed struct {
	model.Empty
}

func (Feed) Design() {
	GRPC()

	Route("board/feeds/watch", func() {
		Stream(func() {
			Public()
			Service("watch")
			Payload[*FeedWatchReq]()
			StreamingResult[*FeedWatchRsp]()
		})
	})
	Route("board/feeds/upload", func() {
		Stream(func() {
			Service("upload")
			StreamingPayload[*FeedUploadReq]()
			Result[*FeedUploadRsp]()
		})
	})
	Route("board/feeds/chat", func() {
		Stream(func() {
			Service("chat")
			StreamingPayload[*FeedChatReq]()
			StreamingResult[*FeedChatRsp]()
		})
	})
}

type (
	// FeedWatchReq names the topic to watch.
	FeedWatchReq struct {
		Topic string `json:"topic" pb:"1"`
	}

	// FeedEvent is one event of a feed: what every stream carries, each
	// under a name of its own.
	FeedEvent struct {
		Seq  int64  `json:"seq" pb:"1"`
		Body string `json:"body" pb:"2"`
	}

	// FeedWatchRsp is an event the watch streams out.
	FeedWatchRsp = FeedEvent

	// FeedUploadReq is an event the upload streams in.
	FeedUploadReq = FeedEvent

	// FeedChatReq is an event the chat streams in; FeedChatRsp one it
	// streams back.
	FeedChatReq = FeedEvent
	FeedChatRsp = FeedEvent

	// FeedUploadRsp counts the events a client streamed in.
	FeedUploadRsp struct {
		Accepted int64 `json:"accepted" pb:"1"`
	}
)
