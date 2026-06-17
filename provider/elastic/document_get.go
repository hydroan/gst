package elastic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/util"
)

type GetRequest struct {
	Source []string // 指定返回的字段
}

type GetResult struct {
	ID     string         `json:"_id"`
	Found  bool           `json:"found"`
	Source map[string]any `json:"_source,omitempty"`
}

// Get retrieves a document from elasticsearch by index name and id
func (*document) Get(ctx context.Context, indexName string, id string, req *GetRequest) (*GetResult, error) {
	if err := _check(); err != nil {
		return nil, fmt.Errorf("elasticsearch client check: %w", err)
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
		logger.Infow("get document completed", "cost", util.FormatDurationSmart(time.Since(begin)))
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
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Errorw("failed to read response body", "error", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if res.StatusCode == http.StatusNotFound {
		return &GetResult{
			ID:    id,
			Found: false,
		}, nil
	}
	if res.IsError() {
		logger.Errorw(
			"elasticsearch error response",
			"status", res.Status(),
			"body", string(body),
		)
		return nil, fmt.Errorf("elasticsearch error [%s]: %s", res.Status(), string(body))
	}
	var result GetResult
	if err := json.Unmarshal(body, &result); err != nil {
		logger.Errorw(
			"failed to decode response",
			"error", err,
			"body", string(body),
		)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}
