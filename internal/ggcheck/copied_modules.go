// The model and service subtrees gg module copy writes a framework module
// into. The checks of project-owned code skip them: copied module code is
// tested inside the framework repository.

package ggcheck

import (
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/internal/ggmodule"
)

// copyableModuleOwners returns the first path segments under the model and
// service directories owned by copyable framework modules: gg module copy
// writes module code to model/<module>/... and service/<module>/... subtrees.
func copyableModuleOwners() (map[string]bool, error) {
	names, err := ggmodule.CopyableModuleNames()
	if err != nil {
		return nil, err
	}
	owned := make(map[string]bool, len(names))
	for _, name := range names {
		owned[name] = true
	}
	return owned, nil
}

// moduleOwnedPath reports whether a path below root falls under a subtree
// owned by a copyable framework module.
func moduleOwnedPath(owned map[string]bool, root, path string) bool {
	if len(owned) == 0 {
		return false
	}
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	first, _, _ := strings.Cut(rel, string(filepath.Separator))
	return owned[first]
}
