// Package family has a method several types of the package declare alike,
// each in a one-line version of its own that the caller runs one by one:
// a shared name is no interface, so each is judged on its own and both,
// wrapping one call with one use each, are reported.
package family

import "strings"

// Sink collects the lines the sections write.
type Sink struct{ lines []string }

func (s *Sink) add(line string) { s.lines = append(s.lines, line) }

// Header and Footer are the sections a rendering writes in turn.
type (
	Header struct{ Title string }
	Footer struct{ Note string }
)

func (h Header) write(s *Sink) { s.add(strings.ToUpper(h.Title)) }

func (f Footer) write(s *Sink) { s.add(strings.TrimSpace(f.Note)) }

// Render writes the sections into a new Sink and returns its lines.
func Render(h Header, f Footer) []string {
	s := &Sink{}
	h.write(s)
	f.write(s)
	return s.lines
}
