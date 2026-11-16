package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Notice is a message for every client that is watching, whichever replica
// its connection is on. A WebSocket connection stays with the replica that
// accepted it, so a notice published on one replica reaches the clients of
// the others only through something the replicas share: publishing sends it
// to a redis channel, and the watch stream of every connection is subscribed
// to that channel. The model declares no GRPC(), so its stream is served over
// WebSocket alone.
type Notice struct {
	model.Empty
}

func (Notice) Design() {
	Route("/notices", func() {
		Create(func() {
			Service()
			Payload[*NoticeReq]()
			Result[*NoticeCreateRsp]()
		})
	})

	// The watch takes no request and sends every notice published while the
	// connection is open: a server stream.
	Route("notices/watch", func() {
		Stream(func() {
			Service("watch")
			StreamingResult[*NoticeWatchRsp]()
		})
	})
}

type (
	// NoticeReq is the notice to publish.
	NoticeReq struct {
		Text string `json:"text"`
	}

	// NoticeCreateRsp reports which replica published the notice and how many
	// connections were watching.
	NoticeCreateRsp struct {
		Replica  string `json:"replica"`
		Text     string `json:"text"`
		Watchers int64  `json:"watchers"` // the watching connections redis handed the notice to, over every replica
	}

	// NoticeWatchRsp is a notice as a watching client receives it.
	NoticeWatchRsp struct {
		Text    string `json:"text"`
		From    string `json:"from"`    // the replica that published it
		Replica string `json:"replica"` // the replica this connection is on, which passed the notice on
	}
)
