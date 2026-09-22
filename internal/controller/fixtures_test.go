package controller_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/serviceregistry"
	"github.com/hydroan/gst/internal/types"
	"github.com/stretchr/testify/require"
)

// sampleRecord is the table model the handler tests read and write, keyed by
// a string id.
type sampleRecord struct {
	Name string `json:"name"`

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
	Name    string                `json:"name"`
	Version modelregistry.Version `json:"version,omitempty" gorm:"not null;default:1"`

	modelregistry.Base
}

func (versionedSample) TableName() string { return "controller_versioned_samples" }

// The routes the fixture services are registered under. A factory mounted
// with one of them as its config route resolves that route's service; any
// other route resolves none and runs on the framework's default service.
const (
	sampleRoute        = "controller-samples"
	refusalRoute       = "controller-refusals"
	filterRefusalRoute = "controller-filter-refusals"
	importRoute        = "controller-imports"
	refusedImportRoute = "controller-refused-imports"
	counterRoute       = "controller-counters"
	versionedRoute     = "controller-versioned-samples"
)

// registerFixtureServices registers the fixture services once for the test
// binary: the service registry refuses a second registration of a route and
// phase, which registering per test would attempt under -count.
func registerFixtureServices() {
	for _, phase := range []consts.Phase{
		consts.PHASE_CREATE, consts.PHASE_DELETE, consts.PHASE_UPDATE, consts.PHASE_PATCH, consts.PHASE_LIST,
		consts.PHASE_CREATE_MANY, consts.PHASE_DELETE_MANY, consts.PHASE_UPDATE_MANY,
	} {
		serviceregistry.Register[*sampleRecord, *sampleRecord, *sampleRecord](phase, refusalRoute, &refusingService{})
	}
	serviceregistry.Register[*sampleRecord, *sampleRecord, *sampleRecord](consts.PHASE_LIST, filterRefusalRoute, &filterRefusingService{})
	serviceregistry.Register[*sampleRecord, *sampleRecord, *sampleRecord](consts.PHASE_IMPORT, importRoute, &importingService{})
	serviceregistry.Register[*sampleRecord, *sampleRecord, *sampleRecord](consts.PHASE_IMPORT, refusedImportRoute, &refusingService{})
}

// refusedMsg is what the refusing services answer every refused request with.
const refusedMsg = "sample refused"

// refusingService refuses every write in its before hooks, the way a service
// turns down a request its business rules forbid, and refuses listings in
// both of the list hooks.
type refusingService struct {
	serviceregistry.Base[*sampleRecord, *sampleRecord, *sampleRecord]
}

func refusal() error { return serviceregistry.NewError(http.StatusConflict, refusedMsg) }

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
		return nil, serviceregistry.NewError(http.StatusBadRequest, "malformed sample file")
	}
	return records, nil
}

// configFor is the controller config a factory is mounted with: the route
// the service registry resolves the phase service by, and the id parameter
// of the single-resource paths.
func configFor[M types.Model](route string) *types.ControllerConfig[M] {
	return &types.ControllerConfig[M]{ParamName: "id", Route: route}
}

// serve mounts handler on a fresh engine under method and pattern, sends it
// one request for target, with body as its JSON body unless body is empty,
// and returns the recorded response.
func serve(t *testing.T, method, pattern string, handler gin.HandlerFunc, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Registered routes read their id through the route parameter names
	// the framework's route middleware sets, as router.Register records them.
	middleware.RouteManager.Add(pattern)
	engine := gin.New()
	engine.Handle(method, pattern, func(c *gin.Context) {
		c.Set(consts.PARAMS, middleware.RouteManager.Get(c.FullPath()))
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
	stored := new(sampleRecord)
	require.NoError(t, database.Database[*sampleRecord](context.Background()).Get(stored, id))
	require.Equal(t, name, stored.Name)
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
