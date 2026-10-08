package controller_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// sampleRecord is the table model the handler tests read and write, keyed by
// a string id; the note is a second field for a patch to leave alone.
type sampleRecord struct {
	Name string `json:"name"`
	Note string `json:"note"`

	modelregistry.Query
	modelregistry.Base
}

func (sampleRecord) TableName() string { return "controller_samples" }

// sampleCounter is keyed by an auto-increment integer, so a route id that is
// not a number names no record of it.
type sampleCounter struct {
	Name string `json:"name"`

	modelregistry.AutoBase
}

func (sampleCounter) TableName() string { return "controller_counters" }

// versionedSample is a table model under optimistic locking: a patch of it
// must carry the version it was read at.
type versionedSample struct {
	Name    string              `json:"name"`
	Version modelschema.Version `json:"version,omitempty" gorm:"not null;default:1"`

	modelregistry.Base
}

func (versionedSample) TableName() string { return "controller_versioned_samples" }

// validatedSample declares its name required, so a call's message is
// validated the way a request body is; the note carries no tag, so a patch
// naming the note alone has nothing to meet.
type validatedSample struct {
	Name string `json:"name" binding:"required"`
	Note string `json:"note"`

	modelregistry.Base
}

func (validatedSample) TableName() string { return "controller_validated_samples" }

// shapedSample carries the field shapes a patch applies as a whole beside
// the plain ones: a time stored in a column of its own, a struct stored as
// a JSON column, whose city is required whenever the address is written,
// and the fields of an embedded struct of the model's own, promoted to keys
// of the model's own; and an integer narrower than a JSON number, for a
// value a field cannot hold.
type shapedSample struct {
	Name    string        `json:"name"`
	Rank    int8          `json:"rank"`
	DueAt   time.Time     `json:"due_at"`
	Address sampleAddress `json:"address" gorm:"serializer:json"`
	SampleAudit

	modelregistry.Base
}

func (shapedSample) TableName() string { return "controller_shaped_samples" }

// datedSample carries a calendar date, stored in a date column, beside an
// optional instant: a date the client sends with its own offset is read as
// the UTC day on both transports, in a record and in a filter alike, and a
// filter on either column is checked against the value the column holds.
type datedSample struct {
	Name     string         `json:"name"`
	Day      datatypes.Date `json:"day"`
	ClosedAt *time.Time     `json:"closed_at"`

	modelregistry.Query
	modelregistry.Base
}

func (datedSample) TableName() string { return "controller_dated_samples" }

// sampleAddress is the struct value of a shaped sample.
type sampleAddress struct {
	City string `json:"city" binding:"required"`
	Zip  string `json:"zip"`
}

// SampleAudit is embedded in a shaped sample, its fields promoted.
type SampleAudit struct {
	Reviewer string `json:"reviewer"`
	Reviewed bool   `json:"reviewed"`
}

// The routes the fixture services are registered under. A handler mounted
// with one of them as its config route resolves that route's service; any
// other route resolves none and runs on the framework's default service.
const (
	sampleRoute        = "controller-samples"
	shapedRoute        = "controller-shaped-samples"
	refusalRoute       = "controller-refusals"
	filterRefusalRoute = "controller-filter-refusals"
	importRoute        = "controller-imports"
	refusedImportRoute = "controller-refused-imports"
	counterRoute       = "controller-counters"
	versionedRoute     = "controller-versioned-samples"
	validatedRoute     = "controller-validated-samples"
	datedRoute         = "controller-dated-samples"
	observedRoute      = "controller-observed-samples"
	cookieBeforeRoute  = "controller-cookie-before-samples"
	cookieAfterRoute   = "controller-cookie-after-samples"
	actionRoute        = "controller-sample-actions"
	watchRoute         = "controller-sample-watches"
	uploadRoute        = "controller-sample-uploads"
	chatRoute          = "controller-sample-chats"
	silentRoute        = "controller-sample-silences"
	forkRoute          = "controller-sample-forks"
)

// registerFixtureServices registers the fixture services once for the test
// binary: the service registry refuses a second registration of a route and
// phase, which registering per test would attempt under -count.
func registerFixtureServices() {
	for _, phase := range []consts.Phase{
		consts.Create, consts.Delete, consts.Update, consts.Patch, consts.List,
		consts.CreateMany, consts.DeleteMany, consts.UpdateMany, consts.PatchMany,
	} {
		serviceregistry.RegisterInstance[*sampleRecord, *sampleRecord, *sampleRecord](phase, refusalRoute, &refusingService{})
	}
	serviceregistry.RegisterInstance[*sampleRecord, *sampleRecord, *sampleRecord](consts.List, filterRefusalRoute, &filterRefusingService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleRecord, *sampleRecord](consts.Import, importRoute, &importingService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleRecord, *sampleRecord](consts.Import, refusedImportRoute, &refusingService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleRecord, *sampleRecord](consts.Create, observedRoute, &observingService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleRecord, *sampleRecord](consts.Create, cookieBeforeRoute, &cookieBeforeService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleRecord, *sampleRecord](consts.Create, cookieAfterRoute, &cookieAfterService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.Create, actionRoute, &actionService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.List, actionRoute, &actionService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.Stream, watchRoute, &watchService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.Stream, uploadRoute, &uploadService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.Stream, chatRoute, &chatService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.Stream, silentRoute, &actionService{})
	serviceregistry.RegisterInstance[*sampleRecord, *sampleActionReq, *sampleActionRsp](consts.Stream, forkRoute, forkingChat)
}

// cookieBeforeService and cookieAfterService set a cookie, which only an
// HTTP response carries, in the hook before and the hook after the record
// is written, for the calls that pin where a call refuses that.
type cookieBeforeService struct {
	serviceregistry.Base[*sampleRecord, *sampleRecord, *sampleRecord]
}

func (*cookieBeforeService) CreateBefore(sc *types.ServiceContext, _ *sampleRecord) error {
	sc.SetCookie(&http.Cookie{Name: "probe", Value: "set"})
	return nil
}

type cookieAfterService struct {
	serviceregistry.Base[*sampleRecord, *sampleRecord, *sampleRecord]
}

func (*cookieAfterService) CreateAfter(sc *types.ServiceContext, _ *sampleRecord) error {
	sc.SetCookie(&http.Cookie{Name: "probe", Value: "set"})
	return nil
}

// refusedMsg is what the refusing services answer every refused request with.
const refusedMsg = "sample refused"

// refusingService refuses every write in its before hooks, the way a service
// turns down a request its business rules forbid, and refuses listings in
// both of the list hooks.
type refusingService struct {
	serviceregistry.Base[*sampleRecord, *sampleRecord, *sampleRecord]
}

func refusal() error { return types.NewError(http.StatusConflict, refusedMsg) }

func (*refusingService) CreateBefore(*types.ServiceContext, *sampleRecord) error { return refusal() }

func (*refusingService) DeleteBefore(*types.ServiceContext, *sampleRecord) error { return refusal() }

func (*refusingService) UpdateBefore(*types.ServiceContext, *sampleRecord) error { return refusal() }

func (*refusingService) PatchBefore(*types.ServiceContext, *sampleRecord) error { return refusal() }

func (*refusingService) ListBefore(*types.ServiceContext, *[]*sampleRecord) error {
	return refusal()
}

func (*refusingService) CreateManyBefore(*types.ServiceContext, ...*sampleRecord) error {
	return refusal()
}

func (*refusingService) DeleteManyBefore(*types.ServiceContext, ...*sampleRecord) error {
	return refusal()
}

func (*refusingService) UpdateManyBefore(*types.ServiceContext, ...*sampleRecord) error {
	return refusal()
}

func (*refusingService) PatchManyBefore(*types.ServiceContext, ...*sampleRecord) error {
	return refusal()
}

func (*refusingService) Import(*types.ServiceContext, io.Reader) ([]*sampleRecord, error) {
	return nil, refusal()
}

// filterRefusingService lets a listing through its before hook and refuses
// it in Filter, the hook a service scopes a listing to what the caller may
// see in.
type filterRefusingService struct {
	serviceregistry.Base[*sampleRecord, *sampleRecord, *sampleRecord]
}

func (*filterRefusingService) Filter(_ *types.ServiceContext, m *sampleRecord, opts types.QueryOptions) (*sampleRecord, types.QueryOptions, error) {
	return m, opts, refusal()
}

// importingService parses an uploaded file holding a JSON array of samples,
// leaving their persistence to the import handler.
type importingService struct {
	serviceregistry.Base[*sampleRecord, *sampleRecord, *sampleRecord]
}

func (*importingService) Import(_ *types.ServiceContext, r io.Reader) ([]*sampleRecord, error) {
	var records []*sampleRecord
	if err := json.NewDecoder(r).Decode(&records); err != nil {
		return nil, types.NewError(http.StatusBadRequest, "malformed sample file")
	}
	return records, nil
}

// observedCall is what a service saw on its service context: the box route
// parameter, the caller, the method and route the request or call carried,
// and whether the action requires authentication.
type observedCall struct {
	Box          string
	Username     string
	Method       string
	Route        string
	RequiresAuth bool
	Query        url.Values
}

// observe reads what sc answers into an observedCall.
func observe(sc *types.ServiceContext) observedCall {
	return observedCall{
		Box:          sc.Param("box"),
		Username:     sc.Username(),
		Method:       sc.Method(),
		Route:        sc.Route(),
		RequiresAuth: sc.RequiresAuth(),
		Query:        sc.Query(),
	}
}

// The call the observing service's hook saw last, for the tests to read
// back what a flow's hooks find on their service context.
var (
	observedMu   sync.Mutex
	lastObserved observedCall
)

// observingService records what its CreateBefore hook finds on the service
// context, the way a hook reads the request it runs for.
type observingService struct {
	serviceregistry.Base[*sampleRecord, *sampleRecord, *sampleRecord]
}

func (*observingService) CreateBefore(sc *types.ServiceContext, _ *sampleRecord) error {
	observedMu.Lock()
	defer observedMu.Unlock()
	lastObserved = observe(sc)
	return nil
}

// sampleActionReq and sampleActionRsp are the payload and result of the
// sample's custom action, what a service declaring Payload and Result of its
// own takes and answers; the note is required, so the payload is validated
// like any request body.
type (
	sampleActionReq struct {
		Note string `json:"note" binding:"required"`
	}
	sampleActionRsp struct {
		Note string
		observedCall
	}
)

// The notes that make the action service misbehave on purpose.
const (
	actionRefuse = "refuse"
	actionBreak  = "break"
	actionWrite  = "write"
	actionRead   = "read"
	actionHang   = "hang"
)

// actionEntered is signaled once the action service hangs in a call (see
// actionHang), for the test canceling that call to know it is in.
var actionEntered = make(chan struct{}, 1)

// actionService serves the sample's custom action: it answers what it found
// on the service context, refuses with a service error for actionRefuse,
// fails with a plain error for actionBreak, writes a raw response, which
// only HTTP can carry, for actionWrite, reads a form value, which only an
// HTTP request carries, for actionRead, and hangs until the call ends for
// actionHang, returning the error the context reports. Its List answers the
// same, for the query a GET action reads.
type actionService struct {
	serviceregistry.Base[*sampleRecord, *sampleActionReq, *sampleActionRsp]
}

func (*actionService) Create(sc *types.ServiceContext, req *sampleActionReq) (*sampleActionRsp, error) {
	switch req.Note {
	case actionRefuse:
		return nil, types.NewError(http.StatusForbidden, "not yours")
	case actionBreak:
		return nil, errors.New("dial tcp: connection refused")
	case actionWrite:
		sc.Data(http.StatusOK, "text/plain", []byte("plain"))
	case actionRead:
		_ = sc.PostForm("name")
	case actionHang:
		actionEntered <- struct{}{}
		<-sc.Done()
		return nil, sc.Err()
	}
	return &sampleActionRsp{Note: req.Note, observedCall: observe(sc)}, nil
}

func (*actionService) List(sc *types.ServiceContext, req *sampleActionReq) (*sampleActionRsp, error) {
	return &sampleActionRsp{Note: req.Note, observedCall: observe(sc)}, nil
}

// configFor is the controller config a handler is mounted with: the route
// the service registry resolves the phase service by, and the id parameter
// of the single-resource paths.
func configFor[M types.Model](route string) *types.ControllerConfig[M] {
	return &types.ControllerConfig[M]{ParamName: "id", Route: route}
}

// serve mounts handler on a fresh engine under method and pattern, sends it
// one request for target, anonymous, with body as its JSON body unless body
// is empty, and returns the recorded response.
func serve(t *testing.T, method, pattern string, handler gin.HandlerFunc, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	return serveAs(t, method, pattern, handler, target, body, "")
}

// serveAs is serve for a request of the user named username, the caller
// the identity middleware would have named, whom the flows record as the
// creator and updater; "" sends the request anonymous.
func serveAs(t *testing.T, method, pattern string, handler gin.HandlerFunc, target, body, username string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Registered routes read their id through the route parameter names
	// the framework's route middleware sets, as router.Register records them.
	middleware.RouteManager.Add(pattern)
	engine := gin.New()
	engine.Handle(method, pattern, func(c *gin.Context) {
		c.Set(consts.PARAMS, middleware.RouteManager.Get(c.FullPath()))
		if username != "" {
			c.Set(consts.CTX_USERNAME, username)
			c.Set(consts.CTX_USER_ID, "u-1")
		}
	}, handler)

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

// createSample stores a sample named name and returns it with its id.
func createSample(t *testing.T, name string) *sampleRecord {
	t.Helper()
	record := &sampleRecord{Name: name}
	require.NoError(t, database.Database[*sampleRecord](context.Background()).Create(record))
	require.NotEmpty(t, record.GetID())
	return record
}

// requireSampleName requires the stored sample id to be named name.
func requireSampleName(t *testing.T, id, name string) {
	t.Helper()
	require.Equal(t, name, loadSample(t, id).Name)
}

// loadSample returns the stored sample id names.
func loadSample(t *testing.T, id string) *sampleRecord {
	t.Helper()
	stored := new(sampleRecord)
	require.NoError(t, database.Database[*sampleRecord](context.Background()).Get(stored, id))
	return stored
}

// createDated stores a dated sample named name on the UTC day and returns it
// with its id.
func createDated(t *testing.T, name string, day time.Time) *datedSample {
	t.Helper()
	record := &datedSample{Name: name, Day: datatypes.Date(day)}
	require.NoError(t, database.Database[*datedSample](context.Background()).Create(record))
	require.NotEmpty(t, record.GetID())
	return record
}

// requireDatedDay requires the dated sample id to hold want as its day.
func requireDatedDay(t *testing.T, id string, want time.Time) {
	t.Helper()

	record := new(datedSample)
	require.NoError(t, database.Database[*datedSample](context.Background()).Get(record, id))
	require.True(t, time.Time(record.Day).Equal(want), "stored day %s, want %s", time.Time(record.Day), want)
}

// shapedDueAt is the time the shaped samples are stored with, to the second
// so that every dialect stores it as is.
var shapedDueAt = time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)

// createShaped stores a shaped sample named name, due at shapedDueAt, at the
// city "old" with the zip "1", reviewed by "first", and returns it with its
// id.
func createShaped(t *testing.T, name string) *shapedSample {
	t.Helper()
	record := &shapedSample{Name: name, DueAt: shapedDueAt, Address: sampleAddress{City: "old", Zip: "1"}, Reviewer: "first", Reviewed: true}
	require.NoError(t, database.Database[*shapedSample](context.Background()).Create(record))
	require.NotEmpty(t, record.GetID())
	return record
}

// loadShaped returns the stored shaped sample id names.
func loadShaped(t *testing.T, id string) *shapedSample {
	t.Helper()
	stored := new(shapedSample)
	require.NoError(t, database.Database[*shapedSample](context.Background()).Get(stored, id))
	return stored
}

// uniqueName returns prefix followed by a suffix no earlier call returned,
// so a test that counts the rows of a name counts only its own, however many
// times the test binary runs it.
func uniqueName(prefix string) string {
	return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// countSamplesNamed counts the stored samples named name.
func countSamplesNamed(t *testing.T, name string) int {
	t.Helper()
	var total int
	require.NoError(t, database.Database[*sampleRecord](context.Background()).
		WithQuery(&sampleRecord{Name: name}).Count(&total))
	return total
}

// countShapedNamed counts the stored shaped samples named name.
func countShapedNamed(t *testing.T, name string) int {
	t.Helper()
	var total int
	require.NoError(t, database.Database[*shapedSample](context.Background()).
		WithQuery(&shapedSample{Name: name}).Count(&total))
	return total
}

// watchService, uploadService, chatService and forkingChatService are the
// stream fixture services, one per kind of stream, each answering what it
// found on the service context beside its notes. watchService streams as
// many responses as the request's note counts, numbered from 0, refusing
// and failing for the notes the action service does; uploadService reads
// the requests until the client is done and answers their notes joined;
// chatService echoes each request as it comes; forkingChatService receives
// on a goroutine of its own. The silent route registers the action
// service, which streams nothing, for the call to refuse.
type watchService struct {
	serviceregistry.Base[*sampleRecord, *sampleActionReq, *sampleActionRsp]
}

func (*watchService) Stream(sc *types.ServiceContext, req *sampleActionReq, stream *types.ServerStream[*sampleActionRsp]) error {
	switch req.Note {
	case actionRefuse:
		return types.NewError(http.StatusForbidden, "not yours")
	case actionBreak:
		return errors.New("dial tcp: connection refused")
	}
	count, _ := strconv.Atoi(req.Note)
	for i := range count {
		if err := stream.Send(&sampleActionRsp{Note: strconv.Itoa(i), observedCall: observe(sc)}); err != nil {
			return err
		}
	}
	return nil
}

type uploadService struct {
	serviceregistry.Base[*sampleRecord, *sampleActionReq, *sampleActionRsp]
}

func (*uploadService) Stream(sc *types.ServiceContext, stream *types.ClientStream[*sampleActionReq]) (*sampleActionRsp, error) {
	var notes []string
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return &sampleActionRsp{Note: strings.Join(notes, ","), observedCall: observe(sc)}, nil
		}
		if err != nil {
			// Wrapped the way a project's service wraps a failed read: what
			// the client is answered must not depend on it.
			return nil, errors.Wrap(err, "failed to read the request")
		}
		notes = append(notes, req.Note)
	}
}

type chatService struct {
	serviceregistry.Base[*sampleRecord, *sampleActionReq, *sampleActionRsp]
}

func (*chatService) Stream(sc *types.ServiceContext, stream *types.BidiStream[*sampleActionReq, *sampleActionRsp]) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "failed to read the request")
		}
		if err := stream.Send(&sampleActionRsp{Note: "echo " + req.Note, observedCall: observe(sc)}); err != nil {
			return err
		}
	}
}

// forkingChatService receives on a goroutine of its own, which grpc-go
// allows beside the goroutine the service runs on, and returns a moment
// later without waiting for it: the refusal its read records (see
// controller.requests) is then written on one goroutine and read by the
// call on another with time alone between the two, nothing ordering them,
// which is what the lock on it is for. A channel or a wait here would
// order the two and hide the race, and so does ending the stream from the
// client once the read is done: the cancellation reaches the service
// through the transport, which orders it after the read. The test reads
// what the goroutine got through read.
type forkingChatService struct {
	serviceregistry.Base[*sampleRecord, *sampleActionReq, *sampleActionRsp]
	read chan error
}

var forkingChat = &forkingChatService{read: make(chan error, 1)}

func (s *forkingChatService) Stream(_ *types.ServiceContext, stream *types.BidiStream[*sampleActionReq, *sampleActionRsp]) error {
	go func() {
		_, err := stream.Recv()
		s.read <- err
	}()
	time.Sleep(time.Second)
	return nil
}
