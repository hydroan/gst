// Package converted has functions that pass their parameters on unchanged but
// under different types, so taking them away would change a conversion.
package converted

import "fmt"

// Failure is an error with a code.
type Failure struct{ Code int }

func (f *Failure) Error() string { return fmt.Sprintf("failure %d", f.Code) }

// Check fails for a negative n.
func Check(n int) error { return check(n) }

func check(n int) *Failure {
	if n < 0 {
		return &Failure{Code: n}
	}
	return nil
}

// Sample is a named value.
type Sample struct{ Name string }

func (s Sample) String() string { return s.Name }

// Describe describes s.
func Describe(s Sample) string { return describe(s) }

func describe(v fmt.Stringer) string { return v.String() }
