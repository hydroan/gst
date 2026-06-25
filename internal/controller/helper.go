package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	. "github.com/hydroan/gst/internal/response"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.opentelemetry.io/otel/trace"
)

func patchValue(log types.Logger, typ reflect.Type, oldVal reflect.Value, newVal reflect.Value) {
	for i := range typ.NumField() {
		// fmt.Println(typ.Field(i).Name, typ.Field(i).Type, typ.Field(i).Type.Kind(), newVal.Field(i).IsValid(), newVal.Field(i).CanSet())
		switch typ.Field(i).Type.Kind() {
		case reflect.Struct: // skip update base model.
			switch typ.Field(i).Type.Name() {
			case "GormTime": // The underlying type of model.GormTime(type of time.Time) is struct, we should continue handle.

			case "Base":
				// Base contains framework-managed fields and should not be patched directly.
				/*
					Legacy Base field patching kept as reference after Remark and Order
					were moved out of model.Base.

					fieldRemark := "Remark"
					if oldVal.FieldByName(fieldRemark).CanSet() {
						if newVal.FieldByName(fieldRemark).IsValid() {
							if !newVal.FieldByName(fieldRemark).IsZero() {
								if newVal.FieldByName(fieldRemark).Kind() == reflect.Pointer {
									var oldValue, newValue any
									if !oldVal.FieldByName(fieldRemark).IsNil() {
										oldValue = oldVal.FieldByName(fieldRemark).Elem().Interface()
									} else {
										oldValue = "<nil>"
									}
									if !newVal.FieldByName(fieldRemark).IsNil() {
										newValue = newVal.FieldByName(fieldRemark).Elem().Interface()
									} else {
										newValue = "<nil>"
									}
									log.Info(fmt.Sprintf("[PATCH %s] field: %q: %v --> %v", fieldRemark, typ.Name(), oldValue, newValue))
								} else {
									log.Info(fmt.Sprintf("[PATCH %s] field: %q: %v --> %v", fieldRemark, typ.Name(),
										oldVal.FieldByName(fieldRemark).Interface(), newVal.FieldByName(fieldRemark).Interface()))
								}
								oldVal.FieldByName(fieldRemark).Set(newVal.FieldByName(fieldRemark))
							}
						}
					}

					fieldOrder := "Order"
					if oldVal.FieldByName(fieldOrder).CanSet() {
						if newVal.FieldByName(fieldOrder).IsValid() {
							if !newVal.FieldByName(fieldOrder).IsZero() {
								if newVal.FieldByName(fieldOrder).Kind() == reflect.Pointer {
									var oldValue, newValue any
									if !oldVal.FieldByName(fieldOrder).IsNil() {
										oldValue = oldVal.FieldByName(fieldOrder).Elem().Interface()
									} else {
										oldValue = "<nil>"
									}
									if !newVal.FieldByName(fieldOrder).IsNil() {
										newValue = newVal.FieldByName(fieldOrder).Elem().Interface()
									} else {
										newValue = "<nil>"
									}
									log.Info(fmt.Sprintf("[PATCH %s] field: %q: %v --> %v", fieldOrder, typ.Name(), oldValue, newValue))
								} else {
									log.Info(fmt.Sprintf("[PATCH %s] field: %q: %v --> %v", fieldOrder, typ.Name(),
										oldVal.FieldByName(fieldOrder).Interface(), newVal.FieldByName(fieldOrder).Interface()))
								}
								oldVal.FieldByName(fieldOrder).Set(newVal.FieldByName(fieldOrder))
							}
						}
					}
				*/
				continue

			default:
				continue
			}
		}
		if !oldVal.Field(i).CanSet() {
			log.Warnf("field %q is cannot set, skip", typ.Field(i).Name)
			continue
		}
		if !newVal.Field(i).IsValid() {
			// log.Warnf("field %s is invalid, skip", typ.Field(i).Name)
			continue
		}
		// base type such like int and string have default value(zero value).
		// If the struct field(the field type is golang base type) supported by patch update,
		// the field type must be pointer to base type, such like *string, *int.
		if newVal.Field(i).IsZero() {
			// log.Warnf("field %s is zero value, skip", typ.Field(i).Name)
			// log.Warnf("DeepEqual: %v : %v : %v : %v", typ.Field(i).Name, newVal.Field(i).Interface(), oldVal.Field(i).Interface(), reflect.DeepEqual(newVal.Field(i), oldVal.Field(i)))
			continue
		}
		// output log must before set value.
		if newVal.Field(i).Kind() == reflect.Pointer {
			var oldValue, newValue any
			if !oldVal.Field(i).IsNil() {
				oldValue = oldVal.Field(i).Elem().Interface()
			} else {
				oldValue = "<nil>"
			}
			if !newVal.Field(i).IsNil() {
				newValue = newVal.Field(i).Elem().Interface()
			} else {
				newValue = "<nil>"
			}
			log.Info(fmt.Sprintf("[PATCH %s] field: %q: %v --> %v", typ.Name(), typ.Field(i).Name, oldValue, newValue))
		} else {
			log.Info(fmt.Sprintf("[PATCH %s] field: %q: %v --> %v", typ.Name(), typ.Field(i).Name, oldVal.Field(i).Interface(), newVal.Field(i).Interface()))
		}
		oldVal.Field(i).Set(newVal.Field(i)) // set old value by new value
	}
}

func extractConfig[M types.Model](cfg ...*types.ControllerConfig[M]) (handler func(ctx context.Context) types.Database[M], db any) {
	if len(cfg) > 0 {
		if cfg[0] != nil {
			db = cfg[0].DB
		}
	}
	handler = func(ctx context.Context) types.Database[M] {
		fn := database.Database[M](ctx)
		if len(cfg) > 0 {
			if cfg[0] != nil {
				if len(cfg[0].TableName) > 0 {
					fn = database.Database[M](ctx).WithDB(cfg[0].DB).WithTable(cfg[0].TableName)
				} else {
					fn = database.Database[M](ctx).WithDB(cfg[0].DB)
				}
			}
		}
		return fn
	}
	return handler, db
}

func requestContext(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return types.ContextWithRequestMetadata(c.Request.Context(), types.RequestMetadataFromGin(c))
}

// startControllerSpan starts a span for controller operations
func startControllerSpan[M types.Model](c *gin.Context, phase consts.Phase) (context.Context, trace.Span) {
	// Get the model name(struct name).
	modelName := reflect.TypeOf(*new(M)).Elem().Name()

	// Create child span for controller operation
	spanName := fmt.Sprintf("Controller.%s %s", phase.MethodName(), modelName)
	parentCtx := gstotel.RequestRootContext(c.Request.Context())
	spanCtx, span := gstotel.StartSpan(parentCtx, spanName)

	// Update request context with new span context
	c.Request = c.Request.WithContext(types.ContextWithRequestMetadata(spanCtx, types.RequestMetadataFromGin(c)))

	if gstotel.IsSpanRecording(span) {
		// Add controller-specific attributes
		gstotel.AddSpanTags(span, map[string]any{
			"component":            "controller",
			"controller.operation": phase.MethodName(),
			"controller.model":     modelName,
			"controller.method":    c.Request.Method,
			"controller.path":      c.FullPath(),
		})
	}

	return spanCtx, span
}

// traceServiceHook traces the service hook execution.
func traceServiceHook[M types.Model](parentCtx context.Context, phase consts.Phase, fn func(context.Context) error) error {
	// Get the model name(struct name).
	modelName := reflect.TypeOf(*new(M)).Elem().Name()

	// Create children span for service operation
	spanName := fmt.Sprintf("Service.%s %s", phase.MethodName(), modelName)
	spanCtx, span := gstotel.StartSpan(parentCtx, spanName)
	defer span.End()

	// // Update request context
	// c.Request = c.Request.WithContext(spanCtx)

	// // Get caller information
	// file, line := getCallerInfo(2)

	recording := gstotel.IsSpanRecording(span)
	if recording {
		// Add service-specific attributes
		gstotel.AddSpanTags(span, map[string]any{
			"component":         "service",
			"service.operation": phase.MethodName(),
			"service.model":     modelName,
			// "code.file":         file,
			// "code.line":         line,
		})
	}

	// Declare error variable for use in defer
	var err error

	var startTime time.Time
	if recording {
		// Record start time and ensure duration + success recorded at the end
		startTime = time.Now()
	}
	defer func() {
		if recording {
			duration := time.Since(startTime)
			gstotel.AddSpanTags(span, map[string]any{
				"hook.duration_ms": duration.Milliseconds(),
				"hook.success":     err == nil,
			})
			if err != nil {
				gstotel.RecordError(span, err)
			}
		}
	}()

	err = fn(spanCtx)
	return err
}

// traceServiceOperation traces the service operation.
func traceServiceOperation[M types.Model, RSP types.Response](parentCtx context.Context, phase consts.Phase, fn func(context.Context) (RSP, error)) (RSP, error) {
	// Get the model name(struct name).
	modelName := reflect.TypeOf(*new(M)).Elem().Name()

	// Create children span for service operation
	spanName := fmt.Sprintf("Service.%s %s", phase.MethodName(), modelName)
	spanCtx, span := gstotel.StartSpan(parentCtx, spanName)
	defer span.End()

	// // Update request context
	// c.Request = c.Request.WithContext(spanCtx)

	// // Get caller information
	// file, line := getCallerInfo(2)

	recording := gstotel.IsSpanRecording(span)
	if recording {
		// Add service-specific attributes
		gstotel.AddSpanTags(span, map[string]any{
			"component":         "service",
			"service.operation": phase.MethodName(),
			"service.model":     modelName,
			// "code.file":         file,
			// "code.line":         line,
		})
	}

	// Declare error variable for use in defer
	var err error
	var rsp RSP

	var startTime time.Time
	if recording {
		// Record start time and ensure duration + success recorded at the end
		startTime = time.Now()
	}
	defer func() {
		if recording {
			duration := time.Since(startTime)
			gstotel.AddSpanTags(span, map[string]any{
				"hook.duration_ms": duration.Milliseconds(),
				"hook.success":     err == nil,
			})
			if err != nil {
				gstotel.RecordError(span, err)
			}
		}
	}()

	rsp, err = fn(spanCtx)
	return rsp, err
}

// traceServiceExport traces the service export operation.
func traceServiceExport[M types.Model, T []byte](parentCtx context.Context, phase consts.Phase, fn func(context.Context) (T, error)) (T, error) {
	// Get the model name(struct name).
	modelName := reflect.TypeOf(*new(M)).Elem().Name()

	// Create children span for service operation
	spanName := fmt.Sprintf("Service.%s %s", phase.MethodName(), modelName)
	spanCtx, span := gstotel.StartSpan(parentCtx, spanName)
	defer span.End()

	// // Update request context
	// c.Request = c.Request.WithContext(spanCtx)

	// // Get caller information
	// file, line := getCallerInfo(2)

	recording := gstotel.IsSpanRecording(span)
	if recording {
		// Add service-specific attributes
		gstotel.AddSpanTags(span, map[string]any{
			"component":         "service",
			"service.operation": phase.MethodName(),
			"service.model":     modelName,
			// "code.file":         file,
			// "code.line":         line,
		})
	}

	// Declare error variable for use in defer
	var err error
	var data T

	var startTime time.Time
	if recording {
		// Record start time and ensure duration + success recorded at the end
		startTime = time.Now()
	}
	defer func() {
		if recording {
			duration := time.Since(startTime)
			gstotel.AddSpanTags(span, map[string]any{
				"hook.duration_ms": duration.Milliseconds(),
				"hook.success":     err == nil,
			})
			if err != nil {
				gstotel.RecordError(span, err)
			}
		}
	}()

	data, err = fn(spanCtx)
	return data, err
}

// traceServiceImport traces the service import operation.
func traceServiceImport[M types.Model](parentCtx context.Context, phase consts.Phase, fn func(context.Context) ([]M, error)) ([]M, error) {
	// Get the model name(struct name).
	modelName := reflect.TypeOf(*new(M)).Elem().Name()

	// Create children span for service operation
	spanName := fmt.Sprintf("Service.%s %s", phase.MethodName(), modelName)
	spanCtx, span := gstotel.StartSpan(parentCtx, spanName)
	defer span.End()

	// // Update request context
	// c.Request = c.Request.WithContext(spanCtx)

	// // Get caller information
	// file, line := getCallerInfo(2)

	recording := gstotel.IsSpanRecording(span)
	if recording {
		// Add service-specific attributes
		gstotel.AddSpanTags(span, map[string]any{
			"component":         "service",
			"service.operation": phase.MethodName(),
			"service.model":     modelName,
			// "code.file":         file,
			// "code.line":         line,
		})
	}

	// Declare error variable for use in defer
	var err error
	var ml []M

	var startTime time.Time
	if recording {
		// Record start time and ensure duration + success recorded at the end
		startTime = time.Now()
	}
	defer func() {
		if recording {
			duration := time.Since(startTime)
			gstotel.AddSpanTags(span, map[string]any{
				"hook.duration_ms": duration.Milliseconds(),
				"hook.success":     err == nil,
			})
			if err != nil {
				gstotel.RecordError(span, err)
			}
		}
	}()

	ml, err = fn(spanCtx)
	return ml, err
}

// handleServiceError handles service-layer errors.
func handleServiceError(c *gin.Context, ctx *types.ServiceContext, err error) {
	if serviceErr, ok := errors.AsType[*service.Error](err); ok {
		JSON(c, serviceErr)
		return
	}

	// Default error handling
	JSON(c, CodeFailure.WithErr(err))
}
