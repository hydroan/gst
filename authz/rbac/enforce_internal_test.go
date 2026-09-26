package rbac

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// enforceLogRecorder captures the fields of every entry Enforce writes.
// Only the two methods it calls are implemented; the embedded interface
// stays nil, so reaching for any other one panics the test instead of
// passing unnoticed.
type enforceLogRecorder struct {
	types.Logger
	entries [][]zap.Field
}

func (r *enforceLogRecorder) Infoz(_ string, fields ...zap.Field) {
	r.entries = append(r.entries, fields)
}

func (r *enforceLogRecorder) Errorz(_ string, fields ...zap.Field) {
	r.entries = append(r.entries, fields)
}

// fieldMap renders captured fields the way an encoder would, so an inlined
// field shows up under the keys it actually writes.
func fieldMap(fields []zap.Field) map[string]any {
	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}
	return encoder.Fields
}

// recordEnforceLog points logger.Authz at a recorder until the test ends.
func recordEnforceLog(t *testing.T) *enforceLogRecorder {
	t.Helper()
	recorder := new(enforceLogRecorder)
	saved := logger.Authz
	logger.Authz = recorder
	t.Cleanup(func() { logger.Authz = saved })
	return recorder
}

// TestEnforceLogFieldsFitTheCountInTheWorstCase pins the entry with the most
// fields, a grant by a policy rule, to enforceLogFieldCount exactly, so a
// field added without bumping the count fails here instead of regrowing
// the slice on every authorized request.
func TestEnforceLogFieldsFitTheCountInTheWorstCase(t *testing.T) {
	recorder := recordEnforceLog(t)

	logGrant(execctx.WithTraceID(context.Background(), "trace-1"), Subject{Username: "alice"}, "tenant-1", "u-1", "/api/samples", http.MethodGet, consts.GrantSourceRole, []string{"role-1", "/api/samples", http.MethodGet}, time.Millisecond)

	require.Len(t, recorder.entries, 1)
	require.Len(t, recorder.entries[0], enforceLogFieldCount, "the worst case must fill the count exactly: a new field bumps enforceLogFieldCount")
}

// TestEnforceRefusesAnAnonymousSubject pins the refusal both listeners
// answer a subject without a user id with, before any decision: 403
// "permission denied", logged as denied for being unauthenticated and
// without a tenant.
func TestEnforceRefusesAnAnonymousSubject(t *testing.T) {
	recorder := recordEnforceLog(t)

	_, tenantID, err := Enforce(execctx.WithTraceID(context.Background(), "trace-1"), Subject{}, "/api/samples", http.MethodGet)

	var serviceErr *serviceregistry.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, http.StatusForbidden, serviceErr.Status())
	require.Equal(t, "permission denied", serviceErr.Msg())
	require.Empty(t, tenantID)
	require.Len(t, recorder.entries, 1)
	fields := fieldMap(recorder.entries[0])
	require.Equal(t, string(consts.EffectDeny), fields["eft"])
	require.Equal(t, string(consts.DenyReasonUnauthenticated), fields["denied_by"])
	require.Equal(t, "trace-1", fields[consts.TRACE_ID])
}

// TestEnforceDeniesThroughAnUninitializedPolicySet pins that with no policy
// set installed a subject is denied, the decision naming the missing
// initialization, in the default tenant when it names none.
func TestEnforceDeniesThroughAnUninitializedPolicySet(t *testing.T) {
	recorder := recordEnforceLog(t)

	_, tenantID, err := Enforce(context.Background(), Subject{UserID: "u-1", Username: "alice"}, "/api/samples", http.MethodGet)

	var serviceErr *serviceregistry.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, http.StatusForbidden, serviceErr.Status())
	require.Equal(t, "default", tenantID)
	require.Len(t, recorder.entries, 1)
	fields := fieldMap(recorder.entries[0])
	require.Equal(t, string(consts.DenyReasonNotInitialized), fields["denied_by"])
	require.Equal(t, "alice", fields["username"])
}
