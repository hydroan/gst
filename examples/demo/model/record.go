package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// RecordType identifies the content type handled by a record.
type RecordType string

const (
	RecordTypeText  RecordType = "text"
	RecordTypeImage RecordType = "image"
)

// Record demonstrates a database-backed resource with custom service hooks.
type Record struct {
	Type RecordType `json:"type" query:"type"`

	UserID string `json:"user_id" query:"user_id"`
	Title  string `json:"title" query:"title"`

	// Username is returned to clients and is not stored in the database.
	Username string `json:"username,omitempty" gorm:"-"`

	model.Base
}

func (Record) TableName() string { return "records" }

func (Record) Design() {
	Migrate()
	Endpoint("records")
	Param("rec")

	Create(func() {
		Service()
	})
	Delete(func() {
		Service()
	})
	Patch(func() {
		Service()
	})
	List(func() {
		Service()
	})
	Get(func() {})
}

func (Record) Purge() bool { return true }
