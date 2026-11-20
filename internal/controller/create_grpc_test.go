package controller_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestCreateCallCreatesTheRecordForTheCaller pins the create call: it
// creates the record the message carried, for the caller the auth
// interceptor named, and answers it as stored; what the hooks find on their
// service context is what an HTTP request's hooks find; a message failing
// the binding tags is refused the way a body is; a hook's refusal answers
// with the hook's status and creates nothing; a message carrying no record
// is refused the way an absent body is; and a route parameter left empty is
// refused before anything runs.
func TestCreateCallCreatesTheRecordForTheCaller(t *testing.T) {
	conn := sampleServer(t)
	name := uniqueName("call-create")

	created, err := invoke(t, conn, "Create", map[string]any{"record": map[string]any{"name": name}})
	require.NoError(t, err)
	require.Equal(t, name, created["name"])
	stored := new(sampleRecord)
	require.NoError(t, database.Database[*sampleRecord](context.Background()).Get(stored, stringOf(created["id"])))
	require.Equal(t, name, stored.Name)
	require.Equal(t, "u-1", stored.GetCreatedBy())
	require.Equal(t, "u-1", stored.GetUpdatedBy())

	t.Run("the hooks find the call on their service context", func(t *testing.T) {
		_, err := invoke(t, conn, "ObservedCreate", map[string]any{
			"params": map[string]any{"box": "b-1"},
			"record": map[string]any{"name": uniqueName("call-observed")},
		})
		require.NoError(t, err)

		observedMu.Lock()
		defer observedMu.Unlock()
		require.Equal(t, "b-1", lastObserved.Box)
		require.Equal(t, "alice", lastObserved.Username)
		require.Equal(t, http.MethodPost, lastObserved.Method)
		require.Equal(t, "/api/controller-sample-observedcreate", lastObserved.Route, "the route of the action, not the full method")
		require.True(t, lastObserved.RequiresAuth)
	})

	t.Run("a message failing validation is refused", func(t *testing.T) {
		_, err := invoke(t, conn, "ValidatedCreate", map[string]any{"record": map[string]any{}})

		requireStatus(t, err, codes.InvalidArgument, "name is a required field")
		details := status.Convert(err).Details()
		require.Len(t, details, 1, "the field refused travels as a BadRequest detail as well")
		bad, ok := details[0].(*errdetails.BadRequest)
		require.True(t, ok, "%T", details[0])
		require.Len(t, bad.GetFieldViolations(), 1)
		require.Equal(t, "name", bad.GetFieldViolations()[0].GetField())
		require.Equal(t, "name is a required field", bad.GetFieldViolations()[0].GetDescription())
	})

	t.Run("a before hook refusal creates nothing", func(t *testing.T) {
		name := uniqueName("call-create-refused")

		_, err := invoke(t, conn, "RefusedCreate", map[string]any{"record": map[string]any{"name": name}})

		requireStatus(t, err, codes.AlreadyExists, refusedMsg)
		require.Zero(t, countSamplesNamed(t, name))
	})

	t.Run("a message carrying no record is refused", func(t *testing.T) {
		_, err := invoke(t, conn, "Create", map[string]any{})
		requireStatus(t, err, codes.InvalidArgument, "record is required")
	})

	t.Run("an empty route parameter is refused", func(t *testing.T) {
		name := uniqueName("call-empty-param")
		_, err := invoke(t, conn, "Create", map[string]any{"params": map[string]string{"parent": ""}, "record": map[string]any{"name": name}})
		requireStatus(t, err, codes.InvalidArgument, `route parameter "parent" is required`)
		require.Zero(t, countSamplesNamed(t, name))
	})

	t.Run("a route parameter spanning segments is refused", func(t *testing.T) {
		// Over HTTP a parameter matches one segment of the path; a value
		// with a slash in it is one no request could carry.
		name := uniqueName("call-slashed-param")
		_, err := invoke(t, conn, "Create", map[string]any{"params": map[string]string{"parent": "a/b"}, "record": map[string]any{"name": name}})
		requireStatus(t, err, codes.InvalidArgument, `route parameter "parent" must not contain "/"`)
		require.Zero(t, countSamplesNamed(t, name))
	})
}

// TestCreateCallRefusesAnHTTPOnlyMethodBeforeWriting pins where a call
// refuses a service that called a method of its context only an HTTP
// request can serve: right after the hook that called it, so a before hook
// leaves nothing written, where an after hook, running once the record is
// written, leaves it written the way its own error would.
func TestCreateCallRefusesAnHTTPOnlyMethodBeforeWriting(t *testing.T) {
	conn := sampleServer(t)

	t.Run("called in a before hook, nothing is written", func(t *testing.T) {
		name := uniqueName("call-cookie-before")
		_, err := invoke(t, conn, "CookieBeforeCreate", map[string]any{"record": map[string]any{"name": name}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
		require.Zero(t, countSamplesNamed(t, name))
	})

	t.Run("called in an after hook, the record written stays", func(t *testing.T) {
		name := uniqueName("call-cookie-after")
		_, err := invoke(t, conn, "CookieAfterCreate", map[string]any{"record": map[string]any{"name": name}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
		require.Equal(t, 1, countSamplesNamed(t, name))
	})
}
