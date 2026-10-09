package ggcheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// ActionTypeUniqueness holds every explicit DSL action type name to the one
// action declaring it.
var ActionTypeUniqueness = Check{
	Name: "Action type uniqueness",
	Rule: "an explicit DSL Payload, Result, StreamingPayload or StreamingResult type name must be declared by one action alone: each action names request and response types of its own, an alias of a shared type when the fields are the same",
	run:  checkActionTypeUniqueness,
}

// actionTypeBinding is one explicit action type declaration: the file, the
// action and the route it is made on, the side declared and the type as the
// declaration wrote it.
type actionTypeBinding struct {
	relPath string
	action  string
	route   string
	kind    string
	raw     string
}

// checkActionTypeUniqueness reports the explicit action type names two
// actions of one model package declare. An action's Payload, Result,
// StreamingPayload and StreamingResult name the request and response types
// the action owns: two actions declaring one name share one contract, a
// field added for one reaching the other, and the generated service code,
// the API documentation and the TypeScript types present them as one. Two
// actions answering the same shape name it separately, an alias each
// (type SampleGetRsp = SampleRsp): the alias keeps the action's own name in
// the model, the generated service and the TypeScript types, and becomes a
// type of its own the day the action diverges, while the generated messages
// and the API documentation carry the aliased type. The model's own type,
// which the standard CRUD actions carry on both sides and a custom action may
// answer with, is the resource itself and counts for no action, as the naming
// check exempts it. Violations name the action declaring the name first.
func checkActionTypeUniqueness(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	var paths []string
	err := ignore.Walk(ggconst.DirModel, func(path string, info os.FileInfo) error {
		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") && !isGeneratedFileName(path) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return []string{fmt.Sprintf("walking model directory: %v", err)}
	}

	var violations []string
	// first holds the binding declaring each name first, by the package
	// directory and the name: packages name their types apart.
	first := make(map[string]actionTypeBinding)
	fset := token.NewFileSet()
	for _, path := range paths {
		// A file that does not parse is left to the compiler, as the other
		// checks leave it.
		node, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			continue
		}
		relPath, dir := gghelper.RelativePath(path), filepath.Dir(path)
		designs := dsl.Parse(node)
		for _, modelName := range slices.Sorted(maps.Keys(designs)) {
			designs[modelName].Range(func(route string, action *dsl.Action) {
				for _, side := range actionTypeSides(action) {
					name := strings.TrimPrefix(side.raw, "*")
					if name == modelName {
						continue
					}
					binding := actionTypeBinding{relPath: relPath, action: action.Phase.Name(), route: route, kind: side.kind, raw: side.raw}
					key := dir + " " + name
					if earlier, bound := first[key]; bound {
						violations = append(violations, actionTypeDeclaredTwice(binding, earlier))
						continue
					}
					first[key] = binding
				}
			})
		}
	}
	return violations
}

// actionTypeDeclaredTwice words the violation of later, a declaration of the
// name first declared already, the file of first named when it is another:
//
//	model/sample/sample.go: List action on samples declares Result[*SampleRsp], which the Create action on samples declares as Result[*SampleRsp] already; each action names request and response types of its own
func actionTypeDeclaredTwice(later, first actionTypeBinding) string {
	where := ""
	if first.relPath != later.relPath {
		where = " in " + first.relPath
	}
	return fmt.Sprintf("%s: %s action on %s declares %s[%s], which the %s action on %s declares as %s[%s] already%s; each action names request and response types of its own",
		later.relPath, later.action, later.route, later.kind, later.raw, first.action, first.route, first.kind, first.raw, where)
}
