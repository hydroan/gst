package config

import (
	"github.com/hydroan/gst/consts"

	"github.com/spf13/viper"
)

const (
	AUDIT_ENABLED            = "AUDIT_ENABLED"
	AUDIT_EXCLUDE_OPERATIONS = "AUDIT_EXCLUDE_OPERATIONS"
)

// Audit configures the operation log: whether the framework records what its
// handlers did, and which operations it leaves out. An entry is written in
// the request that produced it, so a request that answered success has its
// record before it answers.
type Audit struct {
	Enabled           bool        `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
	ExcludeOperations []consts.OP `json:"exclude_operations" mapstructure:"exclude_operations" ini:"exclude_operations" yaml:"exclude_operations"`
}

func (*Audit) setDefault(v *viper.Viper) {
	v.SetDefault("audit.enabled", false)
	// Reads are left out by default: they are the bulk of the traffic, and an
	// entry per read would cost a write per read.
	v.SetDefault("audit.exclude_operations", []consts.OP{consts.OP_LIST, consts.OP_GET})
}
