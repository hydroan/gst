// Package record declares fixture types the sample fixture refers to from
// another package.
package record

import "github.com/cockroachdb/errors"

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

// Grade is the grade of a record, written as its name.
type Grade int

const (
	GradeLow  Grade = iota + 1 // GradeLow is the lower grade.
	GradeHigh                  // GradeHigh is the higher grade.
)

// MarshalText writes the grade as its name, the zero value as no text.
func (g Grade) MarshalText() ([]byte, error) {
	switch g {
	case 0:
		return nil, nil
	case GradeLow:
		return []byte("low"), nil
	case GradeHigh:
		return []byte("high"), nil
	}
	return nil, errors.Newf("unknown grade %d", int(g))
}

// UnmarshalText reads the grade from its name, the zero value from no text.
func (g *Grade) UnmarshalText(text []byte) error {
	switch string(text) {
	case "":
		*g = 0
	case "low":
		*g = GradeLow
	case "high":
		*g = GradeHigh
	default:
		return errors.Newf("unknown grade %q", text)
	}
	return nil
}
