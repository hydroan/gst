package model

import (
	"github.com/hydroan/gst/datatypes"
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Meeting is a meeting on the team's calendar, and the model showing the
// field types of the datatypes package: a calendar day, a time of day, and
// the three ways a column stores JSON, each written in a request and a
// response the one way its comment below tells. Every action is the
// framework's own. Embedding model.Query lets a list take the framework's
// filters, a day and a time of day by comparison:
// GET /api/meetings?day[gte]=2026-01-01&day[lte]=2026-01-31 is a month of
// meetings. The List hook in service/meeting serves two parameters of the
// model's own, tag and upcoming, with the column references generated for
// these fields. /api/meetings and /api/meetings/:id.
type Meeting struct {
	Title string `json:"title" query:"title" gorm:"size:191;not null" binding:"required,max=191"`
	// Day is the day of the meeting, a date with no time of day and no zone,
	// written 2026-01-02; a request must name it.
	Day datatypes.Date `json:"day" query:"day" binding:"required"`
	// StartsAt is the time of day the meeting starts at, written 17:30:00.
	// It is optional, so it is a pointer: a meeting with none lasts the day.
	StartsAt *datatypes.TimeOfDay `json:"starts_at,omitempty" query:"starts_at"`
	// Tags is a JSON array of strings, sent and answered as the array it is:
	// ["weekly","planning"].
	Tags datatypes.JSONSlice[string] `json:"tags"`
	// Links is a JSON object whose keys the client chooses:
	// {"agenda":"https://example.com/agenda"}.
	Links datatypes.JSONMap `json:"links,omitempty"`
	// Options is a value of a Go type stored as one JSON document, sent and
	// answered as the object MeetingOptions is.
	Options datatypes.JSON[MeetingOptions] `json:"options"`

	// Tag and Upcoming are parameters of a list and no columns: tag=weekly
	// keeps the meetings carrying the tag, upcoming=true the ones of the
	// week ahead.
	Tag      string `json:"-" query:"tag" gorm:"-"`
	Upcoming bool   `json:"-" query:"upcoming" gorm:"-"`

	model.Query
	model.Base
}

// MeetingOptions is how a meeting is held.
type MeetingOptions struct {
	Online  bool `json:"online"`
	Minutes int  `json:"minutes"`
}

func (Meeting) TableName() string { return "meetings" }

func (Meeting) Design() {
	Migrate()
	Endpoint("meetings")

	Create(func() {})
	Delete(func() {})
	Update(func() {})
	Patch(func() {})
	List(func() {
		Service()
	})
	Get(func() {})
}
