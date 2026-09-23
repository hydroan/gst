package dsl

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"

	"github.com/hydroan/gst/consts"
)

var routePhaseOrder = []consts.Phase{
	consts.PHASE_CREATE,
	consts.PHASE_DELETE,
	consts.PHASE_UPDATE,
	consts.PHASE_PATCH,
	consts.PHASE_LIST,
	consts.PHASE_IMPORT,
	consts.PHASE_EXPORT,
	consts.PHASE_SSE,
	consts.PHASE_GET,
	consts.PHASE_CREATE_MANY,
	consts.PHASE_DELETE_MANY,
	consts.PHASE_UPDATE_MANY,
	consts.PHASE_PATCH_MANY,
}

func emitRouteActions(route string, actions []*Action, fn func(string, *Action)) {
	if len(actions) == 0 || fn == nil {
		return
	}
	for _, phase := range routePhaseOrder {
		for _, action := range actions {
			if action == nil || !action.Enabled || action.Phase != phase {
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
