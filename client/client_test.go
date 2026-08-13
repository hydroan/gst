package client_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

var baseURL = testutil.BaseURL()

const recordPath = "/api/test-record"

// TestRecord is the neutral fixture model the client end-to-end tests run
// against; the routes below expose the full standard CRUD matrix for it.
type TestRecord struct {
	Name string `json:"name,omitempty"`
	Note string `json:"note,omitempty"`
	Tag  string `json:"tag,omitempty"`

	model.Query
	model.Base
}

func (r *TestRecord) GetTableName() string {
	return "test_records"
}

func (r *TestRecord) Purge() bool { return true }

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Register: func() { model.Register[*TestRecord]() },
		Routes: func() error {
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record", nil, consts.Create)
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record/:id", &types.ControllerConfig[*TestRecord]{ParamName: "id"}, consts.Delete)
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record/:id", &types.ControllerConfig[*TestRecord]{ParamName: "id"}, consts.Update)
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record/:id", &types.ControllerConfig[*TestRecord]{ParamName: "id"}, consts.Patch)
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record", nil, consts.List)
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record/:id", &types.ControllerConfig[*TestRecord]{ParamName: "id"}, consts.Get)
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record/batch", nil, consts.CreateMany)
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record/batch", nil, consts.DeleteMany)
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record/batch", nil, consts.UpdateMany)
			router.Register[*TestRecord, *TestRecord, *TestRecord](router.Auth(), "test-record/batch", nil, consts.PatchMany)

			return nil
		},
	})
}

// newRecordID returns an id unique to this test binary run.
func newRecordID(prefix string) string {
	return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func TestClientCRUDRoundTrip(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	id := newRecordID("crud")
	record := &TestRecord{Name: "sample-a", Note: "note-1", Base: model.Base{ID: id}}

	created, err := client.Post[TestRecord](cli, recordPath, record)
	require.NoError(t, err)
	require.Equal(t, id, created.ID)
	require.Equal(t, "sample-a", created.Name)

	got, err := client.Get[TestRecord](cli, recordPath+"/"+id)
	require.NoError(t, err)
	require.Equal(t, "sample-a", got.Name)
	require.Equal(t, "note-1", got.Note)

	_, err = client.Put[TestRecord](cli, recordPath+"/"+id, &TestRecord{Name: "sample-b", Base: model.Base{ID: id}})
	require.NoError(t, err)

	got, err = client.Get[TestRecord](cli, recordPath+"/"+id)
	require.NoError(t, err)
	require.Equal(t, "sample-b", got.Name)
	// A full update replaces the row, so the note written at create is gone.
	require.Empty(t, got.Note)

	patched, err := client.Patch[TestRecord](cli, recordPath+"/"+id, &TestRecord{Tag: "tag-1", Base: model.Base{ID: id}})
	require.NoError(t, err)
	require.Equal(t, "tag-1", patched.Tag)
	// A partial update keeps the fields it does not name.
	require.Equal(t, "sample-b", patched.Name)

	_, err = client.Delete[struct{}](cli, recordPath+"/"+id, nil)
	require.NoError(t, err)

	// The bare CRUD route has no service layer; a vanished record answers 400
	// with the database message rather than a service-shaped 404.
	_, err = client.Get[TestRecord](cli, recordPath+"/"+id)
	testutil.RequireError(t, err, http.StatusBadRequest, "record not found")
}

func TestClientListQueryOptions(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	ids := make([]string, 0, 3)
	for i := range 3 {
		id := newRecordID("list" + strconv.Itoa(i))
		_, createErr := client.Post[TestRecord](cli, recordPath, &TestRecord{Name: "list-sample", Base: model.Base{ID: id}})
		require.NoError(t, createErr)
		ids = append(ids, id)
	}
	t.Cleanup(func() {
		_, cleanupErr := client.Delete[struct{}](cli, recordPath+"/batch", client.BatchIDs(ids))
		require.NoError(t, cleanupErr)
	})

	list, err := client.Get[client.ListResult[*TestRecord]](cli, recordPath,
		client.WithPage(1, 2), client.WithSortBy("created_at desc"))
	require.NoError(t, err)
	require.Len(t, list.Items, 2)
	require.GreaterOrEqual(t, list.Total, 3)
}

func TestClientBatchRoundTrip(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	id1 := newRecordID("batch_a")
	id2 := newRecordID("batch_b")
	records := []*TestRecord{
		{Name: "batch-sample", Base: model.Base{ID: id1}},
		{Name: "batch-sample", Base: model.Base{ID: id2}},
	}

	_, err = client.Post[struct{}](cli, recordPath+"/batch", client.BatchItems(records))
	require.NoError(t, err)

	got, err := client.Get[TestRecord](cli, recordPath+"/"+id1)
	require.NoError(t, err)
	require.Equal(t, "batch-sample", got.Name)

	records[0].Note = "batch-note"
	records[1].Note = "batch-note"
	_, err = client.Put[struct{}](cli, recordPath+"/batch", client.BatchItems(records))
	require.NoError(t, err)

	got, err = client.Get[TestRecord](cli, recordPath+"/"+id2)
	require.NoError(t, err)
	require.Equal(t, "batch-note", got.Note)

	_, err = client.Delete[struct{}](cli, recordPath+"/batch", client.BatchIDs([]string{id1, id2}))
	require.NoError(t, err)

	_, err = client.Get[TestRecord](cli, recordPath+"/"+id1)
	testutil.RequireError(t, err, http.StatusBadRequest, "record not found")
}

func TestClientRejectionCarriesEnvelope(t *testing.T) {
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	_, err = client.Get[TestRecord](cli, recordPath+"/absent-record-id")
	var respErr *client.Error
	require.ErrorAs(t, err, &respErr)
	require.Equal(t, http.StatusBadRequest, respErr.StatusCode)
	require.NotZero(t, respErr.Code)
	require.NotEmpty(t, respErr.Msg)
	require.NotEmpty(t, respErr.TraceID)
}

func TestClientEnvelopeCompleteness(t *testing.T) {
	// The one place asserting envelope integrity (TraceID present on success)
	// so the module tests do not have to repeat it per case.
	cli, err := client.New(baseURL)
	require.NoError(t, err)

	resp, err := cli.Do(http.MethodGet, recordPath, nil)
	require.NoError(t, err)
	require.NotEmpty(t, resp.TraceID)
	require.Zero(t, resp.Code)
}
