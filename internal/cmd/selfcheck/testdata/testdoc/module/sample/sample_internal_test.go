package sample

// helperValue is what the helper returns; the comment opens with another name.
func helper() int { return Sample{}.Value() }

var _ = helper
