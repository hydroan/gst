package modelauthz

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// CasbinRule stores Casbin policy and grouping rules.
//
// The table is written and read exclusively by the Casbin adapter, never
// through the framework's CRUD chain, so this model exists to own the schema:
// it is what gg migrate builds the table and its unique index from. The adapter
// is configured not to migrate, which keeps a single definition of the table.
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
type CasbinRule struct {
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

func (CasbinRule) Design() {
	dsl.Migrate()
}

// Indexes declares the uniqueness Casbin's adapter assumes. Every policy insert
// it issues carries an on-conflict-do-nothing clause, which needs a matching
// unique constraint to have anything to conflict with; without one, concurrent
// writers store the same rule twice and a single-rule revocation then removes
// only one of the copies.
//
// The seven columns total 2800 bytes under utf8mb4, within InnoDB's 3072-byte
// index key limit. Widening any of them past size 109 would exceed it.
func (cr *CasbinRule) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"Ptype", "V0", "V1", "V2", "V3", "V4", "V5"}, Unique: true},
	}
}

// Purge hard-deletes. The adapter neither writes nor filters on the soft-delete
// column, so a soft-deleted rule would stay in force while the framework
// reported it gone.
func (cr *CasbinRule) Purge() bool { return true }

// GetTableName returns the Casbin adapter table name, which GORM's default
// pluralization would otherwise turn into casbin_rules.
func (cr *CasbinRule) GetTableName() string { return "casbin_rule" }
