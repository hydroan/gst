package grpcserver_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestNarrowRefusesWhatTheTypeCannotHold pins Narrow, what a generated
// FromProto reads a narrow integer through: a value in range comes back as
// the field's type, a named one included, and one out of range is refused
// with InvalidArgument naming the field, the way encoding/json refuses it
// over HTTP, rather than folded into the type.
func TestNarrowRefusesWhatTheTypeCannotHold(t *testing.T) {
	type level int8
	count, err := grpcserver.Narrow[int16]("count", int32(300))
	require.NoError(t, err)
	require.Equal(t, int16(300), count)
	lvl, err := grpcserver.Narrow[level]("level", int32(-5))
	require.NoError(t, err)
	require.Equal(t, level(-5), lvl)
	port, err := grpcserver.Narrow[uint8]("port", uint32(255))
	require.NoError(t, err)
	require.Equal(t, uint8(255), port)

	_, err = grpcserver.Narrow[int8]("count", int32(300))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, `field "count": 300 does not fit int8`, status.Convert(err).Message())
	_, err = grpcserver.Narrow[uint16]("port", uint32(70000))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = grpcserver.Narrow[level]("level", int32(128))
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "does not fit grpcserver_test.level")
}

// TestNumberRefusesWhatIsNoJSONNumber pins Number, what a generated
// FromProto reads a json.Number field through: a JSON number literal comes
// back as it is, the empty string as the unset field, and anything else,
// text, a quoted number, a literal with trailing space, is refused with
// InvalidArgument naming the field, the way encoding/json refuses it over
// HTTP.
func TestNumberRefusesWhatIsNoJSONNumber(t *testing.T) {
	amount, err := grpcserver.Number("amount", "-12.5e3")
	require.NoError(t, err)
	require.Equal(t, json.Number("-12.5e3"), amount)
	amount, err = grpcserver.Number("amount", "")
	require.NoError(t, err)
	require.Equal(t, json.Number(""), amount)
	for _, s := range []string{"abc", `"12"`, "12 ", "0x1f", "1.", "null"} {
		_, err := grpcserver.Number("amount", s)
		require.Equal(t, codes.InvalidArgument, status.Code(err), s)
		require.Equal(t, `field "amount": `+strconv.Quote(s)+` is not a JSON number`, status.Convert(err).Message())
	}
}
