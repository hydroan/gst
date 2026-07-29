package main

import (
	"sort"
	"strings"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
)

// modelIgnoreMatch records one model whose generated model.Register call is
// skipped by a gst.yaml gen.models.ignore rule.
type modelIgnoreMatch struct {
	Model string
	File  string
}

// modelIgnoreResult reports how the gst.yaml model ignore rules applied to
// the scanned models.
type modelIgnoreResult struct {
	// Matches lists the ignored models in scan order.
	Matches []modelIgnoreMatch

	// Unmatched lists rules that matched no migrating model, usually a sign
	// the configuration is stale after a framework module update: the table
	// would silently come back.
	Unmatched []ggconfig.ModelRule

	// MultiSourceRules lists rules without a From prefix that matched models
	// under more than one model directory. Such a rule likely swallows a
	// project's own model of the same name and should be scoped with "from".
	MultiSourceRules []multiSourceRule

	// LiveActionModels lists ignored models that still have enabled actions:
	// their routes will operate on a table the framework no longer creates,
	// which is only sound when another model owns that table.
	LiveActionModels []modelIgnoreMatch
}

// applyModelIgnores skips the generated model.Register call of every model
// matched by an ignore rule, by clearing Design.Migrate and marking the
// model as RegisterIgnored for the column generation path. Routes, service
// registrations, and generated files are not touched. It must run after
// applyRouteIgnores so the live-action report sees the final action set.
func applyModelIgnores(allModels []*gen.ModelInfo, rules []ggconfig.ModelRule) modelIgnoreResult {
	result := modelIgnoreResult{}
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
			matchedDirs[i][modelRootDir(m.ModelFilePath)] = true

			match := modelIgnoreMatch{Model: m.ModelName, File: m.ModelFilePath}
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
			result.MultiSourceRules = append(result.MultiSourceRules, multiSourceRule{Raw: rule.Raw, Dirs: dirs})
		}
	}
	return result
}

// reportModelIgnoreWarnings warns about model ignore rules that matched no
// migrating model (a stale rule means a previously removed table silently
// comes back), about From-less rules matching models under several
// directories, and about ignored models whose routes are still enabled.
// Warnings are emitted even in quiet mode.
func reportModelIgnoreWarnings(result modelIgnoreResult) {
	for _, rule := range result.Unmatched {
		clioutput.Warn("", "gst.yaml model ignore rule matched no migrating model: %s", rule.Raw)
	}
	for _, rule := range result.MultiSourceRules {
		clioutput.Warn("", "gst.yaml model ignore rule %q matched models under %s; add \"from\" to scope it to one directory", rule.Raw, strings.Join(rule.Dirs, ", "))
	}
	for _, match := range result.LiveActionModels {
		clioutput.Warn("", "gst.yaml ignores registration of model %s (%s) but its routes stay enabled; add gen.routes.ignore entries or ensure another model owns its table", match.Model, match.File)
	}
}
