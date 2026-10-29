package controller_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// The fixed messages of the controller's refusals, as errors.go answers
// them on both transports.
const (
	invalidArgumentMsg = "The request contains invalid parameters."
	notFoundMsg        = "The requested resource was not found."
	staleObjectMsg     = "The resource was modified by another operation. Reload and retry."
)

// TestTransportsAnswerTheContractAlike pins the contract of the actions on both
// transports at once: every scenario of the table is sent as the HTTP request
// and as the gRPC call carrying the same input, the request through the
// built-in chain of the HTTP listener and the handler of the action, the call
// through the server and the call function of the action, and both must answer
// the status the scenario states — the HTTP status, and the gRPC code it maps
// to (see TRANSPORTS.md, section 4) — the same msg, and leave the records the
// same. The scenarios the transports answer differently on purpose state what
// each answers (see TRANSPORTS.md, section 9): a change to one side alone turns
// the table red, and a new difference has to be written into the table and the
// document together.
func TestTransportsAnswerTheContractAlike(t *testing.T) {
	samples := standardFixture[*sampleRecord](sampleRoute, "")
	refusals := standardFixture[*sampleRecord](refusalRoute, "Refused")
	filterRefusals := standardFixture[*sampleRecord](filterRefusalRoute, "FilterRefused")
	hanging := standardFixture[*sampleRecord](hangingRoute, "Hanging")
	cookieBefore := standardFixture[*sampleRecord](cookieBeforeRoute, "CookieBefore")
	cookieAfter := standardFixture[*sampleRecord](cookieAfterRoute, "CookieAfter")
	counters := standardFixture[*sampleCounter](counterRoute, "Counter")
	versioned := standardFixture[*versionedSample](versionedRoute, "Versioned")
	validated := standardFixture[*validatedSample](validatedRoute, "Validated")
	dated := standardFixture[*datedSample](datedRoute, "Dated")
	scoped := standardFixture[*scopedSample](scopedRoute, "Scoped")
	scopedAcross := scoped
	scopedAcross.across = true
	public := standardFixture[*sampleRecord](publicRoute, publicPrefix)
	public.anonymous = true
	shaped := standardFixture[*shapedSample](shapedRoute, "Shaped")
	observed := standardFixture[*sampleRecord](observedRoute, "Observed")
	nested := standardFixture[*nestedSample](nestedRoute, "Nested")

	ok := contractWant{status: http.StatusOK}
	refused := contractWant{status: http.StatusConflict, msg: refusedMsg}
	notFound := contractWant{status: http.StatusNotFound, msg: notFoundMsg}
	invalid := contractWant{status: http.StatusBadRequest, msg: invalidArgumentMsg}
	nameRequired := contractWant{status: http.StatusBadRequest, msg: "name is a required field"}
	// Each transport words the refusal by what its request lacks: a body,
	// a record.
	noRecord := contractWant{status: http.StatusBadRequest, msg: "request body is required", grpc: &grpcAnswer{code: codes.InvalidArgument, msg: "record is required"}}
	// A patch naming no field to apply: HTTP writes the record back as
	// stored, gRPC refuses the mask.
	noField := &grpcAnswer{code: codes.InvalidArgument, msg: "update_mask must name at least one field a patch applies"}
	// A service calling a method of its context only an HTTP request can
	// serve: served over HTTP, refused over gRPC.
	httpOnly := &grpcAnswer{code: codes.Internal, msg: types.FailureMsg}
	// twice is the input of a batch naming one stored record in two items,
	// the second carrying change: the batch is refused before any record is
	// read, so both transports are sent the same input, and namedTwice
	// checks the refusal names the items and the record stays as stored.
	twice := func(second map[string]any) contractInput {
		id := createSample(t, "contract-named-twice").GetID()
		second["id"] = id
		return contractInput{items: []map[string]any{{"id": id, "name": "contract-named-twice-first"}, second}}
	}
	namedTwice := func(t *testing.T, in contractInput, got contractAnswer) {
		t.Helper()
		require.Equal(t, fmt.Sprintf("items[1] names the record %q, which items[0] already names", in.items[0]["id"]), got.msg)
		requireSampleName(t, stringOf(in.items[0]["id"]), "contract-named-twice")
		require.Empty(t, loadSample(t, stringOf(in.items[0]["id"])).Note)
	}

	scenarios := []contractScenario{
		{
			name: "Create stores the record for the caller", fixture: samples, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-create")}},
			want:  ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.Equal(t, in.record["name"], data["name"])
				require.Equal(t, "u-1", data["created_by"])
				requireSampleName(t, stringOf(data["id"]), stringOf(in.record["name"]))
			},
		},
		{
			name: "Create refuses a request carrying no record", fixture: samples, phase: consts.Create,
			want: noRecord,
		},
		{
			// A date sent with the client's own offset names the UTC day of
			// that instant, stored and answered as the day at midnight UTC.
			name: "Create stores a date as the UTC day", fixture: dated, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-dated"), "day": "2026-01-02T00:00:00+08:00"}},
			want:  ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.Equal(t, "2026-01-01T00:00:00Z", data["day"])
				requireDatedDay(t, stringOf(data["id"]), time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
			},
		},
		{
			// A time a model declares a date column by its type tag is a day
			// the same way.
			name: "Create stores a date declared on a time as the UTC day", fixture: dated, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-due"), "day": "2026-01-01T00:00:00Z", "due": "2026-01-02T00:00:00+08:00"}},
			want:  ok,
			check: func(t *testing.T, _ contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.Equal(t, "2026-01-01T00:00:00Z", data["due"])
				requireDatedDue(t, stringOf(data["id"]), time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
			},
		},
		{
			// A time of day travels as a duration since midnight over gRPC, so
			// what a duration says is what is compared: a fraction of a second
			// kept as written on both. A spelling HTTP alone carries, abc or
			// 17:30, is HTTP's to refuse (see
			// TestCreateReadsATimeOfDayAsWritten).
			name: "Create stores a time of day with the fraction of a second it names", fixture: dated, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-clock"), "day": "2026-01-01T00:00:00Z", "clock": "17:30:00.5"}},
			want:  ok,
			check: func(t *testing.T, _ contractInput, got contractAnswer) {
				t.Helper()
				require.Equal(t, "17:30:00.500000000", dataMap(t, got)["clock"])
			},
		},
		{
			// A duration of a whole day or more is no time of day, and nothing
			// is stored for it.
			name: "Create refuses a time of day past the end of a day", fixture: dated, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-clock-overflow"), "clock": "25:00:00"}},
			want:  contractWant{status: http.StatusBadRequest, msg: "invalid value for field 'clock'"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countDatedNamed(t, stringOf(in.record["name"])))
			},
		},
		{
			// The tenant is the caller's, stamped when the record is written:
			// a tenant the record names is ignored on both transports, a
			// generated FromProto not even reading it.
			name: "Create files the record under the caller's tenant whatever tenant it names", fixture: scoped, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-scoped"), "tenant_id": "contract-other-tenant"}},
			want:  ok,
			check: func(t *testing.T, _ contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.Equal(t, tenant.Default, data["tenant_id"])
				require.Equal(t, tenant.Default, storedScoped(t, stringOf(data["id"])).GetTenantID())
			},
		},
		{
			// A caller acting across tenants files the record under the
			// tenant the record names, which HTTP alone reads: a generated
			// FromProto never reads tenant_id, so over gRPC the record names
			// none, and a call naming no tenant of its own has none to file it
			// under (see TRANSPORTS.md, section 9).
			name: "Create by a caller acting across tenants files the record under the tenant it names over HTTP alone", fixture: scopedAcross, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-scoped-across"), "tenant_id": "contract-other-tenant"}},
			want:  contractWant{status: http.StatusOK, grpc: &grpcAnswer{code: codes.InvalidArgument, msg: invalidArgumentMsg}},
			check: func(t *testing.T, _ contractInput, got contractAnswer) {
				t.Helper()
				if got.status != http.StatusOK {
					return
				}
				require.Equal(t, "contract-other-tenant", storedScoped(t, stringOf(dataMap(t, got)["id"])).GetTenantID())
			},
		},
		{
			// The round trip of a client: a record read back carries its own
			// tenant, and the update writing it back is applied, the tenant
			// it names being the one it has.
			name: "Update carrying the record's own tenant is applied", fixture: scoped, phase: consts.Update,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createScoped(t, "contract-scoped-own").GetID(), record: map[string]any{"name": "contract-scoped-own-updated", "tenant_id": tenant.Default}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				stored := storedScoped(t, in.id)
				require.Equal(t, tenant.Default, stored.GetTenantID())
				require.Equal(t, "contract-scoped-own-updated", stored.Name)
			},
		},
		{
			// The tenant is written once, when the record is created: an
			// update naming another one cannot move the record, so HTTP
			// refuses it rather than answer a move that never happened, and
			// gRPC, whose tenant_id is output-only, never reads one (see
			// TRANSPORTS.md, section 9).
			name: "Update refuses another tenant over HTTP and gRPC never reads one", fixture: scoped, phase: consts.Update,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createScoped(t, "contract-scoped-update").GetID(), record: map[string]any{"name": "contract-scoped-updated", "tenant_id": "contract-other-tenant"}}
			},
			want: contractWant{status: http.StatusBadRequest, msg: invalidArgumentMsg, grpc: &grpcAnswer{code: codes.OK}},
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				stored := storedScoped(t, in.id)
				require.Equal(t, tenant.Default, stored.GetTenantID(), "the record stays in its tenant on both transports")
				name := "contract-scoped-updated"
				if got.status == http.StatusBadRequest {
					name = "contract-scoped-update"
				}
				require.Equal(t, name, stored.Name, "HTTP writes nothing of the update it refuses, gRPC applies the record it reads")
			},
		},
		{
			// An anonymous request names no creator or updater, so the record
			// is stored with none, whatever the request carries.
			name: "Create stores no creator or updater an anonymous request names", fixture: public, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-anonymous"), "created_by": "mallory", "updated_by": "mallory"}},
			want:  ok,
			check: func(t *testing.T, _ contractInput, got contractAnswer) {
				t.Helper()
				stored := loadSample(t, stringOf(dataMap(t, got)["id"]))
				require.Empty(t, stored.GetCreatedBy())
				require.Empty(t, stored.GetUpdatedBy())
			},
		},
		{
			name: "Create refuses a record failing validation", fixture: validated, phase: consts.Create,
			input: contractInput{record: map[string]any{}},
			want:  nameRequired,
		},
		{
			name: "Create refuses a value a nested field cannot hold and stores nothing", fixture: shaped, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-nested"), "address": map[string]any{"city": "c", "floor": 300}}},
			want:  contractWant{status: http.StatusBadRequest, msg: "invalid value for field 'address.floor'"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countShapedNamed(t, stringOf(in.record["name"])))
			},
		},
		{
			name: "Create refuses a value the field cannot hold and stores nothing", fixture: shaped, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-narrow"), "rank": 300}},
			want:  contractWant{status: http.StatusBadRequest, msg: "invalid value for field 'rank'"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countShapedNamed(t, stringOf(in.record["name"])))
			},
		},
		{
			name: "Create refuses a time the field cannot read and stores nothing", fixture: shaped, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-time"), "due_at": "yesterday"}},
			want:  contractWant{status: http.StatusBadRequest, msg: "invalid value for field 'due_at'"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countShapedNamed(t, stringOf(in.record["name"])))
			},
		},
		{
			name: "Create refuses a number the field cannot read and stores nothing", fixture: shaped, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-number"), "amount": "abc"}},
			want:  contractWant{status: http.StatusBadRequest, msg: "invalid value for field 'amount'"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countShapedNamed(t, stringOf(in.record["name"])))
			},
		},
		{
			name: "Create answers the hook's refusal and stores nothing", fixture: refusals, phase: consts.Create,
			input: contractInput{record: map[string]any{"name": uniqueName("contract-create-refused")}},
			want:  refused,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countSamplesNamed(t, stringOf(in.record["name"])))
			},
		},
		{
			name: "Get answers the record", fixture: samples, phase: consts.Get,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-get").GetID()}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.Equal(t, in.id, data["id"])
				require.Equal(t, "contract-get", data["name"])
			},
		},
		{
			name: "Get answers NotFound for an id no record carries", fixture: samples, phase: consts.Get,
			input: contractInput{id: "missing"},
			want:  notFound,
		},
		{
			name: "Get answers NotFound for an id the integer key cannot hold", fixture: counters, phase: consts.Get,
			input: contractInput{id: "first"},
			want:  notFound,
		},
		{
			name: "List answers the records the query matches in the order asked", fixture: samples, phase: consts.List,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				prefix := uniqueName("contract-list")
				createSample(t, prefix+"-a")
				createSample(t, prefix+"-b")
				return contractInput{query: controller.Query{
					Filters: []controller.Filter{{Field: "name", Op: "startswith", Values: []string{prefix}}},
					SortBy:  []string{"name desc"},
				}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.EqualValues(t, 2, data["total"])
				listed := ids(data["items"])
				require.Len(t, listed, 2)
				prefix := in.query.Filters[0].Values[0]
				require.Equal(t, prefix+"-b", loadSample(t, listed[0]).Name)
				require.Equal(t, prefix+"-a", loadSample(t, listed[1]).Name)
			},
		},
		{
			name: "List refuses a filter on a field the model lacks", fixture: samples, phase: consts.List,
			input: contractInput{query: controller.Query{Filters: []controller.Filter{{Field: "missing", Op: "eq", Values: []string{"value"}}}}},
			want:  contractWant{status: http.StatusBadRequest},
		},
		{
			name: "List refuses a bare key on a column hidden from clients", fixture: samples, phase: consts.List,
			input: contractInput{query: controller.Query{Filters: []controller.Filter{{Field: "secret", Values: []string{"value"}}}}},
			want:  contractWant{status: http.StatusBadRequest},
		},
		{
			name: "List refuses a filter given twice, every key so given listed", fixture: samples, phase: consts.List,
			input: contractInput{query: controller.Query{Filters: []controller.Filter{
				{Field: "name", Op: "ne", Values: []string{"a"}},
				{Field: "name", Op: "eq", Values: []string{"a"}},
				{Field: "name", Op: "ne", Values: []string{"b"}},
				{Field: "name", Op: "eq", Values: []string{"b"}},
			}}},
			want: contractWant{status: http.StatusBadRequest, msg: "duplicate query parameter: name[eq], name[ne]"},
		},
		{
			name: "List filters by nothing on an empty value over HTTP and refuses it over gRPC", fixture: samples, phase: consts.List,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				createSample(t, uniqueName("contract-empty-filter"))
				return contractInput{query: controller.Query{Filters: []controller.Filter{{Field: "name", Values: []string{""}}}}}
			},
			// A query string's empty parameter filters by nothing, while a
			// filter a message spells out and leaves empty is a mistake to
			// report (see TRANSPORTS.md, section 9).
			want:  contractWant{status: http.StatusOK, grpc: &grpcAnswer{code: codes.InvalidArgument, msg: `filter "name" has an empty value`}},
			check: filtersByNothingOverHTTP,
		},
		{
			name: "List filters by nothing on a filter without a value over HTTP and refuses it over gRPC", fixture: samples, phase: consts.List,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				createSample(t, uniqueName("contract-valueless-filter"))
				return contractInput{query: controller.Query{Filters: []controller.Filter{{Field: "name"}}}}
			},
			// A query string carries a parameter with a value, empty at the
			// least, so a filter without one is the empty parameter there.
			want:  contractWant{status: http.StatusOK, grpc: &grpcAnswer{code: codes.InvalidArgument, msg: `filter "name" has no value`}},
			check: filtersByNothingOverHTTP,
		},
		{
			name: "List accepts a bare key on a field that is not a column and leaves the records to the service", fixture: samples, phase: consts.List,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				prefix := uniqueName("contract-virtual")
				createSample(t, prefix+"-a")
				return contractInput{query: controller.Query{Filters: []controller.Filter{
					{Field: "name", Op: "startswith", Values: []string{prefix}},
					{Field: "virtual", Values: []string{"value"}},
				}}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				require.EqualValues(t, 1, dataMap(t, got)["total"])
			},
		},
		{
			name: "List matches a date filter by the UTC day", fixture: dated, phase: consts.List,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				name := uniqueName("contract-dated-list")
				createDated(t, name, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
				createDated(t, name, time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC))
				// Midnight in Shanghai on the 2nd is the 1st in UTC, the day
				// the first sample stores (see API_CONTRACT.md).
				return contractInput{query: controller.Query{Filters: []controller.Filter{
					{Field: "name", Op: "eq", Values: []string{name}},
					{Field: "day", Op: "eq", Values: []string{"2026-01-02T00:00:00+08:00"}},
				}}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.EqualValues(t, 1, data["total"])
				listed := ids(data["items"])
				require.Len(t, listed, 1)
				requireDatedDay(t, listed[0], time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
			},
		},
		{
			// A page without an order of its own is read in primary key order
			// on both lines, so the pages of a listing hold still.
			name: "List answers a page in primary key order when no order is given", fixture: dated, phase: consts.List,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				name := uniqueName("contract-dated-page")
				for range 3 {
					createDated(t, name, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
				}
				return contractInput{query: controller.Query{
					Filters: []controller.Filter{{Field: "name", Op: "eq", Values: []string{name}}},
					Size:    3,
				}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				listed := ids(dataMap(t, got)["items"])
				require.Len(t, listed, 3)
				require.True(t, slices.IsSorted(listed), "the page is in primary key order: %v", listed)
			},
		},
		{
			// A cursor parameter without a boundary value asks for the first
			// page of the feed, read in the feed's order from the start; a
			// feed answers no total, on the first page as on any other.
			name: "List reads the first page of a feed in the feed's order and answers no total", fixture: dated, phase: consts.List,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				name := uniqueName("contract-dated-feed")
				for range 3 {
					createDated(t, name, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
				}
				return contractInput{query: controller.Query{
					Filters:    []controller.Filter{{Field: "name", Op: "eq", Values: []string{name}}},
					Size:       2,
					CursorNext: true,
				}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.EqualValues(t, 0, data["total"], "a feed answers no total")
				listed := ids(data["items"])
				require.Len(t, listed, 2)
				require.Less(t, listed[0], listed[1], "the page comes back in the feed's order, by id")
			},
		},
		{
			name: "List refuses a date filter it cannot read", fixture: dated, phase: consts.List,
			input: contractInput{query: controller.Query{Filters: []controller.Filter{{Field: "day", Op: "eq", Values: []string{"not-a-date"}}}}},
			want:  contractWant{status: http.StatusBadRequest},
		},
		{
			name: "List refuses a time filter it cannot read on an optional time", fixture: dated, phase: consts.List,
			input: contractInput{query: controller.Query{Filters: []controller.Filter{{Field: "closed_at", Op: "gte", Values: []string{"not-a-time"}}}}},
			want:  contractWant{status: http.StatusBadRequest},
		},
		{
			name: "List refuses a comparison on a JSON column", fixture: dated, phase: consts.List,
			input: contractInput{query: controller.Query{Filters: []controller.Filter{{Field: "meta", Op: "ne", Values: []string{"value"}}}}},
			want:  contractWant{status: http.StatusBadRequest},
		},
		{
			name: "List matches a JSON column by its text", fixture: dated, phase: consts.List,
			input: contractInput{query: controller.Query{Filters: []controller.Filter{{Field: "meta", Op: "like", Values: []string{"value"}}}}},
			want:  ok,
		},
		{
			name: "List matches a numeric column by the number's text", fixture: dated, phase: consts.List,
			input: contractInput{query: controller.Query{Filters: []controller.Filter{{Field: "count", Op: "like", Values: []string{"1"}}}}},
			want:  ok,
		},
		{
			name: "List refuses a page on a model that does not page", fixture: counters, phase: consts.List,
			input: contractInput{query: controller.Query{Page: 2}},
			want:  contractWant{status: http.StatusBadRequest},
		},
		{name: "List answers the hook's refusal", fixture: refusals, phase: consts.List, want: refused},
		{name: "List answers the Filter hook's refusal", fixture: filterRefusals, phase: consts.List, want: refused},
		{
			name: "List answers a client that went away as canceled, not as a failure of the server", fixture: hanging, phase: consts.List,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				armHangCancel(cancel)
				return contractInput{ctx: ctx}
			},
			want: contractWant{status: types.StatusClientClosedRequest, msg: "context canceled"},
		},
		{
			name: "List reads under the parent its route names", fixture: parentFixture, phase: consts.List,
			input: contractInput{params: map[string]string{"id": "contract-parent"}},
			want:  ok,
		},
		{
			// A path carries an empty parameter as an empty segment,
			// controller-parents//samples, the way a message carries one.
			name: "List refuses an empty route parameter", fixture: parentFixture, phase: consts.List,
			input: contractInput{params: map[string]string{"id": ""}},
			want:  contractWant{status: http.StatusBadRequest, msg: `route parameter "id" is required`},
		},
		{
			name: "Update replaces the record for the caller", fixture: samples, phase: consts.Update,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-update").GetID(), record: map[string]any{"name": "contract-updated"}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.Equal(t, "contract-updated", data["name"])
				require.Equal(t, "u-1", data["updated_by"])
				requireSampleName(t, in.id, "contract-updated")
			},
		},
		{
			// An anonymous update names no updater: the record is stored with
			// none, neither the one the request carries nor the one stored
			// before, and its creator stays as stored.
			name: "Update stores no updater an anonymous request names", fixture: public, phase: consts.Update,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSampleBy(t, "contract-anonymous-update", "u-0").GetID(), record: map[string]any{"name": "contract-anonymous-updated", "created_by": "mallory", "updated_by": "mallory"}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				stored := loadSample(t, in.id)
				require.Equal(t, "contract-anonymous-updated", stored.Name)
				require.Equal(t, "u-0", stored.GetCreatedBy())
				require.Empty(t, stored.GetUpdatedBy())
			},
		},
		{
			name: "Update writes the record the request names, whatever id the record carries", fixture: samples, phase: consts.Update,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				other := createSample(t, "contract-update-other")
				return contractInput{id: createSample(t, "contract-update-target").GetID(), record: map[string]any{"id": other.GetID(), "name": "contract-update-moved"}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				require.Equal(t, in.id, dataMap(t, got)["id"])
				requireSampleName(t, in.id, "contract-update-moved")
				requireSampleName(t, stringOf(in.record["id"]), "contract-update-other")
			},
		},
		{
			name: "Update answers NotFound for an id no record carries", fixture: samples, phase: consts.Update,
			input: contractInput{id: "missing", record: map[string]any{"name": "contract-updated"}},
			want:  notFound,
		},
		{
			name: "Update refuses a request carrying no record", fixture: samples, phase: consts.Update,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-update-kept").GetID()}
			},
			want: noRecord,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, in.id, "contract-update-kept")
			},
		},
		{
			name: "Update refuses a record failing validation", fixture: validated, phase: consts.Update,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createValidated(t, "contract-update-validated").GetID(), record: map[string]any{"name": ""}}
			},
			want: nameRequired,
		},
		{
			name: "Update answers the hook's refusal and keeps the record", fixture: refusals, phase: consts.Update,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-update-refused").GetID(), record: map[string]any{"name": "contract-updated"}}
			},
			want: refused,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, in.id, "contract-update-refused")
			},
		},
		{
			name: "Patch applies the fields the request names and leaves the others as stored", fixture: samples, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				record := &sampleRecord{Name: "contract-patch", Note: "kept"}
				require.NoError(t, database.Database[*sampleRecord](context.Background()).Create(record))
				return contractInput{id: record.GetID(), record: map[string]any{"name": "contract-patched"}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				require.Equal(t, "contract-patched", dataMap(t, got)["name"])
				stored := loadSample(t, in.id)
				require.Equal(t, "contract-patched", stored.Name)
				require.Equal(t, "kept", stored.Note)
				require.Equal(t, "u-1", stored.GetUpdatedBy())
			},
		},
		{
			// An anonymous patch names no updater, as an anonymous update
			// does: the record is stored with none rather than the one stored
			// before, and its creator stays as stored.
			name: "Patch stores no updater an anonymous request names", fixture: public, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSampleBy(t, "contract-anonymous-patch", "u-0").GetID(), record: map[string]any{"name": "contract-anonymous-patched"}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				stored := loadSample(t, in.id)
				require.Equal(t, "contract-anonymous-patched", stored.Name)
				require.Equal(t, "u-0", stored.GetCreatedBy())
				require.Empty(t, stored.GetUpdatedBy())
			},
		},
		{
			name: "Patch naming no field", fixture: samples, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-patch-nothing").GetID(), record: map[string]any{}}
			},
			want: contractWant{status: http.StatusOK, grpc: noField},
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				stored := loadSample(t, in.id)
				require.Equal(t, "contract-patch-nothing", stored.Name)
				if got.status == http.StatusOK {
					require.Equal(t, "u-1", stored.GetUpdatedBy(), "the record is written back as stored")
				} else {
					require.Empty(t, stored.GetUpdatedBy(), "nothing is written")
				}
			},
		},
		{
			// HTTP passes over a key naming no field, the way encoding/json
			// does; gRPC refuses the path.
			name: "Patch naming a field the model lacks", fixture: samples, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-patch-unknown").GetID(), record: map[string]any{"nope": 1}}
			},
			want: contractWant{status: http.StatusOK, grpc: &grpcAnswer{code: codes.InvalidArgument, msg: `update_mask names "nope", which is no field a patch applies`}},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, in.id, "contract-patch-unknown")
			},
		},
		{
			// HTTP binds a key differing only in case the way encoding/json
			// does; gRPC refuses the path, which must be exact.
			name: "Patch applies a field named by a key differing only in case", fixture: samples, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-patch-cased").GetID(), record: map[string]any{"Name": "contract-patched-cased"}}
			},
			want: contractWant{status: http.StatusOK, grpc: &grpcAnswer{code: codes.InvalidArgument, msg: `update_mask names "Name", which is no field a patch applies`}},
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				if got.status == http.StatusOK {
					requireSampleName(t, in.id, "contract-patched-cased")
				} else {
					requireSampleName(t, in.id, "contract-patch-cased")
				}
			},
		},
		{
			name: "Patch leaves a field the framework manages as stored and applies the field beside it", fixture: samples, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-patch-managed").GetID(), record: map[string]any{"created_at": "2001-02-03T04:05:06Z", "name": "contract-patched-beside"}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				stored := loadSample(t, in.id)
				require.Equal(t, "contract-patched-beside", stored.Name)
				require.NotEqual(t, 2001, stored.GetCreatedAt().Year(), "created_at stays as stored")
			},
		},
		{
			name: "Patch naming only a field the framework manages", fixture: samples, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-patch-managed-alone").GetID(), record: map[string]any{"created_at": "2001-02-03T04:05:06Z"}}
			},
			want: contractWant{status: http.StatusOK, grpc: noField},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				stored := loadSample(t, in.id)
				require.Equal(t, "contract-patch-managed-alone", stored.Name)
				require.NotEqual(t, 2001, stored.GetCreatedAt().Year(), "created_at stays as stored")
			},
		},
		{
			name: "Patch refuses a field failing validation", fixture: validated, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createValidated(t, "contract-patch-validated").GetID(), record: map[string]any{"name": ""}}
			},
			want: nameRequired,
		},
		{
			name: "Patch leaves a required field the request does not name alone", fixture: validated, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createValidated(t, "contract-patch-unnamed").GetID(), record: map[string]any{"note": "only the note"}}
			},
			want: ok,
		},
		{
			name: "Patch refuses a request carrying no record", fixture: samples, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-patch-no-record").GetID()}
			},
			want: noRecord,
		},
		{
			// A request carrying no record carries no version either: the
			// record it lacks is what it is refused for.
			name: "Patch refuses a request carrying no record on a versioned model", fixture: versioned, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createVersioned(t, "contract-patch-versioned-no-record").GetID()}
			},
			want: noRecord,
		},
		{
			name: "Patch refuses a versioned record without its version", fixture: versioned, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createVersioned(t, "contract-patch-unversioned").GetID(), record: map[string]any{"name": "contract-patched"}}
			},
			want: invalid,
		},
		{
			name: "Patch refuses a stale version", fixture: versioned, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createVersioned(t, "contract-patch-stale").GetID(), record: map[string]any{"name": "contract-patched", "version": 5}}
			},
			want: contractWant{status: http.StatusConflict, msg: staleObjectMsg, code: codes.Aborted},
		},
		{
			// HTTP finds the version under a key differing only in case the
			// way encoding/json binds it; gRPC refuses the path.
			name: "Patch accepts the version under a key differing only in case", fixture: versioned, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				record := createVersioned(t, "contract-patch-cased-version")
				return contractInput{id: record.GetID(), record: map[string]any{"name": "contract-patched", "Version": record.Version}}
			},
			want: contractWant{status: http.StatusOK, grpc: &grpcAnswer{code: codes.InvalidArgument, msg: `update_mask names "Version", which is no field a patch applies`}},
		},
		{
			name: "Patch answers NotFound for an id no record carries", fixture: samples, phase: consts.Patch,
			input: contractInput{id: "missing", record: map[string]any{"name": "contract-patched"}},
			want:  notFound,
		},
		{
			name: "Patch answers NotFound for an id the integer key cannot hold", fixture: counters, phase: consts.Patch,
			input: contractInput{id: "first", record: map[string]any{"name": "contract-patched"}},
			want:  notFound,
		},
		{
			name: "Patch answers the hook's refusal and keeps the record", fixture: refusals, phase: consts.Patch,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-patch-refused").GetID(), record: map[string]any{"name": "contract-patched"}}
			},
			want: refused,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, in.id, "contract-patch-refused")
			},
		},
		{
			name: "Delete removes the record", fixture: samples, phase: consts.Delete,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-delete").GetID()}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countSamplesWithID(t, in.id))
			},
		},
		{
			name: "Delete answers the hook's refusal and keeps the record", fixture: refusals, phase: consts.Delete,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{id: createSample(t, "contract-delete-refused").GetID()}
			},
			want: refused,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, in.id, "contract-delete-refused")
			},
		},
		{
			name: "Delete answers NotFound for an id no record carries", fixture: samples, phase: consts.Delete,
			input: contractInput{id: "missing"},
			want:  notFound,
		},
		{
			name: "Delete answers NotFound for an id the integer key cannot hold", fixture: counters, phase: consts.Delete,
			input: contractInput{id: "first"},
			want:  notFound,
		},
		{
			name: "CreateMany stores the items", fixture: samples, phase: consts.CreateMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				prefix := uniqueName("contract-create-many")
				return contractInput{items: []map[string]any{{"name": prefix + "-a"}, {"name": prefix + "-b"}}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				created := ids(dataMap(t, got)["items"])
				require.Len(t, created, 2)
				requireSampleName(t, created[0], stringOf(in.items[0]["name"]))
				requireSampleName(t, created[1], stringOf(in.items[1]["name"]))
			},
		},
		{
			name: "CreateMany stores no creator or updater an anonymous request names", fixture: public, phase: consts.CreateMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				prefix := uniqueName("contract-anonymous-many")
				return contractInput{items: []map[string]any{{"name": prefix + "-a", "created_by": "mallory", "updated_by": "mallory"}, {"name": prefix + "-b"}}}
			},
			want: ok,
			check: func(t *testing.T, _ contractInput, got contractAnswer) {
				t.Helper()
				created := ids(dataMap(t, got)["items"])
				require.Len(t, created, 2)
				for _, id := range created {
					stored := loadSample(t, id)
					require.Empty(t, stored.GetCreatedBy())
					require.Empty(t, stored.GetUpdatedBy())
				}
			},
		},
		{
			name: "CreateMany stores the dates of the items as UTC days", fixture: dated, phase: consts.CreateMany,
			input: contractInput{items: []map[string]any{{"name": uniqueName("contract-dated-many"), "day": "2026-01-02T00:00:00+08:00"}}},
			want:  ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				created := ids(dataMap(t, got)["items"])
				require.Len(t, created, 1)
				requireDatedDay(t, created[0], time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
			},
		},
		{
			name: "CreateMany refuses an item failing validation and stores nothing", fixture: validated, phase: consts.CreateMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"name": uniqueName("contract-create-many-valid")}, {}}}
			},
			want: contractWant{status: http.StatusBadRequest, msg: "items[1].name is a required field"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countValidatedNamed(t, stringOf(in.items[0]["name"])))
			},
		},
		{
			name: "CreateMany answers the hook's refusal and stores nothing", fixture: refusals, phase: consts.CreateMany,
			input: contractInput{items: []map[string]any{{"name": uniqueName("contract-create-many-refused")}}},
			want:  refused,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countSamplesNamed(t, stringOf(in.items[0]["name"])))
			},
		},
		{
			name: "UpdateMany replaces the items for the caller", fixture: samples, phase: consts.UpdateMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				a, b := createSample(t, "contract-update-many-a"), createSample(t, "contract-update-many-b")
				return contractInput{items: []map[string]any{{"id": a.GetID(), "name": "contract-update-many-a2", "updated_by": "mallory"}, {"id": b.GetID(), "name": "contract-update-many-b2"}}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				answered := itemMaps(t, dataMap(t, got)["items"])
				require.Equal(t, []string{stringOf(in.items[0]["id"]), stringOf(in.items[1]["id"])}, ids(dataMap(t, got)["items"]))
				requireSampleName(t, stringOf(in.items[0]["id"]), "contract-update-many-a2")
				requireSampleName(t, stringOf(in.items[1]["id"]), "contract-update-many-b2")
				for i, item := range answered {
					stored := loadSample(t, stringOf(in.items[i]["id"]))
					require.Equal(t, "u-1", stored.GetUpdatedBy(), "the caller is who updated the record, whatever the item carried")
					require.Equal(t, "u-1", item["updated_by"])
					requireSameInstant(t, stored.GetCreatedAt(), item["created_at"], "the answer carries the creation audit as stored")
				}
			},
		},
		{
			name: "UpdateMany stores no updater an anonymous request names", fixture: public, phase: consts.UpdateMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				stored := createSampleBy(t, "contract-anonymous-update-many", "u-0")
				return contractInput{items: []map[string]any{{"id": stored.GetID(), "name": "contract-anonymous-update-many2", "created_by": "mallory", "updated_by": "mallory"}}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				stored := loadSample(t, stringOf(in.items[0]["id"]))
				require.Equal(t, "contract-anonymous-update-many2", stored.Name)
				require.Equal(t, "u-0", stored.GetCreatedBy())
				require.Empty(t, stored.GetUpdatedBy())
			},
		},
		{
			name: "UpdateMany refuses an item naming no record and writes nothing", fixture: samples, phase: consts.UpdateMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createSample(t, "contract-update-many-kept").GetID(), "name": "contract-update-many-renamed"}, {"name": "contract-update-many-other"}}}
			},
			want: contractWant{status: http.StatusBadRequest, msg: "items[1] names no id"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, stringOf(in.items[0]["id"]), "contract-update-many-kept")
			},
		},
		{
			name: "UpdateMany refuses a batch naming a record twice and writes nothing", fixture: samples, phase: consts.UpdateMany,
			input: twice(map[string]any{"name": "contract-named-twice-second"}),
			want:  contractWant{status: http.StatusBadRequest},
			check: namedTwice,
		},
		{
			name: "UpdateMany answers NotFound when an item names no stored record and writes nothing", fixture: samples, phase: consts.UpdateMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createSample(t, "contract-update-many-kept").GetID(), "name": "contract-update-many-renamed"}, {"id": "missing", "name": "contract-update-many-other"}}}
			},
			want: notFound,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, stringOf(in.items[0]["id"]), "contract-update-many-kept")
			},
		},
		{
			name: "UpdateMany answers the hook's refusal and writes nothing", fixture: refusals, phase: consts.UpdateMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createSample(t, "contract-update-many-refused").GetID(), "name": "contract-update-many-renamed"}}}
			},
			want: refused,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, stringOf(in.items[0]["id"]), "contract-update-many-refused")
			},
		},
		{
			name: "PatchMany applies the fields each item names", fixture: samples, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				a, b := createSample(t, "contract-patch-many-a"), createSample(t, "contract-patch-many-b")
				return contractInput{items: []map[string]any{{"id": a.GetID(), "name": "contract-patch-many-a2"}, {"id": b.GetID(), "note": "masked"}}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				require.Equal(t, []string{stringOf(in.items[0]["id"]), stringOf(in.items[1]["id"])}, ids(dataMap(t, got)["items"]))
				first := loadSample(t, stringOf(in.items[0]["id"]))
				require.Equal(t, "contract-patch-many-a2", first.Name)
				require.Equal(t, "u-1", first.GetUpdatedBy(), "the caller is who patched the record")
				second := loadSample(t, stringOf(in.items[1]["id"]))
				require.Equal(t, "contract-patch-many-b", second.Name, "a field the item does not name stays as stored")
				require.Equal(t, "masked", second.Note)
				require.Equal(t, "u-1", second.GetUpdatedBy(), "the caller is who patched the record")
			},
		},
		{
			name: "PatchMany stores no updater an anonymous request names", fixture: public, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				stored := createSampleBy(t, "contract-anonymous-patch-many", "u-0")
				return contractInput{items: []map[string]any{{"id": stored.GetID(), "name": "contract-anonymous-patch-many2"}}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				stored := loadSample(t, stringOf(in.items[0]["id"]))
				require.Equal(t, "contract-anonymous-patch-many2", stored.Name)
				require.Equal(t, "u-0", stored.GetCreatedBy())
				require.Empty(t, stored.GetUpdatedBy())
			},
		},
		{
			name: "PatchMany refuses an item naming no record and writes nothing", fixture: samples, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createSample(t, "contract-patch-many-kept").GetID(), "name": "contract-patch-many-renamed"}, {"name": "contract-patch-many-other"}}}
			},
			want: contractWant{status: http.StatusBadRequest, msg: "items[1] names no id"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, stringOf(in.items[0]["id"]), "contract-patch-many-kept")
			},
		},
		{
			name: "PatchMany refuses a batch naming a record twice and writes nothing", fixture: samples, phase: consts.PatchMany,
			input: twice(map[string]any{"note": "second"}),
			want:  contractWant{status: http.StatusBadRequest},
			check: namedTwice,
		},
		{
			name: "PatchMany answers NotFound when an item names no stored record and writes nothing", fixture: samples, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createSample(t, "contract-patch-many-kept").GetID(), "name": "contract-patch-many-renamed"}, {"id": "missing", "name": "contract-patch-many-other"}}}
			},
			want: notFound,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, stringOf(in.items[0]["id"]), "contract-patch-many-kept")
			},
		},
		{
			name: "PatchMany refuses a versioned item without its version", fixture: versioned, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createVersioned(t, "contract-patch-many-unversioned").GetID(), "name": "contract-patched"}}}
			},
			want: invalid,
		},
		{
			// Every item's version is checked before any item is validated, so
			// a batch defective in several items is refused for the same one.
			name: "PatchMany refuses a missing version before an invalid field", fixture: versioned, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				invalidCode := createVersioned(t, "contract-patch-many-invalid-code").GetID()
				unversioned := createVersioned(t, "contract-patch-many-second-unversioned").GetID()
				return contractInput{items: []map[string]any{{"id": invalidCode, "version": 1, "code": "too-long"}, {"id": unversioned, "name": "contract-patched"}}}
			},
			want: invalid,
		},
		{
			name: "PatchMany refuses an item failing validation", fixture: validated, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createValidated(t, "contract-patch-many-validated").GetID(), "name": ""}}}
			},
			want: contractWant{status: http.StatusBadRequest, msg: "items[0].name is a required field"},
		},
		{
			name: "PatchMany refuses an item carrying a value its field cannot hold and writes nothing", fixture: shaped, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				a, b := createShaped(t, "contract-patch-many-narrow-a"), createShaped(t, "contract-patch-many-narrow-b")
				return contractInput{items: []map[string]any{{"id": a.GetID(), "rank": 1}, {"id": b.GetID(), "rank": 300}}}
			},
			want: contractWant{status: http.StatusBadRequest, msg: "invalid value for field 'items[1].rank'"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, loadShaped(t, stringOf(in.items[0]["id"])).Rank, "the item in range is not written either")
			},
		},
		{
			name: "PatchMany refuses an item whose nested field cannot hold a value and writes nothing", fixture: shaped, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				a, b := createShaped(t, "contract-patch-many-nested-a"), createShaped(t, "contract-patch-many-nested-b")
				return contractInput{items: []map[string]any{{"id": a.GetID(), "address": map[string]any{"city": "a", "floor": 1}}, {"id": b.GetID(), "address": map[string]any{"city": "b", "floor": 300}}}}
			},
			want: contractWant{status: http.StatusBadRequest, msg: "invalid value for field 'items[1].address.floor'"},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Equal(t, "old", loadShaped(t, stringOf(in.items[0]["id"])).Address.City, "the item in range is not written either")
			},
		},
		{
			name: "PatchMany answers the hook's refusal and writes nothing", fixture: refusals, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createSample(t, "contract-patch-many-refused").GetID(), "name": "contract-patch-many-renamed"}}}
			},
			want: refused,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, stringOf(in.items[0]["id"]), "contract-patch-many-refused")
			},
		},
		{
			name: "DeleteMany removes the records", fixture: samples, phase: consts.DeleteMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{ids: []string{createSample(t, "contract-delete-many-a").GetID(), createSample(t, "contract-delete-many-b").GetID()}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Zero(t, countSamplesWithID(t, in.ids[0]))
				require.Zero(t, countSamplesWithID(t, in.ids[1]))
			},
		},
		{
			name: "DeleteMany refuses an empty id and removes nothing", fixture: samples, phase: consts.DeleteMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{ids: []string{createSample(t, "contract-delete-many-kept").GetID(), ""}}
			},
			want: invalid,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, in.ids[0], "contract-delete-many-kept")
			},
		},
		{
			name: "DeleteMany answers NotFound when an id names no stored record and removes nothing", fixture: samples, phase: consts.DeleteMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{ids: []string{createSample(t, "contract-delete-many-missing-kept").GetID(), "missing"}}
			},
			want: notFound,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, in.ids[0], "contract-delete-many-missing-kept")
			},
		},
		{
			name: "DeleteMany answers NotFound for an id the integer key cannot hold", fixture: counters, phase: consts.DeleteMany,
			input: contractInput{ids: []string{"first"}},
			want:  notFound,
		},
		{
			name: "DeleteMany answers the hook's refusal and removes nothing", fixture: refusals, phase: consts.DeleteMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{ids: []string{createSample(t, "contract-delete-many-refused").GetID()}}
			},
			want: refused,
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				requireSampleName(t, in.ids[0], "contract-delete-many-refused")
			},
		},
		{
			name: "A custom action answers the service's result for the caller", fixture: actionFixture, phase: consts.Create,
			input: contractInput{payload: map[string]any{"note": "hello"}},
			want:  ok,
			check: func(t *testing.T, _ contractInput, got contractAnswer) {
				t.Helper()
				data := dataMap(t, got)
				require.Equal(t, "hello", data["Note"])
				require.Equal(t, "alice", data["Username"])
			},
		},
		{
			name: "A custom action refuses a payload failing validation", fixture: actionFixture, phase: consts.Create,
			input: contractInput{payload: map[string]any{}},
			want:  contractWant{status: http.StatusBadRequest, msg: "note is a required field"},
		},
		{
			name: "A custom action answers the service's error with its status", fixture: actionFixture, phase: consts.Create,
			input: contractInput{payload: map[string]any{"note": actionRefuse}},
			want:  contractWant{status: http.StatusForbidden, msg: "not yours"},
		},
		{
			name: "A custom action answers any other error as the server's own failure", fixture: actionFixture, phase: consts.Create,
			input: contractInput{payload: map[string]any{"note": actionBreak}},
			want:  contractWant{status: http.StatusInternalServerError, msg: types.FailureMsg},
		},
		{
			name: "A custom action answers a client that went away as canceled, not as a failure of the server", fixture: actionFixture, phase: consts.Create,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				go func() {
					select {
					case <-actionEntered:
						cancel()
					case <-ctx.Done():
					}
				}()
				return contractInput{ctx: ctx, payload: map[string]any{"note": actionHang}}
			},
			want: contractWant{status: types.StatusClientClosedRequest, msg: "context canceled"},
		},
		{
			name: "A custom List reads the query", fixture: actionFixture, phase: consts.List,
			input: contractInput{query: controller.Query{Page: 2}},
			want:  ok,
			check: func(t *testing.T, _ contractInput, got contractAnswer) {
				t.Helper()
				query, _ := dataMap(t, got)["Query"].(map[string]any)
				require.Equal(t, []any{"2"}, query["_page"])
			},
		},
		{
			name: "A hook writing a cookie before the write", fixture: cookieBefore, phase: consts.Create,
			prepare: func(*testing.T) contractInput {
				return contractInput{record: map[string]any{"name": uniqueName("contract-cookie-before")}}
			},
			want: contractWant{status: http.StatusOK, grpc: httpOnly},
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				written := 0
				if got.status == http.StatusOK {
					written = 1
				}
				require.Equal(t, written, countSamplesNamed(t, stringOf(in.record["name"])), "refused before the write over gRPC")
			},
		},
		{
			name: "A hook writing a cookie after the write", fixture: cookieAfter, phase: consts.Create,
			prepare: func(*testing.T) contractInput {
				return contractInput{record: map[string]any{"name": uniqueName("contract-cookie-after")}}
			},
			want: contractWant{status: http.StatusOK, grpc: httpOnly},
			check: func(t *testing.T, in contractInput, _ contractAnswer) {
				t.Helper()
				require.Equal(t, 1, countSamplesNamed(t, stringOf(in.record["name"])), "written before the hook refused over gRPC")
			},
		},
		{
			name: "Create writes the record alone and hands back no child the body carried", fixture: nested, phase: consts.Create,
			prepare: func(*testing.T) contractInput {
				name := uniqueName("contract-nested")
				return contractInput{record: map[string]any{"name": name, "children": []map[string]any{{"name": name + "-child"}}}}
			},
			want: ok,
			check: func(t *testing.T, in contractInput, got contractAnswer) {
				t.Helper()
				name := stringOf(in.record["name"])
				require.Nil(t, dataMap(t, got)["children"], "the body's children are not handed back as written")
				require.Equal(t, 1, countNestedNamed(t, name))
				require.Equal(t, 0, countNestedNamed(t, name+"-child"), "no child row is written")
			},
		},
		{
			name: "Create hands the hooks the client address a trusted peer forwards", fixture: observed, phase: consts.Create,
			prepare: func(*testing.T) contractInput {
				return contractInput{record: map[string]any{"name": uniqueName("contract-forwarded")}, forwarded: http.Header{"X-Forwarded-For": {"203.0.113.9"}}}
			},
			want:  ok,
			check: observedClientIP("203.0.113.9"),
		},
		{
			name: "Create hands the hooks the client address a trusted peer names in X-Real-IP alone", fixture: observed, phase: consts.Create,
			prepare: func(*testing.T) contractInput {
				return contractInput{record: map[string]any{"name": uniqueName("contract-real-ip")}, forwarded: http.Header{"X-Real-IP": {"203.0.113.9"}}}
			},
			want:  ok,
			check: observedClientIP("203.0.113.9"),
		},
		{
			name: "Create hands the hooks the first of a chain of trusted proxies", fixture: observed, phase: consts.Create,
			prepare: func(*testing.T) contractInput {
				return contractInput{record: map[string]any{"name": uniqueName("contract-proxy-chain")}, forwarded: http.Header{"X-Forwarded-For": {"::1, 127.0.0.1"}}}
			},
			want:  ok,
			check: observedClientIP("::1"),
		},
		{
			name: "Create hands the hooks the client a trusted proxy names on an X-Forwarded-For line of its own", fixture: observed, phase: consts.Create,
			prepare: func(*testing.T) contractInput {
				return contractInput{record: map[string]any{"name": uniqueName("contract-forwarded-lines")}, forwarded: http.Header{"X-Forwarded-For": {"198.51.100.7", "203.0.113.9"}}}
			},
			want:  ok,
			check: observedClientIP("203.0.113.9"),
		},
		{
			name: "Create hands the hooks X-Real-IP when X-Forwarded-For is malformed", fixture: observed, phase: consts.Create,
			prepare: func(*testing.T) contractInput {
				return contractInput{record: map[string]any{"name": uniqueName("contract-malformed")}, forwarded: http.Header{"X-Forwarded-For": {"junk"}, "X-Real-IP": {"203.0.113.9"}}}
			},
			want:  ok,
			check: observedClientIP("203.0.113.9"),
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			answered := make(map[string]contractAnswer, len(contractTransports))
			for _, tr := range contractTransports {
				t.Run(tr.name, func(t *testing.T) {
					in := s.input
					if s.prepare != nil {
						in = s.prepare(t)
					}
					got := tr.do(t, s.fixture, s.phase, in)
					tr.expect(t, got, s.want)
					if s.check != nil {
						s.check(t, in, got)
					}
					answered[tr.name] = got
				})
			}
			if len(answered) == len(contractTransports) && s.want.grpc == nil && s.want.status != http.StatusOK {
				require.Equal(t, answered[contractTransports[0].name].msg, answered[contractTransports[1].name].msg, "both transports refuse with the same msg")
			}
		})
	}
}

// contractScenario is one scenario of the contract table: the fixture and
// the action its input is sent to, the input, prepared afresh for each
// transport when prepare is set, what both transports must answer, and what
// must hold afterwards of the answer and the records, checked on each
// transport's own run.
type contractScenario struct {
	name    string
	fixture contractFixture
	phase   consts.Phase
	input   contractInput
	prepare func(t *testing.T) contractInput
	want    contractWant
	check   func(t *testing.T, in contractInput, got contractAnswer)
}

// contractInput is what a scenario sends, on either transport: the id of
// the record the action names; the record of a Create, an Update or a
// Patch, the body over HTTP, the record of the message over gRPC, its keys
// the paths of the mask of a Patch; the items of a batch, over gRPC an item
// of a batch patch carrying its id, the record and, as its mask, its keys
// but the id; the ids of a batch delete; the query of a List, the query
// string over HTTP; and the payload of a custom action. A nil record is a
// request carrying none: an absent body, a message without a record.
// params are the parameters of a route carrying some of its own, filled
// into the path over HTTP, an empty one leaving its segment empty the way a
// path carries one, and sent under params over gRPC. forwarded are the
// headers a proxy in front adds, X-Forwarded-For and X-Real-IP, each value
// sent as a header line of its own over HTTP and as a metadata value of its
// own over gRPC. ctx is the context the
// request is sent in, set by a scenario that ends the request on the
// client's behalf; nil sends it in a context of the transport's own.
type contractInput struct {
	ctx       context.Context
	params    map[string]string
	id        string
	record    map[string]any
	items     []map[string]any
	ids       []string
	query     controller.Query
	payload   map[string]any
	forwarded http.Header
}

// contractAnswer is what a transport answered: the HTTP status and the msg
// of the envelope, or the gRPC code and message; and the data answered, the
// envelope's data or the response message.
type contractAnswer struct {
	status int
	code   codes.Code
	msg    string
	data   any
}

// contractWant is what a scenario must be answered: the HTTP status, the
// gRPC code it maps to unless code says otherwise, and the msg of both,
// left empty when the scenario only requires the two to be the same. A
// scenario the transports answer differently on purpose states in grpc
// what gRPC answers instead.
type contractWant struct {
	status int
	code   codes.Code
	msg    string
	grpc   *grpcAnswer
}

// grpcAnswer is what gRPC answers a scenario the transports differ on.
type grpcAnswer struct {
	code codes.Code
	msg  string
}

// contractFixture is a model on a route as both transports serve it: the
// route of its services, which is the path of its HTTP handlers as well,
// the handler of each action and the name of each action's rpc on the
// sample service. An anonymous fixture is sent with no caller: an HTTP
// request no identity names, a gRPC call presenting no credential to a
// public rpc. An across fixture is sent by a caller acting across tenants
// (see tenant.Across): an HTTP request in a context so scoped, a gRPC call
// presenting the credential the sample service scopes so.
type contractFixture struct {
	route     string
	handler   func(phase consts.Phase) gin.HandlerFunc
	rpc       func(phase consts.Phase) string
	anonymous bool
	across    bool
}

// standardFixture is the fixture of the standard actions of M on route,
// whose rpcs are named prefix followed by the action's name (see
// standardRPCs).
func standardFixture[M types.Model](route, prefix string) contractFixture {
	cfg := configFor[M](route)
	return contractFixture{
		route: route,
		handler: func(phase consts.Phase) gin.HandlerFunc {
			switch phase {
			case consts.Create:
				return controller.CreateHandler[M, M, M](cfg)
			case consts.Get:
				return controller.GetHandler[M, M, M](cfg)
			case consts.List:
				return controller.ListHandler[M, M, M](cfg)
			case consts.Update:
				return controller.UpdateHandler[M, M, M](cfg)
			case consts.Patch:
				return controller.PatchHandler[M, M, M](cfg)
			case consts.Delete:
				return controller.DeleteHandler[M, M, M](cfg)
			case consts.CreateMany:
				return controller.CreateManyHandler[M, M, M](cfg)
			case consts.UpdateMany:
				return controller.UpdateManyHandler[M, M, M](cfg)
			case consts.PatchMany:
				return controller.PatchManyHandler[M, M, M](cfg)
			case consts.DeleteMany:
				return controller.DeleteManyHandler[M, M, M](cfg)
			}
			panic("no handler of " + phase.Name())
		},
		rpc: func(phase consts.Phase) string { return prefix + phase.Name() },
	}
}

// actionFixture is the fixture of the sample's custom action, served by
// the action service's Create and List with a payload and result of their
// own.
var actionFixture = contractFixture{
	route: actionRoute,
	handler: func(phase consts.Phase) gin.HandlerFunc {
		cfg := configFor[*sampleRecord](actionRoute)
		if phase == consts.List {
			return controller.ListHandler[*sampleRecord, *sampleActionReq, *sampleActionRsp](cfg)
		}
		return controller.CreateHandler[*sampleRecord, *sampleActionReq, *sampleActionRsp](cfg)
	},
	rpc: func(phase consts.Phase) string {
		if phase == consts.List {
			return "ActionList"
		}
		return "Action"
	},
}

// parentFixture is the fixture of the List of the samples nested under a
// parent (see parentRoute), a route carrying a parameter of its own, which a
// scenario names in its params.
var parentFixture = contractFixture{
	route: parentRoute,
	handler: func(consts.Phase) gin.HandlerFunc {
		return controller.ListHandler[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](parentRoute))
	},
	rpc: func(consts.Phase) string { return "ParentList" },
}

// contractTransport sends a scenario's input over one transport, returns
// what it answered, and holds the answer to what the scenario wants of it.
type contractTransport struct {
	name   string
	do     func(t *testing.T, f contractFixture, phase consts.Phase, in contractInput) contractAnswer
	expect func(t *testing.T, got contractAnswer, want contractWant)
}

var contractTransports = []contractTransport{
	{name: "HTTP", do: overHTTP, expect: requireHTTPAnswer},
	{name: "gRPC", do: overGRPC, expect: requireGRPCAnswer},
}

// overHTTP sends in as the HTTP request of the action, through the built-in
// chain of the HTTP listener (see middleware.Builtin), as alice unless the
// fixture is anonymous, acting across tenants when the fixture is across, and
// decodes the envelope answered: the method and path of the action, the id in
// the path, the query rendered as the query string, the record, the items,
// the ids or the payload as the JSON body, and the forwarding headers as
// headers of the request.
func overHTTP(t *testing.T, f contractFixture, phase consts.Phase, in contractInput) contractAnswer {
	t.Helper()
	pattern, target := "/"+f.route, "/"+routePath(f.route, in.params)
	switch phase {
	case consts.Get, consts.Update, consts.Patch, consts.Delete:
		pattern += "/:id"
		target += "/" + in.id
	case consts.CreateMany, consts.UpdateMany, consts.PatchMany, consts.DeleteMany:
		pattern += "/batch"
		target += "/batch"
	}
	if q := queryString(in.query); q != "" {
		target += "?" + q
	}
	var body string
	switch {
	case phase == consts.DeleteMany:
		body = encodeJSON(map[string]any{"ids": in.ids})
	case in.items != nil:
		body = encodeJSON(map[string]any{"items": in.items})
	case in.payload != nil:
		body = encodeJSON(in.payload)
	case in.record != nil:
		body = encodeJSON(in.record)
	}
	ctx := in.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if f.across {
		ctx = tenant.Across(ctx)
	}
	username := "alice"
	if f.anonymous {
		username = ""
	}
	rsp := serveAs(ctx, t, phase.HTTPMethod(), pattern, f.handler(phase), target, body, username, in.forwarded, middleware.Builtin()...)
	answer := contractAnswer{status: rsp.Code}
	var envelope struct {
		Msg  string `json:"msg"`
		Data any    `json:"data"`
	}
	if err := json.Unmarshal(rsp.Body.Bytes(), &envelope); err == nil {
		answer.msg, answer.data = envelope.Msg, envelope.Data
	}
	return answer
}

// overGRPC sends in as the rpc of the action on the sample service, as
// alice unless the fixture is anonymous, acting across tenants when the
// fixture is across, and returns the status answered, or the response
// message: the route parameters, the id, the record with the mask of a
// Patch, the items, each with its id, its record and its mask in a
// PatchMany, the ids, the query and the payload, as the rpcs of the sample
// service read them (see standardRPCs), and the forwarding headers as
// metadata of the call.
func overGRPC(t *testing.T, f contractFixture, phase consts.Phase, in contractInput) contractAnswer {
	t.Helper()
	message := map[string]any{}
	if in.params != nil {
		message["params"] = in.params
	}
	if in.id != "" {
		message["id"] = in.id
	}
	if in.record != nil {
		message["record"] = in.record
		if phase == consts.Patch {
			message["mask"] = slices.Sorted(maps.Keys(in.record))
		}
	}
	if in.items != nil {
		message["items"] = in.items
		if phase == consts.PatchMany {
			// An item of a batch patch is the request of a single Patch:
			// its id, the record and, as its mask, the keys but the id.
			items := make([]map[string]any, len(in.items))
			for i, item := range in.items {
				items[i] = map[string]any{"id": item["id"], "record": item, "mask": slices.DeleteFunc(slices.Sorted(maps.Keys(item)), func(key string) bool { return key == "id" })}
			}
			message["items"] = items
		}
	}
	if in.ids != nil {
		message["ids"] = in.ids
	}
	if phase == consts.List || phase == consts.Get {
		message["query"] = in.query
	}
	if in.payload != nil {
		message["payload"] = in.payload
	}
	ctx := in.ctx
	if ctx == nil {
		ctx = t.Context()
	}
	if len(in.forwarded) > 0 {
		pairs := make([]string, 0, 2*len(in.forwarded))
		for name, values := range in.forwarded {
			for _, value := range values {
				pairs = append(pairs, strings.ToLower(name), value)
			}
		}
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}
	credential := sampleCredential
	switch {
	case f.anonymous:
		credential = ""
	case f.across:
		credential = sampleAcrossCredential
	}
	out, err := invokeIn(ctx, t, sampleServer(t), f.rpc(phase), message, credential)
	if err != nil {
		st := status.Convert(err)
		return contractAnswer{code: st.Code(), msg: st.Message()}
	}
	return contractAnswer{code: codes.OK, data: out}
}

// observedClientIP is the check of a row sending the observing service a
// forwarded address: the client address its hook saw is want, on the line
// the row just ran. The transports are reached over the loopback, a trusted
// proxy here (see TestMain), so the forwarded address is believed.
func observedClientIP(want string) func(*testing.T, contractInput, contractAnswer) {
	return func(t *testing.T, _ contractInput, _ contractAnswer) {
		t.Helper()
		observedMu.Lock()
		defer observedMu.Unlock()
		require.Equal(t, want, lastObserved.ClientIP)
	}
}

// filtersByNothingOverHTTP is the check of a row whose filter reaches the
// HTTP request as an empty parameter, the row having stored a record first:
// the HTTP answer counts that record at the least, the empty parameter
// filtering by nothing rather than matching the empty text.
func filtersByNothingOverHTTP(t *testing.T, _ contractInput, got contractAnswer) {
	t.Helper()
	if got.status == http.StatusOK {
		require.Positive(t, dataMap(t, got)["total"], "an empty parameter filters by nothing")
	}
}

// requireHTTPAnswer requires got, what HTTP answered, to carry the status
// want states and its msg: the success msg of the envelope for 200, the
// msg of the refusal otherwise, unless want leaves it to the comparison
// of the two transports.
func requireHTTPAnswer(t *testing.T, got contractAnswer, want contractWant) {
	t.Helper()
	require.Equal(t, want.status, got.status, got.msg)
	switch {
	case want.status == http.StatusOK:
		require.Equal(t, response.SuccessMsg, got.msg)
	case want.msg != "":
		require.Equal(t, want.msg, got.msg)
	}
}

// requireGRPCAnswer requires got, what gRPC answered, to carry the code the
// status want states maps to, the code want names instead when it does,
// and the message want states, or what want says gRPC answers on its own.
func requireGRPCAnswer(t *testing.T, got contractAnswer, want contractWant) {
	t.Helper()
	code, msg := grpcCodeOf(want.status), want.msg
	if want.code != codes.OK {
		code = want.code
	}
	if want.grpc != nil {
		code, msg = want.grpc.code, want.grpc.msg
	}
	require.Equal(t, code, got.code, got.msg)
	if msg != "" {
		require.Equal(t, msg, got.msg)
	}
}

// grpcCodeOf is the gRPC code an HTTP status maps to (see TRANSPORTS.md,
// section 4), for the statuses the scenarios answer.
func grpcCodeOf(httpStatus int) codes.Code {
	switch httpStatus {
	case http.StatusOK:
		return codes.OK
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.AlreadyExists
	case http.StatusInternalServerError:
		return codes.Internal
	case types.StatusClientClosedRequest:
		return codes.Canceled
	}
	panic("no gRPC code of " + strconv.Itoa(httpStatus))
}

// routePath spells route as the path of a request carrying params: a
// parameter segment params names a value for holds that value, an empty one
// an empty segment, so controller-parents/:id/samples with an empty id is
// controller-parents//samples; any other segment stays as route spells it.
func routePath(route string, params map[string]string) string {
	segments := strings.Split(route, "/")
	for i, segment := range segments {
		if name, ok := strings.CutPrefix(segment, ":"); ok {
			if value, given := params[name]; given {
				segments[i] = value
			}
		}
	}
	return strings.Join(segments, "/")
}

// queryString renders q as the query string of the HTTP request: each
// filter as a parameter field[op]=value of its own, a filter given twice
// repeating its parameter, the members of an in joined by commas, the
// orderings under _sort_by, the page and size and the cursor under their
// parameters, the way the request message of the rpc carries them (see
// controller.Query).
func queryString(q controller.Query) string {
	values := url.Values{}
	for _, f := range q.Filters {
		key := f.Field
		if f.Op != "" {
			key += "[" + f.Op + "]"
		}
		values.Add(key, strings.Join(f.Values, ","))
	}
	if len(q.SortBy) > 0 {
		values.Set(consts.QUERY_SORT_BY, strings.Join(q.SortBy, ","))
	}
	if q.Page != 0 {
		values.Set(consts.QUERY_PAGE, strconv.FormatUint(uint64(q.Page), 10))
	}
	if q.Size != 0 {
		values.Set(consts.QUERY_SIZE, strconv.FormatUint(uint64(q.Size), 10))
	}
	if q.CursorField != "" {
		values.Set(consts.QUERY_CURSOR_FIELD, q.CursorField)
	}
	if q.CursorValue != "" {
		values.Set(consts.QUERY_CURSOR_VALUE, q.CursorValue)
	}
	if q.CursorNext {
		values.Set(consts.QUERY_CURSOR_NEXT, "true")
	}
	return values.Encode()
}

// encodeJSON renders v as the JSON body of a request.
func encodeJSON(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// dataMap returns the data got answered as the object it is.
func dataMap(t *testing.T, got contractAnswer) map[string]any {
	t.Helper()
	data, ok := got.data.(map[string]any)
	require.True(t, ok, "%#v", got.data)
	return data
}

// itemMaps returns the items of an answer, each as the map it decoded into.
func itemMaps(t *testing.T, items any) []map[string]any {
	t.Helper()
	list, ok := items.([]any)
	require.True(t, ok, "%#v", items)
	maps := make([]map[string]any, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		require.True(t, ok, "%#v", item)
		maps = append(maps, m)
	}
	return maps
}

// requireSameInstant requires got, the RFC 3339 string a timestamp decoded
// into over either transport, to name the instant want.
func requireSameInstant(t *testing.T, want time.Time, got any, msg string) {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, stringOf(got))
	require.NoError(t, err, "%s: %#v", msg, got)
	require.True(t, parsed.Equal(want), "%s: want %s, got %s", msg, want, parsed)
}

// createValidated stores a validated sample named name and returns it with
// its id.
func createValidated(t *testing.T, name string) *validatedSample {
	t.Helper()
	record := &validatedSample{Name: name}
	require.NoError(t, database.Database[*validatedSample](context.Background()).Create(record))
	return record
}

// createVersioned stores a versioned sample named name and returns it with
// its id, at version 1.
func createVersioned(t *testing.T, name string) *versionedSample {
	t.Helper()
	record := &versionedSample{Name: name}
	require.NoError(t, database.Database[*versionedSample](context.Background()).Create(record))
	return record
}

// createScoped stores a scoped sample named name, filed under the default
// tenant, and returns it with its id.
func createScoped(t *testing.T, name string) *scopedSample {
	t.Helper()
	record := &scopedSample{Name: name}
	require.NoError(t, database.Database[*scopedSample](context.Background()).Create(record))
	return record
}

// countValidatedNamed counts the stored validated samples named name.
func countValidatedNamed(t *testing.T, name string) int {
	t.Helper()
	var total int
	require.NoError(t, database.Database[*validatedSample](context.Background()).
		WithQuery(&validatedSample{Name: name}).Count(&total))
	return total
}

// storedScoped reads the scoped sample with the given id as stored, across
// every tenant, so that a record filed under another one is found as well.
func storedScoped(t *testing.T, id string) *scopedSample {
	t.Helper()
	record := new(scopedSample)
	require.NoError(t, database.Database[*scopedSample](tenant.Across(context.Background())).Get(record, id))
	return record
}

// countSamplesWithID counts the stored samples carrying id: 1 or 0.
func countSamplesWithID(t *testing.T, id string) int {
	t.Helper()
	named := &sampleRecord{}
	named.SetID(id)
	var total int
	require.NoError(t, database.Database[*sampleRecord](context.Background()).WithQuery(named).Count(&total))
	return total
}
