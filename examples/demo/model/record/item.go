package record

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// ItemKind tells where an item came from.
type ItemKind string

const (
	ItemKindInput  ItemKind = "input"
	ItemKindOutput ItemKind = "output"
)

// Item is a child resource of Record: model/record/item.go sits in the
// directory named after model/record.go, so its routes nest under the
// parent's, /api/records/:record/items. The nesting is the URL's; what the
// parent means to an item is the service's: Create takes the parent from
// the route and List keeps to it, in service/record/item, while the other
// actions are the framework's own. The batch actions, declared on a route
// of their own, take and answer lists of items at /api/items/batch.
type Item struct {
	RecordID string   `json:"record_id" query:"record_id"`
	Kind     ItemKind `json:"kind" query:"kind"`
	Content  string   `json:"content" gorm:"type:text"`

	model.Base
}

func (Item) TableName() string { return "items" }

func (Item) Design() {
	Migrate()
	Endpoint("items")

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

	Route("items", func() {
		CreateMany(func() {})
		DeleteMany(func() {})
		UpdateMany(func() {})
		PatchMany(func() {})
	})
}
