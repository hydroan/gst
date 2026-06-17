package zap

import (
	casbinl "github.com/casbin/casbin/v3/log"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
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
			zap.Duration("duration", entry.Duration),
		}
		if entry.Subject != "" {
			fields = append(fields, zap.String("subject", entry.Subject))
		}
		if entry.Object != "" {
			fields = append(fields, zap.String("object", entry.Object))
		}
		if entry.Action != "" {
			fields = append(fields, zap.String("action", entry.Action))
		}
		if entry.Domain != "" {
			fields = append(fields, zap.String("domain", entry.Domain))
		}
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

func (c *CasbinLogger) SetLogCallback(callback func(entry *casbinl.LogEntry) error) error {
	c.callback = callback
	return nil
}
