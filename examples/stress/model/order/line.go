package order

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Line is an order's line: model/order/line.go sits in the directory named
// after model/order.go, so its routes nest under the parent's,
// /api/orders/:order/lines. The index on the order is what the preload of
// Order.Lines reads.
type Line struct {
	OrderID string `json:"order_id" query:"order_id" gorm:"size:36;not null"`
	Product string `json:"product" query:"product" gorm:"size:64" binding:"max=64"`
	Qty     int    `json:"qty" query:"qty"`
	Price   int    `json:"price" query:"price"`

	model.Base
}

func (Line) TableName() string      { return "order_lines" }
func (Line) Indexes() []model.Index { return []model.Index{{Fields: []string{"OrderID"}}} }

func (Line) Design() {
	Migrate()
	Endpoint("lines")

	List(func() {})
	Get(func() {})
}
