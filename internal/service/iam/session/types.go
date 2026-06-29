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
	// SessionCookieName is the HTTP cookie carrying the opaque IAM session id.
	SessionCookieName = "session_id"
	sessionCookiePath = "/"
	sessionIDBytes    = 32
)
