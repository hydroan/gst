package controller

import (
	"reflect"

	"github.com/hydroan/gst/internal/gst"
)

// This file holds the normalization both transports run on what they
// decoded: a request a JSON null left nil restored to its zero value, and
// nil elements removed from slices (see normalizeValue).

// normalizeRequest restores req to the zero-value instance when a JSON null
// body left it nil — indistinguishable from an empty body for the service —
// and normalizes what it holds (see normalizeValue). Both transports run it
// on what they decoded, so a request reads the same whichever carried it.
func (a *action[M, REQ, RSP]) normalizeRequest(req *REQ) {
	if a.reqKind == reflect.Pointer && reflect.ValueOf(*req).IsNil() {
		*req = a.newRequest()
	}
	normalizeValue(reflect.ValueOf(req))
}

// normalizeModel is the model-path counterpart of normalizeRequest for
// handlers that bind the request body straight into the model type. Model
// types are pointers by construction, so only the nil restore and the
// normalization of what the model holds apply.
func (a *action[M, REQ, RSP]) normalizeModel(m *M) {
	if reflect.ValueOf(*m).IsNil() {
		*m = a.newModel()
	}
	normalizeValue(reflect.ValueOf(m))
}

// normalizeBatch normalizes what the bound batch payload holds (see
// normalizeValue), nil entries compacted away so the shared batch pipeline
// never dereferences a nil item.
func normalizeBatch[M gst.Model](req *batch[M]) {
	normalizeValue(reflect.ValueOf(req))
}

// nilableKind reports whether values of kind k can hold nil, i.e. whether a
// JSON null entry can decode into them.
func nilableKind(k reflect.Kind) bool {
	switch k {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return true
	default:
		return false
	}
}

// normalizeValue walks the value graph reachable from v and removes nil
// elements from every settable slice whose elements can hold nil. Only
// exported struct fields are visited, matching what encoding/json can bind.
// JSON-decoded values are acyclic, so the walk needs no cycle tracking.
// Interface values are descended into only when they carry a pointer: other
// interface payloads are not addressable, so their inner values cannot be
// changed in place and are left untouched.
func normalizeValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			normalizeValue(v.Elem())
		}
	case reflect.Interface:
		if !v.IsNil() && v.Elem().Kind() == reflect.Pointer {
			normalizeValue(v.Elem())
		}
	case reflect.Struct:
		t := v.Type()
		for i := range v.NumField() {
			if !t.Field(i).IsExported() {
				continue
			}
			normalizeValue(v.Field(i))
		}
	case reflect.Slice:
		if v.IsNil() {
			return
		}
		if nilableKind(v.Type().Elem().Kind()) && v.CanSet() {
			kept := 0
			for i := range v.Len() {
				if v.Index(i).IsNil() {
					continue
				}
				if kept != i {
					v.Index(kept).Set(v.Index(i))
				}
				kept++
			}
			if kept != v.Len() {
				v.SetLen(kept)
			}
		}
		for i := range v.Len() {
			normalizeValue(v.Index(i))
		}
	case reflect.Array:
		for i := range v.Len() {
			normalizeValue(v.Index(i))
		}
	case reflect.Map:
		if v.IsNil() {
			return
		}
		for _, key := range v.MapKeys() {
			value := v.MapIndex(key)
			tmp := reflect.New(value.Type()).Elem()
			tmp.Set(value)
			normalizeValue(tmp)
			v.SetMapIndex(key, tmp)
		}
	default:
	}
}
