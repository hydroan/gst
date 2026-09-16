package serviceregistry

import (
	"reflect"
	"strings"
	"sync"

	"github.com/hydroan/gst/logger"
)

// initOnce keeps Init to a single pass per process.
var initOnce sync.Once

// Init injects logger.Service into the services registered before the logger
// was initialized, once per process; a service registered later gets it as it
// registers. Bootstrap runs it once the loggers exist.
func Init() error {
	initOnce.Do(func() {
		mu.Lock()
		defer mu.Unlock()

		for _, svc := range services {
			setLogger(svc)
		}
	})
	return nil
}

func setLogger(s any) {
	// Check logger.Service is nil to avoid panic "panic: reflect: call of reflect.Value.Set on zero Value"
	// in statement "fieldLogger.Set(reflect.ValueOf(logger.Service))".
	if logger.Service == nil {
		return
	}
	typ := reflect.TypeOf(s)
	val := reflect.ValueOf(s)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	for val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	for i := range typ.NumField() {
		switch strings.ToLower(typ.Field(i).Name) {
		case "logger": // service object has itself types.Logger
			if val.Field(i).IsZero() {
				val.Field(i).Set(reflect.ValueOf(logger.Service))
			}
		case "base": // service object's types.Logger extends from 'base' struct.
			fieldLogger := val.Field(i).FieldByName("Logger")
			if fieldLogger.IsZero() {
				fieldLogger.Set(reflect.ValueOf(logger.Service))
			}
		}
	}
}
