package model

import (
	. "github.com/hydroan/gst/dsl"
	pkgmodel "github.com/hydroan/gst/model"
)

type User8 struct {
	Name string

	pkgmodel.Empty
}

func (*User8) Design() {
	Migrate(true)
}

type User9 struct {
	Name string
}
