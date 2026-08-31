package database_test

import (
	"context"
	"net/http"
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

	// A request's statements carry its URL-encoded method, route, and trace
	// id. Asserting the keys as one run pins the ascending order the
	// sqlcommenter convention requires.
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.Contains(t, capture.last(), "method='GET',route='%2Fapi%2Fv1%2Fusers',trace_id='trace-0001'")

	// Outside a request there is nothing to report and statements stay clean.
	require.NoError(t, database.DatabaseOn[*TestUser](context.Background(), session).List(&users))
	require.NotContains(t, capture.last(), "route=")
}

func TestSQLCommentEscapesHostileValues(t *testing.T) {
	// A value must never be able to close the comment and smuggle SQL: the
	// URL encoding turns the closing sequence into inert text.
	defer cleanupTestData()
	setupTestData(t)

	capture := &sqlTextCaptureLogger{Interface: database.DB().Logger}
	session := database.DB().Session(&gorm.Session{Logger: capture})
	ctx := requestContext("", "/x */ DROP TABLE test_users --", "")

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.NotContains(t, capture.last(), "*/ DROP")
	require.Contains(t, capture.last(), "route='")
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
	ctx := requestContext("", "/a b+c", "")

	users := make([]*TestUser, 0)
	require.NoError(t, database.DatabaseOn[*TestUser](ctx, session).List(&users))
	require.Contains(t, capture.last(), "route='%2Fa%20b%2Bc'")
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
