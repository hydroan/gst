package database

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
	"github.com/stoewer/go-strcase"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// trace returns a timing function for database operations that provides comprehensive
// performance monitoring, logging, and distributed tracing capabilities.
// The returned function should be called with the operation result to complete tracing and logging.
//
// Parameters:
//   - op: Operation name for logging and tracing identification (Create, List, Update, Delete, etc.)
//   - batch: Optional batch size for batch operations (used for span attributes and logging)
//
// Returns a function that accepts an error and completes the operation tracing and logging.
//
// Features:
//   - Automatic timing measurement from call to completion
//   - OTEL distributed tracing integration with OpenTelemetry spans
//   - Comprehensive span attributes including operation metadata
//   - Error-aware logging and span status management
//   - Batch operation support with size tracking
//   - Cache and dry-run mode status recording
//   - Smart duration formatting for readability
//   - Context propagation to GORM operations
//
// OTEL Tracing Integration:
//   - Creates OpenTelemetry spans with naming pattern: "database.{Model}.{Operation}"
//   - Records detailed span attributes: component, operation, model, table, batch_size, etc.
//   - Propagates span context to GORM operations for complete tracing hierarchy
//   - Automatically handles span lifecycle (creation, attribute setting, completion)
//   - Integrates with existing tracing infrastructure (controller and service layers)
//   - Ensures trace_id is available in database logs through request metadata or
//     the active OTEL span context
//
// Usage Pattern:
//
//	done := db.trace("Create", len(models))
//	defer done(err)
//
// Tracing Hierarchy:
//
//	HTTP → Controller → Service → Database → GORM
//
// Note: Must be called after `defer db.reset()` to ensure proper cleanup order.
// Jaeger tracing is automatically enabled when gstotel.IsEnabled() returns true.
func (db *database[M]) trace(op string, batch ...int) (func(error), context.Context, trace.Span) {
	begin := time.Now()
	var _batch int
	if len(batch) > 0 {
		_batch = batch[0]
	}

	ctx := db.ctx
	var span trace.Span
	if gstotel.IsEnabled() && ctx != nil {
		modelName := reflect.TypeOf(*new(M)).Elem().Name()
		spanName := gstotel.FrameworkSpanName("database", modelName, op)
		ctx, span = gstotel.StartSpan(ctx, spanName)
		ctx = requestctx.WithMetadata(ctx, requestctx.FromContext(db.ctx))
		db.ctx = ctx

		// Update GORM database context with new span context
		db.ins = db.ins.WithContext(db.ctx)

		if gstotel.IsSpanRecording(span) {
			attrs := []attribute.KeyValue{
				attribute.String("component", "database"),
				attribute.String("database.operation", op),
				attribute.String("database.model", modelName),
				attribute.String("database.table", modelName),
				attribute.Bool("database.cache_enabled", db.enableCache),
				attribute.Bool("database.dry_run", db.dryRun),
			}
			if _batch > 0 {
				attrs = append(attrs, attribute.Int("database.batch_size", _batch))
			}
			span.SetAttributes(attrs...)
		}
	}

	return func(err error) {
		if span != nil {
			defer span.End()
		}

		// Record duration
		duration := time.Since(begin)

		// Update span with results if available
		if gstotel.IsSpanRecording(span) {
			span.SetAttributes(attribute.Int64("database.duration_ms", duration.Milliseconds()))

			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				gstotel.RecordError(span, err)
				span.SetAttributes(attribute.Bool("error", true))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		}

		// Log operation results
		if err != nil {
			logger.Database.WithContext(db.ctx, consts.Phase(op)).Errorz(
				"",
				zap.Error(err),
				zap.String("table", reflect.TypeOf(*new(M)).Elem().Name()),
				zap.String("batch", strconv.Itoa(_batch)),
				zap.String("cost", util.FormatDurationSmart(duration)),
				zap.Bool("cache_enabled", db.enableCache),
				zap.Bool("dry_run", db.dryRun),
			)
		} else {
			logger.Database.WithContext(db.ctx, consts.Phase(op)).Infoz(
				"",
				zap.String("table", reflect.TypeOf(*new(M)).Elem().Name()),
				zap.String("batch", strconv.Itoa(_batch)),
				zap.String("cost", util.FormatDurationSmart(time.Since(begin))),
				zap.Bool("cache_enabled", db.enableCache),
				zap.Bool("dry_run", db.dryRun),
			)
		}
	}, ctx, span
}

// queryColumnName resolves the condition column name for a struct field:
// the "query" tag wins over the "json" tag, which wins over the field name,
// and the result is converted to snake case to match gorm column naming.
func queryColumnName(ctx context.Context, field reflect.StructField) string {
	// "json" tag priority is higher than field.Name
	jsonTag := strings.TrimSpace(field.Tag.Get("json"))
	if idx := strings.Index(jsonTag, ","); idx >= 0 {
		jsonTag = jsonTag[:idx]
	}
	if len(jsonTag) == 0 {
		// the structure lowercase field name as the query condition.
		jsonTag = field.Name
	}
	// "query" tag have higher priority than "json" tag
	queryTag := strings.TrimSpace(field.Tag.Get("query"))
	if idx := strings.Index(queryTag, ","); idx >= 0 {
		queryTag = queryTag[:idx]
	}
	if len(queryTag) > 0 && queryTag != jsonTag {
		logger.Database.WithContext(ctx, consts.Phase("StructFieldToMap")).Infoz("json tag replace by query tag", zap.String("old", jsonTag), zap.String("new", queryTag))
		jsonTag = queryTag
	}
	// json tag name naming format must be same as gorm table columns,
	// both should be "snake case" or "camel case".
	// gorm table columns naming format default to 'snake case',
	// so the json tag name is converted to "snake case" here.
	return strcase.SnakeCase(jsonTag)
}

// structFieldToMap extracts the field tags from a struct and writes them into a map.
// This map can then be used to build SQL query conditions.
//
// Zero-value fields are treated as unset and skipped, unless their column name
// is listed in present: presence marks filter values explicitly provided by the
// caller, so explicit zero values such as false and 0 still become conditions.
// FIXME: if the field type is boolean or integer, disable the fuzzy matching.
func structFieldToMap(ctx context.Context, typ reflect.Type, val reflect.Value, q map[string]string, present map[string]struct{}) {
	if q == nil {
		q = make(map[string]string)
	}
	for i := range typ.NumField() {
		field := typ.Field(i)
		fieldTyp := field.Type
		fieldVal := val.Field(i)

		if fieldVal.IsZero() {
			if len(present) == 0 {
				continue
			}
			if _, ok := present[queryColumnName(ctx, field)]; !ok {
				continue
			}
		}
		if !fieldVal.CanInterface() {
			continue
		}
		fieldTyp, fieldVal, ok := indirectTypeAndValue(fieldTyp, fieldVal)
		if !ok {
			continue
		}
		// The marker interfaces are sealed in modelregistry, so recognition of
		// framework query fields lives there as the single source of truth.
		if modelregistry.IsQueryMarkerType(fieldTyp) {
			continue
		}

		switch fieldTyp.Kind() {
		case reflect.Chan, reflect.Map, reflect.Func:
			continue
		case reflect.Struct:
			// Base and AutoBase are the framework base models: lift only their
			// query-relevant fields instead of recursing, so framework-managed
			// fields such as DeletedAt never leak into query conditions.
			if field.Name == "Base" || field.Name == "AutoBase" {
				if !fieldVal.FieldByName("CreatedBy").IsZero() {
					// Not overwrite the "CreatedBy" value set in types.Model.
					// The "CreatedBy" value set in types.Model has higher priority than base model.
					if _, loaded := q["created_by"]; !loaded {
						q["created_by"] = fieldVal.FieldByName("CreatedBy").Interface().(string) //nolint:errcheck
					}
				}
				if !fieldVal.FieldByName("UpdatedBy").IsZero() {
					// Not overwrite the "UpdatedBy" value set in types.Model.
					// The "UpdatedBy" value set in types.Model has higher priority than base model.
					if _, loaded := q["updated_by"]; !loaded {
						q["updated_by"] = fieldVal.FieldByName("UpdatedBy").Interface().(string) //nolint:errcheck
					}
				}
				if !fieldVal.FieldByName("ID").IsZero() {
					// Not overwrite the "ID" value set in types.Model.
					// The "ID" value set in types.Model has higher priority than base model.
					if _, loaded := q["id"]; !loaded {
						switch idField := fieldVal.FieldByName("ID"); idField.Kind() {
						case reflect.String: // Base: UUIDv7 string primary key.
							q["id"] = idField.String()
						case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64: // AutoBase: auto-increment integer primary key.
							q["id"] = strconv.FormatUint(idField.Uint(), 10)
						}
					}
				}
				/*
					Legacy Base Remark query mapping kept as reference after Remark
					was moved out of model.Base.

					remarkField := fieldVal.FieldByName("Remark")
					if remarkField.IsValid() && !remarkField.IsZero() {
						if _, loaded := q["remark"]; !loaded {
							if remarkField.Kind() == reflect.Pointer {
								if !remarkField.IsNil() {
									q["remark"] = remarkField.Elem().Interface().(string) //nolint:errcheck
								}
							} else {
								q["remark"] = remarkField.Interface().(string) //nolint:errcheck
							}
						}
					}
				*/
			} else {
				structFieldToMap(ctx, fieldTyp, fieldVal, q, present)
			}
			continue
		}
		columnName := queryColumnName(ctx, field)

		if !fieldVal.CanInterface() {
			continue
		}
		v := fieldVal.Interface()
		var _v string
		switch fieldVal.Kind() {
		case reflect.Bool:
			// 由于 WHERE IN 语句会自动加上单引号,比如 WHERE `default` IN ('true')
			// 但是我们想要的是 WHERE `default` IN (true),
			// 所以没办法就只能直接转成 int 了.
			_v = strconv.Itoa(boolToInt(v.(bool))) //nolint:errcheck
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			_v = fmt.Sprintf("%d", v)
		case reflect.Float32, reflect.Float64:
			_v = fmt.Sprintf("%g", v)
		case reflect.String:
			_v = fmt.Sprintf("%s", v)
		case reflect.Pointer:
			v = fieldVal.Elem().Interface()
			// switch typ.Elem().Kind() {
			switch fieldVal.Elem().Kind() {
			case reflect.Bool:
				_v = strconv.Itoa(boolToInt(v.(bool))) //nolint:errcheck
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				_v = fmt.Sprintf("%d", v)
			case reflect.Float32, reflect.Float64:
				_v = fmt.Sprintf("%g", v)
			case reflect.String:
				_v = fmt.Sprintf("%s", v)
			case reflect.Struct, reflect.Map, reflect.Chan, reflect.Func: // ignore the struct, map, chan, func
			default:
				_v = fmt.Sprintf("%v", v)
			}
		case reflect.Slice:
			_len := fieldVal.Len()
			if _len == 0 {
				logger.Database.WithContext(ctx, consts.Phase("WithQuery")).Warn("reflect.Slice length is 0")
				_len = 1
			}
			slice := reflect.MakeSlice(fieldVal.Type(), _len, _len)
			// fmt.Println("--------------- slice element", slice.Index(0), slice.Index(0).Kind(), slice.Index(0).Type())
			switch slice.Index(0).Kind() {
			case reflect.String: // handle string slice.
				// WARN: fieldVal.Type() may be a named slice type (e.g.
				// datatypes.JSONSlice[string]) instead of []string, so asserting
				// slice.Interface().([]string) directly would panic. Rebuild the
				// value as a plain []string first.
				slice = reflect.MakeSlice(reflect.TypeFor[[]string](), _len, _len)
				reflect.Copy(slice, fieldVal)
				_v = strings.Join(slice.Interface().([]string), ",") //nolint:errcheck
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			// TODO: handle integer slice.
			case reflect.Float32, reflect.Float64:
			// TODO: handle float slice.
			default:
				_v = fmt.Sprintf("%v", v)
			}
		case reflect.Struct, reflect.Map, reflect.Chan, reflect.Func: // ignore the struct, map, chan, func
		default:
			_v = fmt.Sprintf("%v", v)
		}

		q[columnName] = _v
	}
}

// buildCacheKey constructs Redis cache keys for database operations.
// Generates both prefix and full key based on GORM statement and operation type.
// Uses consistent naming convention for cache key organization and collision avoidance.
//
// Parameters:
//   - stmt: GORM statement containing SQL and table information
//   - action: Operation type ("get", "list", "count", etc.)
//   - id: Optional ID for get operations to create simpler keys
//
// Returns prefix, table name and full cache key for Redis operations.
//
// Key Structure:
//   - Prefix: namespace:table_name
//   - Full Key: namespace:table_name:action:identifier
//   - Get operations with ID: namespace:table_name:get:id_value
//   - Other operations: namespace:table_name:action:sql_statement
//
// Features:
//   - Namespace isolation for multi-tenant applications
//   - Table-based key organization
//   - Operation-specific key generation
//   - SQL statement-based cache invalidation
//
// Reference: https://gorm.io/docs/sql_builder.html
func buildCacheKey(stmt *gorm.Statement, action string, id ...string) (prefix, table, key string) {
	prefix = strings.Join([]string{config.App.Redis.Namespace, stmt.Table}, ":")
	table = stmt.Table
	switch strings.ToLower(action) {
	case "get":
		if len(id) > 0 {
			key = strings.Join([]string{config.App.Redis.Namespace, stmt.Table, action, id[0]}, ":")
		} else {
			key = strings.Join([]string{config.App.Redis.Namespace, stmt.Table, action, stmt.SQL.String()}, ":")
		}
	default:
		key = strings.Join([]string{config.App.Redis.Namespace, stmt.Table, action, stmt.SQL.String()}, ":")
	}
	return prefix, table, key
}

// boolToInt converts a boolean value to an integer.
// Returns 1 for true, 0 for false.
// Useful for database operations that require integer representations of boolean values.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// traceModelHook traces model hook execution with OpenTelemetry spans.
// Creates a span for the hook execution and records timing and error information.
//
// Parameters:
//   - ctx: Database context for span creation
//   - hookName: Name of the hook being executed (CreateBefore, CreateAfter, etc.)
//   - modelName: Name of the model type
//   - fn: Hook function to execute
//
// Returns error from hook execution, with span automatically completed.
//
// Features:
//   - Automatic span creation with naming pattern: "Hook.{HookName} {ModelName}"
//   - Records hook execution timing and success/failure status
//   - Integrates with existing tracing infrastructure
//   - Error recording and span status management
//
// Usage Pattern:
//
//	err := traceModelHook(db.ctx, "CreateBefore", "User", func() error {
//		return obj.CreateBefore()
//	})
func traceModelHook[M types.Model](ctx context.Context, phase consts.Phase, parentSpan trace.Span, fn func(ctx context.Context) error) error {
	hookCtx := context.Background()
	if ctx != nil {
		hookCtx = ctx
	}
	if !gstotel.IsEnabled() || ctx == nil || parentSpan == nil {
		return fn(hookCtx)
	}

	modelName := reflect.TypeOf(*new(M)).Elem().Name()
	// Use a structured gst span name under the database span for hook execution.
	spanName := gstotel.FrameworkSpanName("model", modelName, phase.MethodName())
	parentCtx := trace.ContextWithSpan(hookCtx, parentSpan)
	childCtx, span := gstotel.StartSpan(parentCtx, spanName)
	defer span.End()

	recording := gstotel.IsSpanRecording(span)
	var start time.Time
	if recording {
		// Add hook-specific attributes
		span.SetAttributes(
			attribute.String("component", "model"),
			attribute.String("model.model", modelName),
			attribute.String("model.phase", phase.MethodName()),
		)

		// Record start time
		start = time.Now()
	}

	// Execute hook function
	err := fn(childCtx)

	if recording {
		// Record execution results
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("model.duration_ms", duration.Milliseconds()),
			attribute.Bool("model.success", err == nil),
		)

		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			gstotel.RecordError(span, err)
			span.SetAttributes(attribute.Bool("error", true))
		} else {
			span.SetStatus(codes.Ok, "")
		}
	}

	return err
}

// transactionSpanName returns the canonical OTel span name for Transaction/TransactionFunc,
// e.g. "database.User.Transaction". Uses the same "database" component as trace() so
// transaction spans sit in the same namespace as ordinary Create/List/Update/Delete spans.
func transactionSpanName(modelName, operation string) string {
	return gstotel.FrameworkSpanName("database", modelName, operation)
}

// contains checks if a string item exists in a string slice.
// Uses a map-based approach for O(n) time complexity with O(n) space complexity.
// More efficient than linear search for larger slices.
//
// Parameters:
//   - slice: The string slice to search in
//   - item: The string item to search for
//
// Returns true if the item is found, false otherwise.
func contains(slice []string, item string) bool {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}
	_, ok := set[item]
	return ok
}

// indirectTypeAndValue recursively dereferences pointer types and values.
// Follows pointer chains until reaching a non-pointer type.
// Used for reflection operations that need to work with the underlying concrete type.
//
// Parameters:
//   - t: The reflect.Type to dereference
//   - v: The reflect.Value to dereference
//
// Returns:
//   - reflect.Type: The final non-pointer type
//   - reflect.Value: The final non-pointer value
//   - bool: true if successful, false if encountered nil pointer
//
// Example:
//   - Input: **int (pointer to pointer to int)
//   - Output: int (the underlying int type)
func indirectTypeAndValue(t reflect.Type, v reflect.Value) (reflect.Type, reflect.Value, bool) {
	for t.Kind() == reflect.Pointer {
		if v.IsNil() {
			return t, v, false
		}
		t = t.Elem()
		v = v.Elem()
	}
	return t, v, true
}

// getDBIdentifier returns a unique identifier for the database instance.
// It uses the pointer address of the underlying database connection to distinguish different database instances.
func getDBIdentifier(db *gorm.DB) string {
	if db == nil {
		return "nil"
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		// Fallback to gorm.DB pointer address if we can't get the underlying database connection
		return fmt.Sprintf("%p", db)
	}
	return fmt.Sprintf("%p", sqlDB)
}
