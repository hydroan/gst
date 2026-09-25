package apidocmodel

// Excluded must not be extracted when excludes lists its file.
type Excluded struct {
	// Value must not be extracted.
	Value string `json:"value"`
}
