package configx

// The environment variables the keys of the cleanup section resolve from.
const (
	CLEANUP_AUDIT_DAYS = "CLEANUP_AUDIT_DAYS"
	CLEANUP_BATCH_SIZE = "CLEANUP_BATCH_SIZE"
)

// Cleanup configures what the cleanup jobs let go of, the section [cleanup]
// of config.ini.
type Cleanup struct {
	// AuditDays is how many days an audit row is kept.
	AuditDays int `json:"audit_days" mapstructure:"audit_days" default:"30"`
	// BatchSize is how many rows one round of a job deletes at most.
	BatchSize int `json:"batch_size" mapstructure:"batch_size" default:"500"`
}
