package elastic_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hydroan/gst/provider/elastic"
	"github.com/stretchr/testify/require"
)

// TestQueryBuilderWritesEachClauseWhereItBelongs pins the query each clause
// method adds and the bool clause it lands in: the must_not and the should
// forms of term, match and match_phrase, with and without options,
// match_all and exists under must, the one-sided time ranges under filter,
// and a should clause with its minimum.
func TestQueryBuilderWritesEachClauseWhereItBelongs(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	slop := 2
	for _, tt := range []struct {
		name  string
		build func(qb *elastic.QueryBuilder) *elastic.QueryBuilder
		want  string
	}{
		{"term_not", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.TermNot("kind", "a") }, `{"bool":{"must_not":[{"term":{"kind":"a"}}]}}`},
		{"term_should", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.TermShould("kind", "a") }, `{"bool":{"should":[{"term":{"kind":"a"}}]}}`},
		{"match_all", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.MatchAll() }, `{"bool":{"must":[{"match_all":{}}]}}`},
		{"match_not", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.MatchNot("title", "a") }, `{"bool":{"must_not":[{"match":{"title":"a"}}]}}`},
		{"match_should", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.MatchShould("title", "a") }, `{"bool":{"should":[{"match":{"title":"a"}}]}}`},
		{"match_phrase", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.MatchPhrase("title", "a b") }, `{"bool":{"must":[{"match_phrase":{"title":"a b"}}]}}`},
		{"match_phrase_not", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.MatchPhraseNot("title", "a b") }, `{"bool":{"must_not":[{"match_phrase":{"title":"a b"}}]}}`},
		{"match_phrase_should", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.MatchPhraseShould("title", "a b") }, `{"bool":{"should":[{"match_phrase":{"title":"a b"}}]}}`},
		{"match_phrase_options_not", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder {
			return qb.MatchPhraseOptionsNot("title", elastic.MatchPhraseOptions{Query: "a b", Slop: &slop})
		}, `{"bool":{"must_not":[{"match_phrase":{"title":{"query":"a b","slop":2}}}]}}`},
		{"match_phrase_options_should", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder {
			return qb.MatchPhraseOptionsShould("title", elastic.MatchPhraseOptions{Query: "a b", Slop: &slop})
		}, `{"bool":{"should":[{"match_phrase":{"title":{"query":"a b","slop":2}}}]}}`},
		{"exists", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.Exists("title") }, `{"bool":{"must":[{"exists":{"field":"title"}}]}}`},
		{"time_range_gte", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.TimeRangeGte("at", at) }, `{"bool":{"filter":[{"range":{"at":{"gte":"2026-01-02T03:04:05Z","format":"strict_date_optional_time||epoch_millis"}}}]}}`},
		{"time_range_lte", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder { return qb.TimeRangeLte("at", at) }, `{"bool":{"filter":[{"range":{"at":{"lte":"2026-01-02T03:04:05Z","format":"strict_date_optional_time||epoch_millis"}}}]}}`},
		{"should_with_minimum", func(qb *elastic.QueryBuilder) *elastic.QueryBuilder {
			return qb.TermShould("kind", "a").TermShould("kind", "b").MinimumShouldMatch(1)
		}, `{"bool":{"should":[{"term":{"kind":"a"}},{"term":{"kind":"b"}}],"minimum_should_match":1}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.build(elastic.NewQueryBuilder()).BuildQuery())
			require.NoError(t, err)
			require.JSONEq(t, tt.want, string(body))
		})
	}
}

// TestQueryBuilderWritesSourceAndTheMetricAggregations pins the _source
// filter and the metric aggregations of a request: each aggregation lands
// under its name with the field it runs on.
func TestQueryBuilderWritesSourceAndTheMetricAggregations(t *testing.T) {
	req, err := elastic.NewQueryBuilder().Size(0).Source("title", "kind").
		AggsCardinality("kinds", "kind").
		AggsStats("stats", "seq").
		AggsMin("lo", "seq").
		AggsMax("hi", "seq").
		AggsAvg("mean", "seq").
		AggsSum("total", "seq").
		Build()
	require.NoError(t, err)
	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"query": {"match_all": {}},
		"size": 0,
		"_source": ["title", "kind"],
		"aggs": {
			"kinds": {"cardinality": {"field": "kind"}},
			"stats": {"stats": {"field": "seq"}},
			"lo": {"min": {"field": "seq"}},
			"hi": {"max": {"field": "seq"}},
			"mean": {"avg": {"field": "seq"}},
			"total": {"sum": {"field": "seq"}}
		}
	}`, string(body))
}

// TestQueryBuilderValidateAndStringReportTheRequest pins Validate, which
// answers what Build would refuse without building, and String, the JSON of
// a valid request or the refusal of an invalid one.
func TestQueryBuilderValidateAndStringReportTheRequest(t *testing.T) {
	require.NoError(t, elastic.NewQueryBuilder().Term("kind", "a").Validate())
	require.ErrorContains(t, elastic.NewQueryBuilder().Size(-1).Validate(), "size cannot be negative")

	require.JSONEq(t, `{"query":{"bool":{"must":[{"term":{"kind":"a"}}]}}}`, elastic.NewQueryBuilder().Term("kind", "a").String())
	require.Contains(t, elastic.NewQueryBuilder().Size(-1).String(), "invalid query: size cannot be negative")
}
