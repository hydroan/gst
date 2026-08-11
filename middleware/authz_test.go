package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestAuthzLogsDecisionDuration pins the elapsed time onto every entry the
// middleware writes, including the refusal that never reaches a decision.
//
// No policy set is installed here, so rbac.RBAC() answers with its
// pre-bootstrap behavior and refuses every request it is asked about. That is
// enough for what this test is about: one request that runs the decision and
// one that is turned away before it, both of which must report how long the
// middleware took.
func TestAuthzLogsDecisionDuration(t *testing.T) {
	t.Run("a decided request reports how long deciding took", func(t *testing.T) {
		recorder := setupAuthzLoggerTest(t)

		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set(consts.CTX_USER_ID, "u-1") }, Authz())
		router.GET("/api/records", func(c *gin.Context) { c.Status(http.StatusOK) })

		w := performAuthzRequest(router, "/api/records")

		require.Equal(t, http.StatusForbidden, w.Code)
		require.Len(t, recorder.entries, 1)
		fields := authzLogFieldMap(t, recorder.entries[0])
		require.Equal(t, string(consts.EffectDeny), fields["eft"])
		require.Equal(t, string(consts.DenyReasonNotInitialized), fields["denied_by"])
		requireAuthzLogDuration(t, fields)
	})

	// An anonymous request is refused before Authorize is called, so its
	// duration covers only the refusal. It is reported all the same: leaving
	// the field off this path alone would make an absent duration mean two
	// different things. denied_by is what separates the two.
	t.Run("an anonymous refusal reports its own duration", func(t *testing.T) {
		recorder := setupAuthzLoggerTest(t)

		router := gin.New()
		router.Use(Authz())
		router.GET("/api/records", func(c *gin.Context) { c.Status(http.StatusOK) })

		w := performAuthzRequest(router, "/api/records")

		require.Equal(t, http.StatusForbidden, w.Code)
		require.Len(t, recorder.entries, 1)
		fields := authzLogFieldMap(t, recorder.entries[0])
		require.Equal(t, string(consts.EffectDeny), fields["eft"])
		require.Equal(t, string(consts.DenyReasonUnauthenticated), fields["denied_by"])
		requireAuthzLogDuration(t, fields)
	})
}

// requireAuthzLogDuration asserts the pair util.LogDuration renders. Both keys
// come from that one field, so a grant and a failure carry them too: all three
// entry kinds take them from authzLogFields.
func requireAuthzLogDuration(t *testing.T, fields map[string]any) {
	t.Helper()

	duration, ok := fields[consts.LOG_DURATION].(int64)
	require.True(t, ok, "log field %q is not an int64", consts.LOG_DURATION)
	require.Positive(t, duration)
	require.NotEmpty(t, fields[consts.LOG_DURATION_HUMAN])
}

// authzLogFieldMap renders captured fields the way an encoder would, so an
// inlined field shows up under the keys it actually writes.
func authzLogFieldMap(t *testing.T, fields []zap.Field) map[string]any {
	t.Helper()

	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}
	return encoder.Fields
}

func setupAuthzLoggerTest(t *testing.T) *authzLogRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	// Authz turns RBAC on through the environment; bind that to this test so it
	// cannot leak into the rest of the package.
	t.Setenv(config.AUTH_RBAC_ENABLED, "true")

	recorder := new(authzLogRecorder)
	originalLogger := logger.Authz
	logger.Authz = recorder
	t.Cleanup(func() {
		logger.Authz = originalLogger
	})

	return recorder
}

func performAuthzRequest(router *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// authzLogRecorder captures the fields of every entry the middleware writes.
// Only the two methods it calls are implemented; the embedded interface stays
// nil, so reaching for any other one panics the test instead of passing
// unnoticed.
type authzLogRecorder struct {
	types.Logger
	entries [][]zap.Field
}

func (r *authzLogRecorder) Infoz(_ string, fields ...zap.Field) {
	r.entries = append(r.entries, fields)
}

func (r *authzLogRecorder) Errorz(_ string, fields ...zap.Field) {
	r.entries = append(r.entries, fields)
}
