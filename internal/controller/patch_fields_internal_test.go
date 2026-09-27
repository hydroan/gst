package controller

import (
	"context"
	"reflect"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// patchFieldsRecord is the model the patch field tests apply patches to.
type patchFieldsRecord struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Enabled bool   `json:"enabled"`
}

// patchFieldsKeyedRecord declares its primary key itself, the way a model
// shadowing model.Base.ID does.
type patchFieldsKeyedRecord struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TestPatchFieldSetsLeaveThePrimaryKeyOut pins that no patch applies the
// primary key, which names the record patched and moves it nowhere: a body
// key naming it names no field, and a mask path naming it is refused.
func TestPatchFieldSetsLeaveThePrimaryKeyOut(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsKeyedRecord]()

	fields, err := patchFieldSetFromJSONBody(typ, []byte(`{"id":"other","name":"renamed"}`))
	require.NoError(t, err)
	require.Equal(t, patchFieldSet{"Name": {}}, fields)

	_, err = maskFieldSet(typ, []string{"id"})
	require.EqualError(t, err, `update_mask names "id", which is no field a patch applies`)

	oldRecord := &patchFieldsKeyedRecord{ID: "kept", Name: "before"}
	newRecord := &patchFieldsKeyedRecord{ID: "other", Name: "after"}
	applyPatch(nopControllerLogger{}, typ, reflect.ValueOf(oldRecord).Elem(), reflect.ValueOf(newRecord).Elem())
	require.Equal(t, "kept", oldRecord.ID)
	require.Equal(t, "after", oldRecord.Name)
}

func TestApplyPatchAppliesExplicitZeroValues(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()
	oldRecord := &patchFieldsRecord{
		Name:    "enabled feature",
		Count:   10,
		Enabled: true,
	}
	newRecord := &patchFieldsRecord{
		Name:    "",
		Count:   0,
		Enabled: false,
	}

	applyPatch(nopControllerLogger{}, typ, reflect.ValueOf(oldRecord).Elem(), reflect.ValueOf(newRecord).Elem())

	require.Empty(t, oldRecord.Name)
	require.Zero(t, oldRecord.Count)
	require.False(t, oldRecord.Enabled)
}

func TestApplyPatchSkipsMissingFields(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()
	oldRecord := &patchFieldsRecord{
		Name:    "enabled feature",
		Count:   10,
		Enabled: true,
	}
	newRecord := &patchFieldsRecord{
		Name:    "",
		Count:   0,
		Enabled: false,
	}

	applyPatch(nopControllerLogger{}, typ, reflect.ValueOf(oldRecord).Elem(), reflect.ValueOf(newRecord).Elem(), patchFieldSet{
		"Enabled": {},
	})

	require.Equal(t, "enabled feature", oldRecord.Name)
	require.Equal(t, 10, oldRecord.Count)
	require.False(t, oldRecord.Enabled)
}

func BenchmarkApplyPatch(b *testing.B) {
	typ := reflect.TypeFor[patchFieldsRecord]()
	newRecord := &patchFieldsRecord{
		Name:    "",
		Count:   0,
		Enabled: false,
	}
	newVal := reflect.ValueOf(newRecord).Elem()
	log := nopControllerLogger{}
	fields := patchFieldSet{
		"Name":    {},
		"Count":   {},
		"Enabled": {},
	}

	b.ReportAllocs()
	for range b.N {
		oldRecord := &patchFieldsRecord{
			Name:    "enabled feature",
			Count:   10,
			Enabled: true,
		}
		applyPatch(log, typ, reflect.ValueOf(oldRecord).Elem(), newVal, fields)
	}
}

func TestPatchFieldSetFromJSONBodyUsesJSONTags(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()

	fields, err := patchFieldSetFromJSONBody(typ, []byte(`{"enabled":false,"count":0}`))

	require.NoError(t, err)
	require.Contains(t, fields, "Enabled")
	require.Contains(t, fields, "Count")
	require.NotContains(t, fields, "Name")
}

// TestPatchFieldSetFromJSONBodyWrapsDecodeError pins that patch-path body
// decoding failures carry the client-safe message instead of the raw decoder
// text, matching the bindJSONRequest contract.
func TestPatchFieldSetFromJSONBodyWrapsDecodeError(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()

	_, err := patchFieldSetFromJSONBody(typ, []byte(`{"enabled":`))

	var serviceErr *serviceregistry.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "request body is not valid JSON", serviceErr.Msg())
}

func TestPatchManyFieldSetsFromJSONBodyKeepItemFieldsSeparate(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()

	fieldSets, err := patchManyFieldSetsFromJSONBody(typ, []byte(`{"items":[{"enabled":false},{"name":""}]}`))

	require.NoError(t, err)
	require.Len(t, fieldSets, 2)
	require.Contains(t, fieldSets[0], "Enabled")
	require.NotContains(t, fieldSets[0], "Name")
	require.Contains(t, fieldSets[1], "Name")
	require.NotContains(t, fieldSets[1], "Enabled")
}

// TestPatchManyFieldSetsFromJSONBodyWrapsDecodeError is the batch-path
// counterpart: a type mismatch on the items envelope must name the field
// through the client-safe message.
func TestPatchManyFieldSetsFromJSONBodyWrapsDecodeError(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsRecord]()

	_, err := patchManyFieldSetsFromJSONBody(typ, []byte(`{"items":3}`))

	var serviceErr *serviceregistry.Error
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "invalid value for field 'items'", serviceErr.Msg())
}

// nopControllerLogger is the logger the patch tests hand applyPatch, which
// logs every field it copies.
type nopControllerLogger struct{}

func (nopControllerLogger) With(fields ...string) types.Logger { return nopControllerLogger{} }
func (nopControllerLogger) WithContext(context.Context, consts.Phase) types.Logger {
	return nopControllerLogger{}
}
func (nopControllerLogger) Debug(args ...any)                       {}
func (nopControllerLogger) Info(args ...any)                        {}
func (nopControllerLogger) Warn(args ...any)                        {}
func (nopControllerLogger) Error(args ...any)                       {}
func (nopControllerLogger) Fatal(args ...any)                       {}
func (nopControllerLogger) Debugf(format string, args ...any)       {}
func (nopControllerLogger) Infof(format string, args ...any)        {}
func (nopControllerLogger) Warnf(format string, args ...any)        {}
func (nopControllerLogger) Errorf(format string, args ...any)       {}
func (nopControllerLogger) Fatalf(format string, args ...any)       {}
func (nopControllerLogger) Debugw(msg string, keysAndValues ...any) {}
func (nopControllerLogger) Infow(msg string, keysAndValues ...any)  {}
func (nopControllerLogger) Warnw(msg string, keysAndValues ...any)  {}
func (nopControllerLogger) Errorw(msg string, keysAndValues ...any) {}
func (nopControllerLogger) Fatalw(msg string, keysAndValues ...any) {}
func (nopControllerLogger) Debugz(msg string, fields ...zap.Field)  {}
func (nopControllerLogger) Infoz(msg string, fields ...zap.Field)   {}
func (nopControllerLogger) Warnz(msg string, fields ...zap.Field)   {}
func (nopControllerLogger) Errorz(msg string, fields ...zap.Field)  {}
func (nopControllerLogger) Fatalz(msg string, fields ...zap.Field)  {}
