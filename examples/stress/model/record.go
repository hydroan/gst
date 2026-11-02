package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Record is the table the data scenarios read and write: a row of a
// realistic size (a code, a title, a category, two numbers and a body of a
// few hundred bytes) behind the standard actions on /api/records and, the
// model declaring GRPC(), the RecordService rpcs. Embedding model.Query takes
// the list parameters: field filters, _sort_by, _page and _size, and the
// cursor. The code is unique, so a create carrying a taken code is refused
// by the index; the category carries the index a list filters by, the status
// carries none, so a filter on it scans the table, and the score carries none,
// so a sort by it sorts the table.
type Record struct {
	Code     string `json:"code" query:"code" gorm:"size:32;not null" binding:"required,max=32" pb:"11"`
	Title    string `json:"title" query:"title" gorm:"size:200" binding:"max=200" pb:"12"`
	Category string `json:"category" query:"category" gorm:"size:32;not null" binding:"required,max=32" pb:"13"`
	Status   int    `json:"status" query:"status" pb:"14"`
	Score    int    `json:"score" query:"score" pb:"15"`
	Body     string `json:"body" gorm:"type:text" pb:"16"`

	model.Query
	model.Base
}

func (Record) TableName() string { return "records" }

// Indexes declares the code unique, the index a list filtering by category
// and sorting by creation time reads, and the one the default order of an
// unfiltered list reads; the status and the score have none, so a filter on
// the status scans the table and a sort by the score sorts it.
func (Record) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"Code"}, Unique: true},
		{Fields: []string{"Category", "CreatedAt"}},
		{Fields: []string{"CreatedAt"}},
	}
}

func (Record) Design() {
	GRPC()
	Migrate()
	Endpoint("records")

	Create(func() {})
	Delete(func() {})
	Update(func() {})
	Patch(func() {})
	List(func() {})
	Get(func() {})
	DeleteMany(func() {})
	Export(func() {
		Service()
	})

	Route("records/upsert", func() {
		Create(func() {
			Service("upsert")
			Payload[*RecordUpsertReq]()
			Result[*Record]()
		})
	})
}

// RecordUpsertReq is the body of POST /api/records/upsert: the record to
// insert, or to overwrite where its code is taken.
type RecordUpsertReq struct {
	Code     string `json:"code" binding:"required,max=32" pb:"1"`
	Title    string `json:"title" binding:"max=200" pb:"2"`
	Category string `json:"category" binding:"required,max=32" pb:"3"`
	Status   int    `json:"status" pb:"4"`
	Score    int    `json:"score" pb:"5"`
	Body     string `json:"body" pb:"6"`
}
