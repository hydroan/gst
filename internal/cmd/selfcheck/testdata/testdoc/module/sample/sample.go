package sample

// Other is what a non-test file may open its comment with: the lint
// configuration holds non-test files, not this check.
type Sample struct{}

// Value is the one value of a Sample.
func (Sample) Value() int { return 1 }
