// The warnings the gst.yaml ignore rules raise when gg gen applies them. The
// rules themselves apply in codegen: the route ignores in
// codegen.ResolveRoutes, the model ignores in codegen.ApplyModelIgnores.

package main

import (
	"strings"

	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/codegen"
)

// reportModelIgnoreWarnings warns about model ignore rules that matched no
// migrating model (a stale rule means a previously removed table silently
// comes back), about From-less rules matching models under several
// directories, and about ignored models whose routes are still enabled.
// Warnings are emitted even in quiet mode.
func reportModelIgnoreWarnings(result codegen.ModelIgnoreResult) {
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

// reportRouteIgnoreWarnings warns about ignore rules that matched no
// generated route (a stale rule means a previously ignored route may have
// silently come back) and about From-less rules matching models under
// several directories (likely swallowing the project's own re-declaration).
// Warnings are emitted even in quiet mode.
func reportRouteIgnoreWarnings(result codegen.RouteIgnoreResult) {
	for _, rule := range result.Unmatched {
		clioutput.Warn("", "gst.yaml ignore rule matched no route: %s", rule.Raw)
	}
	for _, rule := range result.MultiSourceRules {
		clioutput.Warn("", "gst.yaml ignore rule %q matched models under %s; add \"from\" to scope it to one directory", rule.Raw, strings.Join(rule.Dirs, ", "))
	}
}
