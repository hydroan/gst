package interceptor

import (
	"context"
	"testing"
	"time"

	"github.com/hydroan/gst/consts"
	gstgrpc "github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// authzLogRecorder captures the fields of every entry the interceptor
// writes. Only the methods it calls are implemented; the embedded interface
// stays nil, so reaching for any other one panics the test instead of
// passing unnoticed.
type authzLogRecorder struct {
	types.Logger
	fields [][]zap.Field
}

func (r *authzLogRecorder) WithContext(context.Context, consts.Phase) types.Logger { return r }
func (r *authzLogRecorder) Infoz(_ string, fields ...zap.Field)                    { r.fields = append(r.fields, fields) }

func (r *authzLogRecorder) Errorz(_ string, fields ...zap.Field) { r.fields = append(r.fields, fields) }

// TestAuthzLogFieldsFitTheCountInTheWorstCase pins the entry with the most
// fields, a grant by a policy rule, to authzLogFieldCount exactly, so a
// field added without bumping the count fails here instead of regrowing
// the slice on every authorized call.
func TestAuthzLogFieldsFitTheCountInTheWorstCase(t *testing.T) {
	recorder := new(authzLogRecorder)
	saved := logger.Authz
	logger.Authz = recorder
	t.Cleanup(func() { logger.Authz = saved })

	logAuthzGrant(context.Background(), gstgrpc.Caller{Username: "alice"}, "tenant-1", "u-1", "/api/samples", "GET", consts.GrantSourceRole, []string{"role-1", "/api/samples", "GET"}, time.Millisecond)

	require.Len(t, recorder.fields, 1)
	require.Len(t, recorder.fields[0], authzLogFieldCount, "the worst case must fill the count exactly: a new field bumps authzLogFieldCount")
}
