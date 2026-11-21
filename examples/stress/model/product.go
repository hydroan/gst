package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Product is the table the data scenarios read and write: a row of a
// realistic size (a code, a title, a category, two numbers and a body of a
// few hundred bytes) behind the standard actions on /api/products and, the
// model declaring GRPC(), the ProductService rpcs. Embedding model.Query takes
// the operator filters and _sort_by; the list pages by _page and _size, as
// every list does, and ProductFeed reads the same table by cursor. The code
// is unique, so a create carrying a taken code is refused by the index; the
// category carries the index a list filters by, the status carries none, so
// a filter on it scans the table, and the score carries none, so a sort by
// it sorts the table.
type Product struct {
	Code     string `json:"code" query:"code" gorm:"size:32;not null" binding:"required,max=32" pb:"11"`
	Title    string `json:"title" query:"title" gorm:"size:200" binding:"max=200" pb:"12"`
	Category string `json:"category" query:"category" gorm:"size:32;not null" binding:"required,max=32" pb:"13"`
	Status   int    `json:"status" query:"status" pb:"14"`
	Score    int    `json:"score" query:"score" pb:"15"`
	Body     string `json:"body" gorm:"type:text" pb:"16"`

	model.Query
	model.Base
}

func (Product) TableName() string { return "products" }

// Indexes declares the code unique, the index a list filtering by category
// and sorting by creation time reads, and the one the default order of an
// unfiltered list reads; the status and the score have none, so a filter on
// the status scans the table and a sort by the score sorts it.
func (Product) Indexes() []model.Index {
	return []model.Index{
		{Fields: []string{"Code"}, Unique: true},
		{Fields: []string{"Category", "CreatedAt"}},
		{Fields: []string{"CreatedAt"}},
	}
}

func (Product) Design() {
	GRPC()
	Migrate()
	Endpoint("products")

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

	Route("products/upsert", func() {
		Create(func() {
			Service("upsert")
			Payload[*ProductUpsertReq]()
			Result[*Product]()
		})
	})
}

// ProductUpsertReq is the body of POST /api/products/upsert: the product to
// insert, or to overwrite where its code is taken.
type ProductUpsertReq struct {
	Code     string `json:"code" binding:"required,max=32" pb:"1"`
	Title    string `json:"title" binding:"max=200" pb:"2"`
	Category string `json:"category" binding:"required,max=32" pb:"3"`
	Status   int    `json:"status" pb:"4"`
	Score    int    `json:"score" pb:"5"`
	Body     string `json:"body" pb:"6"`
}
