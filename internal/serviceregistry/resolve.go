package serviceregistry

import (
	"fmt"

	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

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
// of panicking mid-request.
func Resolve[M types.Model, REQ types.Request, RSP types.Response](key string) types.Service[M, REQ, RSP] {
	mu.RLock()
	svc, ok := services[key]
	mu.RUnlock()
	if !ok {
		if logger.Service != nil {
			logger.Service.Debugz(errNotFoundService.Error(), zap.String("key", key))
		}
		return new(Base[M, REQ, RSP])
	}
	typed, ok := svc.(types.Service[M, REQ, RSP])
	if !ok {
		if logger.Service != nil {
			logger.Service.Errorz("service registered under key does not match the resolved service types; route and service registrations disagree",
				zap.String("key", key), zap.String("registered", fmt.Sprintf("%T", svc)))
		}
		return new(Base[M, REQ, RSP])
	}
	return typed
}
