// Package database stands for the public package running a transaction.
package database

// Transaction runs fn and returns what it returns.
func Transaction(name string, fn func() error) error { return fn() }
