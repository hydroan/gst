package gst

import "github.com/hydroan/gst/internal/datatypes"

// Date is a calendar day, a year, a month and a day with no time of day and
// no zone: a model declares a day as a Date (`Day gst.Date`) and an optional
// one as a *Date, stored in a date column. A day is written 2026-01-02 in a
// request body, a response and a URL filter, and is a google.type.Date over
// gRPC; an instant sent for it is refused. Date(t) is the day the time t
// reads in its own location.
type Date = datatypes.Date
