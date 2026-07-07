package main

import (
	"path/filepath"
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

	// KeptServiceFiles are the service files owned by ignored Service()
	// actions. Ignoring a route only removes its registrations from the
	// generated files; the service files stay on disk so the project remains
	// file-identical with gg module copy output, and prune must not treat
	// them as deletable.
	KeptServiceFiles map[string]bool

	// KeptServiceDirs are the cleaned service directories that contain the
	// kept service files, protected from orphan-directory cleanup.
	KeptServiceDirs map[string]bool
}

// applyRouteIgnores disables every action whose generated route matches an
// ignore rule. A disabled action drops out of the generated registration
// files (model/model.go, service/service.go, router/router.go) and no new
// service file is generated for it, but existing service files are kept on
// disk. Models must have hierarchical endpoints built before calling this.
func applyRouteIgnores(allModels []*gen.ModelInfo, rules []ggconfig.RouteRule) routeIgnoreResult {
	result := routeIgnoreResult{}
	if len(rules) == 0 {
		return result
	}
	result.KeptServiceFiles = make(map[string]bool)
	result.KeptServiceDirs = make(map[string]bool)

	matched := make([]bool, len(rules))
	for _, m := range allModels {
		m.Design.Range(func(route string, act *dsl.Action) {
			finalRoute, _ := routerTargetForAction(route, m.Design, act)
			method := routePhaseMethod(act.Phase.MethodName())
			for i, rule := range rules {
				if !rule.Match(method, finalRoute) {
					continue
				}
				if act.Service {
					target := gen.ServiceTarget(m, act, modelDir, serviceDir)
					result.KeptServiceFiles[target.FilePath] = true
					result.KeptServiceDirs[filepath.Clean(target.Dir)] = true
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
