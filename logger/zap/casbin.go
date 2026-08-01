package zap

import (
	casbinl "github.com/casbin/casbin/v3/log"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

type CasbinLogger struct {
	l          types.Logger
	eventTypes map[casbinl.EventType]bool
	callback   func(entry *casbinl.LogEntry) error
}

var _ casbinl.Logger = (*CasbinLogger)(nil)

func (c *CasbinLogger) SetEventTypes(eventTypes []casbinl.EventType) error {
	c.eventTypes = make(map[casbinl.EventType]bool, len(eventTypes))
	for _, eventType := range eventTypes {
		c.eventTypes[eventType] = true
	}
	return nil
}

func (c *CasbinLogger) OnBeforeEvent(entry *casbinl.LogEntry) error {
	if entry == nil {
		return errors.New("casbin log entry is nil")
	}
	entry.IsActive = len(c.eventTypes) == 0 || c.eventTypes[entry.EventType]
	return nil
}

func (c *CasbinLogger) OnAfterEvent(entry *casbinl.LogEntry) error {
	if entry == nil {
		return errors.New("casbin log entry is nil")
	}
	if entry.IsActive {
		fields := []any{
			zap.String("event", string(entry.EventType)),
			util.LogDuration(entry.Duration),
		}
		fields = append(fields, enforceRequestFields(entry)...)
		if entry.EventType == casbinl.EventEnforce {
			fields = append(fields, zap.Bool("allowed", entry.Allowed))
		}
		if len(entry.Rules) > 0 {
			fields = append(fields, zap.Any("rules", entry.Rules))
		}
		if entry.RuleCount > 0 {
			fields = append(fields, zap.Int("rule_count", entry.RuleCount))
		}
		if entry.Error != nil {
			fields = append(fields, zap.Error(entry.Error))
		}
		c.l.Infow("", fields...)
	}
	if c.callback != nil {
		return c.callback(entry)
	}
	return nil
}

// enforceRequestFields relabels the request tuple Casbin reports on an enforce
// event.
//
// Casbin fills LogEntry by position, assuming the request is (sub, obj, act,
// dom), and never consults the model's request definition. This framework
// defines it as (tenant, sub, obj, act), so every value Casbin hands over sits
// under the name of the field before it: Subject holds the tenant, Object the
// subject, Action the object, Domain the action. Emitting those names verbatim
// produces a log whose every label is wrong, which is worse than no log at all
// when tracing why a request was allowed.
//
// The shift is a fixed consequence of the two definitions, so undoing it is a
// fixed mapping too. Change it if either side moves: the request definition
// lives with the enforcer setup, Casbin's side in its createEnforceLogEntry.
//
// Only enforce events carry a request; the other event types leave these fields
// empty, and an empty value is dropped rather than logged under a wrong name.
func enforceRequestFields(entry *casbinl.LogEntry) []any {
	if entry.EventType != casbinl.EventEnforce {
		return nil
	}

	fields := make([]any, 0, 4)
	for _, f := range []struct {
		name  string
		value string
	}{
		{"tenant", entry.Subject},
		{"sub", entry.Object},
		{"obj", entry.Action},
		{"act", entry.Domain},
	} {
		if f.value != "" {
			fields = append(fields, zap.String(f.name, f.value))
		}
	}
	return fields
}

func (c *CasbinLogger) SetLogCallback(callback func(entry *casbinl.LogEntry) error) error {
	c.callback = callback
	return nil
}
