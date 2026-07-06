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

// ReceiveRobot verifies that the default endpoint of a multi-word model name
// is the pluralized snake_case form, e.g. "receive_robots".
type ReceiveRobot struct {
	Name string

	pkgmodel.Empty
}

func (*ReceiveRobot) Design() {
	Migrate(true)
}
