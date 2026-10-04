package serviceiamsession

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/mssola/useragent"
	"go.uber.org/zap"
)

// loadSession returns the stored snapshot sessionID names and whether the
// store holds one it can use; a store that did not answer is the 500 refusal
// "failed to load session" with the failure as its cause, logged. A store that
// does not answer is the server's failure, not the client's, so it is answered
// as one and changes nothing about the session: the client keeps it for the
// store to come back. A value that is not a snapshot never will be, so it is
// deleted, as a snapshot failing validation is, and reported as none held:
// the client logs in again instead of being answered 500 until the key
// expires. Both the authentication of a request and the cookie path of
// CurrentSession read through it.
func loadSession(ctx context.Context, sessionID string) (modeliamsession.Session, bool, error) {
	current, err := Store.LoadSession(ctx, sessionID)
	switch {
	case err == nil:
		return current, true, nil
	case errors.Is(err, gst.ErrEntryNotFound):
		return modeliamsession.Session{}, false, nil
	case errors.Is(err, errSnapshotUnreadable):
		logStoreWarning("deleting an iam session snapshot that cannot be read", sessionID, err)
		if err = deleteUnreadableSnapshot(ctx, sessionID); err != nil {
			logStoreWarning("failed to delete an iam session snapshot that cannot be read", sessionID, err)
		}
		return modeliamsession.Session{}, false, nil
	default:
		logStoreWarning("failed to load iam session", sessionID, err)
		return modeliamsession.Session{}, false, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to load session", err)
	}
}

// Authenticate resolves the session sessionID names for a request or call
// that arrived with userAgent for the action at the HTTP method and path:
// the one path both the HTTP middleware and the gRPC interceptor of the
// module take, so a session is admitted or refused the same way over both
// listeners. The snapshot is loaded and validated; the device the session
// was established from — operating system, platform, browser engine and
// browser, as the user agent names them — has to be the one calling, which
// is what keeps a stolen session id from being used elsewhere, over either
// listener; the mutable state of the session's user is refreshed; while the
// user must change the password, only the actions for that are admitted
// (see MustChangePasswordExempt); and the session is touched.
//
// A refusal is a service error carrying the status and the one fixed
// message the client is answered with: 401 "no session" with no session id,
// 401 "session invalid" for a snapshot the store no longer has, one that
// failed validation or one bound to another device, the user state's own
// service error or 403 "session invalid" for any other failure of it, and
// 403 "password change required before using this resource". The reasons
// are graded — a snapshot storage no longer has, one that expired, one
// issued to another browser or another OS — and answering each in its own
// words would hand the bearer of a stolen session id a probe: it could vary
// one component of the request at a time and read back which one the server
// objected to. The holder of a live session is told nothing by the
// distinction either, since every one of these is answered by logging in
// again, so only the log keeps it. A store that does not answer is the
// server's failure, answered 500 "failed to load session" (see loadSession);
// so is a user state that could not be read, answered with its own 500.
//
// A snapshot that fails validation is deleted on the way out, and so are one
// whose user is refused — gone, disabled or locked — and a stored value that
// is not a snapshot: none can serve another request, and leaving them would
// let every later request pay to load and reject them again. A session the
// store did not answer for, or whose user's state could not be read, is kept
// for the next request to read it once the store is back.
func Authenticate(ctx context.Context, sessionID, userAgent, method, path string) (modeliamsession.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return modeliamsession.Session{}, gst.NewError(http.StatusUnauthorized, "no session")
	}

	current, found, err := loadSession(ctx, sessionID)
	if err != nil {
		return modeliamsession.Session{}, err
	}
	if !found {
		return modeliamsession.Session{}, rejected("session not found", method, path)
	}
	if err = ValidateSession(sessionID, current); err != nil {
		_, _ = Store.DeleteSession(ctx, sessionID)
		return modeliamsession.Session{}, rejected(err.Error(), method, path)
	}
	if reason := deviceMismatch(current, userAgent); reason != "" {
		return modeliamsession.Session{}, rejected(reason, method, path)
	}

	if current, err = ValidateSessionUserState(ctx, current); err != nil {
		// A service error carries a status and a message written for the
		// client. Anything else is an internal failure whose text belongs
		// in logs, not in the answer.
		var serviceErr *gst.Error
		if !errors.As(err, &serviceErr) {
			zap.S().Warnw("iam session rejected", "reason", err.Error(), "path", path, "method", method)
			serviceErr = gst.NewError(http.StatusForbidden, "session invalid")
			err = serviceErr
		}
		// A refused user — gone, disabled or locked — ends the session; a
		// state that could not be read keeps it for the next request.
		if status := serviceErr.Status(); status == http.StatusUnauthorized || status == http.StatusForbidden {
			_, _ = Store.DeleteSession(ctx, sessionID)
		}
		return modeliamsession.Session{}, err
	}
	if current.MustChangePassword && !MustChangePasswordExempt(method, path) {
		return modeliamsession.Session{}, gst.NewError(http.StatusForbidden, "password change required before using this resource")
	}

	if err = Store.TouchSession(ctx, sessionID, current, time.Now()); err != nil {
		zap.S().Warnw("failed to touch iam session", "session_id", sessionID, "error", err)
	}
	return current, nil
}

// deviceMismatch names the first of the four components of the device the
// session was established from that userAgent does not match, "" when it
// matches them all.
func deviceMismatch(current modeliamsession.Session, userAgent string) string {
	ua := useragent.New(userAgent)
	engineName, _ := ua.Engine()
	browserName, _ := ua.Browser()
	switch {
	case current.OS != ua.OS():
		return "os mismatch"
	case current.Platform != ua.Platform():
		return "platform mismatch"
	case engineName != current.EngineName:
		return "engine mismatch"
	case browserName != current.BrowserName:
		return "browser mismatch"
	default:
		return ""
	}
}

// rejected keeps why a session was rejected in the log and returns the 401
// refusal with the one fixed message.
func rejected(reason, method, path string) error {
	zap.S().Warnw("iam session rejected", "reason", reason, "path", path, "method", method)
	return gst.NewError(http.StatusUnauthorized, "session invalid")
}
