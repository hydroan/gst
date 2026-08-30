package dbruntime

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// Node roles of a replicated database handle.
const (
	RolePrimary = "primary"
	RoleReplica = "replica"
)

// DBNode is one connection pool of a database handle, named by its role.
type DBNode struct {
	Role string
	DB   *sql.DB
}

// nodeRegistry maps a connection handle to its nodes, keyed by the handle
// pointer — the same identity that keys context transactions. Only handles
// with replicas attached appear here; a plain handle's single pool needs no
// registry.
var nodeRegistry sync.Map

// AttachNodes records the nodes behind one handle; nil detaches.
func AttachNodes(handle *gorm.DB, nodes []DBNode) {
	if len(nodes) == 0 {
		nodeRegistry.Delete(handle)
		return
	}
	nodeRegistry.Store(handle, nodes)
}

// NodesFor reports the nodes attached to handle, nil for a plain handle.
func NodesFor(handle *gorm.DB) []DBNode {
	if nodes, ok := nodeRegistry.Load(handle); ok {
		return nodes.([]DBNode) //nolint:errcheck
	}
	return nil
}

// roleContextKey carries the node role that served one statement.
type roleContextKey struct{}

// RoleFromContext reports which node role served the statement this context
// belongs to, and "" when the statement ran on a handle without replicas —
// role stamping is only installed alongside a resolver, so a replica-free
// deployment logs no role field at all.
func RoleFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	role, _ := ctx.Value(roleContextKey{}).(string)
	return role
}

// ParseReplicaEndpoint splits one configured replica entry into host and
// port. The accepted shape is exactly "host:port" — a replica shares every
// other connection setting with the primary, so the address is all an entry
// may carry. A malformed entry fails database initialization instead of
// being skipped: a replica that silently never joined would disguise a
// configuration typo as "all reads on the primary".
func ParseReplicaEndpoint(endpoint string) (host string, port uint, err error) {
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", 0, errors.Wrapf(err, "invalid replica endpoint %q, want host:port", endpoint)
	}
	parsed, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		return "", 0, errors.Wrapf(err, "invalid replica endpoint %q, want host:port", endpoint)
	}
	return host, uint(parsed), nil
}

// AttachResolver wires replica dialectors into a handle and returns the
// Write-pinned handle the application must keep. It is the one place the
// whole replica setup happens, shared by the dialect packages:
//
//   - the resolver is installed with the framework's pool limits applied to
//     EVERY node — a replica pool with no limits would grow unbounded;
//   - a role observer stamps each statement's context with the node role
//     that served it, which the SQL logger surfaces as db_role and the
//     active span records as db.role — with replicas in play, "which node
//     answered this query" must never be a guess;
//   - the handle's nodes are registered under the pinned handle pointer, so
//     database.Stats and the pool metrics see every pool;
//   - the returned handle carries the Write pin that keeps reads on the
//     primary unless a model or call site says otherwise.
func AttachResolver(db *gorm.DB, replicas []gorm.Dialector) (*gorm.DB, error) {
	resolver := dbresolver.Register(dbresolver.Config{Replicas: replicas})
	resolver.SetMaxIdleConns(config.App.Database.MaxIdleConns).
		SetMaxOpenConns(config.App.Database.MaxOpenConns).
		SetConnMaxLifetime(config.App.Database.ConnMaxLifetime).
		SetConnMaxIdleTime(config.App.Database.ConnMaxIdleTime)
	if err := db.Use(resolver); err != nil {
		return nil, errors.Wrap(err, "failed to install read-replica resolver")
	}

	primaryPool := db.Config.ConnPool
	if prepared, ok := primaryPool.(*gorm.PreparedStmtDB); ok {
		primaryPool = prepared.ConnPool
	}
	if err := installRoleObserver(db, primaryPool); err != nil {
		return nil, err
	}

	pinned := db.Clauses(dbresolver.Write)
	nodes := make([]DBNode, 0, len(replicas)+1)
	if err := resolver.Call(func(pool gorm.ConnPool) error {
		sqlDB, ok := pool.(*sql.DB)
		if !ok {
			return nil
		}
		role := RoleReplica
		if pool == primaryPool {
			role = RolePrimary
		}
		nodes = append(nodes, DBNode{Role: role, DB: sqlDB})
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "failed to enumerate database nodes")
	}
	AttachNodes(pinned, nodes)
	return pinned, nil
}

// installRoleObserver registers the role-stamping callbacks. They run after
// the resolver's own callbacks — "*"-anchored callbacks sort first — so the
// statement's ConnPool is already the pool that will serve it.
func installRoleObserver(db *gorm.DB, primaryPool gorm.ConnPool) error {
	mark := func(stmt *gorm.DB) { markStatementRole(stmt, primaryPool) }
	for _, register := range []func() error{
		func() error { return db.Callback().Create().Before("gorm:create").Register("gst:db_role", mark) },
		func() error { return db.Callback().Query().Before("gorm:query").Register("gst:db_role", mark) },
		func() error { return db.Callback().Update().Before("gorm:update").Register("gst:db_role", mark) },
		func() error { return db.Callback().Delete().Before("gorm:delete").Register("gst:db_role", mark) },
		func() error { return db.Callback().Row().Before("gorm:row").Register("gst:db_role", mark) },
		func() error { return db.Callback().Raw().Before("gorm:raw").Register("gst:db_role", mark) },
	} {
		if err := register(); err != nil {
			return errors.Wrap(err, "failed to register database role observer")
		}
	}
	return nil
}

// markStatementRole resolves which role serves the statement and stamps it
// into the statement context, plus onto the active operation span. A
// transaction connection always answers primary — transactions never leave
// it — and everything else compares against the primary pool, unwrapping the
// per-pool prepared-statement layer the resolver adds.
func markStatementRole(stmt *gorm.DB, primaryPool gorm.ConnPool) {
	pool := stmt.Statement.ConnPool
	role := RolePrimary
	if _, inTx := pool.(gorm.TxCommitter); !inTx {
		if prepared, ok := pool.(*gorm.PreparedStmtDB); ok {
			pool = prepared.ConnPool
		}
		if pool != primaryPool {
			role = RoleReplica
		}
	}
	stmt.Statement.Context = context.WithValue(stmt.Statement.Context, roleContextKey{}, role)
	if span := trace.SpanFromContext(stmt.Statement.Context); span.IsRecording() {
		span.SetAttributes(attribute.String("db.role", role))
	}
}

// replicaPoolMetricNames names the pool metric of each node for one handle:
// the primary keeps the base name, replicas append their index.
func replicaPoolMetricNames(base string, nodes []DBNode) []string {
	names := make([]string, 0, len(nodes))
	replicaIndex := 0
	for _, node := range nodes {
		if node.Role == RolePrimary {
			names = append(names, base)
			continue
		}
		names = append(names, fmt.Sprintf("%s_replica_%d", base, replicaIndex))
		replicaIndex++
	}
	return names
}
