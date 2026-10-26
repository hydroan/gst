package modelregistry

import (
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/hookoverride"
)

// ValidateIDAccessors refuses a model declaring an ID of its own over the one
// of the framework base it embeds, a wider key say, that takes any of GetID,
// SetID and ClearID from the base: the base's read and write the shadowed
// ID, which no column maps, while the framework reads and writes a record's
// ID through them. A field of the model's own, or of a struct it embeds,
// shadows the base's when it is the ID Go resolves on the model.
//
// Table preparation fails the start with the error, and the column
// inspection gg builds into a project hands it to gg check; the public
// inspect.ValidateIDAccessors forwards to it. For a model Record embedding
// model.Base beside an ID of its own and declaring GetID alone the error
// reads:
//
//	model Record: its ID shadows model.Base's; declare its own SetID and ClearID too: model.Base's read and write the shadowed field, which no column maps
func ValidateIDAccessors(typ reflect.Type) error {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return nil
	}
	var base reflect.Type
	for _, candidate := range []reflect.Type{reflect.TypeFor[Base](), reflect.TypeFor[AutoBase]()} {
		if field, ok := typ.FieldByName(candidate.Name()); ok && field.Anonymous && field.Type == candidate {
			base = candidate
		}
	}
	id, ok := typ.FieldByName("ID")
	if base == nil || !ok {
		return nil
	}
	owner := typ
	for _, i := range id.Index[:len(id.Index)-1] {
		owner = owner.Field(i).Type
		for owner.Kind() == reflect.Pointer {
			owner = owner.Elem()
		}
	}
	if IsFrameworkBase(owner) {
		return nil
	}

	var missing []string
	for _, name := range []string{"GetID", "SetID", "ClearID"} {
		if !hookoverride.Overridden(reflect.PointerTo(typ), name, IsFrameworkBase) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	names := strings.Join(missing[:len(missing)-1], ", ")
	if names != "" {
		names += " and "
	}
	names += missing[len(missing)-1]
	return errors.Newf("model %s: its ID shadows model.%s's; declare its own %s too: model.%[2]s's read and write the shadowed field, which no column maps", typ.Name(), base.Name(), names)
}
