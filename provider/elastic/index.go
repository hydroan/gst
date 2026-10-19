package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/cockroachdb/errors"
)

// IndexOption carries the settings and mappings Create gives a new index.
type IndexOption struct {
	Settings map[string]any
	Mappings map[string]any
}

// Create creates the index indexName, with the settings and mappings of the
// first option when one is given. An index of that name already existing is
// an error.
//
//	err := elastic.Index.Create(ctx, "sample", &elastic.IndexOption{
//	    Settings: map[string]any{"number_of_shards": 1},
//	    Mappings: map[string]any{"properties": map[string]any{
//	        "title": map[string]any{"type": "text"},
//	    }},
//	})
func (*index) Create(ctx context.Context, indexName string, options ...*IndexOption) error {
	if err := _check(); err != nil {
		return errors.Wrap(err, "elasticsearch client check")
	}
	body := make(map[string]any)
	if len(options) > 0 && options[0] != nil {
		if options[0].Settings != nil {
			body["settings"] = options[0].Settings
		}
		if options[0].Mappings != nil {
			body["mappings"] = options[0].Mappings
		}
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return errors.Wrap(err, "error marshaling index body")
	}

	res, err := client.Indices.Create(
		indexName,
		client.Indices.Create.WithBody(bytes.NewReader(bodyJSON)),
		client.Indices.Create.WithContext(ctx),
	)
	if err != nil {
		return errors.Wrap(err, "error creating index")
	}
	defer res.Body.Close()
	if res.IsError() {
		return errors.Newf("error creating index: %s", res.String())
	}
	return nil
}

// Exists reports whether the index indexName exists. The cluster answers
// found or not found; any other answer, a refused request say, is an error
// rather than an index that is not there.
func (*index) Exists(ctx context.Context, indexName string) (bool, error) {
	if err := _check(); err != nil {
		return false, errors.Wrap(err, "elasticsearch client check")
	}
	res, err := client.Indices.Exists([]string{indexName}, client.Indices.Exists.WithContext(ctx))
	if err != nil {
		return false, errors.Wrap(err, "error checking index existence")
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, errors.Newf("error checking index existence: %s", res.Status())
	}
}

// Delete deletes the index indexName. An index of that name not existing is
// an error.
func (*index) Delete(ctx context.Context, indexName string) error {
	if err := _check(); err != nil {
		return errors.Wrap(err, "elasticsearch client check")
	}
	res, err := client.Indices.Delete([]string{indexName}, client.Indices.Delete.WithContext(ctx))
	if err != nil {
		return errors.Wrap(err, "error deleting index")
	}
	defer res.Body.Close()
	if res.IsError() {
		return errors.Newf("error deleting index: %s", res.String())
	}
	return nil
}
