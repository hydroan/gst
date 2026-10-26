package openapigen

import (
	"reflect"
	"slices"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
)

// hideUnwrittenFields hides, in the request body schemaRef documents for
// phase, a write of model by the framework's own flow, the fields the write
// takes no value for: the audit columns and timestamps of the framework base
// the model embeds (see modelregistry.IsFrameworkBase), which the framework
// writes, the columns gorm leaves out of the write, an insert writing none
// it writes on update only or never, an update none it writes on insert only
// or never (see modelschema.Column), and the primary key of an update, whose
// route names the record. A create takes the primary key, optional: the
// insert keeps a key the client names and generates one otherwise. A patch
// requires none of the fields its binding rules require, checking the ones it
// names alone. A batch body holds the records in its items array; an item of
// a batch update keeps the primary key, which names the record the item
// writes, as a required field, as the flow refuses an item without one.
//
// For a model embedding model.Base beside Name, Code `gorm:"<-:create"`,
// Stamp `gorm:"<-:update"` and Mark `gorm:"->"`, a create documents code, id
// and name, an update or a patch name and stamp, and an item of a batch
// update or patch id, name and stamp, id required.
func hideUnwrittenFields(schemaRef *openapi3.SchemaRef, model reflect.Type, phase consts.Phase) {
	if schemaRef == nil || schemaRef.Value == nil {
		return
	}
	insert := phase == consts.Create || phase == consts.CreateMany
	batch := phase == consts.CreateMany || phase == consts.UpdateMany || phase == consts.PatchMany
	schema := schemaRef.Value
	if batch {
		schema = batchItemSchema(schema)
	}
	columns, err := modelschema.Columns(model)
	if schema == nil || err != nil {
		return
	}
	if phase == consts.Patch || phase == consts.PatchMany {
		// A patch checks the fields it names alone, so the binding rules
		// require none of the others.
		schema.Required = nil
	}
	for _, col := range columns {
		key, base, ok := bodyKey(model, col.Index)
		if !ok {
			continue
		}
		written := col.Updatable
		if insert {
			written = col.Creatable
		}
		switch {
		case col.PrimaryKey && insert:
			// Kept, optional: the insert keeps a key the client names.
		case col.PrimaryKey && batch:
			schema.Required = append(schema.Required, key)
		case col.PrimaryKey || base || !written:
			delete(schema.Properties, key)
			schema.Required = slices.DeleteFunc(schema.Required, func(name string) bool { return name == key })
		}
	}
}

// bodyKey returns the key the JSON object of model holds the field at index
// under, a column's path (see modelschema.Column.Index), and whether the field
// is one of a framework base's; ok is false for a field the object does not
// hold at its top level, one inside a struct field encoded as a value of its
// own, or one JSON leaves out. For the ID of a model embedding model.Base it
// returns "id", true, true.
func bodyKey(model reflect.Type, index []int) (key string, base, ok bool) {
	typ := model
	for _, i := range index[:len(index)-1] {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		step := typ.Field(i)
		if !step.Anonymous || jsonName(step) != "" {
			return "", false, false
		}
		typ = step.Type
		base = base || modelregistry.IsFrameworkBase(typ)
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	key = jsonName(typ.Field(index[len(index)-1]))
	return key, base, key != ""
}

// batchItemSchema returns the element schema of the items array a batch request
// body carries, or nil when the body does not describe one.
func batchItemSchema(schema *openapi3.Schema) *openapi3.Schema {
	if schema.Properties == nil {
		return nil
	}
	itemsProp, exists := schema.Properties["items"]
	if !exists || itemsProp.Value == nil || itemsProp.Value.Items == nil {
		return nil
	}
	return itemsProp.Value.Items.Value
}
