package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// RecordType tells what a record holds.
type RecordType string

const (
	RecordTypeText  RecordType = "text"
	RecordTypeImage RecordType = "image"
)

// Record is a table-backed resource with the standard actions on
// /api/records and /api/records/:record. Delete, Update, Patch and Get are
// the framework's own; Create and List declare Service() for the hooks in
// service/record, which stamp the owner, write the audit trail and keep a
// list to the caller's own records. Embedding model.Query lets a list take
// the framework's query parameters: field[op]=value filters such as
// type[in]=text,image, _sort_by, _page and _size, and cursor pagination.
// The summary route counts the records by type in service code of its own,
// and the search route is a custom List reading those parameters itself.
type Record struct {
	Type  RecordType `json:"type" query:"type"`
	Title string     `json:"title" query:"title"`
	// UserID is the owner, taken from the session when the record is created.
	UserID string `json:"user_id" query:"user_id" gorm:"size:36;not null"`
	// Username is the owner's name, filled in for the client; not a column.
	Username string `json:"username,omitempty" gorm:"-"`

	model.Query
	model.Base
}

func (Record) TableName() string      { return "records" }
func (Record) Indexes() []model.Index { return []model.Index{{Fields: []string{"UserID"}}} } // the owner filter of every list relies on it

func (Record) Design() {
	Migrate()
	Endpoint("records")
	Param("record")

	Create(func() {
		Service()
	})
	Delete(func() {})
	Update(func() {})
	Patch(func() {})
	List(func() {
		Service()
	})
	Get(func() {})

	Route("records/summary", func() {
		List(func() {
			Service("summary")
			Result[*RecordSummaryRsp]()
		})
	})
	Route("records/search", func() {
		List(func() {
			Service("search")
			Result[*RecordSearchRsp]()
		})
	})
}

type (
	// RecordSummaryRsp counts the caller's records, in all and by type.
	RecordSummaryRsp struct {
		Total  int                `json:"total"`
		ByType map[RecordType]int `json:"by_type"`
	}

	// RecordSearchRsp is a page of the caller's records and how many match.
	RecordSearchRsp struct {
		Items []*Record `json:"items"`
		Total int       `json:"total"`
	}
)
