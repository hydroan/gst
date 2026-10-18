package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/logfield"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap"
)

// SearchRequest is an Elasticsearch search body: the query, the page, the
// sort, the fields to return, search_after for deep pagination and the
// aggregations. QueryBuilder composes one. What a request leaves unset is
// left to the cluster, which ranks unsorted hits by relevance and answers ten
// of them.
type SearchRequest struct {
	Query map[string]any `json:"query,omitempty"`
	From  int            `json:"from,omitempty"`
	// Size is the number of hits to return. Zero asks for none, the shape of
	// a request that wants aggregations alone; nil leaves the cluster's
	// default of ten.
	Size        *int             `json:"size,omitempty"`
	Sort        []map[string]any `json:"sort,omitempty"`
	Source      []string         `json:"_source,omitempty"`
	SearchAfter []any            `json:"search_after,omitempty"`
	Aggs        map[string]any   `json:"aggs,omitempty"`
	// TrackTotalHits asks the cluster to count every hit. Without it the
	// cluster stops counting at ten thousand, and SearchResult.TotalRelation
	// says whether it did.
	TrackTotalHits bool `json:"track_total_hits,omitempty"`
}

// validate reports a request the cluster would refuse or answer with
// something other than what was asked: a negative size or from, and
// search_after without the sort whose values it continues from.
func (r *SearchRequest) validate() error {
	if r.Size != nil && *r.Size < 0 {
		return errors.New("size cannot be negative")
	}
	if r.From < 0 {
		return errors.New("from cannot be negative")
	}
	if len(r.SearchAfter) > 0 {
		if len(r.Sort) == 0 {
			return errors.New("search_after requires sort")
		}
		if r.From != 0 {
			return errors.New("from must be 0 when using search_after")
		}
	}
	return nil
}

// TotalRelation says how SearchResult.Total relates to the number of hits.
type TotalRelation string

const (
	// TotalRelationEq means Total is the number of hits.
	TotalRelationEq TotalRelation = "eq"
	// TotalRelationGte means at least Total hits matched: the cluster
	// stopped counting at its threshold of ten thousand. A request with
	// TrackTotalHits gets the exact count.
	TotalRelationGte TotalRelation = "gte"
)

// SearchResult is an Elasticsearch search response: the total with how exact
// it is, the top score, the hits and the aggregations.
type SearchResult struct {
	Total         int64                         `json:"total"`
	TotalRelation TotalRelation                 `json:"total_relation"`
	MaxScore      *float64                      `json:"max_score,omitempty"`
	Hits          []SearchHit                   `json:"hits"`
	Aggregations  map[string]*AggregationResult `json:"aggregations,omitempty"`
}

// SearchHit represents a single document in search results.
// It contains document ID, relevance score and source fields.
type SearchHit struct {
	ID     string         `json:"_id"`
	Score  *float64       `json:"_score,omitempty"`
	Source map[string]any `json:"_source"`
}

// AggregationResult represents the result of an aggregation.
// It can contain single value metrics (min, max, avg, sum) or bucket results (terms).
type AggregationResult struct {
	Value    any              `json:"value,omitempty"`     // For metric aggregations
	DocCount int64            `json:"doc_count,omitempty"` // Document count
	Buckets  []map[string]any `json:"buckets,omitempty"`   // For bucket aggregations
}

// SearchPrev searches for N previous hits before the current search_after position.
// It will:
// 1. Reverse all sort orders (asc -> desc, desc -> asc)
// 2. Execute the search with the reversed sort orders
// 3. Reverse the result hits to maintain the original order
// Note: This will override the size parameter in QueryBuilder.
func (d *document) SearchPrev(ctx context.Context, indexName string, req *SearchRequest, size int) (*SearchResult, error) {
	// Reverse all sort orders
	for _, sort := range req.Sort {
		for _, value := range sort {
			if orderMap, ok := value.(map[string]any); ok {
				if order, ok := orderMap["order"].(string); ok {
					if order == "asc" {
						orderMap["order"] = "desc"
					} else {
						orderMap["order"] = "asc"
					}
				}
				if order, ok := orderMap["order"].(Order); ok {
					if order == Asc {
						orderMap["order"] = Desc
					} else {
						orderMap["order"] = Asc
					}
				}
			}
		}
	}
	// Set pagination parameters
	req.From = 0
	req.Size = &size
	result, err := d.Search(ctx, indexName, req)
	if err != nil {
		return nil, err
	}
	// Reverse result order
	for i, j := 0, len(result.Hits)-1; i < j; i, j = i+1, j-1 {
		result.Hits[i], result.Hits[j] = result.Hits[j], result.Hits[i]
	}

	return result, nil
}

// SearchNext searches for N next hits after the current search_after position.
// It performs a forward search using the current sort orders.
// Note: This will override the size parameter in QueryBuilder.
func (d *document) SearchNext(ctx context.Context, indexName string, req *SearchRequest, size int) (*SearchResult, error) {
	req.Size = &size
	req.From = 0
	return d.Search(ctx, indexName, req)
}

// Search runs req against indexName. The request goes to the cluster as it
// is: no sort means relevance order, no size means the cluster's default
// page, and the total is counted up to the cluster's threshold unless
// TrackTotalHits asks for all of it, see SearchResult.TotalRelation. A
// request the cluster would refuse, see validate, fails before it is sent.
func (*document) Search(ctx context.Context, indexName string, req *SearchRequest) (*SearchResult, error) {
	if err := _check(); err != nil {
		return nil, errors.Wrap(err, "elasticsearch client check")
	}
	if indexName == "" {
		return nil, errors.New("index name cannot be empty")
	}
	if err := req.validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	size := "default"
	if req.Size != nil {
		size = strconv.Itoa(*req.Size)
	}
	begin := time.Now()
	logger := logger.Elastic.With(
		"index", indexName,
		"from", strconv.Itoa(req.From),
		"size", size,
	)
	defer func() {
		logger.Infow("search completed", util.LogDuration(time.Since(begin)))
	}()

	// Convert request to JSON
	body, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal search request")
	}

	// Perform search request
	res, err := client.Search(
		client.Search.WithContext(ctx),
		client.Search.WithIndex(indexName),
		client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		logger.Errorw("failed to execute search", "error", err)
		return nil, errors.Wrap(err, "failed to execute search")
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		logger.Errorz(
			"elasticsearch error response",
			logfield.Status(res.StatusCode),
			zap.String("body", string(body)),
		)
		return nil, errors.Newf("elasticsearch error [%s]: %s", res.Status(), string(body))
	}
	var esRes map[string]any
	if err := json.NewDecoder(res.Body).Decode(&esRes); err != nil {
		logger.Errorw("failed to decode response", "error", err)
		return nil, errors.Wrap(err, "failed to decode response")
	}
	return parseSearchResult(esRes)
}

// parseSearchResult parses Elasticsearch response into SearchResult.
// It safely extracts and validates:
// - total hits count and how exact it is
// - max score
// - hits array with their IDs, sources and scores
// - aggregation results
func parseSearchResult(esRes map[string]any) (*SearchResult, error) {
	var (
		ok       bool
		id       string
		err      error
		total    int64
		relation TotalRelation
		maxScore float64
		hits     map[string]any
		hitMap   map[string]any
		source   map[string]any
		hitsList []any
	)

	// Extract hits with type assertion safety
	if hits, ok = esRes["hits"].(map[string]any); !ok {
		return nil, errors.New("invalid response format: hits not found or invalid type")
	}
	// Process search result with safe type assertions
	if total, relation, err = extractTotal(hits); err != nil {
		return nil, errors.Wrap(err, "failed to extract total")
	}
	result := &SearchResult{Total: total, TotalRelation: relation}
	// Safely extract max_score
	if maxScore, ok = hits["max_score"].(float64); ok {
		result.MaxScore = &maxScore
	}
	// Process hits with safe type assertions
	if hitsList, ok = hits["hits"].([]any); !ok {
		return nil, errors.New("invalid response format: hits list not found or invalid type")
	}
	result.Hits = make([]SearchHit, len(hitsList))

	for i, hit := range hitsList {
		if hitMap, ok = hit.(map[string]any); !ok {
			return nil, errors.Newf("invalid hit format at index %d", i)
		}
		if id, ok = hitMap["_id"].(string); !ok {
			return nil, errors.Newf("invalid or missing _id at index %d", i)
		}
		if source, ok = hitMap["_source"].(map[string]any); !ok {
			return nil, errors.Newf("invalid or missing _source at index %d", i)
		}
		var score *float64
		if scoreVal, ok := hitMap["_score"].(float64); ok {
			score = &scoreVal
		}
		result.Hits[i] = SearchHit{
			ID:     id,
			Source: source,
			Score:  score,
		}
	}

	// Parse aggregations if present
	if aggs, ok := esRes["aggregations"].(map[string]any); ok {
		result.Aggregations = make(map[string]*AggregationResult)
		for name, value := range aggs {
			aggResult := &AggregationResult{}

			if aggMap, ok := value.(map[string]any); ok {
				// Parse buckets for bucket aggregations
				if buckets, ok := aggMap["buckets"].([]any); ok {
					aggResult.Buckets = make([]map[string]any, len(buckets))
					for i, bucket := range buckets {
						if bucketMap, ok := bucket.(map[string]any); ok {
							aggResult.Buckets[i] = bucketMap
						}
					}
				}

				// Parse value for metric aggregations
				if value, exists := aggMap["value"]; exists {
					aggResult.Value = value
				}

				// Parse doc_count
				if docCount, exists := aggMap["doc_count"]; exists {
					if count, ok := docCount.(float64); ok {
						aggResult.DocCount = int64(count)
					}
				}
			}

			result.Aggregations[name] = aggResult
		}
	}
	return result, nil
}

// extractTotal reads hits.total, which the cluster answers as a value and
// the relation of the value to the number of hits, see TotalRelation.
func extractTotal(hits map[string]any) (int64, TotalRelation, error) {
	total, ok := hits["total"].(map[string]any)
	if !ok {
		return 0, "", errors.New("invalid total format")
	}
	value, ok := total["value"].(float64)
	if !ok {
		return 0, "", errors.New("invalid total value format")
	}
	relation, ok := total["relation"].(string)
	if !ok {
		return 0, "", errors.New("invalid total relation format")
	}
	return int64(value), TotalRelation(relation), nil
}
