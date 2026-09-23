// Package broken does not type-check.
package broken

// Count is declared with a value of the wrong type.
var Count int = "one"
