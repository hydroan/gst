package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/util"
)

// Documenter is a value BulkIndex writes into an index: its fields and the id
// it is stored under. A model embedding model.Base implements GetID already.
type Documenter interface {
	// Document returns the fields to index, keyed by field name. The values
	// must encode as JSON, and the names should match the index mapping.
	//
	//	return map[string]any{"title": "sample", "tags": []string{"a", "b"}}
	Document() map[string]any

	// GetID returns the id the document is stored under, unique within the
	// index. An empty id leaves it to the cluster, which generates one.
	GetID() string
}

// BulkIndex writes docs into the index indexName in one bulk request, each
// under its id: a document already stored under that id is replaced. A
// document the cluster refuses fails the call, naming the first refusal. No
// documents is no request.
func (*document) BulkIndex(ctx context.Context, indexName string, docs ...Documenter) error {
	if err := _check(); err != nil {
		return errors.Wrap(err, "elasticsearch client check")
	}
	if len(docs) == 0 {
		return nil
	}
	start := time.Now()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, doc := range docs {
		action := map[string]string{}
		if id := doc.GetID(); id != "" {
			action["_id"] = id
		}
		if err := enc.Encode(map[string]any{"index": action}); err != nil {
			return errors.Wrap(err, "failed to encode bulk action")
		}
		if err := enc.Encode(doc.Document()); err != nil {
			// Wrap keeps the cause on the unwrap chain and embeds the run-time
			// stack; see the error-stack contract in the database package doc.
			err = errors.Wrap(err, "failed to marshal document")
			logger.Elastic.Error(err)
			return err
		}
	}

	res, err := client.Bulk(&buf, client.Bulk.WithIndex(indexName), client.Bulk.WithContext(ctx))
	if err != nil {
		err = errors.Wrap(err, "failed to execute bulk request")
		logger.Elastic.Error(err)
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		var raw map[string]any
		if err = json.NewDecoder(res.Body).Decode(&raw); err != nil {
			err = errors.Wrap(err, "failed to parse response body")
			logger.Elastic.Error(err)
			return err
		}
		err = errors.Newf("failed to execute bulk request: %v", raw)
		logger.Elastic.Error(err)
		return err
	}

	var reply struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Error map[string]any `json:"error"`
		} `json:"items"`
	}
	if err = json.NewDecoder(res.Body).Decode(&reply); err != nil {
		err = errors.Wrap(err, "failed to parse response body")
		logger.Elastic.Error(err)
		return err
	}
	if reply.Errors {
		for _, item := range reply.Items {
			if result, ok := item["index"]; ok && result.Error != nil {
				err = errors.Newf("error in item: %v", result.Error)
				logger.Elastic.Error(err)
				return err
			}
		}
	}
	logger.Elastic.Infow("successfully indexed documents", "length", len(docs), util.LogDuration(time.Since(start)))
	return nil
}
