package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/requestctx"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/types"
)

// setRouteID copies the route id parameter into the model and reports whether
// the model accepted it. UUID-keyed models (Base) accept any non-empty string,
// so the check never changes their behavior. Integer-keyed models (AutoBase)
// leave the id unset when the raw value does not parse into their key type;
// handlers must answer such requests with "not found" before any database
// access, because an unset id would silently drop the intended row filter and
// passing the raw value to SQL would rely on the database's implicit
// string-to-integer coercion (MySQL matches id=7 for '7abc').
//
// The caller must pass a non-empty id: UUID-keyed models generate a fresh id
// when given an empty value.
func setRouteID(m types.Model, id string) bool {
	m.SetID(id)
	return len(m.GetID()) > 0
}

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
		if field.Type.Kind() == reflect.Struct { // skip update base model.
			// Base and AutoBase contain framework-managed fields and should not
			// be patched directly; other nested struct fields are skipped from
			// patching as well.
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

// patchJSONFieldNamesCache caches the JSON-name-to-field-name mapping per
// model type. The mapping is derived from struct tags only, so it is computed
// once per type instead of on every patch request; cached maps are read-only.
var patchJSONFieldNamesCache sync.Map // reflect.Type -> map[string]string

func patchJSONFieldNames(typ reflect.Type) map[string]string {
	if cached, ok := patchJSONFieldNamesCache.Load(typ); ok {
		return cached.(map[string]string) //nolint:errcheck
	}
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
	patchJSONFieldNamesCache.Store(typ, fields)
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

// routeFromConfig returns the route carried by the controller config, or an
// empty string when no config declares one.
func routeFromConfig[M types.Model](cfg ...*types.ControllerConfig[M]) string {
	if len(cfg) > 0 && cfg[0] != nil {
		return cfg[0].Route
	}
	return ""
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

// handleServiceError handles service-layer errors.
func handleServiceError(c *gin.Context, err error) {
	var serviceErr *serviceregistry.Error
	if errors.As(err, &serviceErr) {
		JSON(c, serviceErr)
		return
	}

	// Default error handling
	JSON(c, CodeFailure.WithErr(err))
}

// writeErrorCoder maps database write errors to their canonical API codes:
// database.ErrRecordNotFound renders 404 and database.ErrDuplicatedKey renders
// 409 with their fixed client-safe messages; anything else falls back to
// CodeFailure carrying the error text. Handlers log the full error themselves,
// so the not-found/duplicate branches deliberately drop internal detail from
// the response.
func writeErrorCoder(err error) types.Coder {
	switch {
	case errors.Is(err, database.ErrRecordNotFound):
		return CodeNotFound
	case errors.Is(err, database.ErrDuplicatedKey):
		return CodeAlreadyExist
	default:
		return CodeFailure.WithErr(err)
	}
}
