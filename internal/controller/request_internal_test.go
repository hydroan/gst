package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// setIDUUIDRecord is keyed by a string id, which takes any value.
type setIDUUIDRecord struct {
	Name string `json:"name"`

	modelregistry.Base
}

// setIDIntegerRecord is keyed by an auto-increment integer, whose id takes a
// positive number alone.
type setIDIntegerRecord struct {
	Name string `json:"name"`

	modelregistry.AutoBase
}

// TestSetIDAcceptsAnyValueForUUIDKeyedModel pins that a string-keyed model
// takes any non-empty id as it is.
func TestSetIDAcceptsAnyValueForUUIDKeyedModel(t *testing.T) {
	m := new(setIDUUIDRecord)

	require.True(t, setID(m, "custom-id"))
	require.Equal(t, "custom-id", m.GetID())
}

// TestSetIDNormalizesIntegerKeyedModelID pins that an integer-keyed model
// takes an id with leading zeros as the number it names.
func TestSetIDNormalizesIntegerKeyedModelID(t *testing.T) {
	m := new(setIDIntegerRecord)

	require.True(t, setID(m, "007"))
	require.Equal(t, "7", m.GetID())
	require.Equal(t, uint64(7), m.ID)
}

// TestSetIDRejectsUnparsableIntegerKeyedModelID pins that an integer-keyed
// model leaves its id unset for a value naming no positive number it can
// hold, which setID reports.
func TestSetIDRejectsUnparsableIntegerKeyedModelID(t *testing.T) {
	for _, id := range []string{"abc", "7abc", "0", "-1", "18446744073709551616"} {
		m := new(setIDIntegerRecord)

		require.Falsef(t, setID(m, id), "id %q should be rejected", id)
		require.Zero(t, m.ID)
	}
}

// TestRequestContextEndsWithTheClientGoingAway pins what every handler's
// database work runs on. The context the controllers pass to the service
// layer, to their own reads and writes, and to the audit entry is the
// request's own, so a client that goes away — or a write timeout tripping —
// ends the work in flight: a transaction opened on it rolls back, and the
// statement running under it is canceled. Work that must outlive the request
// takes a context of its own.
func TestRequestContextEndsWithTheClientGoingAway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	clientGone, goAway := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/users", nil).WithContext(clientGone)

	ctx := requestContext(c)
	require.NoError(t, ctx.Err(), "the work of a request still in flight runs on")

	goAway()
	require.ErrorIs(t, ctx.Err(), context.Canceled, "the work of a request whose client went away is told to stop")
}
