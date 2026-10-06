package dsl

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/consts"
)

var routePhaseOrder = []consts.Phase{
	consts.Create,
	consts.Delete,
	consts.Update,
	consts.Patch,
	consts.List,
	consts.Import,
	consts.Export,
	consts.SSE,
	consts.Stream,
	consts.Get,
	consts.CreateMany,
	consts.DeleteMany,
	consts.UpdateMany,
	consts.PatchMany,
}

func emitRouteActions(route string, actions []*Action, fn func(string, *Action)) {
	if len(actions) == 0 || fn == nil {
		return
	}
	for _, phase := range routePhaseOrder {
		for _, action := range actions {
			if action == nil || action.Phase != phase {
				continue
			}
			fn(route, action)
		}
	}
}

// is checks if the given name is a valid DSL keywords.
// It verifies against the predefined list of supported DSL keywords.
//
// Parameters:
//   - name: The keyword to check
//
// Returns:
//   - bool: true if the name is a valid DSL keyword, false otherwise
func is(name string) bool {
	return slices.Contains(methodList, name)
}

// stringLiteral returns the value of expr when it is a Go string literal,
// decoded the way the compiler reads it (see strconv.Unquote): "users" and
// `users` both give users, "a\"b" gives a"b, "a\tb" gives a, a tab and b,
// and "'draft'" gives 'draft' with its quotes. ok is false for anything
// else, a string constant named by an identifier included.
func stringLiteral(expr ast.Expr) (value string, ok bool) {
	lit, isLit := expr.(*ast.BasicLit)
	if !isLit || lit == nil || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

// endpointSegment returns the path segment Endpoint(value) declares: value
// without its leading slashes and with each slash left written as a hyphen,
// so Endpoint("/iam/users") declares iam-users and Endpoint("/") declares
// nothing, "", which leaves the model on its default segment.
func endpointSegment(value string) string {
	return strings.ReplaceAll(strings.TrimLeft(value, "/"), "/", "-")
}

// paramName returns the route parameter name Param(value) declares: value
// without the spaces, braces, brackets and colons around it, so
// Param("user"), Param(":user") and Param("{user}") all declare user and
// Param(":") declares nothing, "".
func paramName(value string) string {
	return strings.TrimFunc(value, func(r rune) bool {
		return r == ' ' || r == '{' || r == '}' || r == '[' || r == ']' || r == ':'
	})
}

// routePath returns the path Route(value, block) declares: value without its
// leading slashes, so Route("/archive/items", block) declares archive/items
// and Route("/", block) declares nothing, "".
func routePath(value string) string {
	return strings.TrimLeft(value, "/")
}
