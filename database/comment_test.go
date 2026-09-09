package database_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	gstmysql "github.com/hydroan/gst/database/mysql"
	gstpostgres "github.com/hydroan/gst/database/postgres"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/requestctx"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

// sqlTextCaptureLogger records the statement texts gorm executes through it.
type sqlTextCaptureLogger struct {
	gormlogger.Interface
	mu   sync.Mutex
	sqls []string
}

func (l *sqlTextCaptureLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.mu.Lock()
	l.sqls = append(l.sqls, sql)
	l.mu.Unlock()
	l.Interface.Trace(ctx, begin, fc, err)
}

func (l *sqlTextCaptureLogger) last() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.sqls) == 0 {
		return ""
	}
	return l.sqls[len(l.sqls)-1]
}

// lastStartingWith returns the most recent statement text opening with the
// given verb, and "" when none was recorded.
func (l *sqlTextCaptureLogger) lastStartingWith(verb string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, sql := range slices.Backward(l.sqls) {
		if strings.HasPrefix(sql, verb) {
			return sql
		}
	}
	return ""
}

// requestContext builds a context carrying the trace id the comment draws
// from and the request metadata it must leave out, the way a real request's
// middleware and service context would.
func requestContext(method, route, traceID string) context.Context {
	meta := requestctx.New(requestctx.Fields{Method: method, Route: route})
	return execctx.WithTraceID(requestctx.WithMetadata(context.Background(), meta), traceID)
}

func TestSQLCommentAnnotatesStatements(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})
	ctx := requestContext(http.MethodGet, "/api/v1/users", "trace-0001")
	users := make([]*TestUser, 0)

	// A request's statements carry its trace id and nothing else — the
	// request context above holds method and route too, and asserting the
	// full comment pins that they stay out of it.
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.Contains(t, capture.last(), "/* trace_id='trace-0001' */")

	// Every statement verb carries the same comment: a chain runs one verb,
	// and the annotation must reach INSERT, UPDATE and DELETE like SELECT.
	fresh := &TestUser{Name: "user4", Email: "user4@example.com", Age: 21, ID: "u4"}
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).Create(fresh))
	require.Contains(t, capture.lastStartingWith("INSERT"), "/* trace_id='trace-0001' */")
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).UpdateByID(fresh.ID, colName.Set("user4-renamed")))
	require.Contains(t, capture.lastStartingWith("UPDATE"), "/* trace_id='trace-0001' */")
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).Delete(fresh))
	require.Contains(t, capture.lastStartingWith("DELETE"), "/* trace_id='trace-0001' */")

	// Outside a request there is nothing to report and statements stay clean.
	require.NoError(t, database.DatabaseOn[*TestUser](context.Background(), session).List(&users))
	require.NotContains(t, capture.last(), "trace_id=")
}

// BenchmarkSQLCommentChain measures what the statement comment adds to a
// chain: the request-context case renders and attaches it, the background
// case skips it, and the two share every other cost including the database
// round trip. allocs/op is the number to compare between them.
func BenchmarkSQLCommentChain(b *testing.B) {
	defer cleanupTestData()
	setupTestData(b)

	users := make([]*TestUser, 0, len(ul))
	b.Run("request_context", func(b *testing.B) {
		ctx := requestContext(http.MethodGet, "/api/v1/users", "trace-bench")
		for b.Loop() {
			users = users[:0]
			if err := database.Database[*TestUser](ctx).List(&users); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("background_context", func(b *testing.B) {
		for b.Loop() {
			users = users[:0]
			if err := database.Database[*TestUser](context.Background()).List(&users); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// TestSQLCommentCarriesCronjobRound pins the annotation of a cron round: the
// job name joins the trace id, keys in sqlcommenter's ascending order, the
// name encoded like any other value.
func TestSQLCommentCarriesCronjobRound(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})
	ctx := execctx.WithCronjob(context.Background(), "sample job", "trace-cron")

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.Contains(t, capture.last(), "/* cronjob='sample%20job',trace_id='trace-cron' */")
}

// TestSQLCommentMatchesOperationSpanOutsideRequest pins that the comment is
// attached once the operation's span is open: outside any request or cron
// round the statement carries that span's trace id — the id the SQL log
// records for it — instead of staying clean.
func TestSQLCommentMatchesOperationSpanOutsideRequest(t *testing.T) {
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	defer cleanupTestData()
	setupTestData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](context.Background(), session).List(&users))

	span := oteltest.EndedNamed(t, recorder, "database.TestUser.List")
	require.Contains(t, capture.last(), "/* trace_id='"+span.SpanContext().TraceID().String()+"' */")
}

// TestSQLCommentAnnotatesSelects pins the annotation on the aggregate
// paths, which build their statement on a fresh session: the comment the
// operation attached must reach that statement all the same, on the outer
// statement Count issues too and not only inside its derived table.
func TestSQLCommentAnnotatesSelects(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})
	ctx := requestContext(http.MethodGet, "/api/v1/reports", "trace-agg")

	// Each result row type names exactly the terms its read selects: the
	// aggregate builder rejects a result field no term produces.
	type groupRow struct {
		Category string
		Total    int64
	}
	rows := make([]groupRow, 0)
	require.NoError(t, database.SelectOn[*TestAggregateRecord, groupRow](ctx, session, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		Scan(&rows))
	require.Contains(t, capture.last(), "/* trace_id='trace-agg' */")

	type totalRow struct {
		Total int64
	}
	one := totalRow{}
	require.NoError(t, database.SelectOn[*TestAggregateRecord, totalRow](ctx, session, TestAggregateRecordCols.Amount.Sum().As("total")).
		ScanOne(&one))
	require.Contains(t, capture.last(), "/* trace_id='trace-agg' */")

	groups := 0
	require.NoError(t, database.SelectOn[*TestAggregateRecord, groupRow](ctx, session, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		Count(&groups))
	sql := capture.last()
	require.Contains(t, sql, "/* trace_id='trace-agg' */")
	require.Less(t, strings.Index(sql, "trace_id="), strings.Index(sql, " FROM ("),
		"the outer count must carry the comment itself, before the derived table")
	require.Equal(t, 2, strings.Count(sql, "trace_id="),
		"the derived table keeps its own copy inside; the count carries exactly two")

	// A union builds its statement on a chain minted for it and stacks
	// members rendered without a comment: the comment must reach the union's
	// own statement, once, for the scan and for the count alike.
	done := database.SelectOn[*TestAggregateRecord, groupRow](ctx, session, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		Where(TestAggregateRecordCols.Status.Eq("done"))
	failed := database.SelectOn[*TestAggregateRecord, groupRow](ctx, session, TestAggregateRecordCols.Category.Group(), TestAggregateRecordCols.Amount.Sum().As("total")).
		Where(TestAggregateRecordCols.Status.Eq("failed"))
	stacked := make([]groupRow, 0)
	require.NoError(t, database.UnionAllOn[groupRow](ctx, session, done, failed).Scan(&stacked))
	sql = capture.last()
	require.Contains(t, sql, "/* trace_id='trace-agg' */")
	require.Equal(t, 1, strings.Count(sql, "trace_id="), "the members carry no comment of their own")
	require.NoError(t, database.UnionAllOn[groupRow](ctx, session, done, failed).Count(&groups))
	sql = capture.last()
	require.Contains(t, sql, "/* trace_id='trace-agg' */")
	require.Equal(t, 1, strings.Count(sql, "trace_id="))
}

func TestSQLCommentAnnotatesAQualifiedSelectOnce(t *testing.T) {
	defer cleanupAggregateData()
	setupAggregateData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})
	ctx := requestContext(http.MethodGet, "/api/v1/reports", "trace-qualify")

	// Qualify wraps the projection in a derived table; the statement is one
	// statement and carries the comment once, on the outer select.
	type numbered struct {
		ID string
		Rn int64
	}
	rn := types.RowNumber().Over(types.PartitionBy(TestAggregateRecordCols.Category).OrderBy(TestAggregateRecordCols.OccurredAt.Desc())).As("rn")
	latest := database.SelectOn[*TestAggregateRecord, numbered](ctx, session, TestAggregateRecordCols.ID, rn).Qualify(rn.Eq(1))
	rows := make([]numbered, 0)
	require.NoError(t, latest.Scan(&rows))
	sql := capture.last()
	require.Contains(t, sql, "/* trace_id='trace-qualify' */")
	require.Equal(t, 1, strings.Count(sql, "trace_id="))

	// A select that already ran keeps the comment of its own operation; as a
	// member of a union it still renders without one, the union's statement
	// carrying the comment once.
	other := database.SelectOn[*TestAggregateRecord, numbered](ctx, session, TestAggregateRecordCols.ID, rn).Qualify(rn.Eq(2))
	require.NoError(t, database.UnionAllOn[numbered](ctx, session, latest, other).Scan(&rows))
	sql = capture.last()
	require.Contains(t, sql, "/* trace_id='trace-qualify' */")
	require.Equal(t, 1, strings.Count(sql, "trace_id="), "the members carry no comment of their own, however they were used before")
}

// afterVerbExpr stands in for an expression another party registered after
// the SELECT verb before the chain attached its comment.
type afterVerbExpr string

func (e afterVerbExpr) ModifyStatement(stmt *gorm.Statement) {
	verb := stmt.Clauses["SELECT"]
	verb.AfterExpression = e
	stmt.Clauses["SELECT"] = verb
}

func (e afterVerbExpr) Build(builder clause.Builder) { _, _ = builder.WriteString(string(e)) }

func TestSQLCommentJoinsExistingAfterExpression(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	// The session already carries an expression after the verb when the chain
	// attaches its comment; both must render, in registration order.
	session := database.DB().Session(&gorm.Session{Logger: capture}).Clauses(afterVerbExpr("/* first */"))

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](requestContext("", "", "trace-join"), session).List(&users))
	require.Contains(t, capture.last(), "/* first */ /* trace_id='trace-join' */")
}

func TestSQLCommentEscapesHostileValues(t *testing.T) {
	// A value must never be able to close the comment and smuggle SQL: the
	// URL encoding turns the closing sequence into inert text.
	defer cleanupTestData()
	setupTestData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})
	ctx := requestContext("", "", "/x */ DROP TABLE test_users --")

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.NotContains(t, capture.last(), "*/ DROP")
	require.Contains(t, capture.last(), "trace_id='")
}

// TestSQLCommentPercentEncodesValues pins the sqlcommenter escaping
// convention on the value the comment carries.
func TestSQLCommentPercentEncodesValues(t *testing.T) {
	// A space renders as %20 rather than the form-urlencoded +, and a literal
	// plus as %2B, so percent-decoding the value recovers it exactly.
	defer cleanupTestData()
	setupTestData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})
	ctx := requestContext("", "", "/a b+c")

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.Contains(t, capture.last(), "trace_id='%2Fa%20b%2Bc'")
}

func TestSQLCommentTextProtocolConnection(t *testing.T) {
	// mysql and postgres connections run per-statement text protocol
	// (interpolateParams on MySQL, simple protocol on postgres); this pins
	// that such a connection serves real queries, comments included.
	var handle *gorm.DB
	var err error
	switch config.App.Database.Type {
	case config.DBMySQL:
		handle, err = gstmysql.New(config.App.MySQL)
	case config.DBPostgres:
		handle, err = gstpostgres.New(config.App.Postgres)
	default:
		t.Skipf("text-protocol connection settings exist on mysql and postgres, the test database is %s", config.App.Database.Type)
	}
	require.NoError(t, err)

	defer cleanupTestData()
	setupTestData(t)
	capture := &sqlTextCaptureLogger{Interface: handle.Logger}
	session := handle.Session(&gorm.Session{Logger: capture})

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](requestContext(http.MethodGet, "/trace", "trace-conn"), session).
		WithQuery(&TestUser{Name: u1.Name}).List(&users))
	require.Len(t, users, 1)
	require.Contains(t, capture.last(), "trace_id='trace-conn'")
}
