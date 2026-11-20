package controller_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

// TestPatchCallAppliesTheMaskedFields pins the patch call: only the fields
// the mask names are copied onto the stored record, the others the message
// carries staying as stored; a mask naming nothing, or naming what no patch
// applies, is refused, a path naming a field the framework manages passed
// over on the way, as the HTTP handler passes over the key; a message
// carrying no record is refused; a versioned record patched without its
// version is refused; and an id no record carries answers NotFound.
func TestPatchCallAppliesTheMaskedFields(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-patch")

	patched, err := invoke(t, conn, "Patch", map[string]any{
		"id": record.GetID(), "record": map[string]any{"name": "call-patched", "note": "unmasked"}, "mask": []string{"name"},
	})
	require.NoError(t, err)
	require.Equal(t, "call-patched", patched["name"])
	stored := loadSample(t, record.GetID())
	require.Equal(t, "call-patched", stored.Name)
	require.Empty(t, stored.Note, "a field the mask does not name stays as stored")

	for _, tt := range []struct {
		name    string
		mask    []string
		message string
	}{
		{name: "a mask naming nothing", mask: []string{}, message: "update_mask must name at least one field a patch applies"},
		{name: "a mask naming only a field the framework manages", mask: []string{"id"}, message: "update_mask must name at least one field a patch applies"},
		{name: "a mask naming a field the model lacks", mask: []string{"missing"}, message: `update_mask names "missing", which is no field a patch applies`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := invoke(t, conn, "Patch", map[string]any{
				"id": record.GetID(), "record": map[string]any{"name": "call-unmasked"}, "mask": tt.mask,
			})
			requireStatus(t, err, codes.InvalidArgument, tt.message)
			requireSampleName(t, record.GetID(), "call-patched")
		})
	}

	t.Run("a versioned record without its version", func(t *testing.T) {
		_, err := invoke(t, conn, "VersionedPatch", map[string]any{
			"id": "missing", "record": map[string]any{"name": "renamed"}, "mask": []string{"name"},
		})
		requireStatus(t, err, codes.InvalidArgument, "")
	})

	t.Run("an id no record carries", func(t *testing.T) {
		_, err := invoke(t, conn, "Patch", map[string]any{
			"id": "missing", "record": map[string]any{"name": "renamed"}, "mask": []string{"name"},
		})
		requireStatus(t, err, codes.NotFound, "")
	})

	t.Run("a message carrying no record", func(t *testing.T) {
		_, err := invoke(t, conn, "Patch", map[string]any{"id": record.GetID(), "mask": []string{"name"}})
		requireStatus(t, err, codes.InvalidArgument, "record is required")
		requireSampleName(t, record.GetID(), "call-patched")
	})

	t.Run("a mask naming a field the framework manages beside one it applies", func(t *testing.T) {
		patched, err := invoke(t, conn, "Patch", map[string]any{
			"id": record.GetID(), "record": map[string]any{"name": "call-patched-beside", "created_at": "2001-02-03T04:05:06Z"}, "mask": []string{"created_at", "name"},
		})
		require.NoError(t, err)
		require.Equal(t, "call-patched-beside", patched["name"])
		stored := loadSample(t, record.GetID())
		require.Equal(t, "call-patched-beside", stored.Name)
		require.True(t, stored.GetCreatedAt().Equal(record.GetCreatedAt()), "the framework's field stays as stored: %s", stored.GetCreatedAt())
	})
}

// TestPatchCallAppliesAFieldAsAWhole pins what a mask path patches: a
// time value and a struct value are replaced as a whole, the parts of the
// struct the message left out included, a path promoted from an embedded
// struct sets its field while the neighbor stays, a struct the mask names
// is checked against its own binding tags as a whole, and a path into a
// struct is refused with the field to name instead.
func TestPatchCallAppliesAFieldAsAWhole(t *testing.T) {
	conn := sampleServer(t)
	record := createShaped(t, "call-shaped")
	dueAt := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)

	patched, err := invoke(t, conn, "ShapedPatch", map[string]any{
		"id":     record.GetID(),
		"record": map[string]any{"due_at": "2027-01-02T03:04:05Z", "address": map[string]any{"city": "new"}, "reviewer": "second", "name": "unmasked"},
		"mask":   []string{"due_at", "address", "reviewer"},
	})
	require.NoError(t, err)
	require.Equal(t, "second", patched["reviewer"])
	stored := loadShaped(t, record.GetID())
	require.Equal(t, "call-shaped", stored.Name, "a field the mask does not name stays as stored")
	require.True(t, stored.DueAt.Equal(dueAt), stored.DueAt)
	require.Equal(t, sampleAddress{City: "new"}, stored.Address, "a struct value is replaced as a whole")
	require.Equal(t, SampleAudit{Reviewer: "second", Reviewed: true}, stored.SampleAudit)

	_, err = invoke(t, conn, "ShapedPatch", map[string]any{
		"id": record.GetID(), "record": map[string]any{"address": map[string]any{"zip": "2"}}, "mask": []string{"address"},
	})
	requireStatus(t, err, codes.InvalidArgument, "")
	require.Equal(t, sampleAddress{City: "new"}, loadShaped(t, record.GetID()).Address, "the address is checked as a whole, its city required")

	_, err = invoke(t, conn, "ShapedPatch", map[string]any{
		"id": record.GetID(), "record": map[string]any{"address": map[string]any{"city": "part"}}, "mask": []string{"address.city"},
	})
	requireStatus(t, err, codes.InvalidArgument, `update_mask names "address.city", a part of a field; a patch applies "address" as a whole`)
}

// TestPatchCallValidatesTheMaskedFieldsAlone pins that a patch call checks
// the binding tags of the fields its mask names and no other, in the single
// and the batch call alike: a mask leaving the required name out passes,
// one naming the name with an empty value is refused.
func TestPatchCallValidatesTheMaskedFieldsAlone(t *testing.T) {
	conn := sampleServer(t)
	created, err := invoke(t, conn, "ValidatedCreate", map[string]any{"record": map[string]any{"name": "validated-patch"}})
	require.NoError(t, err)
	id, _ := created["id"].(string)
	require.NotEmpty(t, id)

	_, err = invoke(t, conn, "ValidatedPatch", map[string]any{"id": id, "record": map[string]any{"note": "only the note"}, "mask": []string{"note"}})
	require.NoError(t, err, "a mask leaving the required name out has nothing to meet")
	_, err = invoke(t, conn, "ValidatedPatch", map[string]any{"id": id, "record": map[string]any{"name": ""}, "mask": []string{"name"}})
	requireStatus(t, err, codes.InvalidArgument, "name is a required field")

	_, err = invoke(t, conn, "ValidatedPatchMany", map[string]any{"items": []map[string]any{{"id": id, "record": map[string]any{"note": "batch note"}, "mask": []string{"note"}}}})
	require.NoError(t, err)
	_, err = invoke(t, conn, "ValidatedPatchMany", map[string]any{"items": []map[string]any{{"id": id, "record": map[string]any{"name": ""}, "mask": []string{"name"}}}})
	requireStatus(t, err, codes.InvalidArgument, "items[0].name is a required field")
}
