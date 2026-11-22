package controller

import (
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
)

// This file holds how a patch arriving over gRPC names the fields it
// applies: by the paths of its update mask (see patch_fields.go for the
// fields a patch may name).

// maskFieldSet returns the fields of typ the paths of an update mask name,
// as the message names them, the JSON keys of the model: what a Patch rpc
// applies of the values it carries. A path naming a field the framework
// manages, the primary key, the audit columns, another column an update
// leaves alone or a field the model declares read-only, is passed over, the
// way the HTTP handler passes over the key, and the way a mask naming an
// output-only field is read (AIP-161, AIP-203). Every other path must name
// a field the patch applies, as a whole: a path into a field, address.city,
// is refused and told the field to name instead, and so is a path naming a
// field the model does not have, a key the HTTP handler passes over. The
// mask must leave at least one field to apply, a Patch applying nothing
// being a mistake to report rather than a record to answer unchanged, where
// the HTTP handler writes the record back as stored.
func maskFieldSet(typ reflect.Type, paths []string) (patchFieldSet, error) {
	table := patchFieldsOf(typ)
	fields := make(patchFieldSet, len(paths))
	for _, path := range paths {
		if _, managed := table.managed[path]; managed {
			continue
		}
		if head, _, into := strings.Cut(path, "."); into {
			if _, ok := table.byKey[head]; ok {
				return nil, errors.Newf("update_mask names %q, a part of a field; a patch applies %q as a whole", path, head)
			}
		}
		field, ok := table.byKey[path]
		if !ok {
			return nil, errors.Newf("update_mask names %q, which is no field a patch applies", path)
		}
		fields[field.name] = struct{}{}
	}
	if len(fields) == 0 {
		return nil, errors.New("update_mask must name at least one field a patch applies")
	}
	return fields, nil
}
