package serviceiamsession

import (
	"sync"
	"time"
)

var (
	sessionExpiration   time.Duration
	sessionExpirationMu sync.RWMutex
)

const sessionsDeleteOthersID = "others"

const (
	defaultSessionExpiration = 8 * time.Hour
	sessionExpirationEnv     = "IAM_SESSION_EXPIRATION"
)

const (
	// SessionCookieName is the HTTP cookie carrying the opaque IAM session id.
	SessionCookieName = "session_id"
	sessionCookiePath = "/"
	sessionIDBytes    = 32
)
