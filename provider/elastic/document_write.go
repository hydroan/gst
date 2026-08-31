package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/util"
)

func (*document) BulkIndex(_ context.Context, indexName string, docs ...types.ESDocumenter) error {
	var (
		buf  bytes.Buffer
		raw  map[string]any
		data []byte
		res  *esapi.Response
		err  error
	)
	start := time.Now()

	// iterate over the documents
	for i := range docs {
		meta := fmt.Appendf(nil, `{ "index" : { "_id" : "%s" } }%s`, docs[i].GetID(), "\n")
		if data, err = json.Marshal(docs[i].Document()); err != nil {
			// Wrap keeps the cause on the unwrap chain (the string
			// concatenation it replaces dropped it) and embeds the run-time
			// stack; see the error-stack contract in the database package doc.
			err = errors.Wrap(err, "failed to marshaling document")
			logger.Elastic.Error(err)
			return err
		}
		data = append(data, "\n"...)
		buf.Grow(len(meta) + len(data))
		buf.Write(meta)
		buf.Write(data)
	}

	// execute the bulk request
	res, err = client.Bulk(bytes.NewReader(buf.Bytes()), client.Bulk.WithIndex(indexName))
	if err != nil {
		err = errors.Wrap(err, "failed to execute bulk request")
		logger.Elastic.Error(err)
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		if err = json.NewDecoder(res.Body).Decode(&raw); err != nil {
			err = errors.Wrap(err, "failed to parse response body")
			logger.Elastic.Error(err)
			return err
		}
		err = errors.Newf("failed to execute bulk request: %v", raw)
		logger.Elastic.Error(err)
		return err
	}

	var blk map[string]any
	if err = json.NewDecoder(res.Body).Decode(&blk); err != nil {
		err = errors.Wrap(err, "failed to parse response body")
		logger.Elastic.Error(err)
		return err
	}
	if blk["errors"].(bool) { //nolint:errcheck
		for _, item := range blk["items"].([]any) { //nolint:errcheck
			if idx, ok := item.(map[string]any)["index"].(map[string]any); ok {
				if idx["error"] != nil {
					err = errors.Newf("error in item: %v", idx["error"])
					logger.Elastic.Error(err)
					return err
				}
			}
		}
	}
	logger.Elastic.Infow("successfully indexed documents", "length", len(docs), util.LogDuration(time.Since(start)))
	return nil
}
