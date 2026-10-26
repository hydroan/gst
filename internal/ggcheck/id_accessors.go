package ggcheck

import (
	"os"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinspect"
)

// IDAccessors holds the models declaring an ID of their own to the ID
// accessors that go with it.
var IDAccessors = Check{
	Name: "ID accessors",
	Rule: "a model declaring an ID of its own over the one of model.Base or model.AutoBase, a wider key say, must declare its own GetID, SetID and ClearID: the base's read and write the shadowed field, which no column maps",
	run:  checkIDAccessors,
}

// checkIDAccessors reports the models declaring an ID of their own that take
// an ID accessor from the framework base they embed, which table preparation
// refuses at startup (see modelregistry.ValidateIDAccessors). Which ID Go
// resolves on a model and which type declares each accessor are the
// compiler's to decide from the structs the model embeds, so the check asks
// the models themselves through the column inspection (see
// inspectedModelViolations).
func checkIDAccessors(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	return inspectedModelViolations(ignore, "ID accessors", func(model modelinspect.ModelColumns) string {
		return model.IDAccessorViolation
	})
}
