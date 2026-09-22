package fixture

// Fixture must not be extracted because it is declared in a testdata
// directory.
type Fixture struct {
	// Value must not be extracted.
	Value string `json:"value"`
}
