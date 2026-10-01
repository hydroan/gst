package controller

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/modelregistry"
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

// patchFieldsShapedRecord carries the field shapes a patch applies as a
// whole and the structs it looks through: a time and a struct value, the
// fields of an embedded struct of the model's own, promoted to keys of the
// model's own, and the framework base, whose fields no patch applies.
type patchFieldsShapedRecord struct {
	Name    string             `json:"name"`
	DueAt   time.Time          `json:"due_at"`
	Address patchFieldsAddress `json:"address"`
	patchFieldsAudit

	modelregistry.Base
}

type patchFieldsAddress struct {
	City string `json:"city"`
	Zip  string `json:"zip"`
}

type patchFieldsAudit struct {
	Reviewer string `json:"reviewer"`
	Reviewed bool   `json:"reviewed"`
}

// TestPatchFieldSetsNameEveryFieldOfTheModelAsAWhole pins which fields a
// body key or a mask path names: every field of the model's own, a time or
// a struct value as a whole, a field promoted from an embedded struct under
// its own key and Go path; none of the framework base's, a body key or a
// mask path naming one passed over, a mask naming no other field refused;
// and no part of a field, a mask path into a struct being refused with the
// field to name instead.
func TestPatchFieldSetsNameEveryFieldOfTheModelAsAWhole(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsShapedRecord]()
	named := patchFieldSet{"DueAt": {}, "Address": {}, "patchFieldsAudit.Reviewer": {}}

	fields, err := patchFieldSetFromJSONBody(typ, []byte(`{"due_at":"2026-05-06T07:08:09Z","address":{"city":"new"},"reviewer":"second","created_at":"2026-01-01T00:00:00Z","patchFieldsAudit":{}}`))
	require.NoError(t, err)
	require.Equal(t, named, fields)

	fields, err = maskFieldSet(typ, []string{"due_at", "address", "reviewer", "created_at"})
	require.NoError(t, err)
	require.Equal(t, named, fields)

	_, err = maskFieldSet(typ, []string{"created_at"})
	require.EqualError(t, err, "update_mask must name at least one field a patch applies")
	_, err = maskFieldSet(typ, []string{"address.city"})
	require.EqualError(t, err, `update_mask names "address.city", a part of a field; a patch applies "address" as a whole`)
}

// patchFieldsShadowedRecord encodes two fields to the key name: its own,
// and the one the embedded struct promotes, which encoding/json leaves out
// for the shallower, so a patch names the model's own.
type patchFieldsShadowedRecord struct {
	Name string `json:"name"`
	*PatchFieldsLabel
}

type PatchFieldsLabel struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// patchFieldsTitledRecord encodes its own Name under title and the Name the
// embedded struct promotes under Name, two keys: encoding/json tells fields
// apart by their keys, where Go tells them apart by their names and hides
// the promoted one.
type patchFieldsTitledRecord struct {
	Name string `json:"title"`
	PatchFieldsPlainName
}

type PatchFieldsPlainName struct {
	Name string
}

// PatchFieldsFirstName and PatchFieldsSecondName each hold a field named
// Name in Go, under the keys first and second; embedded side by side, Go
// sees one ambiguous Name and encoding/json two keys.
type PatchFieldsFirstName struct {
	Name string `json:"first"`
}

type PatchFieldsSecondName struct {
	Name string `json:"second"`
}

type patchFieldsTwoNamesRecord struct {
	PatchFieldsFirstName
	PatchFieldsSecondName
}

// PatchFieldsLeftLabel and PatchFieldsRightLabel each encode a field to the
// key Label, one with its tag spelt like its Go name; embedded side by side
// they tie at one depth, and encoding/json takes neither. The tie is built
// at run time, a declared struct repeating a tag being what go vet refuses.
type PatchFieldsLeftLabel struct {
	Label string `json:"Label"`
}

type PatchFieldsRightLabel struct {
	Tag string `json:"Label"`
}

// TestPatchFieldSetsFollowTheJSONKeys pins that the fields a patch names
// are the ones encoding/json encodes, under its rules: of two fields
// encoding to one key the shallower is the field, and neither is when two
// tagged fields tie at one depth, a tag spelt like the Go name being a tag
// all the same; a field promoted through an embedded pointer is named
// through it, the pointer allocated on the record when it is nil; and two
// fields Go would hide behind one name are two fields under the two keys
// they encode to, the keys encoding/json writes being the reference.
func TestPatchFieldSetsFollowTheJSONKeys(t *testing.T) {
	// The embedded pointer is allocated for the reference: encoding/json
	// writes nothing of a nil one, where the table names the type's fields.
	for _, record := range []any{patchFieldsShadowedRecord{PatchFieldsLabel: &PatchFieldsLabel{}}, patchFieldsTitledRecord{}, patchFieldsTwoNamesRecord{}} {
		require.Equal(t, jsonKeysOf(t, record), slices.Sorted(maps.Keys(patchFieldsOf(reflect.TypeOf(record)).byKey)), "%T", record)
	}
	titled, err := patchFieldSetFromJSONBody(reflect.TypeFor[patchFieldsTitledRecord](), []byte(`{"title":"own","Name":"promoted"}`))
	require.NoError(t, err)
	require.Equal(t, patchFieldSet{"Name": {}, "PatchFieldsPlainName.Name": {}}, titled)
	twoNames, err := patchFieldSetFromJSONBody(reflect.TypeFor[patchFieldsTwoNamesRecord](), []byte(`{"first":"a","second":"b"}`))
	require.NoError(t, err)
	require.Equal(t, patchFieldSet{"PatchFieldsFirstName.Name": {}, "PatchFieldsSecondName.Name": {}}, twoNames)

	typ := reflect.TypeFor[patchFieldsShadowedRecord]()

	fields, err := patchFieldSetFromJSONBody(typ, []byte(`{"name":"own","label":"promoted"}`))
	require.NoError(t, err)
	require.Equal(t, patchFieldSet{"Name": {}, "PatchFieldsLabel.Label": {}}, fields)

	oldRecord := &patchFieldsShadowedRecord{Name: "before"}
	newRecord := &patchFieldsShadowedRecord{Name: "own", PatchFieldsLabel: &PatchFieldsLabel{Name: "shadowed", Label: "promoted"}}
	applyPatch(nopControllerLogger{}, typ, reflect.ValueOf(oldRecord).Elem(), reflect.ValueOf(newRecord).Elem(), fields)
	require.Equal(t, "own", oldRecord.Name)
	require.Equal(t, &PatchFieldsLabel{Label: "promoted"}, oldRecord.PatchFieldsLabel, "allocated on the way, the shadowed name left alone")

	tied := reflect.StructOf([]reflect.StructField{
		{Name: "PatchFieldsLeftLabel", Type: reflect.TypeFor[PatchFieldsLeftLabel](), Anonymous: true},
		{Name: "PatchFieldsRightLabel", Type: reflect.TypeFor[PatchFieldsRightLabel](), Anonymous: true},
	})
	fields, err = patchFieldSetFromJSONBody(tied, []byte(`{"Label":"tied"}`))
	require.NoError(t, err)
	require.Empty(t, fields, "two tagged fields tied at one depth encode to no key")
}

// TestApplyPatchAppliesAStructValuedFieldAsAWhole pins how the named
// fields are copied: a time value and a struct value are replaced as a
// whole, the parts of the struct the request left out included, a promoted
// field is set through its embedded struct while its neighbor stays, and
// the framework base stays as stored.
func TestApplyPatchAppliesAStructValuedFieldAsAWhole(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsShapedRecord]()
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	dueAt := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	oldRecord := &patchFieldsShapedRecord{
		Name:      "kept",
		DueAt:     createdAt,
		Address:   patchFieldsAddress{City: "old", Zip: "1"},
		Reviewer:  "first",
		Reviewed:  true,
		CreatedAt: createdAt,
	}
	newRecord := &patchFieldsShapedRecord{
		DueAt:    dueAt,
		Address:  patchFieldsAddress{City: "new"},
		Reviewer: "second",
	}

	applyPatch(nopControllerLogger{}, typ, reflect.ValueOf(oldRecord).Elem(), reflect.ValueOf(newRecord).Elem(), patchFieldSet{
		"DueAt": {}, "Address": {}, "patchFieldsAudit.Reviewer": {},
	})

	require.Equal(t, "kept", oldRecord.Name)
	require.True(t, oldRecord.DueAt.Equal(dueAt))
	require.Equal(t, patchFieldsAddress{City: "new"}, oldRecord.Address, "a struct value is replaced as a whole")
	require.Equal(t, patchFieldsAudit{Reviewer: "second", Reviewed: true}, oldRecord.patchFieldsAudit)
	require.True(t, oldRecord.CreatedAt.Equal(createdAt), "the framework base stays as stored")
}

// TestPatchFieldSetsLeaveThePrimaryKeyOut pins that no patch applies the
// primary key, which names the record patched and moves it nowhere: a body
// key or a mask path naming it names no field, and a mask naming it alone
// is refused.
func TestPatchFieldSetsLeaveThePrimaryKeyOut(t *testing.T) {
	typ := reflect.TypeFor[patchFieldsKeyedRecord]()

	fields, err := patchFieldSetFromJSONBody(typ, []byte(`{"id":"other","name":"renamed"}`))
	require.NoError(t, err)
	require.Equal(t, patchFieldSet{"Name": {}}, fields)

	fields, err = maskFieldSet(typ, []string{"id", "name"})
	require.NoError(t, err)
	require.Equal(t, patchFieldSet{"Name": {}}, fields)

	_, err = maskFieldSet(typ, []string{"id"})
	require.EqualError(t, err, "update_mask must name at least one field a patch applies")

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

// jsonKeysOf returns the keys encoding/json encodes record under, sorted.
func jsonKeysOf(t *testing.T, record any) []string {
	t.Helper()
	encoded, err := json.Marshal(record)
	require.NoError(t, err)
	var keyed map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &keyed))
	return slices.Sorted(maps.Keys(keyed))
}
