// Package service exposes the public service extension points for application code.
//
// Application services should embed Base and register service types with Register.
// Controller lookup, concrete instance registration, and registry state are kept
// inside the framework so external projects cannot mutate framework-owned service
// mappings directly.
package service

import (
	"reflect"
	"sync"

	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

var _ types.Service[*model.Empty, any, any] = (*Base[*model.Empty, any, any])(nil)

var once sync.Once

// Register registers a service type for the specified phase.
//
// Register is the public service registration entry point for application and
// generated service code. Framework internals handle service lookup and concrete
// instance registration through an internal registry.
//
// The service type parameter S can be either a pointer to a struct type (e.g., *MyService)
// or a non-pointer struct type (e.g., MyService). The function will automatically handle
// both cases and always store a pointer instance in the service map.
//
// Example usage with pointer type:
//
//	type myService struct {
//	    service.Base[*model.User, *request.CreateUserReq, *response.CreateUserRsp]
//	}
//
//	func init() {
//	    service.Register[*myService](consts.PHASE_CREATE)
//	}
//
// Example usage with non-pointer type:
//
//	type myService struct {
//	    service.Base[*model.User, *request.CreateUserReq, *response.CreateUserRsp]
//	}
//
//	func init() {
//	    service.Register[myService](consts.PHASE_CREATE)
//	}
//
// Logger initialization:
//   - If Register is called in an "init" function, logger.Service may be nil,
//     and the service.Logger will be set later in service.Init().
//   - If Register is called after initialization (e.g., in Init function),
//     logger.Service is already available, and the service.Logger will be set directly.
func Register[S types.Service[M, REQ, RSP], M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase) {
	typ := reflect.TypeFor[S]()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	svc, ok := reflect.New(typ).Interface().(types.Service[M, REQ, RSP])
	if !ok {
		return
	}
	serviceregistry.Register[M, REQ, RSP](phase, svc)
}

func Init() error {
	once.Do(func() {
		serviceregistry.InitLoggers()
	})
	return nil
}

// Base is the default no-op service implementation exposed to application services.
type Base[M types.Model, REQ types.Request, RSP types.Response] = serviceregistry.Base[M, REQ, RSP]
