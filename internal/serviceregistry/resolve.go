package serviceregistry

import (
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
)

// Resolve returns the registered service for a key produced by KeyFor with
// the same type parameters.
//
// The lookup deliberately stays per request: services registered after route
// registration remain visible, only the key construction is hoisted to
// route-registration time. When no service is registered under the key, a
// no-op Base service is returned so default CRUD hooks can continue without
// forcing application code to register services.
func Resolve[M types.Model, REQ types.Request, RSP types.Response](key string) types.Service[M, REQ, RSP] {
	mu.RLock()
	svc, ok := services[key]
	mu.RUnlock()
	if !ok {
		if logger.Service != nil {
			logger.Service.Debugz(errNotFoundService.Error(), zap.String("model", key))
		}
		return new(Base[M, REQ, RSP])
	}
	return svc.(types.Service[M, REQ, RSP]) //nolint:errcheck
}
