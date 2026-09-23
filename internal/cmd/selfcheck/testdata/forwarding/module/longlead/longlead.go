// Package longlead has a function that runs three statements before it
// forwards, too many to count as a shell around the function it forwards to.
package longlead

// Sample counts how often it is opened and closed.
type Sample struct {
	opened int
	closed int
	name   string
}

// Open opens s under name.
func Open(s *Sample, name string) error {
	s.opened++
	s.closed = 0
	s.name = ""
	return open(s, name)
}

func open(s *Sample, name string) error {
	s.name = name
	return nil
}
