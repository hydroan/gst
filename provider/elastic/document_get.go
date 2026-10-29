package elastic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/hydroan/gst/internal/logfield"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

type GetRequest struct {
	Source []string // fields to return
}

type GetResult struct {
	ID     string         `json:"_id"`
	Found  bool           `json:"found"`
	Source map[string]any `json:"_source,omitempty"`
}

// Get retrieves the document id from the index indexName. A document the
// index does not hold answers Found false; an index that does not exist is an
// error, as is every other answer the cluster refuses the request with.
func (*document) Get(ctx context.Context, indexName string, id string, req *GetRequest) (*GetResult, error) {
	if err := _check(); err != nil {
		return nil, errors.Wrap(err, "elasticsearch client check")
	}
	if indexName == "" || id == "" {
		return nil, errors.New("invalid parameters: indexName or id is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	begin := time.Now()
	logger := logger.Elastic.With("index", indexName, "id", id)
	defer func() {
		logger.Infow("get document completed", logfield.Duration(time.Since(begin)))
	}()

	opts := []func(*esapi.GetRequest){client.Get.WithContext(ctx)}
	if req != nil {
		if len(req.Source) > 0 {
			opts = append(opts, client.Get.WithSourceIncludes(req.Source...))
		}
	}

	res, err := client.Get(indexName, id, opts...)
	if err != nil {
		logger.Errorw("failed to get document", "error", err)
		return nil, errors.Wrap(err, "failed to get document")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Errorw("failed to read response body", "error", err)
		return nil, errors.Wrap(err, "failed to read response body")
	}
	// A missing document and a missing index both answer 404, and the body
	// alone tells them apart: found false names the document.
	if res.StatusCode == http.StatusNotFound {
		var missing struct {
			Found *bool `json:"found"`
		}
		if json.Unmarshal(body, &missing) == nil && missing.Found != nil && !*missing.Found {
			return &GetResult{
				ID:    id,
				Found: false,
			}, nil
		}
	}
	if res.IsError() {
		logger.Errorz(
			"elasticsearch error response",
			logfield.Status(res.StatusCode),
			zap.String("body", string(body)),
		)
		return nil, errors.Newf("elasticsearch error [%s]: %s", res.Status(), string(body))
	}
	var result GetResult
	if err := json.Unmarshal(body, &result); err != nil {
		logger.Errorw(
			"failed to decode response",
			"error", err,
			"body", string(body),
		)
		return nil, errors.Wrap(err, "failed to decode response")
	}
	return &result, nil
}
