package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"golang.org/x/tools/go/packages"
)

// validateTagScope is the directory under the root holding the model sources
// of the framework's modules, which gg module copy copies into a project's
// model directory.
const validateTagScope = ggconst.DirModuleModel + "/"

// validateTag is the struct tag go-playground/validator reads rules from when
// it is used on its own, and the one the framework never reads.
const validateTag = "validate"

// checkValidateTag reports the struct fields of the module model sources,
// the files under validateTagScope, that carry a validate struct tag. The
// framework validates a model or a payload with gin's validator, which reads
// the rules of a field from its binding tag and from no other: rules under
// validate read like validation and never run, so the request they are there
// to refuse is served. A field carrying a binding tag beside the validate one
// is reported as well. gg check refuses the tag in a project's own model
// files (the Validate tag ban of internal/ggcheck) and skips the subtrees gg
// module copy writes there, copied module code being checked in the framework
// repository: this is that check. Test files, which only the test variants of
// a package hold, are left alone.
func checkValidateTag(root string, pkgs []*packages.Package) ([]violation, error) {
	var found []violation
	for _, p := range pkgs {
		if p.ForTest != "" {
			continue
		}
		for _, file := range p.Syntax {
			rel := relative(root, p.Fset.Position(file.Pos()).Filename)
			if !strings.HasPrefix(rel, validateTagScope) {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				spec, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				structType, ok := spec.Type.(*ast.StructType)
				if !ok || structType.Fields == nil {
					return true
				}
				for _, field := range structType.Fields.List {
					if field.Tag == nil {
						continue
					}
					raw, err := strconv.Unquote(field.Tag.Value)
					if err != nil {
						continue
					}
					rules, ok := reflect.StructTag(raw).Lookup(validateTag)
					if !ok {
						continue
					}
					name := types.ExprString(field.Type)
					if len(field.Names) > 0 {
						name = field.Names[0].Name
					}
					found = append(found, violation{
						File: rel,
						Message: fmt.Sprintf("Field '%s.%s' at %s:%d declares its validation rules under the validate tag (%s), which the framework never runs: declare them under the binding tag",
							spec.Name.Name, name, rel, p.Fset.Position(field.Pos()).Line, rules),
					})
				}
				return true
			})
		}
	}
	slices.SortFunc(found, func(a, b violation) int {
		return strings.Compare(a.Message, b.Message)
	})
	return found, nil
}
