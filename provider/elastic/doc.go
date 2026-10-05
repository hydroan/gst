package elastic

/*
The bool query of Elasticsearch, which the search helpers of this package
compose, combines clauses of four kinds, each holding any number of queries:

 1. must: every clause has to match, like AND, and the matches count
    toward the relevance score. A record of one kind mentioning a word:

        must: [
            { "term": { "kind.keyword": "note" } },
            { "match": { "body": "hello" } }
        ]

 2. must_not: no clause may match, like NOT, and nothing here is scored.
    Leaving one kind of record out:

        must_not: [
            { "term": { "kind.keyword": "draft" } }
        ]

 3. should: a clause may match, like OR, the number required set by
    minimum_should_match, and a match raises the score. A record written
    by or addressed to one author:

        should: [
            { "term": { "author_id.keyword": "a1" } },
            { "term": { "recipient_id.keyword": "a1" } }
        ]

 4. filter: every clause has to match, like must, but nothing is scored
    and the result is cached, which suits ranges and exact values. A time
    range:

        filter: [
            { "range": { "created_at": { "gte": "2023-01-01", "lte": "2023-12-31" } } }
        ]

Exact conditions go under filter, which skips scoring; full-text conditions
under must, which needs a score; optional conditions under should; records
to leave out under must_not. A search for the notes of one author thus puts
the kind under must, the author under should as writer or recipient, the
time range under filter and the kinds to skip under must_not. A nested bool
query scopes its own clauses.
*/
