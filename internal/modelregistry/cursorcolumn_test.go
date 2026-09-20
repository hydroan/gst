package modelregistry_test

import (
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// identifyingSample declares the three shapes the answer separates: a column
// under a unique index of its own, one under a unique index it shares, and
// one under an index that is not unique at all.
type identifyingSample struct {
	Code  string `json:"code"`
	Shard string `json:"shard"`
	Slot  int    `json:"slot"`
	Name  string `json:"name"`

	modelregistry.Base
}

func (identifyingSample) TableName() string { return "identifying_samples" }

func (identifyingSample) Indexes() []modelregistry.Index {
	return []modelregistry.Index{
		{Fields: []string{"Code"}, Unique: true},
		{Fields: []string{"Shard", "Slot"}, Unique: true},
		{Fields: []string{"Name"}},
	}
}

// TestIdentifyingColumnsSeparatesWhatIdentifiesARow pins what a cursor may
// page by: a column no two rows share. A shared one splits the rows on the
// boundary between pages, and the ones a page had no room for are never read.
func TestIdentifyingColumnsSeparatesWhatIdentifiesARow(t *testing.T) {
	identifying, err := modelregistry.IdentifyingColumns(&identifyingSample{})
	require.NoError(t, err)

	require.Contains(t, identifying, "id", "the primary key identifies a row")
	require.Contains(t, identifying, "code", "a column with a unique index of its own identifies a row")
	require.NotContains(t, identifying, "shard", "a column sharing a unique index identifies nothing on its own")
	require.NotContains(t, identifying, "slot")
	require.NotContains(t, identifying, "name", "an index that is not unique says nothing about uniqueness")
	require.NotContains(t, identifying, "created_at", "rows are created in the same instant all the time")
}

// TestIdentifyingColumnsReadsATypeAlone pins the call the database chain
// makes: it holds no instance, only the type, and a nil pointer must answer
// the same declaration rather than panic inside a value method.
func TestIdentifyingColumnsReadsATypeAlone(t *testing.T) {
	identifying, err := modelregistry.IdentifyingColumns((*identifyingSample)(nil))
	require.NoError(t, err)
	require.Contains(t, identifying, "code")
	require.Contains(t, identifying, "id")
}
