// Package recvarg has a method that only passes its receiver and parameter on
// to a function, and has one use.
package recvarg

import "strings"

// Item is a named value.
type Item struct{ Name string }

// Rename gives it the name, trimmed of spaces.
func Rename(it *Item, name string) { it.rename(strings.TrimSpace(name)) }

func (it *Item) rename(name string) { setName(it, name) }

func setName(it *Item, name string) { it.Name = name }
