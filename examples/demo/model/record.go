package model

import (
	"demo/model/record"

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
// /api/records and /api/records/:record. Get is the framework's own; the
// other actions declare Service() for the hooks in service/record, which
// stamp the owner, write the audit trail, keep a list to the caller's own
// records and every write to them: updating, patching or deleting someone
// else's record answers 404, as if it were not there. Embedding model.Query
// lets a list take the framework's query parameters: field[op]=value
// filters such as type[in]=text,image, _sort_by, _page and _size, and
// cursor pagination. The binding tags validate a body before any hook runs,
// a 400 naming the field. The summary route counts the records by type in
// service code of its own, the search route is a custom List reading those
// parameters itself, and the trash route lists the records the caller
// deleted.
type Record struct {
	Type  RecordType `json:"type" query:"type" binding:"omitempty,oneof=text image"`
	Title string     `json:"title" query:"title" binding:"max=64"`
	// UserID is the owner, taken from the session when the record is created.
	UserID string `json:"user_id" query:"user_id" gorm:"size:36;not null"`
	// Username is the owner's name, filled in for the client; not a column.
	Username string `json:"username,omitempty" gorm:"-"`
	// Items are the record's items, loaded when a request asks for them:
	// Expands names the association and ?_expand=items has the framework
	// preload it on a list or a get. constraint:- keeps the migration from
	// adding a foreign key, the preload resolving the relation in Go.
	Items []*record.Item `json:"items,omitempty" gorm:"foreignKey:RecordID;constraint:-"`
	// Version protects concurrent edits: a client sends back the version it
	// read, an update that carries a stale one answers 409 and one that
	// carries none answers 400. A create sets it to 1, and a successful
	// update or patch bumps it.
	Version model.Version `json:"version,omitempty" gorm:"not null;default:1"`

	model.Query
	model.Base
}

func (Record) TableName() string      { return "records" }
func (Record) Indexes() []model.Index { return []model.Index{{Fields: []string{"UserID"}}} } // the owner filter of every list relies on it
func (Record) Expands() []string      { return []string{"Items"} }                           // the associations a request may ask for

func (Record) Design() {
	Migrate()
	Endpoint("records")
	Param("record")

	Create(func() {
		Service()
	})
	Delete(func() {
		Service()
	})
	Update(func() {
		Service()
	})
	Patch(func() {
		Service()
	})
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
	Route("records/trash", func() {
		List(func() {
			Service("trash")
			Result[*RecordTrashRsp]()
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

	// RecordTrashRsp is the caller's deleted records and how many there are:
	// the same shape as RecordSearchRsp, under the trash route's own name.
	RecordTrashRsp = RecordSearchRsp
)
