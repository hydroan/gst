// Package dbnode names the roles of the nodes a database handle writes to
// and reads from, and stamps the role that serves a statement on the
// statement's context, for the SQL log to report which node ran it.
package dbnode

import "context"

// The roles a node plays: the primary takes every write and the reads not
// sent to a replica, a replica takes the reads sent to it.
const (
	RolePrimary = "primary"
	RoleReplica = "replica"
)

// roleContextKey carries the node role that served one statement.
type roleContextKey struct{}

// WithRole returns ctx stamped with the node role that serves the statement
// it belongs to; RoleFromContext reads it back.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleContextKey{}, role)
}

// RoleFromContext reports which node role served the statement this context
// belongs to, and "" when the statement ran on a handle without replicas:
// role stamping is only installed alongside a resolver, so a replica-free
// deployment logs no role field at all.
func RoleFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	role, _ := ctx.Value(roleContextKey{}).(string)
	return role
}
