package grpc

import (
	"context"

	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/types"
)

// This file holds what a Stream action's service reads and writes, the
// streams, and the calls the generated handlers of streaming rpcs run the
// action through: ServerStreamCall, ClientStreamCall and BidiStreamCall,
// one per kind of stream, the counterparts of ServiceCall, FirstMessage,
// which a handler reads the route parameters of a request stream through,
// and SameParams, which it holds every later message to them with.

// ServerStream is the response stream of a Stream action declaring a
// StreamingResult: the service's Stream method sends each response through
// Send, which fails once the client went away or the call was canceled.
type ServerStream[RSP types.Response] = types.ServerStream[RSP]

// ClientStream is the request stream of a Stream action declaring a
// StreamingPayload: the service's Stream method reads each request through
// Recv, which answers io.EOF once the client finished sending.
type ClientStream[REQ types.Request] = types.ClientStream[REQ]

// BidiStream is the request and response stream of a Stream action
// declaring both a StreamingPayload and a StreamingResult: Recv and Send
// as on the two above, independent of each other.
type BidiStream[REQ types.Request, RSP types.Response] = types.BidiStream[REQ, RSP]

// FirstMessage returns the first message of a request stream read through
// recv, the Recv of the stream the plugin generated: what the generated
// handler of a client or bidirectional stream on a route with parameters
// reads ahead of the call, the parameters being carried by that message. A
// stream the client ended before sending one is refused with
// InvalidArgument; any other error of recv is returned as it is.
func FirstMessage[T any](recv func() (T, error)) (T, error) {
	return controller.FirstMessage(recv)
}

// SameParams holds the route parameters msgParams of the message at index
// i of a request stream, the first message being 1, to params, the ones
// the first message carried: what the generated handler of a client or
// bidirectional stream on a route with parameters reads every message
// after the first through. A parameter the message leaves empty or names
// as the first did agrees; one it names otherwise is refused with
// InvalidArgument naming the message, the parameter and both values.
func SameParams(i int, params, msgParams map[string]string) error {
	return controller.SameParams(i, params, msgParams)
}

// ServerStreamCall returns the call of the Stream action on route whose
// response is streamed: given the route parameters, the request the message
// decoded into and the function sending one response, it runs the
// service's Stream method with a ServerStream sending through the function,
// and answers with nil once the stream is over, or with the status a failure
// maps to, the way ServiceCall does.
func ServerStreamCall[M types.Model, REQ types.Request, RSP types.Response](route string) func(ctx context.Context, params map[string]string, req REQ, send func(RSP) error) error {
	return controller.ServerStreamCall[M, REQ, RSP](route)
}

// ClientStreamCall returns the call of the Stream action on route whose
// request is streamed: given the route parameters and the function
// receiving the next request, io.EOF once the client finished, it runs the
// service's Stream method with a ClientStream reading through the function,
// and answers with the response, or with the status a failure maps to.
func ClientStreamCall[M types.Model, REQ types.Request, RSP types.Response](route string) func(ctx context.Context, params map[string]string, recv func() (REQ, error)) (RSP, error) {
	return controller.ClientStreamCall[M, REQ, RSP](route)
}

// BidiStreamCall returns the call of the Stream action on route streaming
// both ways: given the route parameters, the function receiving the next
// request and the function sending one response, it runs the service's
// Stream method with a BidiStream on the two, and answers with nil once the
// stream is over, or with the status a failure maps to.
func BidiStreamCall[M types.Model, REQ types.Request, RSP types.Response](route string) func(ctx context.Context, params map[string]string, recv func() (REQ, error), send func(RSP) error) error {
	return controller.BidiStreamCall[M, REQ, RSP](route)
}
