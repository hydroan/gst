// Package sample stands for a module source, which answers and registers
// through the public packages alone.
package sample

import "example.com/module"

// answer answers with a sentence the package translates and one it does not.
func answer() []any {
	return []any{
		module.NewError(401, unknownMsg),
		module.NewError(402, unpaidMsg),
	}
}
