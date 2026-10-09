package modelschema_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/stretchr/testify/require"
)

// identifyingSample declares the shapes the answer separates: a column under
// a unique index of its own, one under a unique index it shares, one under an
// index that is not unique at all, and two unique columns that may be NULL,
// one of which the schema forbids NULL on.
type identifyingSample struct {
	Code  string  `json:"code"`
	Shard string  `json:"shard"`
	Slot  int     `json:"slot"`
	Name  string  `json:"name"`
	Alias *string `json:"alias"`
	Token *string `json:"token" gorm:"not null"`

	modelregistry.Base
}

func (identifyingSample) TableName() string { return "identifying_samples" }

func (identifyingSample) Indexes() []modelschema.Index {
	return []modelschema.Index{
		{Fields: []string{"Code"}, Unique: true},
		{Fields: []string{"Shard", "Slot"}, Unique: true},
		{Fields: []string{"Name"}},
		{Fields: []string{"Alias"}, Unique: true},
		{Fields: []string{"Token"}, Unique: true},
	}
}

// TestIdentifyingColumnsSeparatesWhatIdentifiesARow pins what a cursor may
// page by: a column no two rows share. A shared one splits the rows on the
// boundary between pages, and the ones a page had no room for are never read.
func TestIdentifyingColumnsSeparatesWhatIdentifiesARow(t *testing.T) {
	identifying, err := modelschema.IdentifyingColumns(&identifyingSample{})
	require.NoError(t, err)

	require.Contains(t, identifying, "id", "the primary key identifies a row")
	require.Contains(t, identifying, "code", "a column with a unique index of its own identifies a row")
	require.NotContains(t, identifying, "shard", "a column sharing a unique index identifies nothing on its own")
	require.NotContains(t, identifying, "slot")
	require.NotContains(t, identifying, "name", "an index that is not unique says nothing about uniqueness")
	require.NotContains(t, identifying, "created_at", "rows are created in the same instant all the time")
	require.NotContains(t, identifying, "alias", "a unique column that may be NULL is shared by every row without a value")
	require.Contains(t, identifying, "token", "a unique column the schema forbids NULL on identifies a row")
}

// TestIdentifyingColumnsReadsATypeAlone pins the call the database chain
// makes: it holds no instance, only the type, and a nil pointer must answer
// the same declaration rather than panic inside a value method.
func TestIdentifyingColumnsReadsATypeAlone(t *testing.T) {
	identifying, err := modelschema.IdentifyingColumns((*identifyingSample)(nil))
	require.NoError(t, err)
	require.Contains(t, identifying, "code")
	require.Contains(t, identifying, "id")
}
