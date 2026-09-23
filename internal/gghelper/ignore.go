package gghelper

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// ProjectIgnore is the set of paths the project's Git ignore rules exclude.
// gg check and gg gen read the project through the same set, so a file the
// project ignores takes part in neither: runtime artifacts such as the log
// directories a test run leaves behind fail no check, and a scratch model is
// generated from no more than it is checked.
type ProjectIgnore struct {
	matcher gitignore.Matcher
}

// NewProjectIgnore loads the Git ignore rules of the project in the working
// directory, where gg runs. Building it scans the whole worktree for ignore
// files, so a command builds one and shares it across everything it reads.
func NewProjectIgnore() ProjectIgnore {
	patterns, err := gitignore.ReadPatterns(osfs.New("."), nil)
	if err != nil || len(patterns) == 0 {
		return ProjectIgnore{}
	}
	return ProjectIgnore{matcher: gitignore.NewMatcher(patterns)}
}

// Ignores reports whether the project-relative path is ignored.
func (p ProjectIgnore) Ignores(path string, isDir bool) bool {
	if p.matcher == nil {
		return false
	}
	return p.matcher.Match(strings.Split(path, string(filepath.Separator)), isDir)
}

// Walk walks root, pruning the ignored paths, so fn only sees paths the
// project keeps. Walk errors abort the walk instead of being delegated, so fn
// only sees paths that exist.
func (p ProjectIgnore) Walk(root string, fn func(path string, info os.FileInfo) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != root && p.Ignores(path, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return fn(path, info)
	})
}
