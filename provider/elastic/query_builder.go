package elastic

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jinzhu/copier"
)

const (
	defaultTimeFormat = "strict_date_optional_time||epoch_millis"

	Asc  Order = "asc"
	Desc Order = "desc"
)

type Order string

// QueryBuilder helps build Elasticsearch queries.
// It supports must, must_not, should, filter clauses,
// pagination, sorting, field filtering, search_after and the total count.
type QueryBuilder struct {
	must               []map[string]any
	mustNot            []map[string]any
	should             []map[string]any
	filter             []map[string]any
	size               *int
	from               int
	sort               []map[string]any
	source             []string
	searchAfter        []any
	minimumShouldMatch any
	trackTotalHits     bool

	aggs map[string]any
}

// MatchPhraseOptions represents options for match_phrase query
type MatchPhraseOptions struct {
	Query          any      `json:"query"`
	Slop           *int     `json:"slop,omitempty"`
	Analyzer       string   `json:"analyzer,omitempty"`
	ZeroTermsQuery string   `json:"zero_terms_query,omitempty"`
	MaxExpansions  *int     `json:"max_expansions,omitempty"`
	Boost          *float64 `json:"boost,omitempty"`
}

// AggregationOptions represents options for aggregation query
type AggregationOptions struct {
	Field string            `json:"field"`
	Size  *int              `json:"size,omitempty"`
	Order map[string]string `json:"order,omitempty"`
}

// NewQueryBuilder creates a query builder. Until set, the size, the from and
// the sort are left to the cluster, see SearchRequest.
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{}
}

// Bool adds a nested bool query
func (qb *QueryBuilder) Bool(fn func(qb *QueryBuilder)) *QueryBuilder {
	nestedBuilder := NewQueryBuilder()
	fn(nestedBuilder)
	query := nestedBuilder.BuildQuery()
	if query != nil { // only add the nested query when it is not nil
		qb.Must(query)
	}
	return qb
}

// Must adds a must clause to the bool query
func (qb *QueryBuilder) Must(query map[string]any) *QueryBuilder {
	qb.must = append(qb.must, query)
	return qb
}

// MustNot adds a must_not clause to the bool query
func (qb *QueryBuilder) MustNot(query map[string]any) *QueryBuilder {
	qb.mustNot = append(qb.mustNot, query)
	return qb
}

// Should adds a should clause to the bool query
func (qb *QueryBuilder) Should(query map[string]any) *QueryBuilder {
	qb.should = append(qb.should, query)
	return qb
}

// MinimumShouldMatch sets minimum_should_match for should clauses
func (qb *QueryBuilder) MinimumShouldMatch(minimum any) *QueryBuilder {
	qb.minimumShouldMatch = minimum
	return qb
}

// Filter adds a filter clause to the bool query
func (qb *QueryBuilder) Filter(query map[string]any) *QueryBuilder {
	qb.filter = append(qb.filter, query)
	return qb
}

// Term adds a term query to must clauses
func (qb *QueryBuilder) Term(field string, value any) *QueryBuilder {
	return qb.Must(map[string]any{
		"term": map[string]any{
			field: value,
		},
	})
}

// TermNot adds a term query to must_not clauses
func (qb *QueryBuilder) TermNot(field string, value any) *QueryBuilder {
	return qb.MustNot(map[string]any{
		"term": map[string]any{
			field: value,
		},
	})
}

// TermShould adds a term query to should clauses
func (qb *QueryBuilder) TermShould(field string, value any) *QueryBuilder {
	return qb.Should(map[string]any{
		"term": map[string]any{
			field: value,
		},
	})
}

// MatchAll adds a match_all query
func (qb *QueryBuilder) MatchAll() *QueryBuilder {
	return qb.Must(map[string]any{
		"match_all": map[string]any{},
	})
}

// Match adds a match query to must clauses
func (qb *QueryBuilder) Match(field string, value any) *QueryBuilder {
	return qb.Must(map[string]any{
		"match": map[string]any{
			field: value,
		},
	})
}

// MatchNot adds a match query to must_not clauses
func (qb *QueryBuilder) MatchNot(field string, value any) *QueryBuilder {
	return qb.MustNot(map[string]any{
		"match": map[string]any{
			field: value,
		},
	})
}

// MatchShould adds a match query to should clauses
func (qb *QueryBuilder) MatchShould(field string, value any) *QueryBuilder {
	return qb.Should(map[string]any{
		"match": map[string]any{
			field: value,
		},
	})
}

// MatchPhrase adds a match_phrase query to must clauses
func (qb *QueryBuilder) MatchPhrase(field string, value any) *QueryBuilder {
	return qb.Must(map[string]any{
		"match_phrase": map[string]any{
			field: value,
		},
	})
}

// MatchPhraseNot adds a match_phrase query to must_not clauses
func (qb *QueryBuilder) MatchPhraseNot(field string, value any) *QueryBuilder {
	return qb.MustNot(map[string]any{
		"match_phrase": map[string]any{
			field: value,
		},
	})
}

// MatchPhraseShould adds a match_phrase query to should clauses
func (qb *QueryBuilder) MatchPhraseShould(field string, value any) *QueryBuilder {
	return qb.Should(map[string]any{
		"match_phrase": map[string]any{
			field: value,
		},
	})
}

// MatchPhraseOptions adds a match_phrase query with options to must clauses
func (qb *QueryBuilder) MatchPhraseOptions(field string, opts MatchPhraseOptions) *QueryBuilder {
	return qb.Must(map[string]any{
		"match_phrase": map[string]any{
			field: opts,
		},
	})
}

// MatchPhraseOptionsNot adds a match_phrase query with options to must_not clauses
func (qb *QueryBuilder) MatchPhraseOptionsNot(field string, opts MatchPhraseOptions) *QueryBuilder {
	return qb.MustNot(map[string]any{
		"match_phrase": map[string]any{
			field: opts,
		},
	})
}

// MatchPhraseOptionsShould adds a match_phrase query with options to should clauses
func (qb *QueryBuilder) MatchPhraseOptionsShould(field string, opts MatchPhraseOptions) *QueryBuilder {
	return qb.Should(map[string]any{
		"match_phrase": map[string]any{
			field: opts,
		},
	})
}

// Range adds a range query to filter clauses
func (qb *QueryBuilder) Range(field string, ranges map[string]any) *QueryBuilder {
	return qb.Filter(map[string]any{
		"range": map[string]any{
			field: ranges,
		},
	})
}

// Exists adds an exists query to must clauses
func (qb *QueryBuilder) Exists(field string) *QueryBuilder {
	return qb.Must(map[string]any{
		"exists": map[string]any{
			"field": field,
		},
	})
}

// TimeRange adds a time range query with RFC3339 format
func (qb *QueryBuilder) TimeRange(field string, start, end time.Time) *QueryBuilder {
	return qb.Range(field, map[string]any{
		"gte":    start.Format(time.RFC3339),
		"lte":    end.Format(time.RFC3339),
		"format": defaultTimeFormat,
	})
}

// TimeRangeGte adds a time range query with greater than or equal condition
func (qb *QueryBuilder) TimeRangeGte(field string, tm time.Time) *QueryBuilder {
	return qb.Range(field, map[string]any{
		"gte":    tm.Format(time.RFC3339),
		"format": defaultTimeFormat,
	})
}

// TimeRangeLte adds a time range query with less than or equal condition
func (qb *QueryBuilder) TimeRangeLte(field string, tm time.Time) *QueryBuilder {
	return qb.Range(field, map[string]any{
		"lte":    tm.Format(time.RFC3339),
		"format": defaultTimeFormat,
	})
}

// Size sets the number of hits to return; zero asks for none, the shape of
// a request that wants aggregations alone. A negative size fails Build.
func (qb *QueryBuilder) Size(size int) *QueryBuilder {
	qb.size = &size
	return qb
}

// From sets the offset of the first hit. A negative from fails Build.
func (qb *QueryBuilder) From(from int) *QueryBuilder {
	qb.from = from
	return qb
}

// TrackTotalHits asks the cluster to count every hit instead of stopping at
// ten thousand, see SearchResult.TotalRelation.
func (qb *QueryBuilder) TrackTotalHits() *QueryBuilder {
	qb.trackTotalHits = true
	return qb
}

// Source sets the _source field filtering
// if fields is empty, all fields will be returned
// if fields is not empty, only the specified fields will be returned
func (qb *QueryBuilder) Source(fields ...string) *QueryBuilder {
	qb.source = fields
	return qb
}

// SearchAfter sets the search_after parameter for deep pagination
// SearchAfter must be used together with Sort.
func (qb *QueryBuilder) SearchAfter(value ...any) *QueryBuilder {
	qb.searchAfter = value
	return qb
}

// Sort adds a sort condition
// SearchAfter requires at least one sort condition.
func (qb *QueryBuilder) Sort(field string, order Order) *QueryBuilder {
	if field == "" {
		return qb
	}
	if order != Asc && order != Desc {
		order = Desc
	}
	qb.sort = append(qb.sort, map[string]any{
		field: map[string]any{
			"order": order,
		},
	})
	return qb
}

// Aggs adds a custom aggregation
func (qb *QueryBuilder) Aggs(name string, agg map[string]any) *QueryBuilder {
	if qb.aggs == nil {
		qb.aggs = make(map[string]any)
	}
	qb.aggs[name] = agg
	return qb
}

// AggsTerm adds a terms aggregation.
// orderBy accepts two parameters: field and order.
// field can be "_count" or "_key",
// order can be "asc" or "desc"
func (qb *QueryBuilder) AggsTerm(name string, field string, size int, orderBy ...string) *QueryBuilder {
	opts := AggregationOptions{
		Field: field,
		Size:  &size,
	}
	if len(orderBy) >= 2 {
		opts.Order = map[string]string{
			orderBy[0]: orderBy[1],
		}
	}
	return qb.Aggs(name, map[string]any{
		"terms": opts,
	})
}

// AggsCardinality adds a cardinality aggregation
func (qb *QueryBuilder) AggsCardinality(name string, field string) *QueryBuilder {
	return qb.Aggs(name, map[string]any{
		"cardinality": AggregationOptions{
			Field: field,
		},
	})
}

// AggsStats adds a stats aggregation
func (qb *QueryBuilder) AggsStats(name string, field string) *QueryBuilder {
	return qb.Aggs(name, map[string]any{
		"stats": AggregationOptions{
			Field: field,
		},
	})
}

// AggsMin adds a min aggregation
func (qb *QueryBuilder) AggsMin(name string, field string) *QueryBuilder {
	return qb.Aggs(name, map[string]any{
		"min": AggregationOptions{
			Field: field,
		},
	})
}

// AggsMax adds a max aggregation
func (qb *QueryBuilder) AggsMax(name string, field string) *QueryBuilder {
	return qb.Aggs(name, map[string]any{
		"max": AggregationOptions{
			Field: field,
		},
	})
}

// AggsAvg adds an avg aggregation
func (qb *QueryBuilder) AggsAvg(name string, field string) *QueryBuilder {
	return qb.Aggs(name, map[string]any{
		"avg": AggregationOptions{
			Field: field,
		},
	})
}

// AggsSum adds a sum aggregation
func (qb *QueryBuilder) AggsSum(name string, field string) *QueryBuilder {
	return qb.Aggs(name, map[string]any{
		"sum": AggregationOptions{
			Field: field,
		},
	})
}

// Validate reports what Build would refuse: a negative size or from, and
// search_after without a sort or with a from.
func (qb *QueryBuilder) Validate() error {
	return qb.request().validate()
}

// Build creates the SearchRequest, refusing one the cluster would, see
// Validate.
func (qb *QueryBuilder) Build() (*SearchRequest, error) {
	req := qb.request()
	if err := req.validate(); err != nil {
		return nil, err
	}
	return req, nil
}

// request assembles the search body: a bool query of the clauses added, or
// match_all when there are none, with the rest of the parameters as set.
func (qb *QueryBuilder) request() *SearchRequest {
	query := map[string]any{"match_all": map[string]any{}}
	if len(qb.must) > 0 || len(qb.mustNot) > 0 || len(qb.should) > 0 || len(qb.filter) > 0 {
		boolQuery := make(map[string]any)
		if len(qb.must) > 0 {
			boolQuery["must"] = qb.must
		}
		if len(qb.mustNot) > 0 {
			boolQuery["must_not"] = qb.mustNot
		}
		if len(qb.should) > 0 {
			boolQuery["should"] = qb.should
		}
		if len(qb.should) > 0 && qb.minimumShouldMatch != nil {
			boolQuery["minimum_should_match"] = qb.minimumShouldMatch
		}
		if len(qb.filter) > 0 {
			boolQuery["filter"] = qb.filter
		}
		query = map[string]any{"bool": boolQuery}
	}

	return &SearchRequest{
		Query:          query,
		From:           qb.from,
		Size:           qb.size,
		Sort:           qb.sort,
		Source:         qb.source,
		SearchAfter:    qb.searchAfter,
		Aggs:           qb.aggs,
		TrackTotalHits: qb.trackTotalHits,
	}
}

// BuildForce creates a SearchRequest without validation
func (qb *QueryBuilder) BuildForce() *SearchRequest {
	req, _ := qb.Build()
	return req
}

// BuildQuery creates the query part of SearchRequest
func (qb *QueryBuilder) BuildQuery() map[string]any {
	req, err := qb.Build()
	if err != nil || req.Query == nil {
		return nil
	}
	return req.Query
}

// Clone returns a builder with the same clauses; clauses added to it leave
// the original alone.
func (qb *QueryBuilder) Clone() *QueryBuilder {
	clone := new(QueryBuilder)
	_ = copier.Copy(clone, qb)
	return clone
}

// String returns the JSON representation of the query
func (qb *QueryBuilder) String() string {
	req, err := qb.Build()
	if err != nil {
		return fmt.Sprintf("invalid query: %v", err)
	}

	bytes, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return fmt.Sprintf("failed to marshal query: %v", err)
	}

	return string(bytes)
}
