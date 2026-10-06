package controller_test

import (
	"context"
	"encoding/json"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/controller"
	"github.com/hydroan/gst/internal/grpcserver"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// The tests of the call functions run them the way a project's generated
// handlers will: behind a real listener, through the server's chain and an
// auth interceptor naming the caller. The sample service stands for a
// generated service: every rpc takes a Struct holding what the request
// message of the action would carry, runs the call function of the action
// and answers with a Struct holding what the response message would.

// TestCreateCallCreatesTheRecordForTheCaller pins the create call: it
// creates the record the message carried, for the caller the auth
// interceptor named, and answers it as stored; what the hooks find on their
// service context is what an HTTP request's hooks find; a message failing
// the binding tags is refused the way a body is; a hook's refusal answers
// with the hook's status and creates nothing; a message carrying no record
// is refused the way an absent body is; and a route parameter left empty is
// refused before anything runs.
func TestCreateCallCreatesTheRecordForTheCaller(t *testing.T) {
	conn := sampleServer(t)
	name := uniqueName("call-create")

	created, err := invoke(t, conn, "Create", map[string]any{"record": map[string]any{"name": name}})
	require.NoError(t, err)
	require.Equal(t, name, created["name"])
	stored := new(sampleRecord)
	require.NoError(t, database.Database[*sampleRecord](context.Background()).Get(stored, stringOf(created["id"])))
	require.Equal(t, name, stored.Name)
	require.Equal(t, "u-1", stored.GetCreatedBy())
	require.Equal(t, "u-1", stored.GetUpdatedBy())

	t.Run("the hooks find the call on their service context", func(t *testing.T) {
		_, err := invoke(t, conn, "ObservedCreate", map[string]any{
			"params": map[string]any{"box": "b-1"},
			"record": map[string]any{"name": uniqueName("call-observed")},
		})
		require.NoError(t, err)

		observedMu.Lock()
		defer observedMu.Unlock()
		require.Equal(t, "b-1", lastObserved.Box)
		require.Equal(t, "alice", lastObserved.Username)
		require.Equal(t, http.MethodPost, lastObserved.Method)
		require.Equal(t, "/api/controller-sample-observedcreate", lastObserved.Route, "the route of the action, not the full method")
		require.True(t, lastObserved.RequiresAuth)
	})

	t.Run("a message failing validation is refused", func(t *testing.T) {
		_, err := invoke(t, conn, "ValidatedCreate", map[string]any{"record": map[string]any{}})

		requireStatus(t, err, codes.InvalidArgument, "name is a required field")
		details := status.Convert(err).Details()
		require.Len(t, details, 1, "the field refused travels as a BadRequest detail as well")
		bad, ok := details[0].(*errdetails.BadRequest)
		require.True(t, ok, "%T", details[0])
		require.Len(t, bad.GetFieldViolations(), 1)
		require.Equal(t, "name", bad.GetFieldViolations()[0].GetField())
		require.Equal(t, "name is a required field", bad.GetFieldViolations()[0].GetDescription())
	})

	t.Run("a before hook refusal creates nothing", func(t *testing.T) {
		name := uniqueName("call-create-refused")

		_, err := invoke(t, conn, "RefusedCreate", map[string]any{"record": map[string]any{"name": name}})

		requireStatus(t, err, codes.AlreadyExists, refusedMsg)
		require.Zero(t, countSamplesNamed(t, name))
	})

	t.Run("a message carrying no record is refused", func(t *testing.T) {
		_, err := invoke(t, conn, "Create", map[string]any{})
		requireStatus(t, err, codes.InvalidArgument, "record is required")
	})

	t.Run("an empty route parameter is refused", func(t *testing.T) {
		name := uniqueName("call-empty-param")
		_, err := invoke(t, conn, "Create", map[string]any{"params": map[string]string{"parent": ""}, "record": map[string]any{"name": name}})
		requireStatus(t, err, codes.InvalidArgument, `route parameter "parent" is required`)
		require.Zero(t, countSamplesNamed(t, name))
	})

	t.Run("a route parameter spanning segments is refused", func(t *testing.T) {
		// Over HTTP a parameter matches one segment of the path; a value
		// with a slash in it is one no request could carry.
		name := uniqueName("call-slashed-param")
		_, err := invoke(t, conn, "Create", map[string]any{"params": map[string]string{"parent": "a/b"}, "record": map[string]any{"name": name}})
		requireStatus(t, err, codes.InvalidArgument, `route parameter "parent" must not contain "/"`)
		require.Zero(t, countSamplesNamed(t, name))
	})
}

// TestCreateCallRefusesAnHTTPOnlyMethodBeforeWriting pins where a call
// refuses a service that called a method of its context only an HTTP
// request can serve: right after the hook that called it, so a before hook
// leaves nothing written, where an after hook, running once the record is
// written, leaves it written the way its own error would.
func TestCreateCallRefusesAnHTTPOnlyMethodBeforeWriting(t *testing.T) {
	conn := sampleServer(t)

	t.Run("called in a before hook, nothing is written", func(t *testing.T) {
		name := uniqueName("call-cookie-before")
		_, err := invoke(t, conn, "CookieBeforeCreate", map[string]any{"record": map[string]any{"name": name}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
		require.Zero(t, countSamplesNamed(t, name))
	})

	t.Run("called in an after hook, the record written stays", func(t *testing.T) {
		name := uniqueName("call-cookie-after")
		_, err := invoke(t, conn, "CookieAfterCreate", map[string]any{"record": map[string]any{"name": name}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
		require.Equal(t, 1, countSamplesNamed(t, name))
	})
}

// TestCallsRefuseAnEmptyParentParameterWhateverItsName pins that only the
// parameter carrying the id of an item action, the last one of its route, is
// left to the call's own refusal: a parent parameter named id, the default
// name of a model declaring no Param, is refused when empty on a collection
// action and on an item action alike, and the item action refuses an empty
// id by the field's name.
func TestCallsRefuseAnEmptyParentParameterWhateverItsName(t *testing.T) {
	conn := sampleServer(t)

	_, err := invoke(t, conn, "ParentList", map[string]any{"params": map[string]string{"id": ""}})
	requireStatus(t, err, codes.InvalidArgument, `route parameter "id" is required`)

	_, err = invoke(t, conn, "ParentGet", map[string]any{"params": map[string]string{"id": ""}, "id": "s-1"})
	requireStatus(t, err, codes.InvalidArgument, `route parameter "id" is required`)

	_, err = invoke(t, conn, "ParentGet", map[string]any{"params": map[string]string{"id": "p-1"}})
	requireStatus(t, err, codes.InvalidArgument, "id is required")
}

// TestGetCallAnswersTheRecordOrNotFound pins the get call: the record the id
// names, NotFound for an id no record carries or the integer key cannot
// hold, and a refusal for a message naming no id at all.
func TestGetCallAnswersTheRecordOrNotFound(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-get")

	got, err := invoke(t, conn, "Get", map[string]any{"id": record.GetID()})
	require.NoError(t, err)
	require.Equal(t, record.GetID(), got["id"])
	require.Equal(t, "call-get", got["name"])

	t.Run("an id no record carries", func(t *testing.T) {
		_, err := invoke(t, conn, "Get", map[string]any{"id": "missing"})
		requireStatus(t, err, codes.NotFound, "")
	})

	t.Run("an id the integer key cannot hold", func(t *testing.T) {
		_, err := invoke(t, conn, "CounterGet", map[string]any{"id": "first"})
		requireStatus(t, err, codes.NotFound, "")
	})

	t.Run("no id at all", func(t *testing.T) {
		_, err := invoke(t, conn, "Get", map[string]any{})
		requireStatus(t, err, codes.InvalidArgument, "id is required")
	})
}

// TestListCallListsLikeTheHTTPQuery pins the list call: the query of the
// message is read the way the HTTP query is, a filter with an operator as
// field[op]=value, one without as the bare key, the orderings and the
// paging under their parameters; what the HTTP listener refuses, a filter on
// a field the model lacks, a page on a model that does not page, a filter
// given twice, is refused the same; and a hook's refusal answers with the
// hook's status.
func TestListCallListsLikeTheHTTPQuery(t *testing.T) {
	conn := sampleServer(t)
	prefix := uniqueName("call-list")
	first := createSample(t, prefix+"-a")
	second := createSample(t, prefix+"-b")

	listed, err := invoke(t, conn, "List", map[string]any{"query": map[string]any{
		"Filters": []map[string]any{{"Field": "name", "Op": "like", "Values": []string{prefix}}},
		"SortBy":  []string{"name desc"},
	}})
	require.NoError(t, err)
	require.EqualValues(t, 2, listed["total"])
	require.Equal(t, []string{second.GetID(), first.GetID()}, ids(listed["items"]))

	t.Run("a filter without an operator is the bare key", func(t *testing.T) {
		listed, err := invoke(t, conn, "List", map[string]any{"query": map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Values": []string{prefix + "-a"}}},
		}})
		require.NoError(t, err)
		require.Equal(t, []string{first.GetID()}, ids(listed["items"]))
	})

	t.Run("a page of a model that pages", func(t *testing.T) {
		listed, err := invoke(t, conn, "List", map[string]any{"query": map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Op": "startswith", "Values": []string{prefix}}},
			"SortBy":  []string{"name"},
			"Page":    2,
			"Size":    1,
		}})
		require.NoError(t, err)
		require.EqualValues(t, 2, listed["total"])
		require.Equal(t, []string{second.GetID()}, ids(listed["items"]))
	})

	for _, tt := range []struct {
		name  string
		rpc   string
		query map[string]any
	}{
		{name: "a filter on a field the model lacks", rpc: "List", query: map[string]any{
			"Filters": []map[string]any{{"Field": "missing", "Op": "eq", "Values": []string{"value"}}},
		}},
		{name: "a page on a model that does not page", rpc: "CounterList", query: map[string]any{"Page": 2}},
		{name: "a filter given twice", rpc: "List", query: map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Op": "eq", "Values": []string{"a"}}, {"Field": "name", "Op": "eq", "Values": []string{"b"}}},
		}},
		{name: "several values under an operator taking one", rpc: "List", query: map[string]any{
			"Filters": []map[string]any{{"Field": "name", "Op": "eq", "Values": []string{"a", "b"}}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := invoke(t, conn, tt.rpc, map[string]any{"query": tt.query})
			requireStatus(t, err, codes.InvalidArgument, "")
		})
	}

	t.Run("a hook refusal answers with the hook's status", func(t *testing.T) {
		for _, rpc := range []string{"RefusedList", "FilterRefusedList"} {
			_, err := invoke(t, conn, rpc, map[string]any{})
			requireStatus(t, err, codes.AlreadyExists, refusedMsg)
		}
	})
}

// TestUpdateCallReplacesTheRecord pins the update call: the record the id
// names is replaced by the message's, its creation audit kept; an id no
// record carries answers NotFound; and a message carrying no record is
// refused, the record left as stored rather than replaced by a zero one.
func TestUpdateCallReplacesTheRecord(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-update")

	updated, err := invoke(t, conn, "Update", map[string]any{"id": record.GetID(), "record": map[string]any{"name": "call-updated"}})
	require.NoError(t, err)
	require.Equal(t, "call-updated", updated["name"])
	require.Equal(t, "u-1", updated["updated_by"])
	requireSampleName(t, record.GetID(), "call-updated")

	_, err = invoke(t, conn, "Update", map[string]any{"id": "missing", "record": map[string]any{"name": "call-updated"}})
	requireStatus(t, err, codes.NotFound, "")

	_, err = invoke(t, conn, "Update", map[string]any{"id": record.GetID()})
	requireStatus(t, err, codes.InvalidArgument, "record is required")
	requireSampleName(t, record.GetID(), "call-updated")

	t.Run("a record naming another id is written under the id of the call", func(t *testing.T) {
		other := createSample(t, "call-update-other")

		updated, err := invoke(t, conn, "Update", map[string]any{"id": record.GetID(), "record": map[string]any{"id": other.GetID(), "name": "call-moved"}})
		require.NoError(t, err)
		require.Equal(t, record.GetID(), updated["id"])
		requireSampleName(t, record.GetID(), "call-moved")
		requireSampleName(t, other.GetID(), "call-update-other")
	})
}

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

	_, err = invoke(t, conn, "ValidatedPatchMany", map[string]any{"items": []map[string]any{{"id": id, "note": "batch note"}}, "masks": [][]string{{"note"}}})
	require.NoError(t, err)
	_, err = invoke(t, conn, "ValidatedPatchMany", map[string]any{"items": []map[string]any{{"id": id, "name": ""}}, "masks": [][]string{{"name"}}})
	requireStatus(t, err, codes.InvalidArgument, "items[0].name is a required field")
}

// TestDeleteCallDeletesTheRecord pins the delete call: the record the id
// names is gone afterwards, a hook's refusal keeps it, and a message naming
// no record is refused by the name of the field it lacks, id, not by the
// route parameter the handler carries the id under.
func TestDeleteCallDeletesTheRecord(t *testing.T) {
	conn := sampleServer(t)
	record := createSample(t, "call-delete")

	_, err := invoke(t, conn, "Delete", map[string]any{"id": record.GetID()})
	require.NoError(t, err)
	require.Zero(t, countSamplesNamed(t, "call-delete"))

	_, err = invoke(t, conn, "Delete", map[string]any{})
	requireStatus(t, err, codes.InvalidArgument, "id is required")

	kept := createSample(t, "call-delete-refused")
	_, err = invoke(t, conn, "RefusedDelete", map[string]any{"id": kept.GetID()})
	requireStatus(t, err, codes.AlreadyExists, refusedMsg)
	requireSampleName(t, kept.GetID(), "call-delete-refused")
}

// TestBatchCallsWriteAllOrNothing pins the four batch calls: the items are
// created, replaced, patched under their masks and deleted as one batch; a
// hook's refusal writes nothing; an item failing its binding tags refuses
// the batch; a batch patch carries one mask per item and a record in every
// item; a batch update or patch names each record once; and a delete naming
// an empty id is refused before anything is deleted.
func TestBatchCallsWriteAllOrNothing(t *testing.T) {
	conn := sampleServer(t)
	prefix := uniqueName("call-batch")

	created, err := invoke(t, conn, "CreateMany", map[string]any{"items": []map[string]any{{"name": prefix + "-a"}, {"name": prefix + "-b"}}})
	require.NoError(t, err)
	createdIDs := ids(created["items"])
	require.Len(t, createdIDs, 2)
	require.Equal(t, 1, countSamplesNamed(t, prefix+"-a"))

	updated, err := invoke(t, conn, "UpdateMany", map[string]any{"items": []map[string]any{
		{"id": createdIDs[0], "name": prefix + "-a2"}, {"id": createdIDs[1], "name": prefix + "-b2"},
	}})
	require.NoError(t, err)
	require.Equal(t, createdIDs, ids(updated["items"]))
	requireSampleName(t, createdIDs[0], prefix+"-a2")

	patched, err := invoke(t, conn, "PatchMany", map[string]any{
		"items": []map[string]any{{"id": createdIDs[0], "name": prefix + "-a3"}, {"id": createdIDs[1], "name": prefix + "-b3", "note": "masked"}},
		"masks": [][]string{{"name"}, {"note"}},
	})
	require.NoError(t, err)
	require.Equal(t, createdIDs, ids(patched["items"]))
	requireSampleName(t, createdIDs[0], prefix+"-a3")
	second := loadSample(t, createdIDs[1])
	require.Equal(t, prefix+"-b2", second.Name, "the second mask does not name the name")
	require.Equal(t, "masked", second.Note)

	t.Run("a batch patch with fewer masks than items", func(t *testing.T) {
		_, patchErr := invoke(t, conn, "PatchMany", map[string]any{
			"items": []map[string]any{{"id": createdIDs[0], "name": prefix + "-a4"}, {"id": createdIDs[1], "name": prefix + "-b4"}},
			"masks": [][]string{{"name"}},
		})
		requireStatus(t, patchErr, codes.InvalidArgument, "2 items carry 1 update masks; each item names the fields to apply in a mask of its own")
		requireSampleName(t, createdIDs[0], prefix+"-a3")
	})

	t.Run("a batch patch with an item carrying no record", func(t *testing.T) {
		_, patchErr := invoke(t, conn, "PatchMany", map[string]any{
			"items": []any{map[string]any{"id": createdIDs[0], "name": prefix + "-a4"}, nil},
			"masks": [][]string{{"name"}, {"name"}},
		})
		requireStatus(t, patchErr, codes.InvalidArgument, "item 1 carries no record")
		requireSampleName(t, createdIDs[0], prefix+"-a3")
	})

	t.Run("a batch update naming a record twice", func(t *testing.T) {
		_, updateErr := invoke(t, conn, "UpdateMany", map[string]any{"items": []map[string]any{
			{"id": createdIDs[0], "name": prefix + "-a5"}, {"id": createdIDs[0], "name": prefix + "-a6"},
		}})
		requireStatus(t, updateErr, codes.InvalidArgument, `items[1] names the record "`+createdIDs[0]+`", which items[0] already names`)
		requireSampleName(t, createdIDs[0], prefix+"-a3")
	})

	t.Run("a batch patch naming a record twice", func(t *testing.T) {
		_, patchErr := invoke(t, conn, "PatchMany", map[string]any{
			"items": []map[string]any{{"id": createdIDs[0], "name": prefix + "-a5"}, {"id": createdIDs[0], "note": "twice"}},
			"masks": [][]string{{"name"}, {"note"}},
		})
		requireStatus(t, patchErr, codes.InvalidArgument, `items[1] names the record "`+createdIDs[0]+`", which items[0] already names`)
		requireSampleName(t, createdIDs[0], prefix+"-a3")
		require.Empty(t, loadSample(t, createdIDs[0]).Note)
	})

	t.Run("an item failing validation refuses the batch", func(t *testing.T) {
		_, createErr := invoke(t, conn, "ValidatedCreateMany", map[string]any{"items": []map[string]any{{"name": "valid"}, {}}})
		requireStatus(t, createErr, codes.InvalidArgument, "items[1].name is a required field")
		var total int
		require.NoError(t, database.Database[*validatedSample](context.Background()).WithQuery(&validatedSample{Name: "valid"}).Count(&total))
		require.Zero(t, total)
	})

	_, err = invoke(t, conn, "DeleteMany", map[string]any{"ids": createdIDs})
	require.NoError(t, err)
	require.Zero(t, countSamplesNamed(t, prefix+"-a3"))
	require.Zero(t, countSamplesNamed(t, prefix+"-b2"))

	t.Run("a hook refusal writes nothing", func(t *testing.T) {
		name := uniqueName("call-batch-refused")
		_, err := invoke(t, conn, "RefusedCreateMany", map[string]any{"items": []map[string]any{{"name": name}}})
		requireStatus(t, err, codes.AlreadyExists, refusedMsg)
		require.Zero(t, countSamplesNamed(t, name))
	})

	t.Run("a delete naming an empty id", func(t *testing.T) {
		kept := createSample(t, "call-batch-kept")
		_, err := invoke(t, conn, "DeleteMany", map[string]any{"ids": []string{kept.GetID(), ""}})
		requireStatus(t, err, codes.InvalidArgument, "")
		requireSampleName(t, kept.GetID(), "call-batch-kept")
	})
}

// TestPatchItemReadiesTheRecordOfABatchItem pins PatchItem, what the
// generated handler of a PatchMany rpc reads each item through: the item
// names its record by id, written onto the record it carries whatever id
// the record names, the way a patch call writes the record its message
// names; an item naming no id and an item whose route parameter contradicts
// the request's are refused with InvalidArgument; a parameter the item
// leaves empty is the request's; and an item carrying no record is left to
// the call to refuse.
func TestPatchItemReadiesTheRecordOfABatchItem(t *testing.T) {
	readied, err := controller.PatchItem(0, nil, nil, "r1", &sampleRecord{Name: "named"})
	require.NoError(t, err)
	require.Equal(t, "r1", readied.GetID())
	require.Equal(t, "named", readied.Name)

	other := &sampleRecord{}
	other.SetID("r2")
	readied, err = controller.PatchItem(1, nil, nil, "r1", other)
	require.NoError(t, err)
	require.Equal(t, "r1", readied.GetID(), "the id of the item names the record, whatever id the record carries")

	_, err = controller.PatchItem(2, nil, nil, "", &sampleRecord{})
	requireStatus(t, err, codes.InvalidArgument, "item 2 names no id")

	_, err = controller.PatchItem(3, map[string]string{"record": "a"}, map[string]string{"record": "b"}, "r1", &sampleRecord{})
	requireStatus(t, err, codes.InvalidArgument, `item 3 names the record parameter "b", the request names "a"`)

	readied, err = controller.PatchItem(4, map[string]string{"record": "a"}, map[string]string{"record": ""}, "r1", &sampleRecord{})
	require.NoError(t, err)
	require.Equal(t, "r1", readied.GetID())

	var absent *sampleRecord
	readied, err = controller.PatchItem(5, nil, nil, "r1", absent)
	require.NoError(t, err)
	require.Nil(t, readied)
}

// TestServiceCallDelegatesToThePhaseService pins the call of an action with
// a payload and result of its own: the phase service's method runs with the
// payload, on a service context answering the parameters, the query, the
// caller and the method the call carries; its service error answers with
// its status, any other error answers Internal, and so does a response the
// service tried to write, which no call can carry; a payload failing the
// binding tags is refused; and an action described public runs for a call
// naming no caller.
func TestServiceCallDelegatesToThePhaseService(t *testing.T) {
	conn := sampleServer(t)

	result, err := invoke(t, conn, "Action", map[string]any{
		"params": map[string]any{"box": "b-2"}, "payload": map[string]any{"note": "hello"},
	})
	require.NoError(t, err)
	require.Equal(t, "hello", result["Note"])
	require.Equal(t, "b-2", result["Box"])
	require.Equal(t, "alice", result["Username"])
	require.Equal(t, http.MethodPost, result["Method"])
	require.Equal(t, "/api/controller-sample-action", result["Route"], "the route of the action, not the full method")
	require.Equal(t, true, result["RequiresAuth"])

	t.Run("a List reads the query", func(t *testing.T) {
		result, err := invoke(t, conn, "ActionList", map[string]any{"query": map[string]any{"Page": 2, "Expand": []string{"children"}}})
		require.NoError(t, err)
		require.Equal(t, http.MethodGet, result["Method"], "the method of the action, not the POST every call is on the wire")
		require.Equal(t, map[string]any{"_page": []any{"2"}, "_expand": []any{"children"}}, result["Query"])
	})

	t.Run("a public action names no caller", func(t *testing.T) {
		result, err := invokeAs(t, conn, "OpenAction", map[string]any{"payload": map[string]any{"note": "hello"}}, "")
		require.NoError(t, err)
		require.Equal(t, false, result["RequiresAuth"])
		require.Empty(t, result["Username"])
	})

	t.Run("a payload failing validation is refused", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{}})
		requireStatus(t, err, codes.InvalidArgument, "note is a required field")
	})

	t.Run("the service's error answers with its status", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionRefuse}})
		requireStatus(t, err, codes.PermissionDenied, "not yours")
	})

	t.Run("any other error answers Internal", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionBreak}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
	})

	t.Run("a canceled call is answered as canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", sampleCredential)
		before := len(accessLog.FilterMessage("/gst.test.Samples/Action").All())
		done := make(chan error, 1)
		go func() {
			done <- conn.Invoke(ctx, "/gst.test.Samples/Action", encode(map[string]any{"payload": map[string]any{"note": actionHang}}), new(structpb.Struct), grpc.WaitForReady(true))
		}()
		<-actionEntered
		cancel()
		require.Equal(t, codes.Canceled, status.Code(<-done))
		require.Eventually(t, func() bool {
			entries := accessLog.FilterMessage("/gst.test.Samples/Action").All()
			return len(entries) > before && entries[len(entries)-1].ContextMap()["grpc_code"] == codes.Canceled.String()
		}, 10*time.Second, 20*time.Millisecond, "the access log records the call as canceled, not as a failure of the service")
	})

	t.Run("a response the service tried to write answers Internal", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionWrite}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
	})

	t.Run("a form value the service tried to read answers Internal", func(t *testing.T) {
		_, err := invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": actionRead}})
		requireStatus(t, err, codes.Internal, types.FailureMsg)
	})
}

// TestCallsRunInTheControllerSpan pins the spans a call runs in: the
// controller span of the action, named and attributed the way a request's
// is, by the method and route of the action as the registration described
// it, and the service span of a delegated action inside it.
func TestCallsRunInTheControllerSpan(t *testing.T) {
	conn := sampleServer(t)
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	record := createSample(t, "call-span")

	_, err := invoke(t, conn, "Get", map[string]any{"id": record.GetID()})
	require.NoError(t, err)
	_, err = invoke(t, conn, "Action", map[string]any{"payload": map[string]any{"note": "traced"}})
	require.NoError(t, err)

	got := oteltest.EndedNamed(t, recorder, gstotel.FrameworkSpanName("controller", "sampleRecord", "Get"))
	require.ElementsMatch(t, []attribute.KeyValue{
		attribute.String("component", "controller"),
		attribute.String("controller.operation", "Get"),
		attribute.String("controller.model", "sampleRecord"),
		attribute.String("controller.method", http.MethodGet),
		attribute.String("controller.path", "/api/controller-sample-get"),
	}, got.Attributes())
	service := oteltest.EndedNamed(t, recorder, gstotel.FrameworkSpanName("service", "sampleRecord", "Create"))
	controllerSpan := oteltest.EndedNamed(t, recorder, gstotel.FrameworkSpanName("controller", "sampleRecord", "Create"))
	require.Equal(t, controllerSpan.SpanContext().SpanID(), service.Parent().SpanID(), "the service span runs inside the controller span")
}

// The sample service stands for the gRPC service the generator writes for a
// model: gst.test.Samples, with an rpc per call function under test, each
// taking a Struct holding what the request message of the action would
// carry and answering a Struct holding what the response message would. One
// listener serves the whole test binary, behind an auth interceptor naming
// alice as the caller of every call presenting sampleCredential.
var (
	sampleServerOnce sync.Once
	// accessLog collects the gRPC access log of the sample server, the entry
	// of every call it served, for the tests reading how a call ended.
	accessLog        *observer.ObservedLogs
	sampleServerAddr string
	errSampleServer  error
)

// sampleCredential is the authorization a call presents to be named alice.
const sampleCredential = "Bearer alice"

// sampleServer returns a connection to the sample service, starting its
// listener on first use.
func sampleServer(t *testing.T) *grpc.ClientConn {
	t.Helper()
	sampleServerOnce.Do(func() {
		grpcserver.UseAuth(func(ctx context.Context) (context.Context, error) {
			md, _ := metadata.FromIncomingContext(ctx)
			if values := md.Get("authorization"); len(values) == 1 && values[0] == sampleCredential {
				return grpcserver.WithCaller(ctx, grpcserver.Caller{Username: "alice", UserID: "u-1"}), nil
			}
			return nil, status.Error(codes.Unauthenticated, "who are you")
		})
		handlers := sampleHandlers()
		descs := make([]grpc.MethodDesc, 0, len(handlers))
		methods := make([]grpcserver.Method, 0, len(handlers))
		for name, handle := range handlers {
			fullMethod := "/gst.test.Samples/" + name
			descs = append(descs, grpc.MethodDesc{
				MethodName: name,
				Handler: func(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
					in := new(structpb.Struct)
					if err := dec(in); err != nil {
						return nil, err
					}
					handler := func(ctx context.Context, _ any) (any, error) {
						out, err := handle(ctx, in.AsMap())
						if err != nil {
							return nil, err
						}
						return encode(out), nil
					}
					return interceptor(ctx, in, &grpc.UnaryServerInfo{FullMethod: fullMethod}, handler)
				},
			})
			// Every rpc is described with the HTTP action it stands for, the
			// way the generated registration describes one: a route named
			// after the rpc and the method of its kind, so that what a hook
			// reads for the route and method is the action's.
			httpMethod := http.MethodPost
			if strings.Contains(name, "List") || strings.Contains(name, "Get") {
				httpMethod = http.MethodGet
			}
			methods = append(methods, grpcserver.Method{Name: fullMethod, Public: name == "OpenAction", HTTPMethod: httpMethod, Route: "/api/controller-sample-" + strings.ToLower(name)})
		}
		for _, name := range []string{"Watch", "Upload", "Chat", "Fork", "Silence"} {
			methods = append(methods, grpcserver.Method{Name: "/gst.test.Samples/" + name, HTTPMethod: grpcserver.MethodStream, Route: "/api/controller-sample-" + strings.ToLower(name)})
		}
		grpcserver.Register(func(r grpc.ServiceRegistrar) {
			r.RegisterService(&grpc.ServiceDesc{
				ServiceName: "gst.test.Samples",
				HandlerType: (*any)(nil),
				Methods:     descs,
				Streams:     streamDescs(),
				Metadata:    "gst/test/samples.proto",
			}, nil)
		}, methods...)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			errSampleServer = err
			return
		}
		addr, _ := listener.Addr().(*net.TCPAddr)
		_ = listener.Close()
		config.App.GRPC = config.GRPC{Listen: "127.0.0.1", Port: addr.Port}
		sampleServerAddr = addr.String()
		core, entries := observer.New(zap.InfoLevel)
		logger.GRPC = zap.New(core)
		accessLog = entries
		go func() { _ = grpcserver.Run() }()
	})
	require.NoError(t, errSampleServer)
	conn, err := grpc.NewClient(sampleServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// rpcHandler is an rpc of the sample service: it runs the call function of
// its action the way a generated handler does, with what the request
// message carries, and answers what the response message would.
type rpcHandler func(ctx context.Context, in map[string]any) (any, error)

// parentRoute nests the samples under a parent whose parameter is named id,
// the default name of a model declaring no Param, written the way the
// generated code writes the route of a collection action; parentItemRoute
// is the route of its item actions, ending in the parameter carrying the
// sample's id.
const (
	parentRoute     = "controller-parents/:id/samples"
	parentItemRoute = parentRoute + "/:sample"
)

// sampleHandlers builds the rpcs of the sample service: the ten standard
// actions of every fixture model on every fixture route (see standardRPCs),
// the list and get of the samples nested under a parent (see parentRoute),
// and the custom action of the sample, taking the query and the payload.
func sampleHandlers() map[string]rpcHandler {
	handlers := make(map[string]rpcHandler)
	standardRPCs[*sampleRecord](handlers, sampleRoute, "")
	standardRPCs[*sampleRecord](handlers, refusalRoute, "Refused")
	standardRPCs[*sampleRecord](handlers, filterRefusalRoute, "FilterRefused")
	standardRPCs[*sampleRecord](handlers, observedRoute, "Observed")
	standardRPCs[*sampleRecord](handlers, cookieBeforeRoute, "CookieBefore")
	standardRPCs[*sampleRecord](handlers, cookieAfterRoute, "CookieAfter")
	standardRPCs[*sampleCounter](handlers, counterRoute, "Counter")
	standardRPCs[*versionedSample](handlers, versionedRoute, "Versioned")
	standardRPCs[*shapedSample](handlers, shapedRoute, "Shaped")
	standardRPCs[*validatedSample](handlers, validatedRoute, "Validated")
	standardRPCs[*datedSample](handlers, datedRoute, "Dated")

	parentList := controller.ListCall[*sampleRecord](parentRoute)
	parentGet := controller.GetCall[*sampleRecord](parentItemRoute)
	handlers["ParentList"] = func(ctx context.Context, in map[string]any) (any, error) {
		return listing(parentList(ctx, params(in), field[controller.Query](in, "query")))
	}
	handlers["ParentGet"] = func(ctx context.Context, in map[string]any) (any, error) {
		id := field[string](in, "id")
		return parentGet(ctx, itemParams(parentItemRoute, params(in), id), id, field[controller.Query](in, "query"))
	}

	action := controller.ServiceCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.Create, actionRoute)
	actionList := controller.ServiceCall[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.List, actionRoute)
	handlers["Action"] = func(ctx context.Context, in map[string]any) (any, error) {
		return action(ctx, params(in), field[controller.Query](in, "query"), field[*sampleActionReq](in, "payload"))
	}
	handlers["OpenAction"] = handlers["Action"]
	handlers["ActionList"] = func(ctx context.Context, in map[string]any) (any, error) {
		return actionList(ctx, params(in), field[controller.Query](in, "query"), field[*sampleActionReq](in, "payload"))
	}
	return handlers
}

// standardRPCs adds to handlers the rpcs of the ten standard actions of M
// on route, each named prefix followed by the action's name, Create and
// PatchMany for the samples, RefusedCreate for the refusals: each runs the
// call function of its action with what the request message carries, the
// route parameters under params, the id, the record or the items, the
// update mask or masks, the query and the ids.
func standardRPCs[M types.Model](handlers map[string]rpcHandler, route, prefix string) {
	create := controller.CreateCall[M](route)
	get := controller.GetCall[M](route)
	list := controller.ListCall[M](route)
	update := controller.UpdateCall[M](route)
	patch := controller.PatchCall[M](route)
	del := controller.DeleteCall[M](route)
	createMany := controller.CreateManyCall[M](route)
	updateMany := controller.UpdateManyCall[M](route)
	patchMany := controller.PatchManyCall[M](route)
	deleteMany := controller.DeleteManyCall[M](route)

	handlers[prefix+consts.Create.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		return create(ctx, params(in), field[M](in, "record"))
	}
	handlers[prefix+consts.Get.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		id := field[string](in, "id")
		return get(ctx, itemParams(route, params(in), id), id, field[controller.Query](in, "query"))
	}
	handlers[prefix+consts.List.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		return listing(list(ctx, params(in), field[controller.Query](in, "query")))
	}
	handlers[prefix+consts.Update.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		id := field[string](in, "id")
		return update(ctx, itemParams(route, params(in), id), id, field[M](in, "record"))
	}
	handlers[prefix+consts.Patch.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		id := field[string](in, "id")
		return patch(ctx, itemParams(route, params(in), id), id, field[M](in, "record"), field[[]string](in, "mask"))
	}
	handlers[prefix+consts.Delete.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		id := field[string](in, "id")
		return done(del(ctx, itemParams(route, params(in), id), id))
	}
	handlers[prefix+consts.CreateMany.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		return batch(createMany(ctx, params(in), field[[]M](in, "items")))
	}
	handlers[prefix+consts.UpdateMany.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		return batch(updateMany(ctx, params(in), field[[]M](in, "items")))
	}
	handlers[prefix+consts.PatchMany.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		return batch(patchMany(ctx, params(in), field[[]M](in, "items"), field[[][]string](in, "masks")))
	}
	handlers[prefix+consts.DeleteMany.Name()] = func(ctx context.Context, in map[string]any) (any, error) {
		return done(deleteMany(ctx, params(in), field[[]string](in, "ids")))
	}
}

// params returns the route parameters the message carries under params.
func params(in map[string]any) map[string]string {
	return field[map[string]string](in, "params")
}

// itemParams returns the route parameters of an item call the way a
// generated handler builds them: the parameters the message carries plus,
// under the last parameter segment of route (see consts.LastRouteParam), the
// id the message names the record by, whatever it holds (see the golden
// record.gen.go of cmd/gg). A route naming no parameter, as the fixture
// routes are written, gets the parameters as they are.
func itemParams(route string, given map[string]string, id string) map[string]string {
	name := consts.LastRouteParam(route)
	if name == "" {
		return given
	}
	params := maps.Clone(given)
	if params == nil {
		params = make(map[string]string, 1)
	}
	params[name] = id
	return params
}

// field converts in[key] into T through its JSON shape, the way a generated
// handler converts a field of the request message into the Go value the
// call takes; a key the message lacks is the zero value.
func field[T any](in map[string]any, key string) T {
	var value T
	raw, ok := in[key]
	if !ok {
		return value
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		panic(err)
	}
	return value
}

// listing, batch and done shape what a list, a batch and a call answering
// nothing return into what their response messages would carry.
func listing[M any](items []M, total int, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total}, nil
}

func batch[M any](items []M, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items}, nil
}

func done(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

// encode renders out, a record or a map, as the Struct a response message
// would carry, through its JSON shape.
func encode(out any) *structpb.Struct {
	encoded, err := json.Marshal(out)
	if err != nil {
		panic(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(encoded, &fields); err != nil {
		panic(err)
	}
	message, err := structpb.NewStruct(fields)
	if err != nil {
		panic(err)
	}
	return message
}

// invoke calls the rpc name of the sample service as alice with in as the
// request message and returns the response message as a map.
func invoke(t *testing.T, conn *grpc.ClientConn, name string, in map[string]any) (map[string]any, error) {
	t.Helper()
	return invokeAs(t, conn, name, in, sampleCredential)
}

// invokeAs calls the rpc name presenting authorization, none when empty.
func invokeAs(t *testing.T, conn *grpc.ClientConn, name string, in map[string]any, authorization string) (map[string]any, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if authorization != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", authorization)
	}
	out := new(structpb.Struct)
	if err := conn.Invoke(ctx, "/gst.test.Samples/"+name, encode(in), out, grpc.WaitForReady(true)); err != nil {
		return nil, err
	}
	return out.AsMap(), nil
}

// ids returns the ids of the records items holds, in order.
func ids(items any) []string {
	var got []string
	list, _ := items.([]any)
	for _, item := range list {
		record, _ := item.(map[string]any)
		got = append(got, stringOf(record["id"]))
	}
	return got
}

// stringOf returns the string value holds, "" for any other value.
func stringOf(value any) string {
	s, _ := value.(string)
	return s
}

// requireStatus requires err to be a status of code, carrying message when
// one is given.
func requireStatus(t *testing.T, err error, code codes.Code, message string) {
	t.Helper()
	require.Error(t, err)
	st := status.Convert(err)
	require.Equal(t, code, st.Code(), st.Message())
	if message != "" {
		require.Equal(t, message, st.Message())
	}
}
