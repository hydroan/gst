package elastic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/araddon/dateparse"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/provider/elastic"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const Index = "test"

// setupElastic prepares the provider for tests that query a live Elasticsearch
// cluster. The integration is temporarily disabled, so it skips the caller
// before connecting; restoring it means removing the t.Skip below. This setup
// cannot live in init(), which runs before any test and would fail the whole
// package no matter what the tests themselves do.
func setupElastic(t *testing.T) {
	t.Helper()
	t.Skip("Elasticsearch provider integration is temporarily disabled: it needs a live cluster.")

	config.SetConfigFile("../../examples/demo/config.ini")
	if err := config.Init(); err != nil {
		t.Fatalf("init config: %v", err)
	}
	if err := zap.Init(); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	if err := elastic.Init(); err != nil {
		t.Fatalf("init elastic: %v", err)
	}
}

func TestIndex(t *testing.T) {
	setupElastic(t)

	settings := map[string]any{
		"number_of_shards":   3,
		"number_of_replicas": 2,
	}

	mappings := map[string]any{
		"properties": map[string]any{
			"title": map[string]any{
				"type": "text",
			},
			"content": map[string]any{
				"type": "text",
			},
			"date": map[string]any{
				"type": "date",
			},
		},
	}
	idx1 := "my_index"
	idx2 := "my_index2"

	require.NoError(t, elastic.Index.Create(idx1, &elastic.IndexOption{settings, mappings}))
	require.NoError(t, elastic.Index.Create(idx2))
	exists, err := elastic.Index.Exists(idx1)
	assert.True(t, exists)
	require.NoError(t, err)

	exists, err = elastic.Index.Exists(idx2)
	assert.True(t, exists)
	require.NoError(t, err)

	require.NoError(t, elastic.Index.Delete(idx1))
	require.NoError(t, elastic.Index.Delete(idx2))
}

func TestDocumentGet(t *testing.T) {
	setupElastic(t)

	doc, err := elastic.Document.Get(context.TODO(), Index, "message_recv_7143038995084115996_7274598307442327556_7424788642731753476", nil)
	require.NoError(t, err)
	fmt.Println(doc)
}

func TestDocumentSearch(t *testing.T) {
	setupElastic(t)

	// The first two requests are kept as query DSL examples; they are
	// intentionally overwritten below.
	//nolint:ineffassign,wastedassign
	req := &elastic.SearchRequest{
		Query: map[string]any{
			"bool": map[string]any{
				"must": []map[string]any{
					{
						"match": map[string]any{
							"message_text": "hello",
						},
					},
					{
						"term": map[string]any{
							"type": "message_send",
						},
					},
				},
			},
		},
		Size: 10,
	}

	// (type.keyword : "message_send" or type.keyword : "message_recv" or type.keyword : "message_ack" ) and message_user_id.keyword : "7143038995084115996" and message_text: hello
	//nolint:ineffassign,wastedassign
	req = &elastic.SearchRequest{
		Query: map[string]any{
			"bool": map[string]any{
				"must": []map[string]any{
					{
						"bool": map[string]any{
							"should": []map[string]any{
								{
									"term": map[string]any{
										"type.keyword": "message_send",
									},
								},
								{
									"term": map[string]any{
										"type.keyword": "message_recv",
									},
								},
								{
									"term": map[string]any{
										"type.keyword": "message_ack",
									},
								},
							},
							"minimum_should_match": 1,
						},
					},
					{
						"term": map[string]any{
							"message_user_id.keyword": "7143038995084115996",
						},
					},
					{
						"match": map[string]any{
							"message_text": "hello",
						},
					},
				},
			},
		},
		Sort: []map[string]any{
			{
				"created_at": map[string]any{
					"order": "desc",
				},
			},
		},
	}
	req = &elastic.SearchRequest{
		Size: 1,
		Sort: []map[string]any{
			{
				"created_at": map[string]any{
					"order": "desc",
				},
			},
		},
	}

	resp, err := elastic.Document.Search(context.Background(), Index, req)
	require.NoError(t, err)
	fmt.Println(resp.Total, len(resp.Hits))

	formatHit := func(hit elastic.SearchHit) {
		for k, v := range hit.Source {
			if k == "message_text" {
				fmt.Println("message_text:", v)
			}
		}
	}
	for _, hit := range resp.Hits {
		formatHit(hit)
	}
	for len(resp.Hits) > 0 {
		{
			lastHit := resp.Hits[len(resp.Hits)-1]
			req.SearchAfter = []any{lastHit.Source["created_at"]}
			// when sorting by ID, pass the ID value
			// when sorting by created_at, pass the created_at value

			resp, err = elastic.Document.Search(context.Background(), Index, req)
			require.NoError(t, err)
			for i := range resp.Hits {
				formatHit(resp.Hits[i])
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func TestDocumentSearchNormal(t *testing.T) {
	setupElastic(t)

	keyword := "hello"
	size := 2
	// plain search
	req := &elastic.SearchRequest{
		Query: map[string]any{
			"bool": map[string]any{
				"must": []map[string]any{
					{
						"match": map[string]any{
							"message_text": keyword,
						},
					},
				},
			},
		},
		Size: size,
	}

	// run the search

	result, err := elastic.Document.Search(context.TODO(), Index, req)
	require.NoError(t, err)
	// print the search results
	for _, hit := range result.Hits {
		fmt.Println(hit.ID)
	}
}

func TestDocumentSearchTimeRange(t *testing.T) {
	setupElastic(t)

	size := 2
	startTime := time.Now().Add(-24 * 30 * time.Hour)
	endTime := time.Now()

	req := &elastic.SearchRequest{
		Query: map[string]any{
			"bool": map[string]any{
				"filter": []map[string]any{
					{
						"range": map[string]any{
							"created_at": map[string]any{
								"gte":    startTime.Format(time.RFC3339),
								"lte":    endTime.Format(time.RFC3339),
								"format": "strict_date_optional_time||epoch_millis",
							},
						},
					},
				},
			},
		},
		Size: size,
		Sort: []map[string]any{
			{
				"created_at": map[string]any{
					"order": "desc",
				},
			},
		},
	}

	result, err := elastic.Document.Search(context.TODO(), Index, req)
	require.NoError(t, err)
	for _, hit := range result.Hits {
		fmt.Println(hit.ID)
	}

	query := elastic.NewQueryBuilder().
		TimeRange("created_at", startTime, endTime).
		Size(size).
		Sort("created_at", elastic.Desc)
	req2, err := query.Build()
	require.NoError(t, err)

	result2, err := elastic.Document.Search(context.TODO(), Index, req2)
	require.NoError(t, err)
	for _, hit := range result2.Hits {
		fmt.Println(hit.ID)
	}
}

func TestDocumentSearchAfter(t *testing.T) {
	setupElastic(t)

	keyword := "hello"
	size := 2
	dateStr := "2024-10-29T10:38:06.085+08:00"
	date, _ := dateparse.ParseLocal(dateStr)
	// search_after pagination
	req := &elastic.SearchRequest{
		Query: map[string]any{
			"bool": map[string]any{
				"must": []map[string]any{
					{
						"match": map[string]any{
							"message_text": keyword,
						},
					},
				},
			},
		},
		Size: size,
		Sort: []map[string]any{
			{
				"created_at": map[string]any{
					"order": "desc",
				},
			},
		},
		SearchAfter: []any{date},
	}

	// run the search

	result, err := elastic.Document.Search(context.TODO(), Index, req)
	require.NoError(t, err)
	for _, hit := range result.Hits {
		fmt.Println(hit.ID)
	}

	query := elastic.NewQueryBuilder().
		Match("message_text", keyword).
		Sort("created_at", elastic.Desc).
		SearchAfter(date).
		Size(size).
		Source("created_at", "type", "chat_type")
	req2, err := query.Build()
	require.NoError(t, err)

	result2, err := elastic.Document.Search(context.TODO(), Index, req2)
	require.NoError(t, err)
	for _, hit := range result2.Hits {
		fmt.Println(hit)
	}
}

func TestDocumentBoolQueryBuilder(t *testing.T) {
	userID := "7336820045630406684"
	peerUserID := "7156029937089069057"
	{

		query := elastic.NewQueryBuilder().
			// base conditions (the and part)
			Term("chat_type.keyword", "direct").
			Term("type.keyword", "message_send").
			// nested or conditions
			Bool(func(bq *elastic.QueryBuilder) {
				// first group (message_user_id: A and message_peer_user_id: B)
				bq.Should(elastic.NewQueryBuilder().Term("message_user_id.keyword", userID).Term("message_peer_user_id.keyword", peerUserID).BuildQuery())
				// second group (message_user_id: B and message_peer_user_id: A)
				bq.Should(elastic.NewQueryBuilder().Term("message_user_id.keyword", peerUserID).Term("message_peer_user_id.keyword", userID).BuildQuery())
				// require at least one of the should conditions to match
				bq.MinimumShouldMatch(1)
			}).Size(1000)

		if _, err := query.Build(); err != nil {
			t.Fatal(err)
		}
		fmt.Println(query.String())
	}

	{

		query := elastic.NewQueryBuilder().
			Term("chat_type.keyword", "direct").
			Bool(func(bq *elastic.QueryBuilder) {
				bq.Should(elastic.NewQueryBuilder().Term("type.keyword", "message_send").BuildQuery())
				bq.Should(elastic.NewQueryBuilder().Term("type.keyword", "message_recv").BuildQuery())
				bq.MinimumShouldMatch(1)
			}).
			Bool(func(bq *elastic.QueryBuilder) {
				bq.Should(elastic.NewQueryBuilder().Term("message_user_id.keyword", userID).Term("message_peer_user_id.keyword", peerUserID).BuildQuery())
				bq.Should(elastic.NewQueryBuilder().Term("message_user_id.keyword", peerUserID).Term("message_peer_user_id.keyword", userID).BuildQuery())
				bq.MinimumShouldMatch(1)
			})

		if _, err := query.Build(); err != nil {
			t.Fatal(err)
		}
		fmt.Println(query.String())
	}

	{
		query := elastic.NewQueryBuilder().
			Term("chat_type.keyword", "direct").
			Bool(func(bq *elastic.QueryBuilder) {
				bq.Should(elastic.NewQueryBuilder().Term("type.keyword", "message_send").Term("message_user_id.keyword", userID).Term("message_peer_user_id.keyword", peerUserID).BuildQuery())
				bq.Should(elastic.NewQueryBuilder().Term("type.keyword", "message_recv").Term("message_user_id.keyword", userID).Term("message_peer_user_id.keyword", peerUserID).BuildQuery())
				bq.MinimumShouldMatch(1)
			})
		if _, err := query.Build(); err != nil {
			t.Fatal(err)
		}
		fmt.Println(query.String())
	}
}

func TestDocumentSearchAsc(t *testing.T) {
	setupElastic(t)

	dateStr := "2024-10-29T10:34:35.991+08:00" // message_text: "abcdefg"
	date, _ := dateparse.ParseAny(dateStr)
	fmt.Println("date:", date)

	query := elastic.NewQueryBuilder().
		Size(2).
		Source("message_text", "created_at").
		Sort("created_at", elastic.Asc).
		TermNot("message_text.keyword", "").
		Term("type.keyword", "message_send").
		SearchAfter(date)

	res, err := elastic.Document.Search(context.Background(), Index, query.BuildForce())
	require.NoError(t, err)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(res))
}

func TestDocumentSearchDesc(t *testing.T) {
	setupElastic(t)

	dateStr := "2024-10-29T10:34:35.991+08:00" // message_text: "abcdefg"
	date, _ := dateparse.ParseAny(dateStr)
	fmt.Println("date:", date)

	query := elastic.NewQueryBuilder().
		Size(2).
		Source("message_text", "created_at").
		Sort("created_at", elastic.Desc).
		TermNot("message_text.keyword", "").
		Term("type.keyword", "message_send").
		SearchAfter(date)

	res, err := elastic.Document.Search(context.Background(), Index, query.BuildForce())
	require.NoError(t, err)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(res))
}

func TestDocumentSearchNext(t *testing.T) {
	setupElastic(t)

	dateStr := "2024-10-29T10:34:35.991+08:00"
	date, _ := dateparse.ParseAny(dateStr)
	fmt.Println("date:", date)

	query := elastic.NewQueryBuilder().
		Size(2).
		Source("message_text", "created_at").
		Sort("created_at", elastic.Desc).
		TermNot("message_text.keyword", "").
		Term("type.keyword", "message_send").
		SearchAfter(date)

	// fmt.Println(query)
	res, err := elastic.Document.SearchNext(context.Background(), Index, query.BuildForce(), 3)
	require.NoError(t, err)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(res))
}

func TestDocumentSearchPrev(t *testing.T) {
	setupElastic(t)

	dateStr := "2024-10-29T10:34:35.991+08:00"
	date, _ := dateparse.ParseAny(dateStr)
	fmt.Println("date:", date)

	query := elastic.NewQueryBuilder().
		Size(2).
		Source("message_text", "created_at").
		Sort("created_at", elastic.Desc).
		TermNot("message_text.keyword", "").
		Term("type.keyword", "message_send").
		SearchAfter(date)

	fmt.Println(query)
	res, err := elastic.Document.SearchPrev(context.Background(), Index, query.BuildForce(), 4)
	require.NoError(t, err)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(res))
}

func TestDocumentQueryBuilderMatchPharseOptions(t *testing.T) {
	setupElastic(t)

	query := elastic.NewQueryBuilder().
		Size(2).
		Source("message_text", "created_at").
		Sort("created_at", elastic.Desc).
		TermNot("message_text.keyword", "").
		Bool(func(qb *elastic.QueryBuilder) {
			qb.Should(elastic.NewQueryBuilder().Term("type.keyword", "message_send").BuildQuery())
			qb.Should(elastic.NewQueryBuilder().Term("type.keyword", "message_recv").BuildQuery())
			qb.MinimumShouldMatch(1)
		}).
		MatchPhraseOptions("message_text", elastic.MatchPhraseOptions{
			Query: "hello",
			Slop:  util.Pointer(1),
		})
	fmt.Println(query.String())
	res, err := elastic.Document.Search(context.Background(), Index, query.BuildForce())
	require.NoError(t, err)
	_ = res
	// enc := json.NewEncoder(os.Stdout)
	// enc.SetIndent("", "  ")
	// enc.Encode(res)
}

func TestDocumentQueryBuilderAggs(t *testing.T) {
	setupElastic(t)

	now := time.Now()
	query := elastic.NewQueryBuilder().
		Size(0).
		TimeRange("created_at", now.Add(-24*time.Hour*365), now).
		TermNot("message_chat_name.keyword", "").
		AggsTerm("unique_message_chat_name", "message_chat_name.keyword", 1000)
	query2 := elastic.NewQueryBuilder().
		Size(0).
		TimeRange("created_at", now.Add(-24*time.Hour*365), now).
		TermNot("message_chat_name.keyword", "").
		AggsTerm("unique_message_chat_name", "message_chat_name.keyword", 1000, "_count", "asc")
	query3 := elastic.NewQueryBuilder().
		Size(0).
		TimeRange("created_at", now.Add(-24*time.Hour*365), now).
		TermNot("message_user_name.keyword", "").
		AggsTerm("unique_message_user_name", "message_user_name.keyword", 1000, "_count", "asc")
	query4 := elastic.NewQueryBuilder().
		Size(0).
		TimeRange("created_at", now.Add(-24*time.Hour*365), now).
		TermNot("message_peer_user_name.keyword", "").
		AggsTerm("unique_message_peer_user_name", "message_peer_user_name.keyword", 1000, "_count", "asc")
	query5 := elastic.NewQueryBuilder().
		Size(0).
		TermNot("message_chat_name.keyword", "").
		TermNot("message_user_name.keyword", "").
		TermNot("message_peer_user_name.keyword", "").
		AggsTerm("unique_user_name", "message_user_name.keyword", 10000).
		AggsTerm("unique_peer_user_name", "message_peer_user_name.keyword", 10000)

	fmt.Println(query.String())
	fmt.Println(query2.String())
	fmt.Println(query3.String())
	fmt.Println(query4.String())
	fmt.Println(query5.String())
	res, err := elastic.Document.Search(context.Background(), Index, query5.BuildForce())
	require.NoError(t, err)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(res))
}
