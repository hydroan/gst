package controller_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/serviceregistry"

	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// streamDescs are the streaming rpcs of the sample service, served through
// the stream calls of the controller the way a generated handler serves
// them: Watch streams the responses of a request, Upload answers a stream
// of requests, Chat streams both ways, and Silence is a Watch on a route
// whose service streams nothing.
func streamDescs() []grpc.StreamDesc {
	watch := controller.ServerStreamCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](watchRoute)
	upload := controller.ClientStreamCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](uploadRoute)
	chat := controller.BidiStreamCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](chatRoute)
	silence := controller.ServerStreamCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](silentRoute)
	recv := func(stream grpc.ServerStream) func() (*sampleActionReq, error) {
		return func() (*sampleActionReq, error) {
			in := new(structpb.Struct)
			if err := stream.RecvMsg(in); err != nil {
				return nil, err
			}
			m := in.AsMap()
			// A message carrying refuse is one the decoding refuses, the
			// way a generated FromProto refuses a value a field cannot
			// hold (see grpc.Narrow), with the text it carries.
			if refusal, ok := m["refuse"].(string); ok {
				return nil, status.Error(codes.InvalidArgument, refusal)
			}
			return field[*sampleActionReq](m, "payload"), nil
		}
	}
	send := func(stream grpc.ServerStream) func(*sampleActionRsp) error {
		return func(rsp *sampleActionRsp) error { return stream.SendMsg(encode(rsp)) }
	}
	serverStream := func(call func(context.Context, map[string]string, *sampleActionReq, func(*sampleActionRsp) error) error) func(any, grpc.ServerStream) error {
		return func(_ any, stream grpc.ServerStream) error {
			in := new(structpb.Struct)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			m := in.AsMap()
			return call(stream.Context(), params(m), field[*sampleActionReq](m, "payload"), send(stream))
		}
	}
	return []grpc.StreamDesc{
		{StreamName: "Watch", ServerStreams: true, Handler: serverStream(watch)},
		{StreamName: "Silence", ServerStreams: true, Handler: serverStream(silence)},
		{StreamName: "Upload", ClientStreams: true, Handler: func(_ any, stream grpc.ServerStream) error {
			rsp, err := upload(stream.Context(), nil, recv(stream))
			if err != nil {
				return err
			}
			return stream.SendMsg(encode(rsp))
		}},
		{StreamName: "Chat", ServerStreams: true, ClientStreams: true, Handler: func(_ any, stream grpc.ServerStream) error {
			return chat(stream.Context(), nil, recv(stream), send(stream))
		}},
	}
}

// openStream opens the streaming rpc name of the sample service presenting
// authorization, none when empty, with the stream's kind as desc says.
func openStream(t *testing.T, conn *grpc.ClientConn, name, authorization string, desc grpc.StreamDesc) grpc.ClientStream {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	if authorization != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", authorization)
	}
	desc.StreamName = name
	stream, err := conn.NewStream(ctx, &desc, "/gst.test.Samples/"+name, grpc.WaitForReady(true))
	require.NoError(t, err)
	return stream
}

// sendPayload sends payload as a request message of the stream, with params
// as the route parameters when given.
func sendPayload(t *testing.T, stream grpc.ClientStream, payload map[string]any, params ...map[string]any) {
	t.Helper()
	in := map[string]any{"payload": payload}
	if len(params) > 0 {
		in["params"] = params[0]
	}
	require.NoError(t, stream.SendMsg(encode(in)))
}

// recvResponse receives the next response message of the stream as a map.
func recvResponse(stream grpc.ClientStream) (map[string]any, error) {
	out := new(structpb.Struct)
	if err := stream.RecvMsg(out); err != nil {
		return nil, err
	}
	return out.AsMap(), nil
}

// TestServerStreamCallStreamsTheResponses pins the server stream call end to
// end: the service's Stream method gets the request the message carried and
// a service context naming the caller the auth interceptor established and
// the call, sends its responses one by one, and the stream ends when it
// returns; a payload failing validation is refused before the service runs,
// the service's error ends the stream with its status, a call without
// credentials is refused by the interceptor the stream chain runs, and a
// service streaming nothing answers Unimplemented.
func TestServerStreamCallStreamsTheResponses(t *testing.T) {
	conn := sampleServer(t)
	serverStream := grpc.StreamDesc{ServerStreams: true}

	stream := openStream(t, conn, "Watch", sampleCredential, serverStream)
	sendPayload(t, stream, map[string]any{"note": "3"}, map[string]any{"box": "b-3"})
	require.NoError(t, stream.CloseSend())
	for i := range 3 {
		rsp, err := recvResponse(stream)
		require.NoError(t, err)
		require.Equal(t, string(rune('0'+i)), rsp["Note"])
		require.Equal(t, "alice", rsp["Username"])
		require.Equal(t, "b-3", rsp["Box"])
		require.Equal(t, "/api/controller-sample-watch", rsp["Route"], "the route of the action, not the full method")
		require.Equal(t, grpcserver.MethodStream, rsp["Method"])
		require.Equal(t, true, rsp["RequiresAuth"])
	}
	_, err := recvResponse(stream)
	require.ErrorIs(t, err, io.EOF)

	t.Run("a payload failing validation is refused", func(t *testing.T) {
		stream := openStream(t, conn, "Watch", sampleCredential, serverStream)
		sendPayload(t, stream, map[string]any{})
		_, err := recvResponse(stream)
		requireStatus(t, err, codes.InvalidArgument, "note is a required field")
	})

	t.Run("the service's error ends the stream with its status", func(t *testing.T) {
		stream := openStream(t, conn, "Watch", sampleCredential, serverStream)
		sendPayload(t, stream, map[string]any{"note": actionRefuse})
		_, err := recvResponse(stream)
		requireStatus(t, err, codes.PermissionDenied, "not yours")
	})

	t.Run("any other error answers Internal", func(t *testing.T) {
		stream := openStream(t, conn, "Watch", sampleCredential, serverStream)
		sendPayload(t, stream, map[string]any{"note": actionBreak})
		_, err := recvResponse(stream)
		requireStatus(t, err, codes.Internal, serviceregistry.FailureMsg)
	})

	t.Run("without credentials the stream is refused", func(t *testing.T) {
		stream := openStream(t, conn, "Watch", "", serverStream)
		_ = stream.SendMsg(encode(map[string]any{"payload": map[string]any{"note": "1"}}))
		_, err := recvResponse(stream)
		st := status.Convert(err)
		require.Equal(t, codes.Unauthenticated, st.Code(), "the auth interceptor refuses the stream before the service runs")
		require.Equal(t, "who are you", st.Message())
	})

	t.Run("a service streaming nothing answers Unimplemented", func(t *testing.T) {
		stream := openStream(t, conn, "Silence", sampleCredential, serverStream)
		sendPayload(t, stream, map[string]any{"note": "1"})
		_, err := recvResponse(stream)
		st := status.Convert(err)
		require.Equal(t, codes.Unimplemented, st.Code())
		require.Equal(t, "the Stream action is served by no Stream method of its kind", st.Message())
	})

	t.Run("a client canceling the stream ends it as canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", sampleCredential)
		stream, err := conn.NewStream(ctx, &grpc.StreamDesc{StreamName: "Watch", ServerStreams: true}, "/gst.test.Samples/Watch", grpc.WaitForReady(true))
		require.NoError(t, err)
		sendPayload(t, stream, map[string]any{"note": "1000000"})
		_, err = recvResponse(stream)
		require.NoError(t, err)
		before := len(accessLog.FilterMessage("/gst.test.Samples/Watch").All())
		cancel()
		require.Eventually(t, func() bool {
			entries := accessLog.FilterMessage("/gst.test.Samples/Watch").All()
			return len(entries) > before && entries[len(entries)-1].ContextMap()["status"] == codes.Canceled.String()
		}, 10*time.Second, 20*time.Millisecond, "the access log records the stream as canceled, not as a failure of the service")
	})
}

// TestClientStreamCallReadsTheRequestsAndAnswers pins the client stream
// call end to end: the service's Stream method reads the requests the
// client sends until it is done, each validated the way a payload is, and
// its response answers the call; a request failing validation ends the
// stream with the refusal.
func TestClientStreamCallReadsTheRequestsAndAnswers(t *testing.T) {
	conn := sampleServer(t)
	clientStream := grpc.StreamDesc{ClientStreams: true}

	stream := openStream(t, conn, "Upload", sampleCredential, clientStream)
	for _, note := range []string{"a", "b", "c"} {
		sendPayload(t, stream, map[string]any{"note": note})
	}
	require.NoError(t, stream.CloseSend())
	rsp, err := recvResponse(stream)
	require.NoError(t, err)
	require.Equal(t, "a,b,c", rsp["Note"])
	require.Equal(t, "alice", rsp["Username"])

	t.Run("a request failing validation ends the stream", func(t *testing.T) {
		stream := openStream(t, conn, "Upload", sampleCredential, clientStream)
		sendPayload(t, stream, map[string]any{"note": "a"})
		sendPayload(t, stream, map[string]any{})
		require.NoError(t, stream.CloseSend())
		_, err := recvResponse(stream)
		requireStatus(t, err, codes.InvalidArgument, "note is a required field")
	})

	t.Run("a request the decoding refuses ends the stream with the refusal", func(t *testing.T) {
		// The service wraps the error its Recv answers into one of its own;
		// the refusal still reaches the client as the decoding worded it.
		stream := openStream(t, conn, "Upload", sampleCredential, clientStream)
		sendPayload(t, stream, map[string]any{"note": "a"})
		require.NoError(t, stream.SendMsg(encode(map[string]any{"refuse": `field "rank": 300 does not fit int8`})))
		require.NoError(t, stream.CloseSend())
		_, err := recvResponse(stream)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.Equal(t, `field "rank": 300 does not fit int8`, status.Convert(err).Message())
	})
}

// TestBidiStreamCallStreamsBothWays pins the bidirectional stream call end
// to end: each request the client sends is answered as it comes, and the
// stream ends once the client is done.
func TestBidiStreamCallStreamsBothWays(t *testing.T) {
	conn := sampleServer(t)
	stream := openStream(t, conn, "Chat", sampleCredential, grpc.StreamDesc{ServerStreams: true, ClientStreams: true})

	for _, note := range []string{"x", "y"} {
		sendPayload(t, stream, map[string]any{"note": note})
		rsp, err := recvResponse(stream)
		require.NoError(t, err)
		require.Equal(t, "echo "+note, rsp["Note"])
		require.Equal(t, "alice", rsp["Username"])
	}
	require.NoError(t, stream.CloseSend())
	_, err := recvResponse(stream)
	require.ErrorIs(t, err, io.EOF, "the stream ends once the client is done")

	t.Run("a request the decoding refuses ends the stream with the refusal", func(t *testing.T) {
		stream := openStream(t, conn, "Chat", sampleCredential, grpc.StreamDesc{ServerStreams: true, ClientStreams: true})
		require.NoError(t, stream.SendMsg(encode(map[string]any{"refuse": `field "port": 70000 does not fit uint16`})))
		_, err := recvResponse(stream)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.Equal(t, `field "port": 70000 does not fit uint16`, status.Convert(err).Message())
	})
}

// TestFirstMessageRefusesAStreamEndedBeforeIt pins FirstMessage, what a
// generated handler reads the first message of a request stream through:
// the message when there is one, InvalidArgument naming the missing route
// parameters when the client ended the stream before sending any, and the
// stream's error as it is otherwise.
func TestFirstMessageRefusesAStreamEndedBeforeIt(t *testing.T) {
	first, err := controller.FirstMessage(func() (string, error) { return "params", nil })
	require.NoError(t, err)
	require.Equal(t, "params", first)

	_, err = controller.FirstMessage(func() (string, error) { return "", io.EOF })
	st := status.Convert(err)
	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Equal(t, "the stream ended before its first message, which carries the route parameters", st.Message())

	_, err = controller.FirstMessage(func() (string, error) { return "", status.Error(codes.Canceled, "context canceled") })
	require.Equal(t, codes.Canceled, status.Code(err))
}
