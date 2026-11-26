package service

import (
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
	"go.uber.org/zap"
)

// errNotFoundService words the debug entry Resolve writes when no service is
// registered under a key.
var errNotFoundService = errors.New("no service instance matches the given route and phase, skip processing service layer")

// errNotRegistered is the error of a Lookup that found no service under the
// key.
var errNotRegistered = errors.New("no service is registered for it")

// mismatchError is the error of a Lookup that found a service registered
// for other types than the ones asked for.
type mismatchError struct {
	registered any
}

func (e *mismatchError) Error() string {
	return fmt.Sprintf("the one registered, %T, serves other types than the route's", e.registered)
}

// Lookup returns the service registered under key, a key Key produced, for
// the types M, REQ and RSP, and an error telling what is wrong when there
// is none: no service registered under the key, or one registered for
// other types, the route and the service registrations disagreeing about
// them.
func Lookup[M gst.Model, REQ gst.Request, RSP gst.Response](key string) (gst.Service[M, REQ, RSP], error) {
	mu.RLock()
	svc, ok := services[key]
	mu.RUnlock()
	if !ok {
		return nil, errNotRegistered
	}
	typed, ok := svc.(gst.Service[M, REQ, RSP])
	if !ok {
		return nil, &mismatchError{registered: svc}
	}
	return typed, nil
}

// Resolve returns the registered service for a key produced by Key.
//
// The lookup deliberately stays per request: services registered after route
// registration remain visible, only the key construction is hoisted to
// route-registration time. When no service is registered under the key, a
// no-op Base service is returned so default CRUD hooks can continue without
// forcing application code to register services.
//
// A service found under the key but failing the type assertion means the
// route and service registrations disagree about the service types — a wiring
// bug. It is reported at error level and degrades to the no-op Base instead
// of panicking mid-request; a route whose config declares its service is
// held to it as it registers instead (see gst.ControllerConfig).
func Resolve[M gst.Model, REQ gst.Request, RSP gst.Response](key string) gst.Service[M, REQ, RSP] {
	svc, err := Lookup[M, REQ, RSP](key)
	if err == nil {
		return svc
	}
	if logger.Service != nil {
		if mismatch, ok := err.(*mismatchError); ok { //nolint:errorlint // Lookup returns the error unwrapped, and this runs per request.
			logger.Service.Errorz("service registered under key does not match the resolved service types; route and service registrations disagree",
				zap.String("key", key), zap.String("registered", fmt.Sprintf("%T", mismatch.registered)))
		} else {
			logger.Service.Debugz(errNotFoundService.Error(), zap.String("key", key))
		}
	}
	return new(Base[M, REQ, RSP])
}
