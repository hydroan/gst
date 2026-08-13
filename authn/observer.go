package authn

import (
	"sync"
	"time"

	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

// LoginEventKind names the login lifecycle moment a LoginEvent reports.
type LoginEventKind string

const (
	LoginEventSucceeded LoginEventKind = "succeeded"
	LoginEventFailed    LoginEventKind = "failed"
	LoginEventLoggedOut LoginEventKind = "logged_out"
)

// LoginEvent reports one login lifecycle moment to login observers.
//
// UserID is empty when the attempt failed before an account was resolved. The
// user-agent fields carry what IAM already parsed for its session record, so
// observers do not re-parse UserAgent. At is stamped in UTC.
type LoginEvent struct {
	Kind     LoginEventKind
	UserID   string
	Username string
	TenantID string
	ClientIP string

	UserAgent      string
	OS             string
	Platform       string
	EngineName     string
	EngineVersion  string
	BrowserName    string
	BrowserVersion string

	At time.Time
}

// LoginObserver hears login lifecycle events after they happened. Observers
// cannot block or fail the login: a panicking observer is recovered and the
// remaining observers still run. Slow work belongs on the observer's own
// goroutine.
type LoginObserver func(ctx *types.ServiceContext, event LoginEvent)

// loginObserverEntry gives each registration its own identity so the same
// observer function can be added and removed independently.
type loginObserverEntry struct {
	observe LoginObserver
}

var (
	observerMu     sync.Mutex
	loginObservers []*loginObserverEntry
)

// AddLoginObserver appends one login observer. Observers multicast: every
// added observer hears every event, with no ordering defined between them.
// The returned function removes this registration again and is safe to call
// more than once; tests use it to keep observers scoped. A nil observer adds
// nothing.
func AddLoginObserver(observer LoginObserver) (remove func()) {
	if observer == nil {
		return func() {}
	}

	entry := &loginObserverEntry{observe: observer}
	observerMu.Lock()
	loginObservers = append(loginObservers, entry)
	observerMu.Unlock()

	return func() {
		observerMu.Lock()
		defer observerMu.Unlock()
		for i, candidate := range loginObservers {
			if candidate == entry {
				loginObservers = append(loginObservers[:i], loginObservers[i+1:]...)
				return
			}
		}
	}
}

// NotifyLogin delivers one login lifecycle event to every login observer.
// IAM login and logout call it after the outcome is settled; the delivery
// runs synchronously on the calling goroutine and never changes the outcome.
func NotifyLogin(ctx *types.ServiceContext, event LoginEvent) {
	observerMu.Lock()
	observers := make([]*loginObserverEntry, len(loginObservers))
	copy(observers, loginObservers)
	observerMu.Unlock()

	for _, entry := range observers {
		notifyLoginObserver(ctx, entry.observe, event)
	}
}

// notifyLoginObserver isolates one observer call so a panicking observer
// cannot take down the login request or the remaining observers.
func notifyLoginObserver(ctx *types.ServiceContext, observer LoginObserver, event LoginEvent) {
	defer func() {
		if recovered := recover(); recovered != nil {
			// logger.App stays nil until logger initialization; hook machinery
			// must not require a configured logger to stay safe.
			if logger.App != nil {
				logger.App.Errorz(
					"login observer panicked",
					zap.Any("recovered", recovered),
					zap.Stack("stack"),
				)
			}
		}
	}()

	observer(ctx, event)
}
