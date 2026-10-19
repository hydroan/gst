package companion

// Used is used by consumer.
func Used() int { return 1 }

// Companion is used by a test alone, and is marked as the API it is.
//
//gst:api the counterpart of Used
func Companion() int { return 2 }

// Knob is used by a test alone and marked without a reason.
//
//gst:api
var Knob = 3
