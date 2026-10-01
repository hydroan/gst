package serviceiamsession

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hydroan/gst"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	modeliamuser "github.com/hydroan/gst/internal/model/iam/user"
	"github.com/hydroan/gst/service"
	"github.com/mssola/useragent"
	"github.com/stretchr/testify/require"
)

// TestAuthenticateDeletesTheSessionOnlyWhenTheUserIsRefused pins what the
// user state's failure does to the session the request named: a refusal of
// the user — gone (401), disabled or locked (403) — deletes it, since it can
// serve no other request, and a failure to read the state (500) keeps it for
// the next request to try again. The state cache is dropped so the refresh
// is reached, and the refresh is stood in for.
func TestAuthenticateDeletesTheSessionOnlyWhenTheUserIsRefused(t *testing.T) {
	const sampleUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	original := refreshUserState
	t.Cleanup(func() { refreshUserState = original })

	cases := []struct {
		name    string
		state   UserState
		err     error
		status  int
		deleted bool
	}{
		{
			name:   "the state cannot be read",
			err:    service.NewError(http.StatusInternalServerError, "failed to refresh session user state"),
			status: http.StatusInternalServerError,
		},
		{
			name:    "the user is gone",
			err:     service.NewError(http.StatusUnauthorized, "session invalid"),
			status:  http.StatusUnauthorized,
			deleted: true,
		},
		{
			name:    "the user is disabled",
			state:   UserState{Status: modeliamuser.UserStatusInactive},
			status:  http.StatusForbidden,
			deleted: true,
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refreshUserState = func(context.Context, string) (UserState, error) { return tc.state, tc.err }

			ua := useragent.New(sampleUserAgent)
			engine, _ := ua.Engine()
			browser, _ := ua.Browser()
			now := time.Now().UTC()
			session := modeliamsession.Session{
				ID:          fmt.Sprintf("sample-session-%d", i),
				UserID:      "sample-user",
				Username:    "sample",
				OS:          ua.OS(),
				Platform:    ua.Platform(),
				EngineName:  engine,
				BrowserName: browser,
				IssuedAt:    now,
				LastSeenAt:  now,
				ExpiresAt:   now.Add(time.Hour),
			}
			require.NoError(t, Store.SaveSession(t.Context(), session, time.Hour))
			t.Cleanup(func() { _, _ = Store.DeleteSession(context.Background(), session.ID) })
			Store.DropUserState(t.Context(), session.UserID)

			_, err := Authenticate(t.Context(), session.ID, sampleUserAgent, http.MethodGet, "/api/records")
			var serviceErr *service.Error
			require.ErrorAs(t, err, &serviceErr)
			require.Equal(t, tc.status, serviceErr.Status())

			_, err = Store.LoadSession(t.Context(), session.ID)
			if tc.deleted {
				require.ErrorIs(t, err, gst.ErrEntryNotFound, "a refused user's session is deleted")
			} else {
				require.NoError(t, err, "a failure to read the user's state keeps the session")
			}
		})
	}
}
