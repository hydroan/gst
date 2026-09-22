package controller_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/internal/controller"
	"github.com/stretchr/testify/require"
)

// TestImportRefusesARequestWithoutItsFile pins the 400 of an import whose
// form carries no file, and of one whose file is over MAX_IMPORT_SIZE.
func TestImportRefusesARequestWithoutItsFile(t *testing.T) {
	handler := controller.ImportFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](importRoute))

	t.Run("no file", func(t *testing.T) {
		rsp := serve(t, http.MethodPost, "/controller-imports/import", handler, "/controller-imports/import", "")

		require.Equal(t, http.StatusBadRequest, rsp.Code)
		require.Contains(t, rsp.Body.String(), "upload file is required")
	})

	t.Run("a file over the limit", func(t *testing.T) {
		rsp := upload(t, handler, bytes.Repeat([]byte(" "), controller.MAX_IMPORT_SIZE+1))

		require.Equal(t, http.StatusBadRequest, rsp.Code)
		require.Contains(t, rsp.Body.String(), "too large file")
	})
}

// TestImportCreatesAndReplacesByID pins how an import writes the rows the
// service parsed: a row without an id is created, and a row with the id of a
// stored record replaces that record.
func TestImportCreatesAndReplacesByID(t *testing.T) {
	record := createSample(t, "import-stored")
	created := uniqueName("import-created")

	rsp := upload(t, controller.ImportFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](importRoute)),
		[]byte(`[{"name":"`+created+`"},{"id":"`+record.GetID()+`","name":"import-replaced"}]`))

	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Equal(t, 1, countSamplesNamed(t, created))
	requireSampleName(t, record.GetID(), "import-replaced")
}

// TestImportWritesNothingWhenARowNamesAMissingRecord pins that an import is
// all or nothing: a row with the id of no stored record fails it with 404,
// and the row beside it is not created either.
func TestImportWritesNothingWhenARowNamesAMissingRecord(t *testing.T) {
	orphan := uniqueName("import-orphan")

	rsp := upload(t, controller.ImportFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](importRoute)),
		[]byte(`[{"name":"`+orphan+`"},{"id":"missing","name":"import-other"}]`))

	require.Equal(t, http.StatusNotFound, rsp.Code)
	require.Zero(t, countSamplesNamed(t, orphan))
}

// TestImportAnswersTheServiceRefusal pins that an import the service's Import
// refuses answers with the service's error.
func TestImportAnswersTheServiceRefusal(t *testing.T) {
	name := uniqueName("import-refused")

	rsp := upload(t, controller.ImportFactory[*sampleRecord, *sampleRecord, *sampleRecord](configFor[*sampleRecord](refusedImportRoute)),
		[]byte(`[{"name":"`+name+`"}]`))

	require.Equal(t, http.StatusConflict, rsp.Code)
	require.Contains(t, rsp.Body.String(), refusedMsg)
	require.Zero(t, countSamplesNamed(t, name))
}

// upload posts content to handler as the multipart file field an import
// reads, and returns the recorded response.
func upload(t *testing.T, handler gin.HandlerFunc, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	body := new(bytes.Buffer)
	form := multipart.NewWriter(body)
	part, err := form.CreateFormFile("file", "samples.json")
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, form.Close())

	engine := gin.New()
	engine.POST("/controller-imports/import", handler)
	req := httptest.NewRequest(http.MethodPost, "/controller-imports/import", body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}
