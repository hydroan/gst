package configx

import "github.com/cockroachdb/errors"

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

// Validate holds the section to values a cleanup round can run on. The
// framework calls it once the section is loaded, and the application does
// not start on the error it returns.
func (c Cleanup) Validate() error {
	if c.AuditDays < 1 {
		return errors.Newf("audit_days must be at least 1, got %d", c.AuditDays)
	}
	if c.BatchSize < 1 {
		return errors.Newf("batch_size must be at least 1, got %d", c.BatchSize)
	}
	return nil
}
