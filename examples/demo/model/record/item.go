package record

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// ItemKind identifies the kind of an item.
type ItemKind string

const (
	ItemKindInput  ItemKind = "input"
	ItemKindOutput ItemKind = "output"
	ItemKindSystem ItemKind = "system"
)

// SearchSource describes an external reference attached to an item.
type SearchSource struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// Item demonstrates a child resource with nested routes and batch actions.
type Item struct {
	UserID   string         `json:"user_id" query:"user_id"`
	RecordID string         `json:"record_id" query:"record_id"`
	Kind     ItemKind       `json:"kind" query:"kind"`
	Content  string         `json:"content" gorm:"type:text"`
	Sources  []SearchSource `json:"sources,omitempty" gorm:"-"`

	model.Base
}

func (Item) TableName() string { return "items" }

func (Item) Design() {
	Migrate()
	Endpoint("items")

	Create(func() {
		Service()
	})
	Patch(func() {})
	List(func() {
		Service()
	})
	Get(func() {})

	Route("items", func() {
		DeleteMany(func() {
			Service()
		})
	})
}

func (Item) Purge() bool { return true }
