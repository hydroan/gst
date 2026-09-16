// Package notifier declares a fixture type a model refers to from outside the
// model directory.
package notifier

// Endpoint is where a sample reports to.
type Endpoint struct {
	// URL is the address reports go to.
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}
