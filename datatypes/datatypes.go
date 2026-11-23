// Package datatypes holds the types a model declares the fields of a kind of
// its own with: a calendar day, a time of day, and a JSON document stored in
// one column.
//
//	type Record struct {
//		model.Base
//
//		Day     datatypes.Date              `json:"day"`
//		Opens   datatypes.TimeOfDay         `json:"opens"`
//		Tags    datatypes.JSONSlice[string] `json:"tags"`
//		Meta    datatypes.JSONMap           `json:"meta"`
//		Options datatypes.JSON[Options]     `json:"options"`
//	}
//
// Each type creates its column on every database the framework speaks, and
// is written in a request and a response, in a URL filter and over gRPC the
// one way its comment tells. An optional column is a pointer to the type.
//
// The implementation lives in internal/datatypes; this package forwards it
// to projects.
package datatypes

import "github.com/hydroan/gst/internal/datatypes"

// Date is a calendar day, a year, a month and a day with no time of day and
// no zone: a model declares a day as a Date (`Day datatypes.Date`) and an
// optional one as a *Date, stored in a date column. A day is written
// 2026-01-02 in a request body, a response and a URL filter, and is a
// google.type.Date over gRPC; an instant sent for it is refused. Date(t) is
// the day the time t reads in its own location. Two days are compared with
// Equal, Before, After and Compare, and counted with AddDays and DaysSince,
// each by the day whatever time the Date was made of.
type Date = datatypes.Date

// ParseDate reads a day the one way the framework takes it: 2006-01-02, a
// real day of the calendar. An instant is refused.
func ParseDate(value string) (Date, error) {
	return datatypes.ParseDate(value)
}

// TimeOfDay is a time of day, the time since midnight a clock shows, with
// no day and no zone: a model declares one as a TimeOfDay
// (`Opens datatypes.TimeOfDay`) and an optional one as a *TimeOfDay, stored
// in a time column. A time of day is written 17:30:00 in a request body, a
// response and a URL filter, with the nine digits of its fraction of a
// second when it has one, and is a google.type.TimeOfDay over gRPC; a text
// time.TimeOnly does not spell is refused.
type TimeOfDay = datatypes.TimeOfDay

// NewTimeOfDay returns the time of day hour hours, minute minutes, second
// seconds and nanosecond nanoseconds after midnight.
func NewTimeOfDay(hour, minute, second, nanosecond int) TimeOfDay {
	return datatypes.NewTimeOfDay(hour, minute, second, nanosecond)
}

// ParseTimeOfDay reads a time of day the one way the framework takes it:
// time.TimeOnly, 15:04:05, a fraction of a second allowed, so 17:30:00.5 is
// half a second past 17:30. Anything else is refused.
func ParseTimeOfDay(value string) (TimeOfDay, error) {
	return datatypes.ParseTimeOfDay(value)
}

// JSON is a value of type T a column stores as one JSON document: a model
// declares one as a JSON of the type it holds (`Options
// datatypes.JSON[Options]`), a json.RawMessage for a document kept as it was
// written. A request and a response carry the value as T itself is written
// in JSON. NewJSON wraps a value and Data returns it. An array is declared a
// JSONSlice and an object of keys known only as it is read a JSONMap.
type JSON[T any] = datatypes.JSON[T]

// NewJSON returns data as the document a column stores.
func NewJSON[T any](data T) JSON[T] {
	return datatypes.NewJSON(data)
}

// JSONSlice is a slice of T a column stores as one JSON array: a model
// declares one as a JSONSlice of its elements (`Tags
// datatypes.JSONSlice[string]`). It is the slice it is declared over,
// indexed, ranged over and appended to as one, and written in a request and
// a response as the array a []T is. A column of strings is the one a filter
// tests the membership of (see gst.JSONStringsColumn).
type JSONSlice[T any] = datatypes.JSONSlice[T]

// NewJSONSlice returns s as the array a column stores, T taken from s.
func NewJSONSlice[T any](s []T) JSONSlice[T] {
	return datatypes.NewJSONSlice(s)
}

// JSONMap is a map a column stores as one JSON object, its keys known only
// as it is read (`Meta datatypes.JSONMap`). It is the map it is declared
// over, read and written by key as one, and written in a request and a
// response as the object a map[string]any is.
type JSONMap = datatypes.JSONMap
