package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"go.opentelemetry.io/otel/trace"
)

type patchFieldSet map[string]struct{}

func patchValue(log types.Logger, typ reflect.Type, oldVal reflect.Value, newVal reflect.Value, fieldSets ...patchFieldSet) {
	var fields patchFieldSet
	if len(fieldSets) > 0 {
		fields = fieldSets[0]
	}

	for i := range typ.NumField() {
		// fmt.Println(typ.Field(i).Name, typ.Field(i).Type, typ.Field(i).Type.Kind(), newVal.Field(i).IsValid(), newVal.Field(i).CanSet())
		field := typ.Field(i)
		if fields != nil {
			if _, ok := fields[field.Name]; !ok {
				continue
			}
		}
		switch field.Type.Kind() {
		case reflect.Struct: // skip update base model.
			switch field.Type.Name() {
			case "GormTime": // The underlying type of model.GormTime(type of time.Time) is struct, we should continue handle.

			case "Base", "AutoBase":
				// Base and AutoBase contain framework-managed fields and should not be patched directly.
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
			log.Warnf("field %q is cannot set, skip", field.Name)
			continue
		}
		if !newVal.Field(i).IsValid() {
			// log.Warnf("field %s is invalid, skip", typ.Field(i).Name)
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
			log.Infof("[PATCH %s] field: %q: %v --> %v", typ.Name(), field.Name, oldValue, newValue)
		} else {
			log.Infof("[PATCH %s] field: %q: %v --> %v", typ.Name(), field.Name, oldVal.Field(i).Interface(), newVal.Field(i).Interface())
		}
		oldVal.Field(i).Set(newVal.Field(i)) // set old value by new value
	}
}

func readJSONRequestBody(c *gin.Context) ([]byte, error) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil, io.EOF
	}
	body, err := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, err
}

func patchFieldSetFromJSONBody(typ reflect.Type, body []byte) (patchFieldSet, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return patchFieldSet{}, io.EOF
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	return patchFieldSetFromJSONFields(typ, fields), nil
}

func patchManyFieldSetsFromJSONBody(typ reflect.Type, body []byte) ([]patchFieldSet, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, io.EOF
	}
	var req struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	fieldSets := make([]patchFieldSet, 0, len(req.Items))
	for _, item := range req.Items {
		fieldSets = append(fieldSets, patchFieldSetFromJSONFields(typ, item))
	}
	return fieldSets, nil
}

func patchFieldSetFromJSONFields(typ reflect.Type, fields map[string]json.RawMessage) patchFieldSet {
	if len(fields) == 0 {
		return patchFieldSet{}
	}
	jsonFields := patchJSONFieldNames(typ)
	fieldSet := make(patchFieldSet, len(fields))
	for name := range fields {
		if fieldName, ok := jsonFields[name]; ok {
			fieldSet[fieldName] = struct{}{}
		}
	}
	return fieldSet
}

func patchJSONFieldNames(typ reflect.Type) map[string]string {
	fields := make(map[string]string, typ.NumField())
	for field := range typ.Fields() {
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		name, ok := patchJSONFieldName(field)
		if !ok {
			continue
		}
		fields[name] = field.Name
	}
	return fields
}

func patchJSONFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	if comma := strings.IndexByte(tag, ','); comma >= 0 {
		tag = tag[:comma]
	}
	if tag != "" {
		return tag, true
	}
	return field.Name, true
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
	return requestctx.WithMetadata(c.Request.Context(), requestctx.FromGin(c))
}

// startControllerSpan starts a span for controller operations
func startControllerSpan[M types.Model](c *gin.Context, phase consts.Phase) (context.Context, trace.Span) {
	// Get the model name(struct name).
	modelName := reflect.TypeOf(*new(M)).Elem().Name()

	// Use the canonical gst span name so Jaeger labels stay structured.
	spanName := gstotel.FrameworkSpanName("controller", modelName, phase.MethodName())
	parentCtx := gstotel.RequestRootContext(c.Request.Context())
	spanCtx, span := gstotel.StartSpan(parentCtx, spanName)

	// Update request context with new span context
	c.Request = c.Request.WithContext(requestctx.WithMetadata(spanCtx, requestctx.FromGin(c)))

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

	// Use the canonical gst span name so service hooks group by model first.
	spanName := gstotel.FrameworkSpanName("service", modelName, phase.MethodName())
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

	// Use the canonical gst span name so service operations group by model first.
	spanName := gstotel.FrameworkSpanName("service", modelName, phase.MethodName())
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

	// Use the canonical gst span name so service export spans match CRUD spans.
	spanName := gstotel.FrameworkSpanName("service", modelName, phase.MethodName())
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

	// Use the canonical gst span name so service import spans match CRUD spans.
	spanName := gstotel.FrameworkSpanName("service", modelName, phase.MethodName())
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
	var serviceErr *service.Error
	if errors.As(err, &serviceErr) {
		JSON(c, serviceErr)
		return
	}

	// Default error handling
	JSON(c, CodeFailure.WithErr(err))
}
