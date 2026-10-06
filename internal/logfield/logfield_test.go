package logfield_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/logfield"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestFieldsFixTheirKeyAndType pins the key and the zap field type of every
// constructor: the type decides the JSON type a store maps the key to, so a
// constructor changing either changes the type the key has in every stream
// at once.
func TestFieldsFixTheirKeyAndType(t *testing.T) {
	for _, tt := range []struct {
		name  string
		field zap.Field
		key   string
		typ   zapcore.FieldType
	}{
		{name: "route", field: logfield.Route("/api/records/:record"), key: consts.CTX_ROUTE, typ: zapcore.StringType},
		{name: "path", field: logfield.Path("/api/records/42"), key: consts.CTX_PATH, typ: zapcore.StringType},
		{name: "method", field: logfield.Method("GET"), key: consts.CTX_METHOD, typ: zapcore.StringType},
		{name: "username", field: logfield.Username("alice"), key: consts.CTX_USERNAME, typ: zapcore.StringType},
		{name: "user_id", field: logfield.UserID("u-1"), key: consts.CTX_USER_ID, typ: zapcore.StringType},
		{name: "trace_id", field: logfield.TraceID("t-1"), key: consts.TRACE_ID, typ: zapcore.StringType},
		{name: "query", field: logfield.Query("name=a&name=b"), key: consts.QUERY, typ: zapcore.StringType},
		{name: "params", field: logfield.Params(map[string]string{"record": "42"}), key: consts.PARAMS, typ: zapcore.ObjectMarshalerType},
		{name: "status", field: logfield.Status(404), key: "status", typ: zapcore.Int64Type},
		{name: "threshold", field: logfield.Threshold(200 * time.Millisecond), key: "threshold", typ: zapcore.DurationType},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.key, tt.field.Key)
			require.Equal(t, tt.typ, tt.field.Type)
		})
	}
}

// TestParamsIsOneObjectWithAKeyPerParameter pins the shape Params renders,
// an object whose keys are the parameter names, which is what every stream
// writing the route parameters must agree on.
func TestParamsIsOneObjectWithAKeyPerParameter(t *testing.T) {
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	buf, err := enc.EncodeEntry(zapcore.Entry{}, []zap.Field{logfield.Params(map[string]string{"record": "42", "note": "7"})})
	require.NoError(t, err)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, map[string]any{"record": "42", "note": "7"}, entry[consts.PARAMS])
}
