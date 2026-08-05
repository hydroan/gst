package modelauthz

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// AuthzRule stores authorization policy and grouping rules.
//
// The table is written and read exclusively by the policy adapter in
// authz/rbac, never
// through the framework's CRUD chain, so this model exists to own the schema:
// it is what gg migrate builds the table and its unique index from. The
// adapter never migrates, which keeps a single definition of the table.
//
// It embeds AutoBase because the adapter requires an auto-incrementing integer
// primary key and loads policies in primary-key order. The audit and soft-delete
// columns AutoBase brings are never written by the adapter and stay NULL; Purge
// exists to keep the soft-delete column from ever taking effect, because the
// adapter reads without a deleted_at condition and would keep enforcing a rule
// the framework considers deleted.
//
// Policy rows use ptype "p":
//   - V0: tenant, for example "default"
//   - V1: role, for example "admin"
//   - V2: object path, for example "/api/authz/routes"
//   - V3: action, usually the HTTP method such as "GET"
//   - V4: effect, currently "allow"
//
// Grouping rows use ptype "g":
//   - V0: subject, usually the stable subject ID such as "root"
//   - V1: role, for example "admin"
//   - V2: tenant, for example "default"
//
// Example rows:
//   - p, default, admin, /api/authz/routes, GET, allow
//   - p, default, admin, /api/authz/roles, POST, allow
//   - g, root, admin, default
type AuthzRule struct {
	// The rule columns are NOT NULL so the unique index below can do its job:
	// MySQL treats NULLs as distinct, so a single nullable column would let
	// duplicate rules through. The adapter only ever writes strings, so the
	// empty default matches what it stores for unused positions.
	Ptype string `json:"ptype" gorm:"size:100;not null;default:''" query:"ptype"`
	V0    string `json:"v0,omitempty" gorm:"size:100;not null;default:''" query:"v0"`
	V1    string `json:"v1,omitempty" gorm:"size:100;not null;default:''" query:"v1"`
	V2    string `json:"v2,omitempty" gorm:"size:100;not null;default:''" query:"v2"`
	V3    string `json:"v3,omitempty" gorm:"size:100;not null;default:''" query:"v3"`
	V4    string `json:"v4,omitempty" gorm:"size:100;not null;default:''" query:"v4"`
	V5    string `json:"v5,omitempty" gorm:"size:100;not null;default:''" query:"v5"`

	model.AutoBase
}

func (AuthzRule) Design() {
	dsl.Migrate()
}

// Indexes declares the uniqueness the policy adapter assumes, and the one access
// path that uniqueness cannot serve.
//
// The unique index is what every policy insert conflicts against: the adapter
// issues them with an on-conflict-do-nothing clause, which needs a matching
// constraint to have anything to conflict with. Without one, concurrent writers
// store the same rule twice and a single-rule revocation removes one copy.
//
// The seven columns total 2800 bytes under utf8mb4, within InnoDB's 3072-byte
// index key limit. Widening any of them past size 109 would exceed it.
//
// The second index exists for removing a role's assignments, which matches on
// ptype, role and tenant — columns v1 and v2 for a grouping rule. That skips v0
// and so leaves the unique index with only its first column to narrow by: every
// grouping rule in the deployment is then walked to delete one role's. Each
// engine pays for it differently and both pay. MySQL examined 45,348 rows of a
// 200,000-row table for a delete matching one, and under REPEATABLE READ takes
// a next-key lock on every one of them for the rest of the transaction, which
// is the whole grouping range held while a role is deleted. PostgreSQL keeps
// the scan inside the index but walks the same span, at 142 times the cost of
// the seek. With this index both engines reach the rows directly, and neither
// of the other two removals changes plan: they lead with v0 and keep using the
// unique index.
//
// The rule columns mean different things per ptype — v0 is a tenant in a
// permission and a subject in an assignment — so no single column order serves
// both, and reordering the unique index would only move the miss to another
// removal.
func (cr *AuthzRule) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"Ptype", "V0", "V1", "V2", "V3", "V4", "V5"}, Unique: true},
		{Fields: []string{"Ptype", "V1", "V2"}},
	}
}

// Purge hard-deletes. The adapter neither writes nor filters on the soft-delete
// column, so a soft-deleted rule would stay in force while the framework
// reported it gone.
func (cr *AuthzRule) Purge() bool { return true }

// GetTableName pins the policy table name. It matches what GORM's default
// pluralization would derive today, and stating it keeps the table's identity
// out of the naming strategy's hands.
func (cr *AuthzRule) GetTableName() string { return "authz_rules" }
