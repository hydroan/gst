package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// ProductFeed reads the products table as a feed: embedding model.Cursor has
// its list, GET /api/product-feed, read by cursor in place of pages, the
// listing a table too large to page through takes. It answers the columns of
// Product and declares no table of its own.
type ProductFeed struct {
	Code     string `json:"code" query:"code"`
	Title    string `json:"title" query:"title"`
	Category string `json:"category" query:"category"`
	Status   int    `json:"status" query:"status"`
	Score    int    `json:"score" query:"score"`
	Body     string `json:"body"`

	model.Cursor
	model.Base
}

func (ProductFeed) TableName() string { return "products" }

func (ProductFeed) Design() {
	Endpoint("product-feed")

	List(func() {})
}
