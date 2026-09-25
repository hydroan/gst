package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Item struct {
	model.Empty
}

func (Item) Design() {
	dsl.Endpoint("items")
	dsl.Create(func() {})
}
