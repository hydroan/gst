// Package lead has a function that runs one statement of its own before it
// forwards to a function nothing else uses.
package lead

// Sample counts how often it is opened.
type Sample struct {
	opened int
	name   string
}

// Open opens s under name.
func Open(s *Sample, name string) error {
	s.opened++
	return open(s, name)
}

func open(s *Sample, name string) error {
	s.name = name
	return nil
}
