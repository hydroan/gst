package vendored

// Vendored must not be extracted because it is declared in a vendor
// directory.
type Vendored struct {
	// Value must not be extracted.
	Value string `json:"value"`
}
