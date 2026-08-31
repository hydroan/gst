package database_test

import (
	"context"
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

// swapSQLCommentMode switches the comment mode for one test and restores it.
func swapSQLCommentMode(t *testing.T, mode config.SQLCommentMode) {
	t.Helper()
	previous := config.App.Database.SQLComment
	config.App.Database.SQLComment = mode
	t.Cleanup(func() { config.App.Database.SQLComment = previous })
}

// requestContext builds a context carrying the request metadata the comment
// draws from, the way a real request's middleware would.
func requestContext(route, traceID string) context.Context {
	meta := requestctx.New(requestctx.Fields{Route: route, TraceID: traceID})
	return requestctx.WithMetadata(context.Background(), meta)
}

func TestSQLCommentAnnotatesStatements(t *testing.T) {
	defer cleanupTestData()
	setupTestData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})
	ctx := requestContext("/api/v1/users", "trace-0001")
	users := make([]*TestUser, 0)

	// The route mode annotates with the URL-encoded issuing route.
	swapSQLCommentMode(t, config.SQLCommentRoute)
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.Contains(t, capture.last(), "route='%2Fapi%2Fv1%2Fusers'")
	require.NotContains(t, capture.last(), "trace_id")

	// The trace mode adds the request's trace id.
	swapSQLCommentMode(t, config.SQLCommentTrace)
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.Contains(t, capture.last(), "route='%2Fapi%2Fv1%2Fusers'")
	require.Contains(t, capture.last(), "trace_id='trace-0001'")

	// Off renders nothing.
	swapSQLCommentMode(t, config.SQLCommentOff)
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.NotContains(t, capture.last(), "route=")

	// Outside a request there is no route to report and statements stay
	// clean, whatever the mode.
	swapSQLCommentMode(t, config.SQLCommentRoute)
	require.NoError(t, database.DatabaseOn[*TestUser](context.Background(), session).List(&users))
	require.NotContains(t, capture.last(), "route=")
}

func TestSQLCommentEscapesHostileValues(t *testing.T) {
	// A value must never be able to close the comment and smuggle SQL: the
	// URL encoding turns the closing sequence into inert text.
	defer cleanupTestData()
	setupTestData(t)
	swapSQLCommentMode(t, config.SQLCommentRoute)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})
	ctx := requestContext("/x */ DROP TABLE test_users --", "")

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.NotContains(t, capture.last(), "*/ DROP")
	require.Contains(t, capture.last(), "route='")
}

func TestSQLCommentTraceConnection(t *testing.T) {
	// The trace mode switches new connections to per-statement text protocol
	// (interpolateParams on MySQL, simple protocol on postgres); this pins
	// that such a connection serves real queries, comments included.
	swapSQLCommentMode(t, config.SQLCommentTrace)

	var handle *gorm.DB
	var err error
	switch config.App.Database.Type {
	case config.DBMySQL:
		handle, err = gstmysql.New(config.App.MySQL)
	case config.DBPostgres:
		handle, err = gstpostgres.New(config.App.Postgres)
	default:
		t.Skipf("trace-mode connections exist on mysql and postgres, the test database is %s", config.App.Database.Type)
	}
	require.NoError(t, err)

	defer cleanupTestData()
	setupTestData(t)
	capture := &sqlTextCaptureLogger{Interface: handle.Logger}
	session := handle.Session(&gorm.Session{Logger: capture})

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](requestContext("GET /trace", "trace-conn"), session).
		WithQuery(&TestUser{Name: u1.Name}).List(&users))
	require.Len(t, users, 1)
	require.Contains(t, capture.last(), "trace_id='trace-conn'")
}
