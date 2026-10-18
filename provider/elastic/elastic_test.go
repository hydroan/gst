package elastic_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/provider/elastic"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{Elasticsearch: true})
}

func TestIndexLifecycle(t *testing.T) {
	name := "sample-lifecycle"
	require.NoError(t, elastic.Index.Create(name, &elastic.IndexOption{
		Settings: map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
		Mappings: map[string]any{"properties": map[string]any{"title": map[string]any{"type": "text"}}},
	}))
	exists, err := elastic.Index.Exists(name)
	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, elastic.Index.Delete(name))
	exists, err = elastic.Index.Exists(name)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDocumentGetReadsAnIndexedDocument(t *testing.T) {
	index := newSampleIndex(t, sampleDocument{id: "one", fields: map[string]any{"title": "sample"}})

	got, err := elastic.Document.Get(context.Background(), index, "one", nil)
	require.NoError(t, err)
	require.True(t, got.Found)
	require.Equal(t, "sample", got.Source["title"])

	missing, err := elastic.Document.Get(context.Background(), index, "two", nil)
	require.NoError(t, err)
	require.False(t, missing.Found)
}

// TestSearchRanksByRelevanceWhenNoSortIsGiven pins that a request without a
// sort of its own leaves the order to the cluster, which ranks by relevance:
// the document saying hello three times comes first, although it was indexed
// second, and every hit carries its score.
func TestSearchRanksByRelevanceWhenNoSortIsGiven(t *testing.T) {
	index := newSampleIndex(t,
		sampleDocument{id: "faint", fields: map[string]any{"title": "hello world"}},
		sampleDocument{id: "loud", fields: map[string]any{"title": "hello there, hello again, hello friend"}},
	)
	req, err := elastic.NewQueryBuilder().Match("title", "hello").Build()
	require.NoError(t, err)

	res, err := elastic.Document.Search(context.Background(), index, req)
	require.NoError(t, err)
	require.Equal(t, []string{"loud", "faint"}, ids(res))
	require.NotNil(t, res.MaxScore)
	for _, hit := range res.Hits {
		require.NotNil(t, hit.Score, "hit %s carries no score", hit.ID)
	}
}

// TestSearchAfterRequiresSort pins that search_after without a sort is
// refused before any request is sent, by the builder and by Search alike.
func TestSearchAfterRequiresSort(t *testing.T) {
	_, err := elastic.NewQueryBuilder().SearchAfter(1).Build()
	require.ErrorContains(t, err, "search_after requires sort")

	_, err = elastic.Document.Search(context.Background(), "sample", &elastic.SearchRequest{SearchAfter: []any{1}})
	require.ErrorContains(t, err, "search_after requires sort")
}

// TestSearchWithSizeZeroAnswersAggregationsAlone pins the aggregation shape:
// a size of zero reaches the cluster, so the answer carries the buckets and
// no hits.
func TestSearchWithSizeZeroAnswersAggregationsAlone(t *testing.T) {
	docs := make([]types.ESDocumenter, 0, 12)
	for i := range 12 {
		kind := "even"
		if i%2 == 1 {
			kind = "odd"
		}
		docs = append(docs, sampleDocument{id: strconv.Itoa(i), fields: map[string]any{"kind": kind}})
	}
	index := newSampleIndex(t, docs...)
	req, err := elastic.NewQueryBuilder().Size(0).AggsTerm("kinds", "kind.keyword", 10).Build()
	require.NoError(t, err)

	res, err := elastic.Document.Search(context.Background(), index, req)
	require.NoError(t, err)
	require.Empty(t, res.Hits)
	require.Len(t, res.Aggregations["kinds"].Buckets, 2)
}

func TestSearchNextAndPrevPageAlongTheSort(t *testing.T) {
	docs := make([]types.ESDocumenter, 0, 5)
	for seq := 1; seq <= 5; seq++ {
		docs = append(docs, sampleDocument{id: strconv.Itoa(seq), fields: map[string]any{"seq": seq}})
	}
	index := newSampleIndex(t, docs...)

	first, err := elastic.NewQueryBuilder().Sort("seq", elastic.Asc).Size(2).Build()
	require.NoError(t, err)
	res, err := elastic.Document.Search(context.Background(), index, first)
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2"}, ids(res))

	next, err := elastic.NewQueryBuilder().Sort("seq", elastic.Asc).SearchAfter(2).Build()
	require.NoError(t, err)
	res, err = elastic.Document.SearchNext(context.Background(), index, next, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"3", "4"}, ids(res))

	prev, err := elastic.NewQueryBuilder().Sort("seq", elastic.Asc).SearchAfter(3).Build()
	require.NoError(t, err)
	res, err = elastic.Document.SearchPrev(context.Background(), index, prev, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2"}, ids(res))
}

func TestQueryBuilderTimeRangeKeepsTheDocumentsInside(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	index := newSampleIndex(t,
		sampleDocument{id: "early", fields: map[string]any{"created_at": base.Add(-48 * time.Hour).Format(time.RFC3339)}},
		sampleDocument{id: "inside", fields: map[string]any{"created_at": base.Add(time.Hour).Format(time.RFC3339)}},
		sampleDocument{id: "late", fields: map[string]any{"created_at": base.Add(72 * time.Hour).Format(time.RFC3339)}},
	)
	req, err := elastic.NewQueryBuilder().TimeRange("created_at", base, base.Add(24*time.Hour)).Build()
	require.NoError(t, err)

	res, err := elastic.Document.Search(context.Background(), index, req)
	require.NoError(t, err)
	require.Equal(t, []string{"inside"}, ids(res))
}

func TestQueryBuilderMatchPhraseOptionsAllowsASlop(t *testing.T) {
	index := newSampleIndex(t,
		sampleDocument{id: "exact", fields: map[string]any{"title": "hello world"}},
		sampleDocument{id: "gapped", fields: map[string]any{"title": "hello big world"}},
		sampleDocument{id: "far", fields: map[string]any{"title": "hello big wide world"}},
	)
	req, err := elastic.NewQueryBuilder().
		MatchPhraseOptions("title", elastic.MatchPhraseOptions{Query: "hello world", Slop: util.Pointer(1)}).
		Build()
	require.NoError(t, err)

	res, err := elastic.Document.Search(context.Background(), index, req)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"exact", "gapped"}, ids(res))
}

func TestQueryBuilderRejectsANegativeSizeOrFrom(t *testing.T) {
	_, err := elastic.NewQueryBuilder().Size(-1).Build()
	require.ErrorContains(t, err, "size cannot be negative")

	_, err = elastic.NewQueryBuilder().From(-1).Build()
	require.ErrorContains(t, err, "from cannot be negative")
}

// TestQueryBuilderMarshalsWhatWasSet pins the request body: a size of zero
// is written out, and a request that sets no size and no sort names neither,
// leaving both to the cluster's defaults.
func TestQueryBuilderMarshalsWhatWasSet(t *testing.T) {
	req, err := elastic.NewQueryBuilder().Size(0).Build()
	require.NoError(t, err)
	body, err := json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, `{"query":{"match_all":{}},"size":0}`, string(body))

	req, err = elastic.NewQueryBuilder().Build()
	require.NoError(t, err)
	body, err = json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, `{"query":{"match_all":{}}}`, string(body))

	req, err = elastic.NewQueryBuilder().TrackTotalHits().Build()
	require.NoError(t, err)
	body, err = json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, `{"query":{"match_all":{}},"track_total_hits":true}`, string(body))
}

// TestSearchReportsWhetherTheTotalIsExact pins how the total reads past the
// cluster's counting threshold of ten thousand hits: by default the total is
// the threshold and the relation says at least, and a request asking for the
// exact count gets it with the relation saying equal.
func TestSearchReportsWhetherTheTotalIsExact(t *testing.T) {
	docs := make([]types.ESDocumenter, 0, 10001)
	for i := range 10001 {
		docs = append(docs, sampleDocument{id: strconv.Itoa(i), fields: map[string]any{"seq": i}})
	}
	index := newSampleIndex(t, docs...)

	req, err := elastic.NewQueryBuilder().Size(0).Build()
	require.NoError(t, err)
	res, err := elastic.Document.Search(context.Background(), index, req)
	require.NoError(t, err)
	require.Equal(t, int64(10000), res.Total)
	require.Equal(t, elastic.TotalRelationGte, res.TotalRelation)

	req, err = elastic.NewQueryBuilder().Size(0).TrackTotalHits().Build()
	require.NoError(t, err)
	res, err = elastic.Document.Search(context.Background(), index, req)
	require.NoError(t, err)
	require.Equal(t, int64(10001), res.Total)
	require.Equal(t, elastic.TotalRelationEq, res.TotalRelation)
}

// TestQueryBuilderCloneIsIndependent pins that a clone starts with the
// original's clauses and that clauses added to it leave the original alone.
func TestQueryBuilderCloneIsIndependent(t *testing.T) {
	original := elastic.NewQueryBuilder().Term("kind.keyword", "note").Sort("seq", elastic.Asc).Size(5)
	clone := original.Clone().Term("author.keyword", "a")

	originalReq, err := original.Build()
	require.NoError(t, err)
	cloneReq, err := clone.Build()
	require.NoError(t, err)
	originalBody, err := json.Marshal(originalReq)
	require.NoError(t, err)
	cloneBody, err := json.Marshal(cloneReq)
	require.NoError(t, err)
	require.JSONEq(t, `{"query":{"bool":{"must":[{"term":{"kind.keyword":"note"}}]}},"sort":[{"seq":{"order":"asc"}}],"size":5}`, string(originalBody))
	require.JSONEq(t, `{"query":{"bool":{"must":[{"term":{"kind.keyword":"note"}},{"term":{"author.keyword":"a"}}]}},"sort":[{"seq":{"order":"asc"}}],"size":5}`, string(cloneBody))
}

func TestQueryBuilderNestsABoolOfShouldClauses(t *testing.T) {
	req, err := elastic.NewQueryBuilder().
		Term("kind.keyword", "note").
		Bool(func(b *elastic.QueryBuilder) {
			b.Should(elastic.NewQueryBuilder().Term("author.keyword", "a").BuildQuery())
			b.Should(elastic.NewQueryBuilder().Term("author.keyword", "b").BuildQuery())
			b.MinimumShouldMatch(1)
		}).
		Build()
	require.NoError(t, err)
	body, err := json.Marshal(req.Query)
	require.NoError(t, err)
	require.JSONEq(t, `{"bool":{"must":[
		{"term":{"kind.keyword":"note"}},
		{"bool":{"should":[
			{"bool":{"must":[{"term":{"author.keyword":"a"}}]}},
			{"bool":{"must":[{"term":{"author.keyword":"b"}}]}}
		],"minimum_should_match":1}}
	]}}`, string(body))
}

// sampleDocument is a document the tests index: its id and its fields.
type sampleDocument struct {
	id     string
	fields map[string]any
}

func (d sampleDocument) GetID() string            { return d.id }
func (d sampleDocument) Document() map[string]any { return d.fields }

// newSampleIndex creates an index named after the test, indexes docs into
// it, refreshes it so a search sees them at once, and deletes it when the
// test ends.
func newSampleIndex(t *testing.T, docs ...types.ESDocumenter) string {
	t.Helper()
	name := "sample-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	require.NoError(t, elastic.Index.Create(name, &elastic.IndexOption{
		Settings: map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
	}))
	t.Cleanup(func() { require.NoError(t, elastic.Index.Delete(name)) })
	if len(docs) > 0 {
		require.NoError(t, elastic.Document.BulkIndex(context.Background(), name, docs...))
	}

	client, err := elastic.Client()
	require.NoError(t, err)
	res, err := client.Indices.Refresh(client.Indices.Refresh.WithIndex(name))
	require.NoError(t, err)
	defer res.Body.Close()
	require.False(t, res.IsError(), res.String())
	return name
}

// ids returns the ids of the hits in their order.
func ids(res *elastic.SearchResult) []string {
	out := make([]string, len(res.Hits))
	for i, hit := range res.Hits {
		out[i] = hit.ID
	}
	return out
}
