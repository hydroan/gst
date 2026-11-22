// Package datatypes holds the value types the framework declares for model
// fields itself, where a project would otherwise reach for a third-party
// type: each stores, scans and writes its JSON the way the framework's
// contract says. The root gst package forwards them to projects.
package datatypes

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"
	gormdatatypes "gorm.io/datatypes"
)

// Date is a calendar day, a year, a month and a day: a birthday, a due day,
// the day a statement is for. It names no instant and carries no zone, so a
// day is written the same wherever it is read: 2026-01-02 in a request body,
// in a response and in a URL filter (see ParseDate), and a google.type.Date
// over gRPC. A model declares a day as a Date and an optional one as a *Date,
// and its table stores either in a date column.
//
// The day is the one the time reads in its own location: Date(t) is the year,
// month and day t.Date() answers, and time.Time(d) is a time of that day. A
// Date read from a request, a filter or the database is its day's midnight in
// UTC.
//
// Only the day's text is the type's own. The date column is the one
// gorm.io/datatypes declares for its Date, reused here by conversion; what
// the library's type cannot be told is its JSON, the instant time.Time
// writes, 2026-01-02T00:00:00Z, which is a day to a reader in the zone it was
// written in alone.
type Date time.Time //nolint:recvcheck // UnmarshalJSON and Scan write through the pointer, MarshalJSON and Value read the value: the receiver pairs encoding/json and database/sql document.

// ParseDate reads a day the one way the framework takes it, in a request
// body and in a URL filter alike: 2006-01-02, a real day of the calendar. An
// instant, 2026-01-02T00:00:00+08:00, is refused on purpose: the day it
// falls on depends on the zone it is read in, and guessing one is worse than
// failing.
func ParseDate(value string) (Date, error) {
	day, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return Date{}, errors.Newf("unsupported date format %q, expect 2006-01-02", value)
	}
	return Date(day), nil
}

// String returns the day as 2006-01-02.
func (d Date) String() string {
	return time.Time(d).Format(time.DateOnly)
}

// MarshalJSON writes the day as a JSON string, "2026-01-02".
func (d Date) MarshalJSON() ([]byte, error) {
	text := make([]byte, 0, len(time.DateOnly)+2)
	text = append(text, '"')
	text = time.Time(d).AppendFormat(text, time.DateOnly)
	return append(text, '"'), nil
}

// UnmarshalJSON reads a day written as a JSON string, "2026-01-02" (see
// ParseDate), and refuses every other value. A null leaves the day as it is,
// the way encoding/json leaves a value a null is decoded into.
func (d *Date) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return errors.New("a date is a JSON string, as 2006-01-02")
	}
	day, err := ParseDate(text)
	if err != nil {
		return err
	}
	*d = day
	return nil
}

// Scan reads the day a date column holds, the way gormdatatypes.Date reads
// it.
func (d *Date) Scan(value any) error {
	return (*gormdatatypes.Date)(d).Scan(value)
}

// Value writes the day for a date column as its midnight in UTC, whatever
// location the time is in. gormdatatypes.Date writes the midnight of the
// time's own location, an instant a connection in UTC, the one the framework
// opens, reads as the day before for a location east of it.
func (d Date) Value() (driver.Value, error) {
	year, month, day := time.Time(d).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

// GormDataType declares the date column, the one gormdatatypes.Date
// declares.
func (Date) GormDataType() string {
	return gormdatatypes.Date{}.GormDataType()
}
