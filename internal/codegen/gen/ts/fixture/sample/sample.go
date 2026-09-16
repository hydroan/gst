// Package sample declares the fixture types the TypeScript generator tests
// start from. Sample carries a field of every form the generator describes.
package sample

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/hydroan/gst/internal/codegen/gen/ts/fixture/record"
	"github.com/hydroan/gst/internal/modelregistry"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Sample is a resource kept by the sample service.
type Sample struct {
	modelregistry.Base

	// Name is the display name.
	Name     string  `json:"name"`
	Summary  string  `json:"summary,omitempty"`
	Remark   *string `json:"remark"`
	Note     *string `json:"note,omitempty"`
	Untagged string
	Dash     string `json:"-,"` //nolint:staticcheck // "-," names the key "-", the form under test.
	Ignored  string `json:"-"`

	Count   int64   `json:"count,string"`
	Limit   *int    `json:"limit,string"`
	Ratio   float64 `json:"ratio"`
	Enabled bool    `json:"enabled"`

	Tags     []string          `json:"tags"`
	Labels   map[string]string `json:"labels,omitempty"`
	Scores   map[int]float64   `json:"scores"`
	Payload  []byte            `json:"payload"`
	Grid     [2]int            `json:"grid"`
	Pointers []*record.Record  `json:"pointers"`

	Status     Status     `json:"status"`
	Previous   *Status    `json:"previous,omitempty"`
	Retired    Status     `json:"retired,omitempty"`
	History    []Status   `json:"history"`
	Kind       Kind       `json:"kind"`
	Level      Level      `json:"level"`
	Permission Permission `json:"permission"`
	Reference  Reference  `json:"reference"`

	Extra    any                          `json:"extra"`
	Raw      json.RawMessage              `json:"raw"`
	Document datatypes.JSON               `json:"document"`
	Meta     datatypes.JSONMap            `json:"meta"`
	Options  datatypes.JSONType[*Options] `json:"options"`
	Items    datatypes.JSONSlice[Item]    `json:"items"`
	Day      datatypes.Date               `json:"day"`
	Deadline time.Time                    `json:"deadline,omitzero"`
	Archived gorm.DeletedAt               `json:"archived"`
	Comment  sql.NullString               `json:"comment"`
	Version  modelregistry.Version        `json:"version,omitempty"`

	Record  *record.Record `json:"record"`
	Records Records        `json:"records"`
	Batch   Batch          `json:"batch"`
	Entry   Entry          `json:"entry"`
	Window  struct {
		From string `json:"from"`
		To   string `json:"to,omitempty"`
	} `json:"window"`

	*Audit
	Left
	Right
}

// Status is the lifecycle state of a sample.
type Status string

const (
	// StatusActive marks a sample in use.
	StatusActive   Status = "active"
	StatusArchived Status = "archived" // StatusArchived marks a sample kept read-only.
)

// Kind classifies a sample. Its empty constant covers the zero value.
type Kind string

const (
	KindNone  Kind = ""
	KindPlain Kind = "plain"
)

// Level ranks a sample.
type Level int

const (
	LevelLow Level = iota + 1
	LevelHigh
)

// Permission is the set of what a sample allows.
type Permission uint8

const (
	PermissionRead Permission = 1 << iota
	PermissionWrite
)

// Reference identifies a sample to an external system. A request may send it
// as a JSON string or number; it always encodes as a string.
type Reference string

// UnmarshalJSON accepts a JSON string or number.
func (r *Reference) UnmarshalJSON(data []byte) error {
	if len(data) > 1 && data[0] == '"' {
		data = data[1 : len(data)-1]
	}
	*r = Reference(data)
	return nil
}

// Options is kept as a JSON document.
type Options struct {
	Theme string `json:"theme"`
}

// Item is an element of a JSON array column.
type Item struct {
	Code  string `json:"code"`
	Label string `json:"label,omitempty"`
}

// Records lists records.
type Records []*record.Record

// Batch is an alias of a slice, which holds nil as a named slice type does.
type Batch = []Item

// Entry is the record type under the name samples use for it.
type Entry = record.Record

// Audit is embedded through a pointer: its keys disappear with a nil pointer.
type Audit struct {
	Reviewer string `json:"reviewer"`
}

// Left is embedded next to Right. Their untagged Label fields share a key at
// the same depth and cancel each other out, while the tagged Mode of Left wins
// over the untagged Mode of Right. Both embed Shared, whose keys therefore
// occur twice one level further down and cancel out as well.
type Left struct {
	Shared

	Label string
	Mode  string `json:"Mode"`
}

// Right is embedded next to Left.
type Right struct {
	Shared

	Label string
	Mode  string
}

// Shared is embedded by both Left and Right.
type Shared struct {
	Topic string
}
