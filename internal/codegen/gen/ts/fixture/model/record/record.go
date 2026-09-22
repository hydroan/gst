// Package record declares a fixture type the sample fixture refers to from
// another package.
package record

// Record is an item a sample points at.
type Record struct {
	// Title is the display title.
	Title    string  `json:"title"`
	Parent   *Record `json:"parent"`
	Progress State   `json:"state"`
}

// State is the progress of a record. Its first constant is the zero value.
type State int

const (
	StateOpen   State = iota // StateOpen marks a record in progress.
	StateClosed              // StateClosed marks a finished record.
)
