// Shared pieces of the gst.yaml ignore machinery. The route rules live in
// gen_ignore_route.go and the model rules in gen_ignore_model.go; this file
// keeps only the symbols both sides consume.

package main

import (
	"path/filepath"
	"strings"
)

// multiSourceRule records a From-less rule that matched models under more
// than one model directory. Both the route and the model ignore reports use
// it to warn about rules that likely swallow a project's own declaration.
type multiSourceRule struct {
	Raw  string
	Dirs []string
}

// modelRootDir returns the first two path segments of a model file path
// (e.g. "model/iam" for "model/iam/user/user.go"), the granularity at which
// module copy lays out framework modules. Files directly under the model
// root yield just the root (e.g. "model" for "model/user.go").
func modelRootDir(modelFilePath string) string {
	parts := strings.SplitN(filepath.ToSlash(modelFilePath), "/", 3)
	if len(parts) < 3 {
		return parts[0]
	}
	return parts[0] + "/" + parts[1]
}
