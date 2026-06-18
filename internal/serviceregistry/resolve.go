package serviceregistry

import (
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.uber.org/zap"
)

// Resolve returns the registered service for the model/request/response/phase tuple.
//
// When no service is registered, Resolve returns a no-op Base service so default
// CRUD hooks can continue without forcing application code to register services.
func Resolve[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase) types.Service[M, REQ, RSP] {
	key := serviceKey[M, REQ, RSP](phase)

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
