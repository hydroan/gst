package elastic_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/provider/elastic"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{Elasticsearch: true})
}

func TestIndexLifecycle(t *testing.T) {
	ctx := context.Background()
	name := "sample-lifecycle"
	require.NoError(t, elastic.Index.Create(ctx, name, &elastic.IndexOption{
		Settings: map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
		Mappings: map[string]any{"properties": map[string]any{"title": map[string]any{"type": "text"}}},
	}))
	require.ErrorContains(t, elastic.Index.Create(ctx, name), "resource_already_exists_exception")
	exists, err := elastic.Index.Exists(ctx, name)
	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, elastic.Index.Delete(ctx, name))
	exists, err = elastic.Index.Exists(ctx, name)
	require.NoError(t, err)
	require.False(t, exists)
	require.ErrorContains(t, elastic.Index.Delete(ctx, name), "index_not_found_exception")
}

// TestIndexHonoursTheContext pins that every index operation runs under the
// caller's context: a canceled one fails each of them before the request.
func TestIndexHonoursTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, elastic.Index.Create(ctx, "sample-canceled"), context.Canceled)
	_, err := elastic.Index.Exists(ctx, "sample-canceled")
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, elastic.Index.Delete(ctx, "sample-canceled"), context.Canceled)
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
	docs := make([]elastic.Documenter, 0, 12)
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
	index := newSampleIndex(t, seqDocuments(5)...)

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
	docs := make([]elastic.Documenter, 0, 10001)
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

// TestSearchPrevAndNextLeaveTheRequestAlone pins that paging works on a copy
// of the request: it keeps its size, and the builder it came from still
// builds the original sort afterwards.
func TestSearchPrevAndNextLeaveTheRequestAlone(t *testing.T) {
	index := newSampleIndex(t, seqDocuments(5)...)
	qb := elastic.NewQueryBuilder().Sort("seq", elastic.Asc).SearchAfter(3)
	req, err := qb.Build()
	require.NoError(t, err)

	_, err = elastic.Document.SearchPrev(context.Background(), index, req, 2)
	require.NoError(t, err)
	_, err = elastic.Document.SearchNext(context.Background(), index, req, 1)
	require.NoError(t, err)
	require.Nil(t, req.Size)

	again, err := qb.Build()
	require.NoError(t, err)
	body, err := json.Marshal(again.Sort)
	require.NoError(t, err)
	require.JSONEq(t, `[{"seq":{"order":"asc"}}]`, string(body))
}

// TestSearchPrevReversesAShortFormSort pins the other way the cluster reads a
// sort, the order given as the field's value.
func TestSearchPrevReversesAShortFormSort(t *testing.T) {
	index := newSampleIndex(t, seqDocuments(5)...)
	req := &elastic.SearchRequest{
		Sort:        []map[string]any{{"seq": "asc"}},
		SearchAfter: []any{4},
	}

	res, err := elastic.Document.SearchPrev(context.Background(), index, req, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"2", "3"}, ids(res))
}

// TestSearchPrevRefusesASortItCannotReverse pins that SearchPrev fails rather
// than answering a page in some other order: without a sort there is nothing
// to page back along, and a sort entry naming no order leaves the order to a
// default that depends on the field.
func TestSearchPrevRefusesASortItCannotReverse(t *testing.T) {
	index := newSampleIndex(t, seqDocuments(3)...)

	_, err := elastic.Document.SearchPrev(context.Background(), index, &elastic.SearchRequest{}, 2)
	require.ErrorContains(t, err, "SearchPrev requires sort")

	_, err = elastic.Document.SearchPrev(context.Background(), index, &elastic.SearchRequest{
		Sort: []map[string]any{{"seq": map[string]any{"missing": "_last"}}},
	}, 2)
	require.ErrorContains(t, err, "sort on seq names no order to reverse")
}

func TestBulkIndexHonoursTheContext(t *testing.T) {
	index := newSampleIndex(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := elastic.Document.BulkIndex(ctx, index, sampleDocument{id: "one", fields: map[string]any{"title": "sample"}})
	require.ErrorIs(t, err, context.Canceled)
}

// TestBulkIndexLetsTheClusterNameADocumentWithoutAnID pins that an empty id
// is left out of the request, so the cluster generates one.
func TestBulkIndexLetsTheClusterNameADocumentWithoutAnID(t *testing.T) {
	index := newSampleIndex(t, sampleDocument{fields: map[string]any{"title": "unnamed"}})

	res, err := elastic.Document.Search(context.Background(), index, &elastic.SearchRequest{})
	require.NoError(t, err)
	require.Len(t, res.Hits, 1)
	require.NotEmpty(t, res.Hits[0].ID)
}

func TestBulkIndexKeepsAnIDWithAQuote(t *testing.T) {
	index := newSampleIndex(t, sampleDocument{id: `quoted"id`, fields: map[string]any{"title": "quoted"}})

	res, err := elastic.Document.Search(context.Background(), index, &elastic.SearchRequest{})
	require.NoError(t, err)
	require.Equal(t, []string{`quoted"id`}, ids(res))
}

func TestBulkIndexWithNoDocumentsIndexesNothing(t *testing.T) {
	require.NoError(t, elastic.Document.BulkIndex(context.Background(), "sample-nothing"))
}

// TestQueryBuilderSortRefusesWhatTheClusterCannotRead pins that Sort records
// what it is given and Build refuses a sort the cluster could not read: an
// empty field, or an order other than asc or desc.
func TestQueryBuilderSortRefusesWhatTheClusterCannotRead(t *testing.T) {
	_, err := elastic.NewQueryBuilder().Sort("seq", "ascending").Build()
	require.ErrorContains(t, err, "sort on seq has order ascending, want asc or desc")

	_, err = elastic.NewQueryBuilder().Sort("", elastic.Asc).Build()
	require.ErrorContains(t, err, "sort field cannot be empty")
}

// TestSearchRefusesASortTheClusterCannotRead pins the same check on a request
// written by hand, made before the request is sent, for both ways a sort
// entry gives its order.
func TestSearchRefusesASortTheClusterCannotRead(t *testing.T) {
	index := newSampleIndex(t, seqDocuments(2)...)

	_, err := elastic.Document.Search(context.Background(), index, &elastic.SearchRequest{
		Sort: []map[string]any{{"seq": map[string]any{"order": "up"}}},
	})
	require.ErrorContains(t, err, "sort on seq has order up, want asc or desc")

	_, err = elastic.Document.Search(context.Background(), index, &elastic.SearchRequest{
		Sort: []map[string]any{{"seq": "sideways"}},
	})
	require.ErrorContains(t, err, "sort on seq has order sideways, want asc or desc")
}

// TestSearchSortsInTheOrderGivenInAnyLetterCase pins that an order written in
// upper case sorts the way it says, the way the cluster reads it.
func TestSearchSortsInTheOrderGivenInAnyLetterCase(t *testing.T) {
	index := newSampleIndex(t, seqDocuments(3)...)
	req, err := elastic.NewQueryBuilder().Sort("seq", "ASC").Build()
	require.NoError(t, err)

	res, err := elastic.Document.Search(context.Background(), index, req)
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2", "3"}, ids(res))
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
func newSampleIndex(t *testing.T, docs ...elastic.Documenter) string {
	t.Helper()
	name := "sample-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	require.NoError(t, elastic.Index.Create(context.Background(), name, &elastic.IndexOption{
		Settings: map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
	}))
	t.Cleanup(func() { require.NoError(t, elastic.Index.Delete(context.Background(), name)) })
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

// seqDocuments returns n documents numbered 1 to n in the field seq, each
// identified by its number.
func seqDocuments(n int) []elastic.Documenter {
	docs := make([]elastic.Documenter, 0, n)
	for seq := 1; seq <= n; seq++ {
		docs = append(docs, sampleDocument{id: strconv.Itoa(seq), fields: map[string]any{"seq": seq}})
	}
	return docs
}

// ids returns the ids of the hits in their order.
func ids(res *elastic.SearchResult) []string {
	out := make([]string, len(res.Hits))
	for i, hit := range res.Hits {
		out[i] = hit.ID
	}
	return out
}
