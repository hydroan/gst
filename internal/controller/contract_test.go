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
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The fixed messages of the controller's refusals, as errors.go answers
// them on both transports.
const (
	invalidArgumentMsg = "The request contains invalid parameters."
	notFoundMsg        = "The requested resource was not found."
	staleObjectMsg     = "The resource was modified by another operation. Reload and retry."
)

// TestTransportsAnswerTheContractAlike pins the contract of the actions on
// both transports at once: every scenario of the table is sent as the HTTP
// request and as the gRPC call carrying the same input, through the handler
// and the call function of the action, and both must answer the status the
// scenario states — the HTTP status, and the gRPC code it maps to (see
// TRANSPORTS.md, section 4) — the same msg, and leave the records the same.
// The scenarios the transports answer differently on purpose state what
// each answers (see TRANSPORTS.md, section 9): a change to one side alone
// turns the table red, and a new difference has to be written into the
// table and the document together.
func TestTransportsAnswerTheContractAlike(t *testing.T) {
	samples := standardFixture[*sampleRecord](sampleRoute, "")
	refusals := standardFixture[*sampleRecord](refusalRoute, "Refused")
	filterRefusals := standardFixture[*sampleRecord](filterRefusalRoute, "FilterRefused")
	cookieBefore := standardFixture[*sampleRecord](cookieBeforeRoute, "CookieBefore")
	cookieAfter := standardFixture[*sampleRecord](cookieAfterRoute, "CookieAfter")
	counters := standardFixture[*sampleCounter](counterRoute, "Counter")
	versioned := standardFixture[*versionedSample](versionedRoute, "Versioned")
	validated := standardFixture[*validatedSample](validatedRoute, "Validated")
	dated := standardFixture[*datedSample](datedRoute, "Dated")

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
			name: "Create refuses a record failing validation", fixture: validated, phase: consts.Create,
			input: contractInput{record: map[string]any{}},
			want:  nameRequired,
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
			name: "List refuses a page on a model that does not page", fixture: counters, phase: consts.List,
			input: contractInput{query: controller.Query{Page: 2}},
			want:  contractWant{status: http.StatusBadRequest},
		},
		{name: "List answers the hook's refusal", fixture: refusals, phase: consts.List, want: refused},
		{name: "List answers the Filter hook's refusal", fixture: filterRefusals, phase: consts.List, want: refused},
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
			name: "UpdateMany refuses an item naming no record and writes nothing", fixture: samples, phase: consts.UpdateMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createSample(t, "contract-update-many-kept").GetID(), "name": "contract-update-many-renamed"}, {"name": "contract-update-many-other"}}}
			},
			want: invalid,
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
			name: "PatchMany refuses an item naming no record and writes nothing", fixture: samples, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createSample(t, "contract-patch-many-kept").GetID(), "name": "contract-patch-many-renamed"}, {"name": "contract-patch-many-other"}}}
			},
			want: invalid,
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
			name: "PatchMany refuses an item failing validation", fixture: validated, phase: consts.PatchMany,
			prepare: func(t *testing.T) contractInput {
				t.Helper()
				return contractInput{items: []map[string]any{{"id": createValidated(t, "contract-patch-many-validated").GetID(), "name": ""}}}
			},
			want: contractWant{status: http.StatusBadRequest, msg: "items[0].name is a required field"},
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
// the paths of the mask of a Patch; the items of a batch, over gRPC each
// item's keys but the id its mask; the ids of a batch delete; the query of
// a List, the query string over HTTP; and the payload of a custom action. A
// nil record is a request carrying none: an absent body, a message without
// a record.
type contractInput struct {
	id      string
	record  map[string]any
	items   []map[string]any
	ids     []string
	query   controller.Query
	payload map[string]any
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
// sample service.
type contractFixture struct {
	route   string
	handler func(phase consts.Phase) gin.HandlerFunc
	rpc     func(phase consts.Phase) string
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

// overHTTP sends in as the HTTP request of the action, as alice, and
// decodes the envelope answered: the method and path of the action, the id
// in the path, the query rendered as the query string, the record, the
// items, the ids or the payload as the JSON body.
func overHTTP(t *testing.T, f contractFixture, phase consts.Phase, in contractInput) contractAnswer {
	t.Helper()
	pattern, target := "/"+f.route, "/"+f.route
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
	rsp := serveAs(t, phase.HTTPMethod(), pattern, f.handler(phase), target, body, "alice")
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
// alice, and returns the status answered, or the response message: the
// route parameters, the id, the record with the mask of a Patch, the items
// with the masks of a PatchMany, the ids, the query and the payload, as the
// rpcs of the sample service read them (see standardRPCs).
func overGRPC(t *testing.T, f contractFixture, phase consts.Phase, in contractInput) contractAnswer {
	t.Helper()
	message := map[string]any{}
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
			masks := make([][]string, len(in.items))
			for i, item := range in.items {
				masks[i] = slices.DeleteFunc(slices.Sorted(maps.Keys(item)), func(key string) bool { return key == "id" })
			}
			message["masks"] = masks
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
	out, err := invoke(t, sampleServer(t), f.rpc(phase), message)
	if err != nil {
		st := status.Convert(err)
		return contractAnswer{code: st.Code(), msg: st.Message()}
	}
	return contractAnswer{code: codes.OK, data: out}
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
	}
	panic("no gRPC code of " + strconv.Itoa(httpStatus))
}

// queryString renders q as the query string of the HTTP request: a filter
// as field[op]=value, the members of an in joined by commas, the orderings
// under _sort_by and the page and size under their parameters, the way the
// request message of the rpc carries them (see controller.Query).
func queryString(q controller.Query) string {
	values := url.Values{}
	for _, f := range q.Filters {
		key := f.Field
		if f.Op != "" {
			key += "[" + f.Op + "]"
		}
		values.Set(key, strings.Join(f.Values, ","))
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

// countValidatedNamed counts the stored validated samples named name.
func countValidatedNamed(t *testing.T, name string) int {
	t.Helper()
	var total int
	require.NoError(t, database.Database[*validatedSample](context.Background()).
		WithQuery(&validatedSample{Name: name}).Count(&total))
	return total
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
