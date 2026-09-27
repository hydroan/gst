package modelinfo

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
)

// RouteIgnoreMatch records one generated route disabled by a gst.yaml
// gen.routes.ignore rule.
type RouteIgnoreMatch struct {
	Method string
	Path   string
	Model  string
}

// RouteIgnoreResult reports how the gst.yaml route ignore rules applied to
// the models.
type RouteIgnoreResult struct {
	// Matches lists the disabled routes in scan order.
	Matches []RouteIgnoreMatch

	// Unmatched lists rules that matched no generated route, usually a sign
	// the configuration is stale after a framework module update.
	Unmatched []ggconfig.RouteRule

	// MultiSourceRules lists rules without a From prefix that matched models
	// under more than one model directory. Such a rule likely swallows a
	// project's own re-declaration of a framework route and should be scoped
	// with "from".
	MultiSourceRules []MultiSourceRule

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
// ignore rule. A disabled action drops out of the generated service and
// router registration files (service/service.gen.go, router/router.gen.go)
// and no new service file is generated for it, but existing service files
// are kept on disk. Rules with a From prefix only apply to models declared
// under that directory. ResolveRoutes runs it once the endpoints are
// resolved, which the rules match against.
func applyRouteIgnores(allModels []*Model, rules []ggconfig.RouteRule) RouteIgnoreResult {
	result := RouteIgnoreResult{}
	if len(rules) == 0 {
		return result
	}
	result.KeptServiceFiles = make(map[string]bool)
	result.KeptServiceDirs = make(map[string]bool)

	matched := make([]bool, len(rules))
	matchedDirs := make([]map[string]bool, len(rules))
	for _, m := range allModels {
		// The matched actions are dropped once Range is over: Drop is not
		// for use inside its callback.
		dropped := make([]*dsl.Action, 0)
		m.Design.Range(func(route string, act *dsl.Action) {
			// The rules name HTTP methods and paths; an action served over
			// gRPC alone has neither.
			if dsl.GRPCOnlyAction(act.Phase.Name()) {
				return
			}
			finalRoute, _ := RouterTargetForAction(route, m.Design, act)
			method := act.Phase.HTTPMethod()
			for i, rule := range rules {
				if !rule.Match(method, finalRoute) || !rule.MatchesSource(m.ModelFilePath) {
					continue
				}
				if act.Service {
					target := ServiceTarget(m, act, ggconst.DirModel, ggconst.DirService)
					result.KeptServiceFiles[target.FilePath] = true
					result.KeptServiceDirs[filepath.Clean(target.Dir)] = true
				}
				dropped = append(dropped, act)
				matched[i] = true
				if matchedDirs[i] == nil {
					matchedDirs[i] = make(map[string]bool)
				}
				matchedDirs[i][ModelRootDir(m.ModelFilePath)] = true
				result.Matches = append(result.Matches, RouteIgnoreMatch{
					Method: method,
					Path:   "/" + strings.Join(ggconfig.NormalizeRoutePath(finalRoute), "/"),
					Model:  m.ModelName,
				})
				break
			}
		})
		for _, act := range dropped {
			m.Design.Drop(act)
		}
	}

	for i, rule := range rules {
		if !matched[i] {
			result.Unmatched = append(result.Unmatched, rule)
			continue
		}
		if rule.From == "" && len(matchedDirs[i]) > 1 {
			dirs := make([]string, 0, len(matchedDirs[i]))
			for dir := range matchedDirs[i] {
				dirs = append(dirs, dir)
			}
			sort.Strings(dirs)
			result.MultiSourceRules = append(result.MultiSourceRules, MultiSourceRule{Raw: rule.Raw, Dirs: dirs})
		}
	}
	return result
}

// MultiSourceRule records a From-less rule that matched models under more
// than one model directory. The route ignore result carries it, and the
// model ignores of gg gen reuse it, to warn about rules that likely swallow
// a project's own declaration.
type MultiSourceRule struct {
	Raw  string
	Dirs []string
}

// ModelRootDir returns the first two path segments of a model file path
// (e.g. "model/iam" for "model/iam/user/user.go"), the granularity at which
// module copy lays out framework modules. Files directly under the model
// root yield just the root (e.g. "model" for "model/user.go").
func ModelRootDir(modelFilePath string) string {
	parts := strings.SplitN(filepath.ToSlash(modelFilePath), "/", 3)
	if len(parts) < 3 {
		return parts[0]
	}
	return parts[0] + "/" + parts[1]
}
