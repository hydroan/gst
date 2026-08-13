package config

import (
	"time"

	"github.com/spf13/viper"
)

const (
	LOGMGMT_RETENTION    = "LOGMGMT_RETENTION"    //nolint:staticcheck
	LOGMGMT_CLEANUP_CRON = "LOGMGMT_CLEANUP_CRON" //nolint:staticcheck
)

// Logmgmt configures the log-management module.
//
// CleanupCron is read through the environment key only: the module registers
// its cronjob at package initialization, before configuration files are
// loaded, so a file-provided schedule could not take effect there anyway.
type Logmgmt struct {
	// Retention is how long operation and login logs are kept before the
	// cleanup job removes them.
	Retention time.Duration `json:"retention" mapstructure:"retention" ini:"retention" yaml:"retention"`
}

func (*Logmgmt) setDefault(v *viper.Viper) {
	v.SetDefault("logmgmt.retention", "2160h") // 90 days
}
