package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/stretchr/testify/require"
)

type listQueryableTestModel struct {
	Name string `query:"name"`

	modelregistry.Query
	modelregistry.Base
}

func TestDecodeListQueryGatesQueryKeys(t *testing.T) {
	t.Run("QueryAcceptsRegularKeys", func(t *testing.T) {
		var m listQueryableTestModel
		require.NoError(t, decodeListQuery(&m, map[string][]string{
			"name":     {"alice"},
			"_sort_by": {"created_at desc"},
		}))
	})

	t.Run("ModelWithoutQueryRejectsRegularKeys", func(t *testing.T) {
		type plainModel struct {
			Name string `query:"name"`

			modelregistry.Base
		}
		var m plainModel
		require.EqualError(t, decodeListQuery(&m, map[string][]string{"_sort_by": {"created_at desc"}}),
			`unsupported query parameter "_sort_by"`,
			"a model that did not embed modelregistry.Query must not accept the List controls")
	})

	t.Run("ListsEveryRejectedKeyAtOnce", func(t *testing.T) {
		type plainModel struct {
			Name string `query:"name"`

			modelregistry.Base
		}
		var m plainModel
		require.EqualError(t, decodeListQuery(&m, map[string][]string{
			"_sort_by":      {"created_at desc"},
			"_expand":       {"all"},
			"_cursor_value": {"abc"},
		}),
			`unsupported query parameters "_cursor_value", "_expand", "_sort_by"`,
			"framework keys rejected by different capability gates are still reported together")
	})

	t.Run("TheExportKeysAreRejected", func(t *testing.T) {
		var m listQueryableTestModel
		require.EqualError(t, decodeListQuery(&m, map[string][]string{"_format": {"csv"}, "_limit": {"10"}}),
			`unsupported query parameters "_format", "_limit"`,
			"the export's format and cap are its own, and a List pages by _size")
	})

	t.Run("UnknownFrameworkKeysAreRejected", func(t *testing.T) {
		for _, key := range []string{"_or", "_index", "_select", "_no_total"} {
			var m listQueryableTestModel
			require.Error(t, decodeListQuery(&m, map[string][]string{key: {"true"}}),
				"framework key %q maps to no model field and must be reported", key)
		}
	})
}

// TestListQueryTakesOneWayOfPagingPerModel pins the paging parameters a list
// takes by what its model embeds, as the list flow reads them, the gate of
// decodeListQuery and then the cursor: page numbers, unless the model embeds
// Cursor, which makes its list a feed read by cursor alone, and a page size
// either way. The parameters of the other way of paging are refused, a page
// beside a cursor as either alone, and so is _sort_by on a feed, whose order
// the cursor defines.
func TestListQueryTakesOneWayOfPagingPerModel(t *testing.T) {
	type plainModel struct {
		Name string `query:"name"`

		modelregistry.Base
	}
	type feedModel struct {
		Name string `query:"name"`

		modelregistry.Cursor
		modelregistry.Base
	}
	type queryModel struct {
		Name string `query:"name"`

		modelregistry.Query
		modelregistry.Base
	}
	type queryFeedModel struct {
		Name string `query:"name"`

		modelregistry.Query
		modelregistry.Cursor
		modelregistry.Base
	}

	for _, tt := range []struct {
		name    string
		model   types.Model
		query   string
		refusal string
	}{
		{name: "a list pages by number", model: &plainModel{}, query: "_page=2&_size=5"},
		{name: "a list takes no cursor", model: &plainModel{}, query: "_cursor_next=true", refusal: `unsupported query parameter "_cursor_next"`},
		{name: "a list refuses a cursor beside a page", model: &plainModel{}, query: "_page=2&_cursor_value=x", refusal: `unsupported query parameter "_cursor_value"`},
		{name: "a feed takes its cursor and a size", model: &feedModel{}, query: "_cursor_value=x&_cursor_next=true&_size=5"},
		{name: "a feed takes no page", model: &feedModel{}, query: "_page=2", refusal: `unsupported query parameter "_page"`},
		{name: "a feed refuses a page beside its cursor", model: &feedModel{}, query: "_page=3&_cursor_value=x", refusal: `unsupported query parameter "_page"`},
		{name: "a list sorts and pages", model: &queryModel{}, query: "_sort_by=id&_page=2&_size=5"},
		{name: "a list that sorts takes no cursor", model: &queryModel{}, query: "_sort_by=id&_cursor_next=true", refusal: `unsupported query parameter "_cursor_next"`},
		{name: "a feed expands", model: &queryFeedModel{}, query: "_expand=all&_depth=2&_cursor_next=true"},
		{name: "a feed takes no order of the client's", model: &queryFeedModel{}, query: "_sort_by=id&_cursor_value=x", refusal: `unsupported query parameter "_sort_by"`},
		{name: "a feed lists every parameter it refuses", model: &queryFeedModel{}, query: "_sort_by=id&_page=2", refusal: `unsupported query parameters "_page", "_sort_by"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			query, err := url.ParseQuery(tt.query)
			require.NoError(t, err)

			err = decodeListQuery(tt.model, query)
			if err == nil {
				_, err = urlquery.Cursor(query, tt.model)
			}
			if tt.refusal == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.refusal)
		})
	}
}

type filterKeyTestModel struct {
	Name string `query:"name"`
	Age  int    `json:"age" query:"age"`

	modelregistry.Query
	modelregistry.Base
}

func TestDecodeListQueryIgnoresFilterKeys(t *testing.T) {
	var m filterKeyTestModel
	require.NoError(t, decodeListQuery(&m, map[string][]string{
		"name":       {"alice"},
		"age":        {"10"},
		"age[gt]":    {"20"},
		"created_at": {"2026-07-01"},
	}))
	require.Equal(t, "alice", m.Name)
	require.Equal(t, 10, m.Age,
		"the bare key keeps feeding the exact business filter while its operator key is left to urlquery.Filters")
}

type listBenchmarkModel struct {
	Name string `query:"name"`

	modelregistry.Base
}

type listBenchmarkQueryableModel struct {
	Name string `query:"name"`

	modelregistry.Query
	modelregistry.Base
}

type listBenchmarkPlainModel struct {
	Name string `query:"name"`

	modelregistry.Base
}

type listBenchmarkCursorableModel struct {
	Name string `query:"name"`

	modelregistry.Cursor
	modelregistry.Base
}

var (
	listBenchmarkPlainQuery = map[string][]string{
		"name": {"alice"},
	}
	listBenchmarkFullQuery = map[string][]string{
		"name":     {"alice"},
		"_sort_by": {"created_at desc"},
		"_expand":  {"all"},
		"_page":    {"2"},
		"_size":    {"10"},
	}
	listBenchmarkPaginationQuery = map[string][]string{
		"_page": {"2"},
		"_size": {"10"},
	}
	listBenchmarkCursorQuery = map[string][]string{
		"_cursor_value": {"0196a0b3-c9d1-713c-870e-adc76af9f857"},
		"_cursor_field": {"id"},
		"_cursor_next":  {"true"},
	}
	listBenchmarkRejectedQuery = map[string][]string{
		"_sort_by": {"created_at desc"},
	}
)

func BenchmarkDecodeListQuery(b *testing.B) {
	b.Run("PlainModelField", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkModel
			if err := decodeListQuery(&m, listBenchmarkPlainQuery); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Query", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkQueryableModel
			if err := decodeListQuery(&m, listBenchmarkFullQuery); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Pagination", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkPlainModel
			if err := decodeListQuery(&m, listBenchmarkPaginationQuery); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Cursor", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkCursorableModel
			if err := decodeListQuery(&m, listBenchmarkCursorQuery); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RejectFrameworkKey", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var m listBenchmarkModel
			if err := decodeListQuery(&m, listBenchmarkRejectedQuery); err == nil {
				b.Fatal("expected framework query key to be rejected")
			}
		}
	})
}

// expandQueryTestModel is a self-referencing tree: Children, ChildItems and
// Parent hold the model itself through ParentID, while Entries holds rows of
// another model.
type expandQueryTestModel struct {
	ParentID   string                  `gorm:"size:191"`
	Children   []*expandQueryTestModel `gorm:"foreignKey:ParentID"`
	Parent     *expandQueryTestModel   `gorm:"foreignKey:ParentID;references:ID"`
	ChildItems []*expandQueryTestModel `gorm:"foreignKey:ParentID"`
	Entries    []*expandQueryTestEntry `gorm:"foreignKey:OwnerID"`

	modelregistry.Base
}

func (*expandQueryTestModel) Expands() []string {
	return []string{"Children", "Parent", "ChildItems", "Entries", "Entries.Notes"}
}

// expandQueryTestEntry is held by expandQueryTestModel and holds notes, which
// the model expands through it by the path Entries.Notes.
type expandQueryTestEntry struct {
	OwnerID string                 `gorm:"size:191"`
	Notes   []*expandQueryTestNote `gorm:"foreignKey:EntryID"`

	modelregistry.Base
}

// expandQueryTestNote is held by expandQueryTestEntry and holds nothing.
type expandQueryTestNote struct {
	EntryID string `gorm:"size:191"`

	modelregistry.Base
}

// TestParseExpandQueryResolvesTheExpandableAssociations pins how _expand and
// _depth name the associations a read preloads: names match the model's
// expands ignoring case, snake case and the spaces around them, all selects
// every one wherever it stands among the names, an item left empty names
// nothing, a path of several steps the model declares is named by its steps,
// and the depth repeats an association holding the model itself, one to ten,
// zero read as the depth left out, while any other association ignores it.
func TestParseExpandQueryResolvesTheExpandableAssociations(t *testing.T) {
	for _, tt := range []struct {
		name   string
		target string
		want   []string
	}{
		{"DepthRepeatsAnAssociationHoldingTheModel", "/items?_expand=Children&_depth=3", []string{"Children.Children.Children"}},
		{"ASingleValuedAssociationIgnoresDepth", "/items?_expand=Parent&_depth=3", []string{"Parent"}},
		// An entry holds no entries: repeating the association would name a
		// relation the entry's model does not have, which gorm refuses.
		{"AnAssociationOfAnotherModelIgnoresDepth", "/items?_expand=Entries&_depth=3", []string{"Entries"}},
		{"AllSelectsEveryModelExpand", "/items?_expand=all", []string{"Children", "Parent", "ChildItems", "Entries", "Entries.Notes"}},
		{"AllAmongTheNamesSelectsEveryModelExpand", "/items?_expand=parent,all", []string{"Children", "Parent", "ChildItems", "Entries", "Entries.Notes"}},
		{"ExpandMatchesCaseInsensitively", "/items?_expand=children", []string{"Children"}},
		{"ExpandMatchesSnakeCaseName", "/items?_expand=child_items", []string{"ChildItems"}},
		{"APathTheModelDeclaresIsNamedByItsSteps", "/items?_expand=entries.notes", []string{"Entries.Notes"}},
		{"NamesAreReadWithoutTheSpacesAroundThem", "/items?_expand=parent,%20children", []string{"Children", "Parent"}},
		{"AllIsReadWithoutTheSpacesAroundIt", "/items?_expand=parent,%20all", []string{"Children", "Parent", "ChildItems", "Entries", "Entries.Notes"}},
		{"AnItemOfSpacesNamesNothing", "/items?_expand=children,%20", []string{"Children"}},
		{"ANameGivenTwiceIsExpandedOnce", "/items?_expand=children,Children", []string{"Children"}},
		{"AnEmptyItemNamesNothing", "/items?_expand=children,", []string{"Children"}},
		{"DepthAcceptsUpperBoundTen", "/items?_expand=Children&_depth=10", []string{strings.Repeat("Children.", 9) + "Children"}},
		{"ZeroDepthReadsAsTheDepthLeftOut", "/items?_expand=Children&_depth=0", []string{"Children"}},
		{"AnEmptyExpandExpandsNothing", "/items?_expand=", nil},
		{"NoExpandParameterExpandsNothing", "/items", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			expands, err := parseExpandQuery(newTestGetContext(t, tt.target).Request.URL.Query(), &expandQueryTestModel{})
			require.NoError(t, err)
			require.Equal(t, tt.want, expands)
		})
	}
}

// TestParseExpandQueryRefusesWhatItCannotRead pins that an expansion naming
// an association the model does not expand, or a depth that is no count or
// runs past the cap, is refused rather than read as no expansion or as the
// depth left out: the answer names the parameter and echoes the value alone,
// a depth that is no count in the words the model decode answers one. The
// depth is read whether or not the request expands anything.
func TestParseExpandQueryRefusesWhatItCannotRead(t *testing.T) {
	for _, tt := range []struct{ name, target, want string }{
		{"UnknownAssociation", "/items?_expand=Bogus", `invalid query parameter "_expand": unknown association "Bogus"`},
		{"UnknownAssociationAmongKnownOnes", "/items?_expand=children,bogus", `invalid query parameter "_expand": unknown association "bogus"`},
		{"UnknownAssociationBesideAll", "/items?_expand=all,bogus", `invalid query parameter "_expand": unknown association "bogus"`},
		{"APathTheModelDoesNotDeclare", "/items?_expand=entries.owner", `invalid query parameter "_expand": unknown association "entries.owner"`},
		{"DepthPastTheCap", "/items?_expand=Children&_depth=11", `invalid query parameter "_depth": expect a value from 1 to 10, got "11"`},
		{"DepthThatIsNoNumber", "/items?_expand=Children&_depth=abc", `invalid query parameter "_depth": expect a numeric value, got "abc"`},
		{"NegativeDepth", "/items?_expand=Children&_depth=-1", `invalid query parameter "_depth": expect a non-negative value, got "-1"`},
		{"DepthPastTheCapWithNothingToExpand", "/items?_depth=11", `invalid query parameter "_depth": expect a value from 1 to 10, got "11"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseExpandQuery(newTestGetContext(t, tt.target).Request.URL.Query(), &expandQueryTestModel{})
			require.EqualError(t, err, tt.want)
		})
	}
}

// newTestGetContext builds a gin context carrying a GET request with the given
// target URL, for exercising query-parameter parsing helpers.
func newTestGetContext(t *testing.T, target string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}
