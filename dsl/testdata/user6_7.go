package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User6 struct {
	Name string

	model.Empty
}

func (User6) Design() {
	dsl.Migrate(true)
}

type User7 struct {
	Name string
}
