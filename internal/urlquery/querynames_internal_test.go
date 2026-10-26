package urlquery

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// widenedKeyTestModel re-declares the id of model.Base, the way a model
// widens its primary key: Go and gorm both resolve id to the model's own
// field, so the name is declared once.
type widenedKeyTestModel struct {
	ID string `json:"id" gorm:"primaryKey;size:191" query:"id"`

	modelregistry.Query
	modelregistry.Base
}

// TestValidateQueryNames pins the query tags a model may carry: a name
// declared once, outside the "_" namespace of the framework's own
// parameters, and a tag holding none of the characters the operator and the
// path syntax read. A field a shallower one shadows declares nothing, and a
// path joined from the tags of a struct embedded by name holds its dots
// legitimately.
func TestValidateQueryNames(t *testing.T) {
	t.Run("AcceptsTheNamesOfAWellDeclaredModel", func(t *testing.T) {
		for _, typ := range []reflect.Type{
			reflect.TypeFor[*filterTestModel](),
			reflect.TypeFor[*namedEmbeddingTestModel](),
			reflect.TypeFor[*hiddenNamedEmbeddingTestModel](),
			reflect.TypeFor[*pointerEmbeddingTestModel](),
			reflect.TypeFor[*widenedKeyTestModel](),
		} {
			require.NoError(t, ValidateQueryNames(typ), typ.String())
		}
	})

	t.Run("RefusesANameTwoColumnsDeclare", func(t *testing.T) {
		type sample struct {
			Name  string `json:"name" query:"title"`
			Title string `json:"title" query:"title"`

			modelregistry.Base
		}
		require.EqualError(t, ValidateQueryNames(reflect.TypeFor[*sample]()),
			`model sample: query name "title" is declared by Name and Title; `+
				`give each field a query name of its own, outside the "_" namespace and without "[", "]" or "."`)
	})

	t.Run("RefusesANameAColumnAndAFieldDeclare", func(t *testing.T) {
		type sample struct {
			Title  string `json:"title" query:"title"`
			Search string `json:"search" gorm:"-" query:"title"`

			modelregistry.Base
		}
		require.ErrorContains(t, ValidateQueryNames(reflect.TypeFor[*sample]()), `query name "title" is declared by Title and Search`)
	})

	t.Run("RefusesANameAHiddenColumnDeclares", func(t *testing.T) {
		type sample struct {
			Title  string `json:"title" query:"title"`
			Secret string `json:"-" query:"title"`

			modelregistry.Base
		}
		require.ErrorContains(t, ValidateQueryNames(reflect.TypeFor[*sample]()), `query name "title" is declared by Secret and Title`)
	})

	t.Run("RefusesABusinessNameInTheFrameworkNamespace", func(t *testing.T) {
		type sample struct {
			Hint string `json:"hint" query:"_hint"`
			Mode string `json:"mode" gorm:"-" query:"_mode"`

			modelregistry.Query
			modelregistry.Base
		}
		err := ValidateQueryNames(reflect.TypeFor[*sample]())
		require.ErrorContains(t, err, `query name "_hint" of Hint takes the "_" namespace of the framework's own parameters`)
		require.ErrorContains(t, err, `query name "_mode" of Mode takes the "_" namespace of the framework's own parameters`)
	})

	t.Run("RefusesATagHoldingABracketOrADot", func(t *testing.T) {
		type part struct {
			Label string `json:"label" query:"la[bel]"`
		}
		type sample struct {
			Path  string `json:"path" query:"a.b"`
			First part   `json:"first" gorm:"embedded;embeddedPrefix:first_" query:"first"`

			modelregistry.Base
		}
		err := ValidateQueryNames(reflect.TypeFor[*sample]())
		require.ErrorContains(t, err, `query tag "a.b" of Path holds "."`)
		require.ErrorContains(t, err, `query tag "la[bel]" of First.Label holds "["`)
	})
}
