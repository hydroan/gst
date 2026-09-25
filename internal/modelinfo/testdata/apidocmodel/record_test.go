package apidocmodel

// InTestFile must not be extracted because it is declared in a test file.
type InTestFile struct {
	// Value must not be extracted.
	Value string `json:"value"`
}
