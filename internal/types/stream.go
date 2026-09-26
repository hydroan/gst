package types

// The streams of a Stream action, what its service reads and writes: a
// server stream sends the responses one by one, a client stream receives
// the requests one by one, and a bidirectional stream does both. The
// controller builds each on the stream of the transport (see the stream
// calls of the controller), so a service depends on no transport; the
// public grpc package forwards the types to a project's service code.

// ServerStreamer is the service of a Stream action declaring a Payload and
// a StreamingResult: Stream answers req with the responses it sends on
// stream, one by one, and returns once the stream is over; an error ends
// the stream with the status the error maps to.
type ServerStreamer[REQ Request, RSP Response] interface {
	Stream(ctx *ServiceContext, req REQ, stream *ServerStream[RSP]) error
}

// ClientStreamer is the service of a Stream action declaring a
// StreamingPayload and a Result: Stream reads the requests off stream, one
// by one until Recv answers io.EOF, and answers the response.
type ClientStreamer[REQ Request, RSP Response] interface {
	Stream(ctx *ServiceContext, stream *ClientStream[REQ]) (RSP, error)
}

// BidiStreamer is the service of a Stream action declaring a
// StreamingPayload and a StreamingResult: Stream reads requests off stream
// and sends responses on it in whatever order the action wants, and
// returns once done.
type BidiStreamer[REQ Request, RSP Response] interface {
	Stream(ctx *ServiceContext, stream *BidiStream[REQ, RSP]) error
}

// ServerStream is the response stream of a Stream action: Send sends one
// response to the client, and fails once the client went away or the call
// was canceled, with the error the transport reports.
type ServerStream[RSP Response] struct {
	send func(RSP) error
}

// NewServerStream builds a ServerStream sending each response through send.
func NewServerStream[RSP Response](send func(RSP) error) *ServerStream[RSP] {
	return &ServerStream[RSP]{send: send}
}

// Send sends rsp to the client.
func (s *ServerStream[RSP]) Send(rsp RSP) error { return s.send(rsp) }

// ClientStream is the request stream of a Stream action: Recv returns the
// next request the client sent, io.EOF once the client finished sending,
// and the error the transport reports otherwise, a request the framework
// refused among them (the call answers the refusal whatever the service
// returns then).
type ClientStream[REQ Request] struct {
	recv func() (REQ, error)
}

// NewClientStream builds a ClientStream reading each request through recv.
func NewClientStream[REQ Request](recv func() (REQ, error)) *ClientStream[REQ] {
	return &ClientStream[REQ]{recv: recv}
}

// Recv returns the next request, io.EOF once the client finished sending.
func (s *ClientStream[REQ]) Recv() (REQ, error) { return s.recv() }

// BidiStream is the request and response stream of a Stream action, a
// ClientStream and a ServerStream in one: Recv reads the requests the way
// ClientStream.Recv does, Send sends responses the way ServerStream.Send
// does, and the two are independent, so responses may go out before the
// requests are all in.
type BidiStream[REQ Request, RSP Response] struct {
	recv func() (REQ, error)
	send func(RSP) error
}

// NewBidiStream builds a BidiStream reading requests through recv and
// sending responses through send.
func NewBidiStream[REQ Request, RSP Response](recv func() (REQ, error), send func(RSP) error) *BidiStream[REQ, RSP] {
	return &BidiStream[REQ, RSP]{recv: recv, send: send}
}

// Recv returns the next request, io.EOF once the client finished sending.
func (s *BidiStream[REQ, RSP]) Recv() (REQ, error) { return s.recv() }

// Send sends rsp to the client.
func (s *BidiStream[REQ, RSP]) Send(rsp RSP) error { return s.send(rsp) }
