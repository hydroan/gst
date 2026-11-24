// Package ab lives in a directory whose name maps to the same TypeScript
// import name as the a-b fixture package.
package ab

// Item is a fixture type referenced from another package.
type Item struct {
	Code string `json:"code"`
}
