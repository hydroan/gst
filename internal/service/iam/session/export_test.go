package serviceiamsession

// The key layout is private to the store so that no caller can address storage
// the store does not offer. The tests that cover the store itself still have to
// look at the keys it writes — an index ttl and a sweep leave no trace any
// public method reports — so the builders are re-exported here.
//
// This file is only compiled into the package's tests, so the names below exist
// for the test binary and are absent from anything that ships. The tests cannot
// live in the package itself: testutil reaches bootstrap, which reaches the
// middleware that imports this package, and an internal test file would close
// that cycle.
var (
	SessionDataKey      = sessionDataKey
	SessionIndexUserKey = sessionIndexUserKey
	SessionIndexAllKey  = sessionIndexAllKey
	SessionIndexSeenKey = sessionIndexSeenKey
	UserStateKey        = userStateKey
)

const (
	SessionNamespace = sessionNamespace
	UserNamespace    = userNamespace
)
