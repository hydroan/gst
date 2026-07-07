package main

import (
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
)

// routeIgnoreMatch records one generated route disabled by a gst.yaml
// gen.routes.ignore rule.
type routeIgnoreMatch struct {
	Method string
	Path   string
	Model  string
}

// routeIgnoreResult reports how the gst.yaml ignore rules applied to the
// scanned models.
type routeIgnoreResult struct {
	// Matches lists the disabled routes in scan order.
	Matches []routeIgnoreMatch

	// Unmatched lists rules that matched no generated route, usually a sign
	// the configuration is stale after a framework module update.
	Unmatched []ggconfig.RouteRule
}

// applyRouteIgnores disables every action whose generated route matches an
// ignore rule. A disabled action is equivalent to an action not declared in
// the model Design: no router registration, no service registration, and no
// service file generation. Models must have hierarchical endpoints built
// before calling this.
func applyRouteIgnores(allModels []*gen.ModelInfo, rules []ggconfig.RouteRule) routeIgnoreResult {
	result := routeIgnoreResult{}
	if len(rules) == 0 {
		return result
	}

	matched := make([]bool, len(rules))
	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			finalRoute, _ := routerTargetForAction(route, m.Design, act)
			method := routePhaseMethod(act.Phase.MethodName())
			for i, rule := range rules {
				if !rule.Match(method, finalRoute) {
					continue
				}
				act.Enabled = false
				matched[i] = true
				result.Matches = append(result.Matches, routeIgnoreMatch{
					Method: method,
					Path:   "/api/" + strings.Join(ggconfig.NormalizeRoutePath(finalRoute), "/"),
					Model:  m.ModelName,
				})
				break
			}
		})
	}

	for i, rule := range rules {
		if !matched[i] {
			result.Unmatched = append(result.Unmatched, rule)
		}
	}
	return result
}

// reportUnmatchedRouteIgnores warns about ignore rules that matched no
// generated route. Warnings are emitted even in quiet mode because a stale
// rule means a previously ignored route may have silently come back.
func reportUnmatchedRouteIgnores(result routeIgnoreResult) {
	for _, rule := range result.Unmatched {
		clioutput.Warn("", "gst.yaml ignore rule matched no route: %s", rule.Raw)
	}
}
