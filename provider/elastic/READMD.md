## DLS 语法

### Match 查询（全文搜索）：

```go
// 基础 match 查询
req := &SearchRequest{
    Query: map[string]any{
        "match": map[string]any{
            "message": "hello world",
        },
    },
}

// match_phrase（短语匹配，要求词序一致）
req := &SearchRequest{
    Query: map[string]any{
        "match_phrase": map[string]any{
            "message": "hello world",
        },
    },
}

// multi_match（多字段匹配）
req := &SearchRequest{
    Query: map[string]any{
        "multi_match": map[string]any{
            "query": "hello world",
            "fields": []string{"message", "title", "content"},
        },
    },
}
```

### Term 查询（精确匹配）：

```go
// 单个 term 查询
req := &SearchRequest{
    Query: map[string]any{
        "term": map[string]any{
            "status": "active",
        },
    },
}

// terms 查询（多值匹配）
req := &SearchRequest{
    Query: map[string]any{
        "terms": map[string]any{
            "status": []string{"active", "pending"},
        },
    },
}
```

### Range 查询（范围查询）：

```go
req := &SearchRequest{
    Query: map[string]any{
        "range": map[string]any{
            "age": map[string]any{
                "gte": 18,
                "lte": 30,
            },
        },
    },
}

// 日期范围查询
req := &SearchRequest{
    Query: map[string]any{
        "range": map[string]any{
            "created_at": map[string]any{
                "gte": "2024-01-01",
                "lte": "now",
                "format": "yyyy-MM-dd",
            },
        },
    },
}
```

### Bool 查询（组合查询）：

```go
req := &SearchRequest{
    Query: map[string]any{
        "bool": map[string]any{
            // must: AND 关系，必须满足
            "must": []map[string]any{
                {
                    "match": map[string]any{
                        "title": "search keyword",
                    },
                },
                {
                    "term": map[string]any{
                        "status": "active",
                    },
                },
            },
            // should: OR 关系，可以满足
            "should": []map[string]any{
                {
                    "term": map[string]any{
                        "tag": "important",
                    },
                },
            },
            // must_not: NOT 关系，必须不满足
            "must_not": []map[string]any{
                {
                    "term": map[string]any{
                        "status": "deleted",
                    },
                },
            },
            // filter: 过滤，不计算相关性得分
            "filter": []map[string]any{
                {
                    "range": map[string]any{
                        "created_at": map[string]any{
                            "gte": "2024-01-01",
                        },
                    },
                },
            },
        },
    },
}
```

### 嵌套查询（Nested Query）：

```go
req := &SearchRequest{
    Query: map[string]any{
        "nested": map[string]any{
            "path": "comments",
            "query": map[string]any{
                "bool": map[string]any{
                    "must": []map[string]any{
                        {
                            "match": map[string]any{
                                "comments.text": "great",
                            },
                        },
                    },
                },
            },
        },
    },
}
```

### 聚合查询（Aggregations）：

```go
req := &SearchRequest{
    Size: 0, // 不需要搜索结果时设为0
    Query: map[string]any{
        "match_all": map[string]any{},
    },
    "aggs": map[string]any{
        "status_count": map[string]any{
            "terms": map[string]any{
                "field": "status",
            },
        },
        "avg_age": map[string]any{
            "avg": map[string]any{
                "field": "age",
            },
        },
    },
}
```

### 排序：

```go
req := &SearchRequest{
    Query: map[string]any{
        "match_all": map[string]any{},
    },
    Sort: []map[string]any{
        {
            "created_at": map[string]any{
                "order": "desc",
            },
        },
        {
            "_score": map[string]any{
                "order": "desc",
            },
        },
    },
}
```

### 分页：

```go
req := &SearchRequest{
    Query: map[string]any{
        "match_all": map[string]any{},
    },
    From: 0,   // 起始位置
    Size: 10,  // 每页大小
}
```



### 建议

1.  对于精确匹配使用 term 查询
2.  对于全文搜索使用 match 查询
3.  使用 bool 查询组合多个条件
4.  使用 filter 进行过滤可以提高性能
5.  合理使用分页避免深分页问题

### 示例

```go
// 复杂查询示例
req := &SearchRequest{
    Query: map[string]any{
        "bool": map[string]any{
            "must": []map[string]any{
                {
                    "match": map[string]any{
                        "title": "search keyword",
                    },
                },
            },
            "filter": []map[string]any{
                {
                    "term": map[string]any{
                        "status": "active",
                    },
                },
                {
                    "range": map[string]any{
                        "created_at": map[string]any{
                            "gte": "2024-01-01",
                            "lte": "now",
                        },
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
    From: 0,
    Size: 20,
}
```

