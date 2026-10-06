package model

import "github.com/hydroan/gst/internal/modelregistry"

// The sentinel rows of a model tree and the names of the columns every model
// shares. RootID and RootName are the root anchor a top-level row hangs from,
// UnknownID and NoneID with their names the placeholders a reference points
// at when its target is unknown or absent; a tree keeps the three for
// bookkeeping and lists them to no one. KeyName and KeyID are the name and id
// columns.
var (
	RootID      = modelregistry.RootID
	RootName    = modelregistry.RootName
	UnknownID   = modelregistry.UnknownID
	UnknownName = modelregistry.UnknownName
	NoneID      = modelregistry.NoneID
	NoneName    = modelregistry.NoneName

	KeyName = modelregistry.KeyName
	KeyID   = modelregistry.KeyID
)
