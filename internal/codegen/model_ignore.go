package codegen

import (
	"sort"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
)

// ModelIgnoreMatch records one model whose generated model.Register call is
// skipped by a gst.yaml gen.models.ignore rule.
type ModelIgnoreMatch struct {
	Model string
	File  string
}

// ModelIgnoreResult reports how the gst.yaml model ignore rules applied to
// the scanned models.
type ModelIgnoreResult struct {
	// Matches lists the ignored models in scan order.
	Matches []ModelIgnoreMatch

	// Unmatched lists rules that matched no migrating model, usually a sign
	// the configuration is stale after a framework module update: the table
	// would silently come back.
	Unmatched []ggconfig.ModelRule

	// MultiSourceRules lists rules without a From prefix that matched models
	// under more than one model directory. Such a rule likely swallows a
	// project's own model of the same name and should be scoped with "from".
	MultiSourceRules []MultiSourceRule

	// LiveActionModels lists ignored models that still have enabled actions:
	// their routes will operate on a table the framework no longer creates,
	// which is only sound when another model owns that table.
	LiveActionModels []ModelIgnoreMatch
}

// ApplyModelIgnores skips the generated model.Register call of every model
// matched by an ignore rule, by clearing Design.Migrate and marking the
// model as RegisterIgnored for the column generation path. Routes, service
// registrations, and generated files are not touched. It must run after
// ResolveRoutes so the live-action report sees the final action set.
func ApplyModelIgnores(allModels []*gen.ModelInfo, rules []ggconfig.ModelRule) ModelIgnoreResult {
	result := ModelIgnoreResult{}
	if len(rules) == 0 {
		return result
	}

	matched := make([]bool, len(rules))
	matchedDirs := make([]map[string]bool, len(rules))
	for _, m := range allModels {
		for i, rule := range rules {
			if rule.Name != m.ModelName || !rule.MatchesSource(m.ModelFilePath) {
				continue
			}
			// A model that never registers (action-only or virtual) gains
			// nothing from the rule; leaving it unmatched surfaces the rule
			// as stale instead of silently succeeding.
			if !m.Design.Enabled || !m.Design.Migrate {
				continue
			}

			m.Design.Migrate = false
			m.RegisterIgnored = true
			matched[i] = true
			if matchedDirs[i] == nil {
				matchedDirs[i] = make(map[string]bool)
			}
			matchedDirs[i][ModelRootDir(m.ModelFilePath)] = true

			match := ModelIgnoreMatch{Model: m.ModelName, File: m.ModelFilePath}
			result.Matches = append(result.Matches, match)
			live := false
			m.Design.Range(func(string, *dsl.Action) { live = true })
			if live {
				result.LiveActionModels = append(result.LiveActionModels, match)
			}
			// Model names are unique rule keys, so at most one rule matches.
			break
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
