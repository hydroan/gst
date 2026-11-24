package logger

import (
	"context"
	"log/slog"
	"runtime"
	"slices"
	"time"

	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// slogHandler is the slog.Handler Init installs as slog.Default, which the
// standard log package writes through as well: a line a project writes
// with slog or log lands in the app stream, under the request metadata and
// the execution identity its context carries, the fields WithContext binds
// for the framework's own entries, so it carries the trace id and the
// route of its request. Groups are folded: a group attribute, and the
// attributes written under WithGroup, reach the core as one reflected
// value, which the encoder collapses into a single string field (see
// newStringifyReflectedEncoder), so a log store's field mapping stays
// bounded whatever a project groups its attributes into.
type slogHandler struct {
	core zapcore.Core
	// name is the name the stream's logger carries, the one its entries
	// name the stream by in stdout mode (see named).
	name string
	// fields are the attributes bound by WithAttrs outside any group, as the
	// fields they are written with.
	fields []zap.Field
	// groups are the groups WithGroup opened, outermost first, and grouped
	// the attributes bound by WithAttrs inside them, each nested under the
	// groups open when it was bound.
	groups  []string
	grouped []slog.Attr
}

func (h *slogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.core.Enabled(zapLevel(level))
}

func (h *slogHandler) Handle(ctx context.Context, record slog.Record) error {
	entry := zapcore.Entry{LoggerName: h.name, Level: zapLevel(record.Level), Time: record.Time, Message: record.Message}
	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	if record.PC != 0 {
		frame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next()
		entry.Caller = zapcore.NewEntryCaller(record.PC, frame.File, frame.Line, true)
	}
	checked := h.core.Check(entry, nil)
	if checked == nil {
		return nil
	}
	fields := make([]zap.Field, 0, contextFieldCap+len(h.fields)+record.NumAttrs()+1)
	if ctx != nil {
		fields = appendContextFields(fields, requestctx.FromContext(ctx), execctx.FromContext(ctx))
	}
	fields = append(fields, h.fields...)
	if len(h.groups) == 0 {
		record.Attrs(func(attr slog.Attr) bool {
			fields = appendAttrField(fields, attr)
			return true
		})
	} else {
		attrs := make([]slog.Attr, 0, record.NumAttrs())
		record.Attrs(func(attr slog.Attr) bool {
			attrs = append(attrs, attr)
			return true
		})
		folded := make(map[string]any)
		foldAttrs(folded, h.grouped)
		foldAttrs(folded, []slog.Attr{nestInGroups(h.groups, attrs)})
		for key, value := range folded {
			fields = append(fields, zap.Reflect(key, value))
		}
	}
	checked.Write(fields...)
	return nil
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	bound := *h
	if len(h.groups) == 0 {
		bound.fields = slices.Clone(h.fields)
		for _, attr := range attrs {
			bound.fields = appendAttrField(bound.fields, attr)
		}
		return &bound
	}
	bound.grouped = append(slices.Clone(h.grouped), nestInGroups(h.groups, attrs))
	return &bound
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	bound := *h
	bound.groups = append(slices.Clone(h.groups), name)
	return &bound
}

// appendAttrField appends attr to fields as the field it is written with:
// a group as one reflected value of its attributes folded (see foldAttrs),
// the attributes of a group without a key inlined, as slog reads them, a
// scalar through the constructor of its kind, an error as its message and
// any other value as one reflected value, which the encoder collapses into
// a string the way it collapses a group. An empty attribute and an empty
// group are left out, as slog's own handlers leave them.
func appendAttrField(fields []zap.Field, attr slog.Attr) []zap.Field {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		group := value.Group()
		if len(group) == 0 {
			return fields
		}
		if attr.Key == "" {
			for _, inner := range group {
				fields = appendAttrField(fields, inner)
			}
			return fields
		}
		folded := make(map[string]any, len(group))
		foldAttrs(folded, group)
		return append(fields, zap.Reflect(attr.Key, folded))
	}
	if attr.Equal(slog.Attr{}) {
		return fields
	}
	switch value.Kind() {
	case slog.KindString:
		return append(fields, zap.String(attr.Key, value.String()))
	case slog.KindInt64:
		return append(fields, zap.Int64(attr.Key, value.Int64()))
	case slog.KindUint64:
		return append(fields, zap.Uint64(attr.Key, value.Uint64()))
	case slog.KindFloat64:
		return append(fields, zap.Float64(attr.Key, value.Float64()))
	case slog.KindBool:
		return append(fields, zap.Bool(attr.Key, value.Bool()))
	case slog.KindDuration:
		return append(fields, zap.Duration(attr.Key, value.Duration()))
	case slog.KindTime:
		return append(fields, zap.Time(attr.Key, value.Time()))
	}
	v := value.Any()
	if err, ok := v.(error); ok {
		return append(fields, zap.String(attr.Key, err.Error()))
	}
	return append(fields, zap.Reflect(attr.Key, v))
}

// foldAttrs folds attrs into the map into: a group becomes the map under
// its key, merged with what the key already holds, and a group without a
// key folds into the same map.
func foldAttrs(into map[string]any, attrs []slog.Attr) {
	for _, attr := range attrs {
		value := attr.Value.Resolve()
		if value.Kind() == slog.KindGroup {
			group := value.Group()
			if len(group) == 0 {
				continue
			}
			if attr.Key == "" {
				foldAttrs(into, group)
				continue
			}
			sub, ok := into[attr.Key].(map[string]any)
			if !ok {
				sub = make(map[string]any, len(group))
				into[attr.Key] = sub
			}
			foldAttrs(sub, group)
			continue
		}
		if attr.Equal(slog.Attr{}) {
			continue
		}
		into[attr.Key] = value.Any()
	}
}

// nestInGroups returns attrs as one group attribute nested under groups,
// outermost first: for the groups batch and item it returns the group
// batch holding the group item holding attrs.
func nestInGroups(groups []string, attrs []slog.Attr) slog.Attr {
	attr := slog.Attr{Key: groups[len(groups)-1], Value: slog.GroupValue(attrs...)}
	for _, group := range slices.Backward(groups[:len(groups)-1]) {
		attr = slog.Attr{Key: group, Value: slog.GroupValue(attr)}
	}
	return attr
}

// zapLevel maps a slog level onto the zap level of the entry: debug below
// info, info below warn, warn below error, and error from there up.
func zapLevel(level slog.Level) zapcore.Level {
	switch {
	case level < slog.LevelInfo:
		return zapcore.DebugLevel
	case level < slog.LevelWarn:
		return zapcore.InfoLevel
	case level < slog.LevelError:
		return zapcore.WarnLevel
	default:
		return zapcore.ErrorLevel
	}
}
