package iam

// RequireRedisEnabled exposes the startup guard so the package's tests can
// assert it without registering the module a second time.
var RequireRedisEnabled = requireRedisEnabled
