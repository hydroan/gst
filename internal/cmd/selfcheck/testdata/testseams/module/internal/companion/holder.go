package companion

// Holder is used by consumer through its method.
type Holder struct{ n int }

// N returns what the holder holds.
func (h Holder) N() int { return h.n }

// NewHolder is used by a test alone, and is marked as the API it is.
//
//gst:api the constructor of Holder, which consumer uses through N
func NewHolder(n int) Holder { return Holder{n: n} }
