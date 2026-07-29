package modelregistry

import "github.com/hydroan/gst/types"

var (
	// TableChan is the internal queue for default-database table registration.
	// It receives model values from model.Register for processing by dbruntime.InitDatabase.
	TableChan = make(chan types.Model, 10240)

	// RecordChan is the internal queue for asynchronous seed record insertion.
	// Records are processed after their table has been created.
	RecordChan = make(chan *Record, 1024)
)

// Record describes seed rows that should exist before database CRUD operations run.
type Record struct {
	Table   types.Model
	Rows    any
	Expands []string
}
