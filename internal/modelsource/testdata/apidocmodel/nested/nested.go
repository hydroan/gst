package nested

// Nested must not be extracted because it is declared in a directory
// holding a go.mod of its own, whose code belongs to another module.
type Nested struct {
	// Value must not be extracted.
	Value string `json:"value"`
}
