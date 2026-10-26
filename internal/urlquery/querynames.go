package urlquery

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
)

// frameworkQueryStructs are the structs declaring the framework's own query
// parameters, the fields alone whose names may take the "_" namespace.
var frameworkQueryStructs = map[reflect.Type]bool{
	reflect.TypeFor[modelregistry.Query]():      true,
	reflect.TypeFor[modelregistry.Pagination](): true,
	reflect.TypeFor[modelregistry.Cursor]():     true,
}

// queryNameMarks are the characters a query tag cannot hold: the operator
// syntax reads the brackets and the path of a struct embedded by name reads
// the dot.
const queryNameMarks = "[]."

// ValidateQueryNames refuses a model whose query tags name its URL query
// parameters ambiguously, which the parsers of this package would each
// resolve their own way: a name two fields declare, two columns or a column
// and a field no column holds, a business field's name in the "_" namespace
// of the framework's own parameters, which the gRPC line refuses as a
// filter, and a tag holding "[", "]" or ".", which the operator and the path
// syntax read. A field a shallower one shadows, the id of model.Base under a
// model's own, declares nothing, and a column hidden from JSON declares its
// name as any other. Fields are named by their Go path.
//
// Every finding is reported in one error, which table preparation fails the
// start with and the column inspection gg builds into a project hands to gg
// check; the public inspect.ValidateQueryNames forwards to it. For a model
// Record declaring Name `query:"title"` beside Title `query:"title"` the
// error reads:
//
//	model Record: query name "title" is declared by Name and Title; give each field a query name of its own, outside the "_" namespace and without "[", "]" or "."
func ValidateQueryNames(typ reflect.Type) error {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return nil
	}
	columns, err := modelschema.Columns(typ)
	if err != nil {
		return err
	}

	var problems []string
	reported := make(map[string]bool)
	report := func(problem string) {
		if !reported[problem] {
			reported[problem] = true
			problems = append(problems, problem)
		}
	}
	checkTag := func(field reflect.StructField, path string) {
		tag := modelschema.QueryColumnName(field)
		if mark := strings.IndexAny(tag, queryNameMarks); mark >= 0 {
			report(fmt.Sprintf("query tag %q of %s holds %q", tag, path, tag[mark:mark+1]))
		}
	}

	// The fields declaring each name, the columns first in the order
	// modelschema returns them, then the fields no column holds.
	declared := make(map[string][]string)
	var names []string
	declare := func(name, path string) {
		if _, seen := declared[name]; !seen {
			names = append(names, name)
		}
		declared[name] = append(declared[name], path)
	}

	columnFields := make(map[string]bool, len(columns))
	for _, col := range columns {
		columnFields[fmt.Sprint(col.Index)] = true
		owner := typ
		var path []string
		for _, i := range col.Index {
			for owner.Kind() == reflect.Pointer {
				owner = owner.Elem()
			}
			step := owner.Field(i)
			if !step.Anonymous {
				path = append(path, step.Name)
				checkTag(step, strings.Join(path, "."))
			}
			owner = step.Type
		}
		if col.QueryName == "" {
			continue
		}
		declare(col.QueryName, col.GoName)
		if strings.HasPrefix(col.QueryName, "_") {
			report(fmt.Sprintf("query name %q of %s takes the \"_\" namespace of the framework's own parameters", col.QueryName, col.GoName))
		}
	}

	// The fields no column holds are walked the way Decode walks them (see
	// walkQueryFields); a field a shallower one shadows declares nothing.
	walkQueryFields(typ, func(owner reflect.Type, field reflect.StructField, index []int, _ int) {
		if columnFields[fmt.Sprint(index)] {
			return
		}
		if promoted, ok := typ.FieldByName(field.Name); !ok || !slices.Equal(promoted.Index, index) {
			return
		}
		name := modelschema.QueryColumnName(field)
		if name == "" {
			return
		}
		checkTag(field, field.Name)
		declare(name, field.Name)
		if strings.HasPrefix(name, "_") && !frameworkQueryStructs[owner] {
			report(fmt.Sprintf("query name %q of %s takes the \"_\" namespace of the framework's own parameters", name, field.Name))
		}
	})

	for _, name := range names {
		if paths := declared[name]; len(paths) > 1 {
			report(fmt.Sprintf("query name %q is declared by %s and %s", name, strings.Join(paths[:len(paths)-1], ", "), paths[len(paths)-1]))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.Newf("model %s: %s; give each field a query name of its own, outside the \"_\" namespace and without \"[\", \"]\" or \".\"", typ.Name(), strings.Join(problems, "; "))
}
