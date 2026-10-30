// The inspection program reports the project models' columns, what their index
// declarations violate on MySQL, the framework columns they rename, the query
// names they declare ambiguously and the ID accessors they lack for an ID of
// their own, as JSON written to the file its argument names: it is what gg gen
// and gg check build and run inside the project. gg embeds it (see
// gghelper.InspectProgram), writes it next to a generated project.go that
// imports the project's model packages and sets the models beyond the registry,
// and builds it with the project's dependency versions, so it resolves exactly
// the columns the framework resolves at run time. It is written as project
// code: it imports the framework's public packages only.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	gstmysql "github.com/hydroan/gst/database/mysql"
	"github.com/hydroan/gst/inspect"
	"github.com/hydroan/gst/model"
)

type columnInfo struct {
	GoName      string            `json:"go_name"`
	DBName      string            `json:"db_name"`
	TypeExpr    string            `json:"type_expr"`
	TypeImports map[string]string `json:"type_imports"`
	TypeName    string            `json:"type_name"`
	Numeric     bool              `json:"numeric"`
	Time        bool              `json:"time"`
	JSONStrings bool              `json:"json_strings"`
}

type modelColumns struct {
	PkgPath                  string       `json:"pkg_path"`
	PkgName                  string       `json:"pkg_name"`
	Name                     string       `json:"name"`
	Columns                  []columnInfo `json:"columns"`
	IndexViolation           string       `json:"index_violation"`
	TenantIndexViolation     string       `json:"tenant_index_violation"`
	FrameworkColumnViolation string       `json:"framework_column_violation"`
	QueryNameViolation       string       `json:"query_name_violation"`
	IDAccessorViolation      string       `json:"id_accessor_violation"`
}

// The project's models beyond the registry, set by the generated project.go
// (see modelinspect on the gg side): those that declare a Design but no
// Migrate, those whose registration gst.yaml gen.models.ignore drops, and
// the table-backed ones by name.
var (
	unregisteredModels []any
	ignoredModels      []any
	tableBackedModels  []any
)

func main() {
	// Only the configuration is loaded, the way the service loads it: gorm's
	// parser calls methods on the models and their field types, and project
	// code in them may read it. The models come from the model packages
	// imported above; resolving their columns never touches the database.
	if err := config.Init(); err != nil {
		fail(err)
	}

	// The index declarations are validated against the MySQL dialect whatever
	// database the configuration names: the handle never connects, and gg
	// check decides from the configuration whether what it finds is reported.
	schemaDB, err := gstmysql.DryRun()
	if err != nil {
		fail(err)
	}

	seen := make(map[string]struct{})
	out := make([]modelColumns, 0)
	models := model.RegisteredModels()
	// Models that declare a Design but no Migrate never reach the registry.
	// Their query columns resolve the same way, so those that opted in to
	// framework query parameters are inspected alongside the registered ones.
	for _, m := range unregisteredModels {
		if !inspect.IsQueryable(m) {
			continue
		}
		models = append(models, m)
	}
	// Models whose registration is ignored by gst.yaml gen.models.ignore
	// stay table-backed: their column files must keep matching the
	// module-copied model sources, so they are inspected unconditionally.
	models = append(models, ignoredModels...)
	// Table-backed models are compiled in by name as well: the registry
	// lists them once gg gen wrote model.gen.go, and gg check validates their
	// index declarations before that too. One the registry already lists is
	// inspected once (see seen).
	models = append(models, tableBackedModels...)
	for _, m := range models {
		typ := reflect.TypeOf(m)
		for typ != nil && typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ == nil || typ.PkgPath() == "" || typ.Name() == "" {
			continue
		}
		key := typ.PkgPath() + "." + typ.Name()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		cols, colsErr := inspect.Columns(typ)
		if colsErr != nil {
			fail(errors.Wrapf(colsErr, "resolve columns of %s", key))
		}
		entry := modelColumns{PkgPath: typ.PkgPath(), PkgName: packageName(typ), Name: typ.Name()}
		if validateErr := model.ValidateIndexes(schemaDB, m); validateErr != nil {
			entry.IndexViolation = validateErr.Error()
		}
		if validateErr := inspect.ValidateTenantIndexes(typ); validateErr != nil {
			entry.TenantIndexViolation = validateErr.Error()
		}
		if validateErr := inspect.ValidateFrameworkColumns(typ); validateErr != nil {
			entry.FrameworkColumnViolation = validateErr.Error()
		}
		if validateErr := inspect.ValidateQueryNames(typ); validateErr != nil {
			entry.QueryNameViolation = validateErr.Error()
		}
		if validateErr := inspect.ValidateIDAccessors(typ); validateErr != nil {
			entry.IDAccessorViolation = validateErr.Error()
		}
		for _, col := range cols {
			expr, imports := describeType(col.Type, typ.PkgPath())
			class := inspect.ClassifyColumn(col)
			entry.Columns = append(entry.Columns, columnInfo{
				GoName:      col.GoName,
				DBName:      col.DBName,
				TypeExpr:    expr,
				TypeImports: imports,
				TypeName:    col.Type.String(),
				Numeric:     class == inspect.ColumnClassNumeric,
				Time:        class == inspect.ColumnClassTime,
				JSONStrings: inspect.IsJSONStringsType(col.Type),
			})
		}
		out = append(out, entry)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		fail(err)
	}
	// The result travels through a file: initialization writes to stdout, so
	// stdout is not a reliable data channel.
	// #nosec G703 -- the result file is the one gg created for this run.
	if err = os.WriteFile(os.Args[1], encoded, 0o600); err != nil {
		fail(err)
	}
}

// packageName returns the package name the compiler recorded for the type,
// which can differ from the last path segment.
func packageName(typ reflect.Type) string {
	if before, _, ok := strings.CutLast(typ.String(), "."); ok {
		return before
	}
	return ""
}

// describeType renders a column type as a source-level expression relative
// to the model's package, modelPkg, plus the imports the expression needs by
// path, each under the package name the compiler recorded, which is how the
// expression qualifies the type: a type of the model's own package is
// written bare, one of another package qualified, the way go/types prints a
// type relative to a package, and a composite type, a slice, an array, a map
// or a pointer, is written from its elements, each qualified and imported
// on its own. The pointer of a pointer column is dereferenced first, since a
// filter compares the pointed-to value. A type that cannot be written back
// as source, a generic instantiation, or an anonymous struct, interface,
// channel or function naming a type of some package, yields an empty
// expression and is generated as Column[any]: the column name stays exact
// and the JSON operators, which take a string, keep working.
func describeType(typ reflect.Type, modelPkg string) (expr string, imports map[string]string) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	imports = make(map[string]string)
	expr, ok := typeExpr(typ, modelPkg, imports)
	if !ok {
		return "", nil
	}
	return expr, imports
}

// typeExpr writes typ the way describeType does, adding the imports it needs
// to imports; false for a type that cannot be written back as source.
// Framework-internal types reach business models only through the public
// packages that alias them. Reflection sees the defined type's internal
// path, which a business project cannot import, so the reference is
// rewritten to the alias the model source actually wrote: model.Base and
// its siblings for internal/modelregistry, model.Version for
// internal/modelschema, the root gst package for internal/types, which
// forwards under the same name every type of it a model can name and this
// function writes back, the generic streams grpc forwards being
// instantiations it declines, the service package for internal/apiaction,
// service.Phase among them, and, for the types a model stores as JSON, the
// grpc package for internal/controller and internal/grpcserver, grpc.Filter
// and grpc.Caller among them, sse for internal/sse and testutil for
// internal/testutil. The tenant column's type is forwarded by no package, a
// model carrying it only through tenant.Scope, so its reference is written
// as the type its values are, string.
func typeExpr(typ reflect.Type, modelPkg string, imports map[string]string) (string, bool) {
	if name := typ.Name(); name != "" {
		if strings.ContainsAny(name, "[]*") {
			return "", false
		}
		switch pkg := typ.PkgPath(); pkg {
		case "", modelPkg:
			return name, true
		case "github.com/hydroan/gst/internal/modelregistry", "github.com/hydroan/gst/internal/modelschema":
			imports["github.com/hydroan/gst/model"] = "model"
			return "model." + name, true
		case "github.com/hydroan/gst/internal/types":
			imports["github.com/hydroan/gst"] = "gst"
			return "gst." + name, true
		case "github.com/hydroan/gst/internal/apiaction":
			imports["github.com/hydroan/gst/service"] = "service"
			return "service." + name, true
		case "github.com/hydroan/gst/internal/controller", "github.com/hydroan/gst/internal/grpcserver":
			imports["github.com/hydroan/gst/grpc"] = "grpc"
			return "grpc." + name, true
		case "github.com/hydroan/gst/internal/sse":
			imports["github.com/hydroan/gst/sse"] = "sse"
			return "sse." + name, true
		case "github.com/hydroan/gst/internal/testutil":
			imports["github.com/hydroan/gst/testutil"] = "testutil"
			return "testutil." + name, true
		case "github.com/hydroan/gst/internal/tenant":
			return typ.Kind().String(), true
		default:
			alias := packageName(typ)
			imports[pkg] = alias
			return alias + "." + name, true
		}
	}
	switch typ.Kind() {
	case reflect.Pointer:
		elem, ok := typeExpr(typ.Elem(), modelPkg, imports)
		return "*" + elem, ok
	case reflect.Slice:
		elem, ok := typeExpr(typ.Elem(), modelPkg, imports)
		return "[]" + elem, ok
	case reflect.Array:
		elem, ok := typeExpr(typ.Elem(), modelPkg, imports)
		return "[" + strconv.Itoa(typ.Len()) + "]" + elem, ok
	case reflect.Map:
		key, ok := typeExpr(typ.Key(), modelPkg, imports)
		elem, elemOK := typeExpr(typ.Elem(), modelPkg, imports)
		return "map[" + key + "]" + elem, ok && elemOK
	}
	// An anonymous struct, interface, channel or function is written as the
	// compiler prints it, struct { X int } or interface {}, as long as it
	// names no type of any package, which the compiler would qualify by a
	// package name the file does not import.
	if text := typ.String(); !strings.Contains(text, ".") {
		return text, true
	}
	return "", false
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
