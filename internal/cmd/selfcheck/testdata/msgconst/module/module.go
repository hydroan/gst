// Package module stands for the public package forwarding the constructors
// of the error a request is answered with.
package module

import "example.com/module/internal/gst"

// NewError forwards the constructor as a variable.
var NewError = gst.NewError
