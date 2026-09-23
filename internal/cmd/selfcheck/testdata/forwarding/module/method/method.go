// Package method has a method that only forwards to another method of its
// receiver and has one use.
package method

// Store holds items.
type Store struct{ items []string }

// Empty reports whether s holds no items.
func (s *Store) Empty() bool { return s.size() == 0 }

// Full reports whether s holds as many items as it has room for.
func (s *Store) Full() bool { return s.length() == cap(s.items) }

func (s *Store) size() int { return s.length() }

func (s *Store) length() int { return len(s.items) }
