package modelinfo

import (
	"github.com/hydroan/gst/apidoc"
)

// StructDocEntry describes the doc comments of one struct extracted from a
// model source file, identified by its package path and type name.
type StructDocEntry struct {
	PkgPath  string
	TypeName string
	Doc      apidoc.StructDoc
}

// EnumDocEntry describes one enum-like named type extracted from a model
// package: its doc comment and declared constant values.
type EnumDocEntry struct {
	PkgPath  string
	TypeName string
	Doc      apidoc.EnumDoc
}

// APIDocEntries bundles everything the generated model/apidoc.gen.go
// registers.
type APIDocEntries struct {
	Structs []StructDocEntry
	Enums   []EnumDocEntry
}
