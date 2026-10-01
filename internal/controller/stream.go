package controller

import (
	"context"
	"fmt"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/types"
	gstotel "github.com/hydroan/gst/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This file holds the calls of a Stream action, the counterparts of
// ServiceCall for the three kinds of stream: ServerStreamCall for an action
// declaring a Payload and a StreamingResult, ClientStreamCall for a
// StreamingPayload and a Result, BidiStreamCall for both streaming. Each
// builds the stream its service reads and writes on the functions the
// generated handler hands it, which move the messages of the transport's
// stream converted to the action's types, and runs the service's Stream
// method in its service span on a service context answering the route
// parameters and the caller, the way ServiceCall runs a method: the
// parameters come with the call, read off the request message of a server
// stream or the first message of the other two by the handler. A request
// is normalized and validated the way ServiceCall validates a payload, each
// one of a request stream as it is read. The service is the one registered
// for the route and the Stream phase, and it must implement the Stream
// method of the action's kind (see types.ServerStreamer and its kind),
// which the generated service file declares; a call finding none answers
// Unimplemented. A stream ends the way the client ends it as well: once the
// call's context is canceled or past its deadline, whatever the service
// returns answers the context's status (see call.ended); a stream the
// listener's stop ended the same way is answered Unavailable by the
// listener in its place (see grpcserver).

// ServerStreamCall returns the call of the Stream action on route whose
// response is streamed: given the route parameters, the request the message
// decoded into and the function sending one response, it runs the service's
// Stream method with a ServerStream sending through the function, and
// answers with nil once the stream is over, or with the status a failure
// maps to (see statusOf).
func ServerStreamCall[M types.Model, REQ types.Request, RSP types.Response](route string) func(ctx context.Context, params map[string]string, req REQ, send func(RSP) error) error {
	a := newAction[M, REQ, RSP](route, consts.Stream)
	return func(ctx context.Context, params map[string]string, req REQ, send func(RSP) error) error {
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return c.invalid(err)
		}
		svc, ok := a.service().(types.ServerStreamer[REQ, RSP])
		if !ok {
			return a.unimplemented(c, route)
		}
		a.normalizeRequest(&req)
		if err = validateRequest(req); err != nil {
			return c.invalidMessage(err)
		}
		_, err = a.traceServiceOperation(c.ctx, consts.Stream, func(spanCtx context.Context) (RSP, error) {
			var zero RSP
			return zero, svc.Stream(c.serviceContext(spanCtx, consts.Stream), req, types.NewServerStream(send))
		})
		if err != nil {
			return c.failService(err)
		}
		return c.finish()
	}
}

// ClientStreamCall returns the call of the Stream action on route whose
// request is streamed: given the route parameters and the function
// receiving the next request, io.EOF once the client finished, it runs the
// service's Stream method with a ClientStream reading through the function,
// and answers with the response, or with the status a failure maps to.
func ClientStreamCall[M types.Model, REQ types.Request, RSP types.Response](route string) func(ctx context.Context, params map[string]string, recv func() (REQ, error)) (RSP, error) {
	a := newAction[M, REQ, RSP](route, consts.Stream)
	return func(ctx context.Context, params map[string]string, recv func() (REQ, error)) (RSP, error) {
		var zero RSP
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return zero, c.invalid(err)
		}
		svc, ok := a.service().(types.ClientStreamer[REQ, RSP])
		if !ok {
			return zero, a.unimplemented(c, route)
		}
		in := a.requests(c, recv)
		rsp, err := a.traceServiceOperation(c.ctx, consts.Stream, func(spanCtx context.Context) (RSP, error) {
			return svc.Stream(c.serviceContext(spanCtx, consts.Stream), types.NewClientStream(in.recv))
		})
		if in.refused != nil {
			return zero, in.refused
		}
		if err != nil {
			return zero, c.failService(err)
		}
		return answer(c, rsp)
	}
}

// BidiStreamCall returns the call of the Stream action on route streaming
// both ways: given the route parameters, the function receiving the next
// request and the function sending one response, it runs the service's
// Stream method with a BidiStream on the two, and answers with nil once the
// stream is over, or with the status a failure maps to.
func BidiStreamCall[M types.Model, REQ types.Request, RSP types.Response](route string) func(ctx context.Context, params map[string]string, recv func() (REQ, error), send func(RSP) error) error {
	a := newAction[M, REQ, RSP](route, consts.Stream)
	return func(ctx context.Context, params map[string]string, recv func() (REQ, error), send func(RSP) error) error {
		c, err := a.beginCall(ctx, params, nil)
		defer c.end()
		if err != nil {
			return c.invalid(err)
		}
		svc, ok := a.service().(types.BidiStreamer[REQ, RSP])
		if !ok {
			return a.unimplemented(c, route)
		}
		in := a.requests(c, recv)
		_, err = a.traceServiceOperation(c.ctx, consts.Stream, func(spanCtx context.Context) (RSP, error) {
			var zero RSP
			return zero, svc.Stream(c.serviceContext(spanCtx, consts.Stream), types.NewBidiStream(in.recv, send))
		})
		if in.refused != nil {
			return in.refused
		}
		if err != nil {
			return c.failService(err)
		}
		return c.finish()
	}
}

// firstMessageMsg is the message a stream the client ended before its
// first message is refused with (see FirstMessage).
const (
	firstMessageMsg = "the stream ended before its first message, which carries the route parameters"
	// unimplementedMsg answers a Stream call nothing serves; see unimplemented.
	unimplementedMsg = "the Stream action is served by no Stream method of its kind"
)

// FirstMessage returns the first message of a request stream read through
// recv, the Recv of the transport's stream: what the generated handler of a
// client or bidirectional stream on a route with parameters reads ahead of
// the call, the parameters being carried by that message. A stream the
// client ended before sending one (io.EOF) is refused with InvalidArgument
// and firstMessageMsg — the client broke the contract, and the transport's
// own answer to the EOF, Unknown, would say nothing of why — and any other
// error is returned as it is. The public grpc.FirstMessage forwards to it.
func FirstMessage[T any](recv func() (T, error)) (T, error) {
	first, err := recv()
	if errors.Is(err, io.EOF) {
		return first, status.Error(codes.InvalidArgument, firstMessageMsg)
	}
	return first, err
}

// SameParams holds the route parameters msgParams of the message at index
// i of a request stream, the first message being 1, to params, the
// parameters the first message carried and the call runs under: what the
// generated handler of a client or bidirectional stream on a route with
// parameters reads every message after the first through. A parameter the
// message leaves empty or names as the first did agrees; one it names
// otherwise is refused with InvalidArgument naming the message, the way
// PatchItem refuses an item of a batch contradicting its request. The
// public grpc.SameParams forwards to it.
func SameParams(i int, params, msgParams map[string]string) error {
	if name, value, differs := disagreeingParam(params, msgParams); differs {
		return grpcserver.StatusError(badRequest(fmt.Sprintf("message %d names the %s parameter %q, the first names %q", i, name, value, params[name])))
	}
	return nil
}

// requests reads the requests of a request stream through the function the
// handler hands the call: recv normalizes and validates each the way
// ServiceCall treats a payload, and one the validator refuses, or one the
// handler's decoding refused before it got here, ends the stream, the
// refusal kept in refused for the call to answer whatever the service
// returns for the error it got.
type requests[REQ types.Request] struct {
	recv    func() (REQ, error)
	refused error
}

// requests builds the requests of the call c read through recv.
func (a *action[M, REQ, RSP]) requests(c *call, recv func() (REQ, error)) *requests[REQ] {
	r := &requests[REQ]{}
	r.recv = func() (REQ, error) {
		req, err := recv()
		if err != nil {
			// An InvalidArgument is the decoding refusing the message, a
			// value a field cannot hold (see grpc.Narrow): the transport's
			// own errors, the stream ending or failing, never carry that
			// code. It is answered as the refusal it is, whatever the
			// service makes of the error its Recv gave it.
			if status.Code(err) == codes.InvalidArgument {
				c.log.Errorz("request message rejected", zap.Error(err))
				gstotel.RecordError(c.span, err)
				r.refused = err
			}
			return req, err
		}
		a.normalizeRequest(&req)
		if err := validateRequest(req); err != nil {
			r.refused = c.invalidMessage(err)
			return req, r.refused
		}
		return req, nil
	}
	return r
}

// unimplemented answers a Stream call whose service, the one registered for
// the route and the Stream phase, has no Stream method of the kind the
// action declares — the service file was not generated, or its method was
// changed — with Unimplemented and unimplementedMsg: the transport's own
// refusal of a call nothing serves. Which action, on which route, is for
// the log and the span, like the cause of any other internal failure.
func (a *action[M, REQ, RSP]) unimplemented(c *call, route string) error {
	err := fmt.Errorf("the Stream action of %s on %s is served by no Stream method of its kind; gg gen declares the method in the service file", a.name, route)
	c.log.Errorz("service operation failed", zap.Error(err))
	gstotel.RecordError(c.span, err)
	return status.Error(codes.Unimplemented, unimplementedMsg)
}
