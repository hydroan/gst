package used

// Used is used by consumer; its directive is stale.
//
//gst:api left over
func Used() int { return 3 }

// Tagged is used by consumer's constrained file alone.
func Tagged() int { return 4 }
