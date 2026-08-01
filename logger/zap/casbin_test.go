package zap

import (
	"testing"

	casbinl "github.com/casbin/casbin/v3/log"
	"go.uber.org/zap"
)

// TestEnforceRequestFieldsRelabelsShiftedTuple pins the mapping that undoes
// Casbin's positional labeling. Casbin fills LogEntry as (sub, obj, act, dom)
// while the framework's request definition is (tenant, sub, obj, act), so each
// value arrives under the name of the field before it. A regression here is
// silent: the log keeps the same shape and only the labels lie.
func TestEnforceRequestFieldsRelabelsShiftedTuple(t *testing.T) {
	entry := &casbinl.LogEntry{
		EventType: casbinl.EventEnforce,
		Subject:   "default",     // rvals[0], the tenant
		Object:    "u1",          // rvals[1], the subject
		Action:    "/api/things", // rvals[2], the object
		Domain:    "GET",         // rvals[3], the action
	}

	want := map[string]string{
		"tenant": "default",
		"sub":    "u1",
		"obj":    "/api/things",
		"act":    "GET",
	}
	got := fieldsByName(t, enforceRequestFields(entry))
	if len(got) != len(want) {
		t.Fatalf("expected %d fields, got %d: %v", len(want), len(got), got)
	}
	for name, value := range want {
		if got[name] != value {
			t.Errorf("field %q: expected %q, got %q", name, value, got[name])
		}
	}
}

// TestEnforceRequestFieldsSkipsNonEnforceEvents keeps request labels off events
// that carry no request: those leave the tuple empty, and labeling emptiness
// would suggest a request took place.
func TestEnforceRequestFieldsSkipsNonEnforceEvents(t *testing.T) {
	entry := &casbinl.LogEntry{EventType: casbinl.EventAddPolicy, Subject: "default"}
	if fields := enforceRequestFields(entry); len(fields) != 0 {
		t.Errorf("expected no fields for a non-enforce event, got %v", fields)
	}
}

// TestEnforceRequestFieldsOmitsEmptyValues covers a request tuple shorter than
// four values, which Casbin reports by leaving the trailing fields empty.
func TestEnforceRequestFieldsOmitsEmptyValues(t *testing.T) {
	entry := &casbinl.LogEntry{
		EventType: casbinl.EventEnforce,
		Subject:   "default",
		Object:    "u1",
	}

	got := fieldsByName(t, enforceRequestFields(entry))
	if len(got) != 2 {
		t.Fatalf("expected only the populated fields, got %v", got)
	}
	if got["tenant"] != "default" || got["sub"] != "u1" {
		t.Errorf("unexpected fields: %v", got)
	}
}

func fieldsByName(t *testing.T, fields []any) map[string]string {
	t.Helper()

	byName := make(map[string]string, len(fields))
	for _, f := range fields {
		field, ok := f.(zap.Field)
		if !ok {
			t.Fatalf("expected zap.Field, got %T", f)
		}
		byName[field.Key] = field.String
	}
	return byName
}
