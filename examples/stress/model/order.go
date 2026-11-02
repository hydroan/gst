package model

import (
	"stress/model/order"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Order is the parent of the association scenario: Expands names Lines, so a
// list or a get asking for ?_expand=lines has the framework preload the lines
// of every order it answers with. The standard actions serve /api/orders;
// /api/orders/public is the same list declared Public(), what the
// authenticated list is measured against: the difference between the two is
// the session check and the authorization.
type Order struct {
	Number   string `json:"number" query:"number" gorm:"size:32;not null" binding:"required,max=32"`
	Customer string `json:"customer" query:"customer" gorm:"size:64" binding:"max=64"`
	Total    int    `json:"total" query:"total"`
	// Lines are the order's lines, loaded when a request asks for them;
	// constraint:- keeps the migration from adding a foreign key, the preload
	// resolving the relation in Go.
	Lines []*order.Line `json:"lines,omitempty" gorm:"foreignKey:OrderID;constraint:-"`

	model.Query
	model.Base
}

func (Order) TableName() string { return "orders" }

// Indexes declares the number unique and the index the default order of a
// list, by creation time, reads.
func (Order) Indexes() []model.Index {
	return []model.Index{{Fields: []string{"Number"}, Unique: true}, {Fields: []string{"CreatedAt"}}}
}

func (Order) Expands() []string { return []string{"Lines"} }

func (Order) Design() {
	Migrate()
	Endpoint("orders")
	Param("order")

	Create(func() {})
	Delete(func() {})
	Update(func() {})
	Patch(func() {})
	List(func() {})
	Get(func() {})

	Route("orders/public", func() {
		List(func() {
			Public()
		})
	})
}
