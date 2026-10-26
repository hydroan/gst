package openapigen

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/internal/urlquery"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// openapiTimeFilterModel filters on a time, which no bare key decodes into,
// under its operators alone.
type openapiTimeFilterModel struct {
	ExpiresAt *time.Time `json:"expires_at,omitempty" query:"expires_at"`

	modelregistry.Query
	modelregistry.Base
}

func init() {
	registerFixtureDoc("openapiTimeFilterModel", "", map[string]string{
		"ExpiresAt": "ExpiresAt is the expiration time.",
	})
}

func TestAddQueryParametersUsesDateTimeFormatForTimeFields(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiTimeFilterModel, *openapiTimeFilterModel, *openapiTimeFilterModel](op, true)
	parameters := queryParametersByName(t, op)

	for _, name := range []string{"expires_at[gte]", "expires_at[lt]", "created_at"} {
		require.NotNil(t, parameters[name], "query parameter %q", name)
		assertDateTimeSchema(t, parameters[name].Schema)
	}
	require.Equal(t, "The expiration time.", parameters["expires_at[gte]"].Description,
		"an operator key is described by the API-facing comment of its field")

	isNull := parameters["expires_at[isnull]"]
	require.NotNil(t, isNull)
	require.True(t, isNull.Schema.Value.Type.Is(openapi3.TypeBoolean), "isnull takes a boolean")
}

type openapiPaginationQueryModel struct {
	modelregistry.Pagination
	modelregistry.Base
}

type openapiCursorQueryModel struct {
	modelregistry.Cursor
	modelregistry.Base
}

// The query branches are exported: the decoder walks into exported embedded
// structs alone, as gorm does.

type OpenAPIDeepQueryFields struct {
	Shared int `json:"shared" query:"shared"`
}

type OpenAPIFirstQueryBranch struct {
	OpenAPIDeepQueryFields
}

type OpenAPISecondQueryBranch struct {
	Shared string `json:"shared" query:"shared"`
}

type openapiShallowQueryModel struct {
	OpenAPIFirstQueryBranch
	OpenAPISecondQueryBranch
	modelregistry.Base
}

func TestAddQueryParametersIncludesEmbeddedFrameworkParameters(t *testing.T) {
	tests := []struct {
		name       string
		add        func(*openapi3.Operation)
		parameters []string
	}{
		{
			name: "query",
			add: func(op *openapi3.Operation) {
				addQueryParameters[*openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel](op, true)
			},
			parameters: []string{
				"page",
				"_page", "_size",
				"_cursor_value", "_cursor_field", "_cursor_next",
				"_expand", "_depth", "_sort_by",
				"id", "created_by", "updated_by",
				"created_at", "updated_at",
			},
		},
		{
			name: "pagination",
			add: func(op *openapi3.Operation) {
				addQueryParameters[*openapiPaginationQueryModel, *openapiPaginationQueryModel, *openapiPaginationQueryModel](op, true)
			},
			parameters: []string{"_page", "_size", "id", "created_by", "updated_by"},
		},
		{
			name: "cursor",
			add: func(op *openapi3.Operation) {
				addQueryParameters[*openapiCursorQueryModel, *openapiCursorQueryModel, *openapiCursorQueryModel](op, true)
			},
			parameters: []string{"_size", "_cursor_value", "_cursor_field", "_cursor_next", "id", "created_by", "updated_by"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &openapi3.Operation{}
			tt.add(op)

			parameters := queryParametersByName(t, op)
			for _, name := range tt.parameters {
				if parameters[name] == nil {
					t.Errorf("query parameter %q is missing", name)
				}
			}
			if len(parameters) != len(tt.parameters) {
				t.Fatalf("query parameters = %v, want exactly %v", parameterNames(parameters), tt.parameters)
			}

			// Framework parameters live in the "_" namespace and describe
			// themselves through the doc comments modelregistry registers from
			// its embedded source, so the description must hold up in a binary
			// shipped without Go source files.
			for _, name := range tt.parameters {
				if !strings.HasPrefix(name, "_") {
					continue
				}
				if parameter := parameters[name]; parameter != nil && parameter.Description == "" {
					t.Errorf("query parameter %q description is empty, want the registered doc comment", name)
				}
			}
		})
	}

	op := &openapi3.Operation{}
	addQueryParameters[*openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel](op, true)
	page := queryParametersByName(t, op)["page"]
	if page.Schema == nil || page.Schema.Value == nil || page.Schema.Value.Type == nil || !page.Schema.Value.Type.Is(openapi3.TypeString) {
		t.Fatalf("page schema = %#v, want the bare business field to keep its own string schema alongside the framework _page parameter", page.Schema)
	}

	op = &openapi3.Operation{}
	addQueryParameters[*openapiShallowQueryModel, *openapiShallowQueryModel, *openapiShallowQueryModel](op, true)
	shared := queryParametersByName(t, op)["shared"]
	if shared == nil || shared.Schema == nil || shared.Schema.Value == nil || shared.Schema.Value.Type == nil || !shared.Schema.Value.Type.Is(openapi3.TypeString) {
		t.Fatalf("shared = %#v, want the shallower embedded string field to override the earlier deeper field", shared)
	}
}

// openapiQueryPart is embedded by name in openapiFilterQueryModel, its column
// named by the path of query tags it takes.
type openapiQueryPart struct {
	Label string `json:"label" query:"label"`
}

// OpenAPIQueryExtra is embedded through a pointer in openapiFilterQueryModel,
// its column promoted as one embedded by value is.
type OpenAPIQueryExtra struct {
	Code string `json:"code" query:"code"`
}

// openapiFilterQueryModel declares a field of each kind that decides which
// parameters a list takes: a scalar column, a bool column, a time column and
// a JSON column no bare key decodes into, a column without a query tag, a
// column hidden from JSON though tagged, a field that is no column, hidden
// from JSON and tagged, a struct embedded by name and one embedded through a
// pointer.
type openapiFilterQueryModel struct {
	Visible   string            `json:"visible" query:"visible"`
	Active    bool              `json:"active" query:"active"`
	ExpiresAt time.Time         `json:"expires_at" query:"expires_at"`
	Meta      datatypes.JSONMap `json:"meta" query:"meta"`
	Note      string            `json:"note"`
	Secret    string            `json:"-" query:"secret"`
	Scope     string            `json:"-" gorm:"-" query:"scope"`
	First     openapiQueryPart  `json:"first" gorm:"embedded;embeddedPrefix:first_" query:"first"`
	*OpenAPIQueryExtra

	modelregistry.Query
	modelregistry.Base
}

// openapiPlainFilterQueryModel is openapiFilterQueryModel paging by offset
// without model.Query, so no column takes an operator.
type openapiPlainFilterQueryModel struct {
	Visible   string            `json:"visible" query:"visible"`
	Active    bool              `json:"active" query:"active"`
	ExpiresAt time.Time         `json:"expires_at" query:"expires_at"`
	Meta      datatypes.JSONMap `json:"meta" query:"meta"`
	Note      string            `json:"note"`
	Secret    string            `json:"-" query:"secret"`
	Scope     string            `json:"-" gorm:"-" query:"scope"`
	First     openapiQueryPart  `json:"first" gorm:"embedded;embeddedPrefix:first_" query:"first"`
	*OpenAPIQueryExtra

	modelregistry.Pagination
	modelregistry.Base
}

// TestAddQueryParametersDocumentsWhatTheURLQueryTakes pins the documented
// parameters of a list to the ones its URL query takes: every documented key
// is taken, a bare key by the decoder or the filters and an operator key by
// the filters, and no key the document leaves out is, under every spelling a
// client could try for a field of the model.
func TestAddQueryParametersDocumentsWhatTheURLQueryTakes(t *testing.T) {
	t.Run("Queryable", func(t *testing.T) {
		requireDocumentedAsTaken[*openapiFilterQueryModel](t)
	})
	t.Run("NotQueryable", func(t *testing.T) {
		requireDocumentedAsTaken[*openapiPlainFilterQueryModel](t)
	})
}

func TestAddQueryParametersOrdersBusinessFieldsBeforeFrameworkParameters(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel](op, true)

	names := make([]string, 0, len(op.Parameters))
	for _, parameterRef := range op.Parameters {
		names = append(names, parameterRef.Value.Name)
	}
	want := []string{
		"page",
		"id", "created_by", "updated_by",
		"created_at", "updated_at",
		"_page", "_size", "_sort_by",
		"_cursor_value", "_cursor_field", "_cursor_next",
		"_expand", "_depth",
	}
	if len(names) != len(want) {
		t.Fatalf("query parameters = %v, want exactly %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("query parameter order = %v, want %v: business filter columns must come first regardless of where the framework structs are embedded", names, want)
		}
	}
}

// openapiExpandableQueryModel is a tree, its children and parent the
// associations a list expands.
type openapiExpandableQueryModel struct {
	ParentID string                         `json:"parent_id" gorm:"size:191"`
	Children []*openapiExpandableQueryModel `json:"children,omitempty" gorm:"foreignKey:ParentID;constraint:-"`
	Parent   *openapiExpandableQueryModel   `json:"parent,omitempty" gorm:"foreignKey:ParentID;references:ID;constraint:-"`

	modelregistry.Query
	modelregistry.Base
}

func (*openapiExpandableQueryModel) Expands() []string { return []string{"Children", "Parent"} }

func TestAddQueryParametersDocumentsExpandableFields(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiExpandableQueryModel, *openapiExpandableQueryModel, *openapiExpandableQueryModel](op, true)
	parameters := queryParametersByName(t, op)

	expand := parameters["_expand"]
	if expand == nil {
		t.Fatal("_expand query parameter was not added")
	}
	for _, token := range []string{"Children", "Parent", "all"} {
		if !strings.Contains(expand.Description, token) {
			t.Fatalf("_expand description = %q, want expandable field %q listed", expand.Description, token)
		}
	}
}

// TestAddQueryParametersDocumentsFieldOperatorFilters pins the operator note
// of a bare parameter to the operators its column takes: every one on a
// string, the ones a bool takes on a bool, and none on a framework parameter,
// a field that is no column or a model without model.Query.
func TestAddQueryParametersDocumentsFieldOperatorFilters(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel](op, true)
	parameters := queryParametersByName(t, op)

	if !strings.Contains(parameters["page"].Description, "page[op]=value") {
		t.Fatalf("page description = %q, want a field operator filter note on queryable models", parameters["page"].Description)
	}
	if !strings.Contains(parameters["page"].Description, "notlike") {
		t.Fatalf("page description = %q, want the known operators listed", parameters["page"].Description)
	}
	if strings.Contains(parameters["_sort_by"].Description, "[op]=value") {
		t.Fatalf("_sort_by description = %q, framework parameters must not advertise operator filters", parameters["_sort_by"].Description)
	}

	op = &openapi3.Operation{}
	addQueryParameters[*openapiFilterQueryModel, *openapiFilterQueryModel, *openapiFilterQueryModel](op, true)
	parameters = queryParametersByName(t, op)
	require.Contains(t, parameters["active"].Description, "op: eq/ne/isnull.", "a bool column takes eq, ne and isnull alone")
	require.NotContains(t, parameters["scope"].Description, "[op]=value", "a field that is no column takes no operator")

	op = &openapi3.Operation{}
	addQueryParameters[*openapiPaginationQueryModel, *openapiPaginationQueryModel, *openapiPaginationQueryModel](op, true)
	parameters = queryParametersByName(t, op)
	if strings.Contains(parameters["id"].Description, "[op]=value") {
		t.Fatalf("id description = %q, models without modelregistry.Query must not advertise operator filters", parameters["id"].Description)
	}
}

// openapiTagsFilterModel stores its tags as a JSON document, which no bare
// key decodes into and the like family alone matches by its text.
type openapiTagsFilterModel struct {
	Tags datatypes.JSONSlice[string] `json:"tags" query:"tags"`

	modelregistry.Query
	modelregistry.Base
}

// TestAddQueryParametersDocumentsEachOperatorOfAColumnWithoutABareKey pins
// a column taking its operators alone to a parameter under each of them, the
// key spelled as the URL spells it, its value described as the operator reads
// it: the text the like family matches and the boolean isnull takes.
func TestAddQueryParametersDocumentsEachOperatorOfAColumnWithoutABareKey(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParameters[*openapiTagsFilterModel, *openapiTagsFilterModel, *openapiTagsFilterModel](op, true)
	parameters := queryParametersByName(t, op)

	for _, name := range []string{"tags[like]", "tags[notlike]", "tags[startswith]", "tags[endswith]"} {
		require.NotNil(t, parameters[name], "query parameter %q", name)
		require.True(t, parameters[name].Schema.Value.Type.Is(openapi3.TypeString), "query parameter %q schema", name)
	}
	require.NotNil(t, parameters["tags[isnull]"])
	require.True(t, parameters["tags[isnull]"].Schema.Value.Type.Is(openapi3.TypeBoolean))
}

type enumQueryModel struct {
	Status enumFieldStatus `json:"status" query:"status"`

	modelregistry.Base
}

func TestAddQueryParametersSetsEnumValues(t *testing.T) {
	registerEnumFieldStatus()

	op := &openapi3.Operation{}
	addQueryParameters[*enumQueryModel, *enumQueryModel, *enumQueryModel](op, true)

	for _, parameter := range op.Parameters {
		if parameter.Value == nil || parameter.Value.Name != "status" {
			continue
		}
		if len(parameter.Value.Schema.Value.Enum) != 2 {
			t.Fatalf("status parameter enum = %#v, want the two enum values", parameter.Value.Schema.Value.Enum)
		}
		if !strings.Contains(parameter.Value.Description, "The demo status enum.") {
			t.Fatalf("status parameter description = %q, want API-facing enum type comment", parameter.Value.Description)
		}
		if !strings.Contains(parameter.Value.Description, "- `active`: the record is active") {
			t.Fatalf("status parameter description = %q, want enum value comments", parameter.Value.Description)
		}
		return
	}
	t.Fatal("status query parameter was not added")
}

// requireDocumentedAsTaken requires the query parameters the list of M
// documents to be exactly the keys its URL query takes, the way the list
// reads them: the decoder, then the filters. A key is documented as a
// parameter of its own, or as an operator the note of a bare parameter lists.
// A key is taken when one of the values its field could hold passes both;
// the keys tried are the documented ones and every spelling a client could
// try for a field of M, bare and under each operator.
func requireDocumentedAsTaken[M types.Model](t *testing.T) {
	t.Helper()
	op := &openapi3.Operation{}
	addQueryParameters[M, M, M](op, true)
	parameters := queryParametersByName(t, op)
	documented := func(key string) bool {
		if parameters[key] != nil {
			return true
		}
		name, filterOp, ok := strings.Cut(strings.TrimSuffix(key, "]"), "[")
		if !ok || parameters[name] == nil {
			return false
		}
		_, ops, ok := strings.Cut(parameters[name].Description, "Operator filter: "+name+"[op]=value, op: ")
		if !ok {
			return false
		}
		ops, _, _ = strings.Cut(ops, ".")
		for listed := range strings.SplitSeq(ops, "/") {
			if listed == filterOp {
				return true
			}
		}
		return false
	}

	taken := func(key string) bool {
		for _, value := range []string{"x", "1", "true", "2026-01-02T15:04:05Z"} {
			m := reflect.New(reflect.TypeFor[M]().Elem()).Interface().(types.Model) //nolint:errcheck
			query := url.Values{key: {value}}
			if urlquery.Decode(query, m) != nil {
				continue
			}
			if _, err := urlquery.Filters(query, m); err != nil {
				continue
			}
			return true
		}
		return false
	}

	keys := make(map[string]bool)
	for name := range parameters {
		keys[name] = true
	}
	for _, name := range []string{
		"visible", "active", "expires_at", "meta", "note", "secret", "scope",
		"first", "label", "first.label", "code", "id", "created_at", "deleted_at",
	} {
		keys[name] = true
		for _, filterOp := range types.FilterOps() {
			keys[name+"["+string(filterOp)+"]"] = true
		}
	}
	for key := range keys {
		require.Equal(t, taken(key), documented(key), "query parameter %q: documented must equal taken", key)
	}
}
