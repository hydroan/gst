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
	Migrate()
}

type User9 struct {
	Name string
}

// SampleRecord verifies that the default endpoint of a multi-word model name
// is the pluralized snake_case form, e.g. "sample_records".
type SampleRecord struct {
	Name string

	pkgmodel.Empty
}

func (*SampleRecord) Design() {
	Migrate()
}
