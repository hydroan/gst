package openapigen

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types/consts"
)

func TestSetMarksPublicRouteSecurity(t *testing.T) {
	set[*openapiTimeQueryModel, *openapiTimeQueryModel, *openapiTimeQueryModel]("/api/test-public-route", false, consts.List)

	item := doc.Paths.Value("/api/test-public-route")
	if item == nil || item.Get == nil {
		t.Fatal("GET /api/test-public-route missing from document")
	}
	if item.Get.Security == nil || len(*item.Get.Security) != 0 {
		t.Fatalf("public op security = %+v, want an empty override", item.Get.Security)
	}
}

func TestSetKeepsAuthRouteSecurityDefault(t *testing.T) {
	set[*openapiTimeQueryModel, *openapiTimeQueryModel, *openapiTimeQueryModel]("/api/test-auth-route", true, consts.List)

	item := doc.Paths.Value("/api/test-auth-route")
	if item == nil || item.Get == nil {
		t.Fatal("GET /api/test-auth-route missing from document")
	}
	if item.Get.Security != nil {
		t.Fatalf("auth op security = %+v, want nil so the document-level security applies", item.Get.Security)
	}
}

func TestSetDocumentsEmbeddedFrameworkQueryParameters(t *testing.T) {
	set[*openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel, *openapiEmbeddedQueryModel]("/api/test-query-contract", true, consts.List)

	item := doc.Paths.Value("/api/test-query-contract")
	if item == nil || item.Get == nil {
		t.Fatal("GET /api/test-query-contract missing from document")
	}
	parameters := queryParametersByName(t, item.Get)
	for _, name := range []string{"_page", "_size", "_cursor_value", "_sort_by"} {
		if parameters[name] == nil {
			t.Errorf("query parameter %q is missing from the generated List operation", name)
		}
	}
}

// openapiActionModel backs two custom Create routes that carry distinct
// payloads, mirroring a resource that exposes several non-CRUD actions.
type openapiActionModel struct {
	// Name is the record name.
	Name string `json:"name"`

	modelregistry.Base
}

type openapiFirstActionReq struct {
	// Reason is the field unique to the first action.
	Reason string `json:"reason"`
}

type openapiFirstActionRsp struct {
	ID string `json:"id"`
}

type openapiSecondActionReq struct {
	// Token is the field unique to the second action.
	Token string `json:"token"`
}

type openapiSecondActionRsp struct {
	Name string `json:"name"`
}

// TestSetGivesEachCustomPayloadItsOwnRequestBody asserts that two actions
// sharing a model and a phase each keep their own payload, instead of
// collapsing onto one component keyed by the model.
func TestSetGivesEachCustomPayloadItsOwnRequestBody(t *testing.T) {
	set[*openapiActionModel, *openapiFirstActionReq, *openapiFirstActionRsp]("/api/openapi-actions/first", true, consts.Create)
	set[*openapiActionModel, *openapiSecondActionReq, *openapiSecondActionRsp]("/api/openapi-actions/second", true, consts.Create)

	first := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-actions/first").RequestBody)
	if _, ok := first.Properties["reason"]; !ok {
		t.Fatalf("first action payload properties = %v, want reason", propertyNames(first))
	}

	second := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-actions/second").RequestBody)
	if _, ok := second.Properties["token"]; !ok {
		t.Fatalf("second action payload properties = %v, want token", propertyNames(second))
	}
}

// TestSetGivesEachCustomResponseItsOwnComponent asserts the same isolation for
// responses, which share the request component key today.
func TestSetGivesEachCustomResponseItsOwnComponent(t *testing.T) {
	set[*openapiActionModel, *openapiFirstActionReq, *openapiFirstActionRsp]("/api/openapi-responses/first", true, consts.Create)
	set[*openapiActionModel, *openapiSecondActionReq, *openapiSecondActionRsp]("/api/openapi-responses/second", true, consts.Create)

	first := registeredResponseSchema(t, responseRefForPath(t, "/api/openapi-responses/first", 200))
	if _, ok := dataSchema(t, first).Properties["id"]; !ok {
		t.Fatalf("first action response data = %v, want id", propertyNames(dataSchema(t, first)))
	}

	second := registeredResponseSchema(t, responseRefForPath(t, "/api/openapi-responses/second", 200))
	if _, ok := dataSchema(t, second).Properties["name"]; !ok {
		t.Fatalf("second action response data = %v, want name", propertyNames(dataSchema(t, second)))
	}
}

type openapiFirstTwinReq struct {
	// Secret is shared with openapiSecondTwinReq, making the two structurally
	// identical while remaining distinct types.
	Secret string `json:"secret"`
}

type openapiSecondTwinReq struct {
	Secret string `json:"secret"`
}

type openapiTwinRsp struct {
	OK bool `json:"ok"`
}

// TestSetSeparatesStructurallyIdenticalPayloads asserts that two payload types
// with identical fields still resolve to their own components, since the key is
// derived from the type rather than from its shape.
func TestSetSeparatesStructurallyIdenticalPayloads(t *testing.T) {
	set[*openapiActionModel, *openapiFirstTwinReq, *openapiTwinRsp]("/api/openapi-twins/first", true, consts.Create)
	set[*openapiActionModel, *openapiSecondTwinReq, *openapiTwinRsp]("/api/openapi-twins/second", true, consts.Create)

	first := operationForPath(t, "/api/openapi-twins/first").RequestBody.Ref
	second := operationForPath(t, "/api/openapi-twins/second").RequestBody.Ref
	if first == second {
		t.Fatalf("both twin payloads resolved to %q, want one component each", first)
	}
}

// TestSetKeepsModelKeyForDefaultCRUD pins the component key of a default CRUD
// action, where the model doubles as request and response.
func TestSetKeepsModelKeyForDefaultCRUD(t *testing.T) {
	set[*openapiActionModel, *openapiActionModel, *openapiActionModel]("/api/openapi-crud", true, consts.Create)

	op := operationForPath(t, "/api/openapi-crud")
	wantRef := "#/components/requestBodies/internal.openapigen.openapiactionmodel_create"
	if op.RequestBody.Ref != wantRef {
		t.Fatalf("default CRUD requestBody ref = %q, want %q", op.RequestBody.Ref, wantRef)
	}
}

// TestSetSeparatesListAndGetResponsesForSameType asserts that one response type
// reused across two phases keeps the list envelope apart from the single-item
// envelope.
func TestSetSeparatesListAndGetResponsesForSameType(t *testing.T) {
	set[*openapiActionModel, *openapiActionModel, *openapiActionModel]("/api/openapi-envelope", true, consts.List)
	set[*openapiActionModel, *openapiActionModel, *openapiActionModel]("/api/openapi-envelope/:id", true, consts.Get)

	list := dataSchema(t, registeredResponseSchema(t, responseRefForPath(t, "/api/openapi-envelope", 200)))
	if _, ok := list.Properties["items"]; !ok {
		t.Fatalf("list response data = %v, want items", propertyNames(list))
	}

	get := dataSchema(t, registeredResponseSchema(t, responseRefForPath(t, "/api/openapi-envelope/{id}", 200)))
	if _, ok := get.Properties["items"]; ok {
		t.Fatalf("get response data = %v, want a single record rather than items", propertyNames(get))
	}
}

type openapiNoDataReq struct {
	// Reason explains the action.
	Reason string `json:"reason"`
}

// openapiNoDataRsp carries no fields: the action answers with the plain
// envelope and a null data member.
type openapiNoDataRsp struct{}

// TestSetDeclaresResponsesForEmptyResponseType asserts that an action whose
// response type has no fields still documents its response. The handler answers
// {"code":0,"data":null,"msg":"success","trace_id":"..."}, and responses is a
// required member of an OpenAPI operation.
func TestSetDeclaresResponsesForEmptyResponseType(t *testing.T) {
	set[*openapiActionModel, *openapiNoDataReq, *openapiNoDataRsp]("/api/openapi-nodata", true, consts.Create)

	op := operationForPath(t, "/api/openapi-nodata")
	if op.Responses == nil || op.Responses.Len() == 0 {
		t.Fatal("operation declares no responses")
	}
	schema := registeredResponseSchema(t, op.Responses.Status(200))
	for _, field := range []string{"code", "msg", "trace_id", "data"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Errorf("response envelope = %v, want a %q member", propertyNames(schema), field)
		}
	}
}

// openapiTreeNode is self-referential, which makes the schema generator emit a
// component $ref to break the cycle instead of inlining forever.
type openapiTreeNode struct {
	// Label is the node label.
	Label string `json:"label"`

	// Children and Parent close the cycle.
	Children []*openapiTreeNode `json:"children,omitempty"`
	Parent   *openapiTreeNode   `json:"parent,omitempty"`

	modelregistry.Base
}

// TestSetResolvesSelfReferentialSchemaRefs asserts that the $ref emitted for a
// cyclic type points at a component that actually exists, so the document stays
// loadable.
func TestSetResolvesSelfReferentialSchemaRefs(t *testing.T) {
	set[*openapiTreeNode, *openapiTreeNode, *openapiTreeNode]("/api/openapi-trees", true, consts.List)

	docMutex.RLock()
	schemas := doc.Components.Schemas
	docMutex.RUnlock()

	refs := collectSchemaRefs(t, schemas)
	if len(refs) == 0 {
		t.Fatal("no component $ref was emitted for the cyclic type, expected one to break the cycle")
	}
	for ref := range refs {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		if _, ok := schemas[name]; !ok {
			t.Errorf("$ref %q points at a component that is not registered; registered: %v", ref, schemaNames(schemas))
		}
	}
}

// openapiTreeHolder reaches the cyclic type through its own fields, the way a
// model holds a collection of tree-shaped records.
type openapiTreeHolder struct {
	// Nodes and Extra both reach openapiTreeNode, whose children close a cycle.
	Nodes []*openapiTreeNode `json:"nodes,omitempty"`
	Extra []*openapiTreeNode `json:"extra,omitempty"`

	modelregistry.Base
}

// TestSetBuildsExampleWithoutNullPlaceholders asserts that the generated
// example holds no null. A self-referential type recurses until the depth limit
// stops it, and stopping with a null puts an illegal value where the schema
// expects a non-nullable member, which fails document validation.
func TestSetBuildsExampleWithoutNullPlaceholders(t *testing.T) {
	set[*openapiTreeHolder, *openapiTreeHolder, *openapiTreeHolder]("/api/openapi-tree-examples", true, consts.Create)

	schema := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-tree-examples").RequestBody)
	if schema.Example == nil {
		t.Fatal("request body schema carries no example")
	}
	if path, found := findNullInExample(schema.Example, ""); found {
		t.Fatalf("example carries null at %q, want a legal value for a non-nullable member", path)
	}
}

// TestSetStopsExampleAtSchemaRefBoundaries asserts that the example does not
// expand a member the schema renders as a $ref. Such a member serializes as a
// bare $ref, so its inline Value is not the schema readers validate against,
// and walking into it invents values the referenced component rejects.
func TestSetStopsExampleAtSchemaRefBoundaries(t *testing.T) {
	set[*openapiTreeHolder, *openapiTreeHolder, *openapiTreeHolder]("/api/openapi-ref-boundaries", true, consts.Create)

	schema := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-ref-boundaries").RequestBody)
	example, ok := schema.Example.(map[string]any)
	if !ok {
		t.Fatalf("example = %#v, want an object", schema.Example)
	}
	nodes, ok := example["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		t.Fatalf("nodes example = %#v, want a populated array", example["nodes"])
	}
	node, ok := nodes[0].(map[string]any)
	if !ok {
		t.Fatalf("nodes[0] example = %#v, want an object", nodes[0])
	}

	// children items and parent both resolve to the cyclic component.
	children, ok := node["children"].([]any)
	if !ok {
		t.Fatalf("children example = %#v, want an array", node["children"])
	}
	if len(children) != 0 {
		t.Errorf("children example = %#v, want an empty array because its items are a $ref", children)
	}
	if parent, ok := node["parent"]; ok {
		t.Errorf("parent example = %#v, want the $ref member left out", parent)
	}
}

// openapiExampleShapeModel carries the field shapes whose example value is
// constrained by more than its JSON type.
type openapiExampleShapeModel struct {
	// Status is constrained to the registered enum values.
	Status enumFieldStatus `json:"status"`

	// Codes constrains its items to the registered enum values.
	Codes []enumFieldStatus `json:"codes"`

	// StartedAt is a date-time formatted string.
	StartedAt time.Time `json:"started_at"`

	modelregistry.Base
}

// TestSetBuildsExampleHonouringFormatAndEnum asserts that an example value
// respects the constraints its schema declares. A plain "string" placeholder
// fails validation where the schema declares a date-time format or an enum.
func TestSetBuildsExampleHonouringFormatAndEnum(t *testing.T) {
	registerEnumFieldStatus()
	set[*openapiExampleShapeModel, *openapiExampleShapeModel, *openapiExampleShapeModel]("/api/openapi-example-shapes", true, consts.Create)

	schema := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-example-shapes").RequestBody)
	example, ok := schema.Example.(map[string]any)
	if !ok {
		t.Fatalf("example = %#v, want an object", schema.Example)
	}

	if got := example["status"]; got != "active" {
		t.Errorf("status example = %#v, want the first enum value %q", got, "active")
	}

	codes, ok := example["codes"].([]any)
	if !ok || len(codes) == 0 {
		t.Fatalf("codes example = %#v, want a populated array", example["codes"])
	}
	if codes[0] != "active" {
		t.Errorf("codes[0] example = %#v, want the first enum value %q", codes[0], "active")
	}

	startedAt, ok := example["started_at"].(string)
	if !ok {
		t.Fatalf("started_at example = %#v, want a string", example["started_at"])
	}
	if _, err := time.Parse(time.RFC3339, startedAt); err != nil {
		t.Errorf("started_at example = %q, want a date-time formatted value: %v", startedAt, err)
	}
}

// openapiAnyMapModel holds a map whose keys and values are user-defined, so the
// value schema declares no type and constrains nothing.
type openapiAnyMapModel struct {
	// Attributes holds user-defined keys and values.
	Attributes map[string]any `json:"attributes"`

	modelregistry.Base
}

// TestSetBuildsExampleForUntypedMapValues asserts that a schema declaring no
// type still gets a non-null example. Such a schema accepts any value, but a
// null is only accepted when the schema is explicitly nullable.
func TestSetBuildsExampleForUntypedMapValues(t *testing.T) {
	set[*openapiAnyMapModel, *openapiAnyMapModel, *openapiAnyMapModel]("/api/openapi-any-maps", true, consts.Create)

	schema := registeredRequestBodySchema(t, operationForPath(t, "/api/openapi-any-maps").RequestBody)
	example, ok := schema.Example.(map[string]any)
	if !ok {
		t.Fatalf("example = %#v, want an object", schema.Example)
	}
	if path, found := findNullInExample(example["attributes"], "/attributes"); found {
		t.Fatalf("example carries null at %q, want a value an untyped schema accepts", path)
	}
}

// openapiKeyModel exercises every verb so each generated component key is
// covered.
type openapiKeyModel struct {
	// Name is the record name.
	Name string `json:"name"`

	modelregistry.Base
}

// componentKeyCharset is the character set the OpenAPI spec allows in a
// components key.
var componentKeyCharset = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// TestSetBuildsComponentKeysWithinSpecCharset asserts that every registered
// component key stays inside the character set the spec allows, across all
// verbs. A key outside it makes the whole document invalid.
func TestSetBuildsComponentKeysWithinSpecCharset(t *testing.T) {
	set[*openapiKeyModel, *openapiKeyModel, *openapiKeyModel]("/api/openapi-keys", true,
		consts.Create, consts.Delete, consts.Update, consts.Patch, consts.List, consts.Get,
		consts.CreateMany, consts.DeleteMany, consts.UpdateMany, consts.PatchMany)

	docMutex.RLock()
	defer docMutex.RUnlock()

	for key := range doc.Components.RequestBodies {
		if !componentKeyCharset.MatchString(key) {
			t.Errorf("requestBodies key %q falls outside the spec charset %s", key, componentKeyCharset)
		}
	}
	for key := range doc.Components.Responses {
		if !componentKeyCharset.MatchString(key) {
			t.Errorf("responses key %q falls outside the spec charset %s", key, componentKeyCharset)
		}
	}
	for key := range doc.Components.Schemas {
		if !componentKeyCharset.MatchString(key) {
			t.Errorf("schemas key %q falls outside the spec charset %s", key, componentKeyCharset)
		}
	}
}

// findNullInExample reports the first path inside an example value that holds a
// null.
func findNullInExample(value any, path string) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return path, true
	case map[string]any:
		for key, member := range typed {
			if found, ok := findNullInExample(member, path+"/"+key); ok {
				return found, true
			}
		}
	case []any:
		for index, member := range typed {
			if found, ok := findNullInExample(member, fmt.Sprintf("%s/%d", path, index)); ok {
				return found, true
			}
		}
	}
	return "", false
}

func collectSchemaRefs(t *testing.T, schemas openapi3.Schemas) map[string]bool {
	t.Helper()

	found := map[string]bool{}
	var walk func(ref *openapi3.SchemaRef, depth int)
	walk = func(ref *openapi3.SchemaRef, depth int) {
		if ref == nil || depth > 10 {
			return
		}
		if ref.Ref != "" {
			found[ref.Ref] = true
			return
		}
		if ref.Value == nil {
			return
		}
		for _, property := range ref.Value.Properties {
			walk(property, depth+1)
		}
		walk(ref.Value.Items, depth+1)
		if ref.Value.AdditionalProperties.Schema != nil {
			walk(ref.Value.AdditionalProperties.Schema, depth+1)
		}
	}
	for _, ref := range schemas {
		walk(ref, 0)
	}
	return found
}

func schemaNames(schemas openapi3.Schemas) []string {
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
