// Package apiaction holds the vocabulary of an action a model declares: the
// phases of an action, the action itself and the hooks run before and after
// it, and the rule of the path of a route, the route under the API prefix
// and the segment the CRUD layout appends to it for an action. It is a leaf
// below internal/types, imported by every package naming a phase or
// spelling a route; the public service and router packages forward the
// part a project writes.
package apiaction
