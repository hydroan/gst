package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/router"
	"github.com/hydroan/gst/testutil"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"github.com/stretchr/testify/require"
)

// pagedRecordRoute is documented but never requested: it exists so the document
// contains one List route whose model embeds the framework query structs, which
// is what puts their query parameters in the document.
const pagedRecordRoute = "paged-records"

type pagedRecord struct {
	Name string `json:"name" query:"name"`

	model.Query
	model.Base
}

// registerOpenAPIDocRoutes registers that route; TestMain calls it after
// router.Init, mirroring where generated route registration runs.
func registerOpenAPIDocRoutes() {
	router.Register[*pagedRecord, *pagedRecord, *pagedRecord](
		router.Auth(), pagedRecordRoute, &types.ControllerConfig[*pagedRecord]{}, consts.List,
	)
}

// openAPIDocument is the part of the served document these tests read.
type openAPIDocument struct {
	OpenAPI string `json:"openapi"`
	Info    struct {
		Title   string `json:"title"`
		Version string `json:"version"`
	} `json:"info"`
	Paths map[string]map[string]struct {
		Summary    string `json:"summary"`
		Parameters []struct {
			Name        string `json:"name"`
			In          string `json:"in"`
			Description string `json:"description"`
		} `json:"parameters"`
	} `json:"paths"`
}

// TestOpenAPIDocumentIsBuiltOnRequest covers the document endpoint end to end.
// Registration only queues the routes, so this is what proves the queue is
// actually drained, and it is the only test that runs the generator the way a
// deployed server runs it.
func TestOpenAPIDocumentIsBuiltOnRequest(t *testing.T) {
	doc := requireOpenAPIDocument(t)

	require.NotEmpty(t, doc.OpenAPI, "the document has to declare its OpenAPI version")
	require.NotEmpty(t, doc.Info.Version, "info.version is required by the spec")
	require.NotEmpty(t, doc.Paths, "registered routes have to reach the document")
}

// TestOpenAPIDocumentDescribesFrameworkQueryParameters pins the descriptions of
// the query parameters a model gets by embedding the framework query structs.
// They are registered at build time from the framework's own sources, so this
// also proves that chain survives into a running server.
func TestOpenAPIDocumentDescribesFrameworkQueryParameters(t *testing.T) {
	doc := requireOpenAPIDocument(t)

	described := map[string]string{}
	for _, item := range doc.Paths {
		for _, op := range item {
			for _, parameter := range op.Parameters {
				if parameter.In == "query" && parameter.Description != "" {
					described[parameter.Name] = parameter.Description
				}
			}
		}
	}

	for _, name := range []string{"_page", "_size"} {
		require.NotEmpty(t, described[name],
			"query parameter %s has to carry the doc comment registered for the framework query structs", name)
	}
}

// TestOpenAPIDocumentIsStableAcrossRequests guards the build queue: draining it
// on the first request must not leave the second request with a thinner
// document.
func TestOpenAPIDocumentIsStableAcrossRequests(t *testing.T) {
	first := requireOpenAPIDocument(t)
	second := requireOpenAPIDocument(t)

	require.Len(t, second.Paths, len(first.Paths),
		"a repeated request has to serve the same document")
	require.Equal(t, first.Info.Version, second.Info.Version)
}

// requireOpenAPIDocument fetches and decodes the served OpenAPI document.
func requireOpenAPIDocument(t *testing.T) openAPIDocument {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, testutil.BaseURL()+"/openapi.json", nil)
	require.NoError(t, err)
	req.SetBasicAuth(config.App.Auth.BaseAuthUsername, config.App.Auth.BaseAuthPassword)

	rsp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()

	body, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode, "response body: %s", body)

	var doc openAPIDocument
	require.NoError(t, json.Unmarshal(body, &doc), "response body: %s", body)
	return doc
}
