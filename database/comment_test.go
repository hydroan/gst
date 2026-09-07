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
	"github.com/hydroan/gst/internal/requestctx"
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

// requestContext builds a context carrying the request metadata the comment
// draws from, the way a real request's middleware would.
func requestContext(method, route, traceID string) context.Context {
	meta := requestctx.New(requestctx.Fields{Method: method, Route: route, TraceID: traceID})
	return requestctx.WithMetadata(context.Background(), meta)
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
