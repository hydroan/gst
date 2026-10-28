package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/logfield"
	"github.com/hydroan/gst/internal/util"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// SearchRequest is an Elasticsearch search body: the query, the page, the
// sort, the fields to return, search_after for deep pagination and the
// aggregations. QueryBuilder composes one. What a request leaves unset is
// left to the cluster, which ranks unsorted hits by relevance and answers ten
// of them. Search, SearchNext and SearchPrev read a nil request as the empty
// one.
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
// something other than what was asked: a negative size or from, a sort on an
// empty field or in an order other than asc or desc, and search_after
// without the sort whose values it continues from.
func (r *SearchRequest) validate() error {
	if r.Size != nil && *r.Size < 0 {
		return errors.New("size cannot be negative")
	}
	if r.From < 0 {
		return errors.New("from cannot be negative")
	}
	for _, entry := range r.Sort {
		for field, spec := range entry {
			if field == "" {
				return errors.New("sort field cannot be empty")
			}
			if _, _, err := sortOrder(field, spec); err != nil {
				return err
			}
		}
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

// orEmpty returns r, or the empty request when r is nil.
func (r *SearchRequest) orEmpty() *SearchRequest {
	if r == nil {
		return new(SearchRequest)
	}
	return r
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

// SearchPrev returns the size hits before the search_after position of req,
// in the order of its sort: it searches with every sort order reversed and
// reverses the hits it gets back. The page is taken on a copy, req is left as
// it is. A request without a sort, or with a sort entry naming no order, is
// an error: the order to page back along is not known.
func (d *document) SearchPrev(ctx context.Context, indexName string, req *SearchRequest, size int) (*SearchResult, error) {
	req = req.orEmpty()
	sort, err := reverseSort(req.Sort)
	if err != nil {
		return nil, err
	}
	page := *req
	page.From = 0
	page.Size = &size
	page.Sort = sort
	result, err := d.Search(ctx, indexName, &page)
	if err != nil {
		return nil, err
	}
	slices.Reverse(result.Hits)
	return result, nil
}

// SearchNext returns the size hits after the search_after position of req,
// in the order of its sort. The page is taken on a copy, req is left as it
// is.
func (d *document) SearchNext(ctx context.Context, indexName string, req *SearchRequest, size int) (*SearchResult, error) {
	page := *req.orEmpty()
	page.From = 0
	page.Size = &size
	return d.Search(ctx, indexName, &page)
}

// reverseSort returns sort with every order flipped and leaves sort as it is.
// An entry gives a field its order either as the value, {"seq": "asc"}, or
// among the field's options, {"seq": {"order": "asc"}}; an entry without an
// order is an error, since the default it falls back to depends on the field.
func reverseSort(sort []map[string]any) ([]map[string]any, error) {
	if len(sort) == 0 {
		return nil, errors.New("SearchPrev requires sort")
	}
	reversed := make([]map[string]any, len(sort))
	for i, entry := range sort {
		flipped := make(map[string]any, len(entry))
		for field, spec := range entry {
			order, given, err := sortOrder(field, spec)
			if err != nil {
				return nil, err
			}
			if !given {
				return nil, errors.Newf("sort on %s names no order to reverse", field)
			}
			opposite := Asc
			if order == Asc {
				opposite = Desc
			}
			if options, ok := spec.(map[string]any); ok {
				options = maps.Clone(options)
				options["order"] = opposite
				flipped[field] = options
			} else {
				flipped[field] = opposite
			}
		}
		reversed[i] = flipped
	}
	return reversed, nil
}

// sortOrder reads the order a sort entry gives field: spec is either the
// order itself, {"seq": "asc"}, or the field's options holding it, {"seq":
// {"order": "asc"}}. It reports whether an order is given at all, and fails
// when the one given is neither asc nor desc. The letter case does not
// matter, the cluster reads an order in any case.
func sortOrder(field string, spec any) (order Order, given bool, err error) {
	var value any
	switch spec := spec.(type) {
	case string, Order:
		value = spec
	case map[string]any:
		if value, given = spec["order"]; !given {
			return "", false, nil
		}
	default:
		return "", false, nil
	}

	var name string
	switch value := value.(type) {
	case string:
		name = value
	case Order:
		name = string(value)
	}
	switch {
	case strings.EqualFold(name, string(Asc)):
		return Asc, true, nil
	case strings.EqualFold(name, string(Desc)):
		return Desc, true, nil
	default:
		return "", true, errors.Newf("sort on %s has order %v, want asc or desc", field, value)
	}
}

// Search runs req against indexName. The request goes to the cluster as it
// is: no sort means relevance order, no size means the cluster's default
// page, and the total is counted up to the cluster's threshold unless
// TrackTotalHits asks for all of it, see SearchResult.TotalRelation. A nil
// req is the empty request, which matches every document. A request the
// cluster would refuse, see validate, fails before it is sent.
func (*document) Search(ctx context.Context, indexName string, req *SearchRequest) (*SearchResult, error) {
	if err := _check(); err != nil {
		return nil, errors.Wrap(err, "elasticsearch client check")
	}
	if indexName == "" {
		return nil, errors.New("index name cannot be empty")
	}
	req = req.orEmpty()
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
