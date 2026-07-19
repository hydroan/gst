package modelauthz

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// CasbinRule stores Casbin policy and grouping rules in the adapter table.
//
// The RBAC module creates this table through the Casbin GORM adapter.
// Ptype identifies policy or grouping rows, and V0 through V5 map directly to
// Casbin's policy fields. The ID must be an integer because the GORM adapter
// creates an auto-increment primary key for the policy table.
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
	ID    uint64 `json:"id" gorm:"primaryKey;autoIncrement:true"`
	Ptype string `json:"ptype" gorm:"size:100" query:"ptype"`
	V0    string `json:"v0,omitempty" gorm:"size:100" query:"v0"`
	V1    string `json:"v1,omitempty" gorm:"size:100" query:"v1"`
	V2    string `json:"v2,omitempty" gorm:"size:100" query:"v2"`
	V3    string `json:"v3,omitempty" gorm:"size:100" query:"v3"`
	V4    string `json:"v4,omitempty" gorm:"size:100" query:"v4"`
	V5    string `json:"v5,omitempty" gorm:"size:100" query:"v5"`

	model.Base
}

func (CasbinRule) Design() {
	dsl.Migrate()
}

// SetID intentionally ignores custom IDs because the Casbin GORM adapter manages
// this table with an auto-incrementing primary key.
func (cr *CasbinRule) SetID(id ...string) {}

// GetTableName returns the Casbin adapter table name.
//
// gormadapter.NewAdapterByDBWithCustomTable uses casbin_rule, while GORM's
// default pluralized table name would be casbin_rules.
func (cr *CasbinRule) GetTableName() string { return "casbin_rule" }
