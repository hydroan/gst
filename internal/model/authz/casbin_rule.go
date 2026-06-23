package modelauthz

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// CasbinRule stores Casbin policy and grouping rules.
//
// The RBAC module creates this table through
// gormadapter.NewAdapterByDBWithCustomTable(database.DB(), new(modelauthz.CasbinRule)).
// The ID must be an integer because the GORM adapter creates an auto-increment
// primary key for the policy table.
//
// Policy example:
//
// INSERT INTO casbin_rule (ptype, v0, v1, v2, v3) VALUES
// ('p', 'role_admin', '/api/config/*', 'GET', 'allow'),
// ('p', 'role_admin', '/api/config/*', 'POST', 'allow'),
// ('p', 'role_user', '/api/config/file', 'GET', 'allow');
//
// Grouping example:
//
// INSERT INTO casbin_rule (ptype, v0, v1) VALUES
// ('g', 'alice', 'role_admin'),
// ('g', 'bob', 'role_user'),
// ('g', 'role_admin', 'admin'); -- admin super role.
type CasbinRule struct {
	// ID uint64 `json:"id" gorm:"primaryKey"`
	ID    uint64 `json:"id" gorm:"primaryKey;autoIncrement:true"`
	Ptype string `json:"ptype" gorm:"size:100" schema:"ptype"`
	V0    string `json:"v0,omitempty" gorm:"size:100" schema:"v0"`
	V1    string `json:"v1,omitempty" gorm:"size:100" schema:"v1"`
	V2    string `json:"v2,omitempty" gorm:"size:100" schema:"v2"`
	V3    string `json:"v3,omitempty" gorm:"size:100" schema:"v3"`
	V4    string `json:"v4,omitempty" gorm:"size:100" schema:"v4"`
	V5    string `json:"v5,omitempty" gorm:"size:100" schema:"v5"`

	User   string  `json:"user,omitempty" schema:"user"`                       // Informational user value copied from V0.
	Role   string  `json:"role,omitempty" schema:"role"`                       // Informational role value copied from V1.
	Remark *string `json:"remark,omitempty" gorm:"size:10240" schema:"remark"` // Optional policy summary.

	model.Base
}

func (CasbinRule) Design() {
	dsl.Migrate(true)
}

// SetID intentionally ignores custom IDs because the Casbin GORM adapter manages
// this table with an auto-incrementing primary key.
func (cr *CasbinRule) SetID(id ...string) {}

// GetTableName returns the Casbin adapter table name.
//
// gormadapter.NewAdapterByDBWithCustomTable uses casbin_rule, while GORM's
// default pluralized table name would be casbin_rules.
func (cr CasbinRule) GetTableName() string { return "casbin_rule" }
