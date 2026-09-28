package controller

import (
	"net/http"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/stretchr/testify/require"
)

// TestDatabaseError pins the canonical mapping of database errors: a service
// error keeps its own status and message, the database sentinels answer
// their fixed status and message with the error behind them as the cause,
// the constraints a client's data breaks among them, and everything else is
// answered as it is, the server's own failure carrying no service error.
func TestDatabaseError(t *testing.T) {
	serviceErr := serviceregistry.NewError(http.StatusForbidden, "operation refused")

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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer := databaseError(tt.err)

			var answered *serviceregistry.Error
			require.ErrorAs(t, answer, &answered)
			require.Equal(t, tt.wantStatus, answered.Status())
			require.Equal(t, tt.wantMsg, answered.Msg())
			require.ErrorIs(t, answer, tt.err, "the error behind travels as the cause")
		})
	}

	t.Run("other_errors_are_the_servers_own_failure", func(t *testing.T) {
		internal := errors.New("Error 1146: Table 'sample' doesn't exist")

		answer := databaseError(internal)

		var answered *serviceregistry.Error
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
	serviceErr := serviceregistry.NewErrorWithCause(http.StatusInternalServerError, "failed to load user", cause)

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
