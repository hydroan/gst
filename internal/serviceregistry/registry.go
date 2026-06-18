package serviceregistry

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

var (
	mu       sync.RWMutex
	services = make(map[string]any)
)

var errNotFoundService = errors.New("no service instance matches the given Model interface, skip processing service layer")

// Register registers a concrete service instance for the specified phase.
//
// Register preserves any preconfigured fields on svc. If svc is a non-pointer
// value, the registry stores a pointer copy so controller lookups always work
// with pointer service instances.
func Register[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, svc types.Service[M, REQ, RSP]) {
	register[M, REQ, RSP](phase, svc)
}

func register[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase, svc any) {
	if svc == nil {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	key := serviceKey[M, REQ, RSP](phase)
	val := reflect.ValueOf(svc)
	if !val.IsValid() {
		return
	}
	if val.Kind() == reflect.Pointer && val.IsNil() {
		svc = reflect.New(val.Type().Elem()).Interface()
	} else if val.Kind() != reflect.Pointer {
		ptr := reflect.New(val.Type())
		ptr.Elem().Set(val)
		svc = ptr.Interface()
	}

	setLogger(svc)
	services[key] = svc
}

func serviceKey[M types.Model, REQ types.Request, RSP types.Response](phase consts.Phase) string {
	mTyp := reflect.TypeFor[M]()
	reqTyp := reflect.TypeFor[REQ]()
	rspTyp := reflect.TypeFor[RSP]()

	for mTyp.Kind() == reflect.Pointer {
		mTyp = mTyp.Elem()
	}
	for reqTyp.Kind() == reflect.Pointer {
		reqTyp = reqTyp.Elem()
	}
	for rspTyp.Kind() == reflect.Pointer {
		rspTyp = rspTyp.Elem()
	}

	key := fmt.Sprintf("%s|%s|%s|%s|%s", mTyp.PkgPath(), mTyp.String(), reqTyp.String(), rspTyp.String(), phase)
	return key
}
