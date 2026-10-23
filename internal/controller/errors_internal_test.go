package controller

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestDatabaseErrorMapsEachDatabaseErrorToItsAnswer pins the canonical
// mapping of database errors: a service error keeps its own status and
// message, the database sentinels answer their fixed status and message with
// the error behind them as the cause, the constraints a client's data breaks
// among them, and everything else is answered as it is, the server's own
// failure carrying no service error.
func TestDatabaseErrorMapsEachDatabaseErrorToItsAnswer(t *testing.T) {
	serviceErr := types.NewError(http.StatusForbidden, "operation refused")

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{"service_error_keeps_status_and_message", serviceErr, http.StatusForbidden, "operation refused"},
		{"record_not_found_answers_404", errors.Wrap(database.ErrRecordNotFound, "get sample"), http.StatusNotFound, "The requested resource was not found."},
		{"duplicated_key_answers_409", errors.Wrap(database.ErrDuplicatedKey, "create sample"), http.StatusConflict, "The resource already exists."},
		{"stale_object_answers_409", errors.Wrap(database.ErrStaleObject, "update sample"), http.StatusConflict, "The resource was modified by another operation. Reload and retry."},
		{"missing_version_answers_400", errors.Wrap(database.ErrVersionRequired, "update sample"), http.StatusBadRequest, "The request contains invalid parameters."},
		{"missing_id_answers_400", errors.Wrap(database.ErrIDRequired, "update sample"), http.StatusBadRequest, "The request contains invalid parameters."},
		{"foreign_key_answers_409", errors.Wrap(database.ErrForeignKeyViolated, "create sample"), http.StatusConflict, "The request refers to a record that does not exist or is still in use."},
		{"check_constraint_answers_400", errors.Wrap(database.ErrCheckConstraintViolated, "create sample"), http.StatusBadRequest, "The request contains invalid parameters."},
		{"value_too_long_answers_400", errors.Wrap(database.ErrValueTooLong, "create sample"), http.StatusBadRequest, "The request contains invalid parameters."},
		{"not_null_answers_400", errors.Wrap(database.ErrNotNullViolated, "create sample"), http.StatusBadRequest, "The request contains invalid parameters."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer := databaseError(tt.err)

			var answered *types.Error
			require.ErrorAs(t, answer, &answered)
			require.Equal(t, tt.wantStatus, answered.Status())
			require.Equal(t, tt.wantMsg, answered.Msg())
			require.ErrorIs(t, answer, tt.err, "the error behind travels as the cause")
		})
	}

	t.Run("other_errors_are_the_servers_own_failure", func(t *testing.T) {
		internal := errors.New("Error 1146: Table 'sample' doesn't exist")

		answer := databaseError(internal)

		var answered *types.Error
		require.False(t, errors.As(answer, &answered), "no status and message were chosen: %v", answer)
		require.Equal(t, internal, answer)
	})
}

// TestInvalidArgumentCarriesTheClientSafeText pins the refusal of a request
// the controller could not carry: 400, with the message of the service error
// the error wraps when it does, and the error's own text otherwise, the
// error behind as the cause either way.
func TestInvalidArgumentCarriesTheClientSafeText(t *testing.T) {
	cause := errors.New("database password leaked")
	serviceErr := types.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause)

	for name, tt := range map[string]struct {
		err     error
		wantMsg string
	}{
		"service error":         {serviceErr, "failed to load user"},
		"wrapped service error": {errors.Wrap(serviceErr, "load account"), "failed to load user"},
		"plain error":           {errors.New("invalid value for field 'name'"), "invalid value for field 'name'"},
	} {
		t.Run(name, func(t *testing.T) {
			answer := invalidArgument(tt.err)

			require.Equal(t, http.StatusBadRequest, answer.Status())
			require.Equal(t, tt.wantMsg, answer.Msg())
			require.ErrorIs(t, answer, tt.err)
		})
	}
}

// levelRecorder records the messages failWith logs at info and at error.
type levelRecorder struct {
	nopControllerLogger
	infos  []string
	errors []string
}

func (r *levelRecorder) Infoz(msg string, _ ...zap.Field)  { r.infos = append(r.infos, msg) }
func (r *levelRecorder) Errorz(msg string, _ ...zap.Field) { r.errors = append(r.errors, msg) }

// TestFailWithTellsACancellationFromAPassedDeadline pins what failWith
// makes of a failure once the request's context ended: a client that went
// away is answered the client-closed status at info, off the span, while a
// deadline that passed is the server's failure to answer in time, answered
// 504 and logged at error, each with the context's own message and the
// failure behind it as cause; a failure in a live context stays the flow's
// answer, logged at error.
func TestFailWithTellsACancellationFromAPassedDeadline(t *testing.T) {
	cause := errors.New("read tcp: connection reset by peer")
	answer := errors.New("the flow's answer")

	t.Run("a live context answers the flow's answer and logs an error", func(t *testing.T) {
		rec := &levelRecorder{}
		require.Same(t, answer, failWith(context.Background(), rec, "database operation failed", cause, answer))
		require.Equal(t, []string{"database operation failed"}, rec.errors)
		require.Empty(t, rec.infos)
	})

	t.Run("a canceled context answers the client-closed status at info", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		rec := &levelRecorder{}
		got := failWith(ctx, rec, "database operation failed", cause, answer)
		var serviceErr *types.Error
		require.ErrorAs(t, got, &serviceErr)
		require.Equal(t, types.StatusClientClosedRequest, serviceErr.Status())
		require.Equal(t, "context canceled", serviceErr.Msg())
		require.ErrorIs(t, got, cause)
		require.Equal(t, []string{"database operation failed"}, rec.infos)
		require.Empty(t, rec.errors, "a client that went away is not a failure of the server")
	})

	t.Run("a passed deadline answers 504 and logs an error", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		rec := &levelRecorder{}
		got := failWith(ctx, rec, "database operation failed", cause, answer)
		var serviceErr *types.Error
		require.ErrorAs(t, got, &serviceErr)
		require.Equal(t, http.StatusGatewayTimeout, serviceErr.Status())
		require.Equal(t, "context deadline exceeded", serviceErr.Msg())
		require.ErrorIs(t, got, cause)
		require.Equal(t, []string{"database operation failed"}, rec.errors, "the server did not answer in time: its failure, not the client's")
		require.Empty(t, rec.infos)
	})
}
