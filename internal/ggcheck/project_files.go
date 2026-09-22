// How every check reads the project: the walk that honors the project's Git
// ignore rules, the files gg generates and owns, and paths as the checks print
// them.

package ggcheck

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

func currentProjectModulePath() string {
	modulePath, err := gghelper.ModulePath()
	if err != nil {
		return ""
	}
	return strings.Trim(modulePath, "/")
}

// relativePath returns filePath relative to the current working directory when possible.
func relativePath(filePath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filePath
	}
	relPath, err := filepath.Rel(cwd, filePath)
	if err != nil {
		return filePath
	}
	return relPath
}

// newProjectIgnoreMatcher loads Git ignore rules for the project root. Every
// check walks the project from its root, so the root is not a parameter.
// Building a matcher scans the whole worktree for ignore files, so
// Run builds one and shares it across every check it runs.
func newProjectIgnoreMatcher() gitignore.Matcher {
	patterns, err := gitignore.ReadPatterns(osfs.New("."), nil)
	if err != nil || len(patterns) == 0 {
		return nil
	}
	return gitignore.NewMatcher(patterns)
}

// isIgnoredProjectPath reports whether a project-relative path is ignored by
// Git rules.
func isIgnoredProjectPath(matcher gitignore.Matcher, path string, isDir bool) bool {
	if matcher == nil {
		return false
	}
	return matcher.Match(strings.Split(path, string(filepath.Separator)), isDir)
}

// walkProjectDir walks root while pruning paths ignored by Git rules, so
// runtime artifacts such as the log directories a test run leaves behind
// never reach checkFn. Walk errors abort the walk instead of being delegated,
// so checkFn only sees paths that exist.
func walkProjectDir(root string, ignore gitignore.Matcher, checkFn func(path string, info os.FileInfo) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != root && isIgnoredProjectPath(ignore, path, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return checkFn(path, info)
	})
}

// fileExists reports whether a file exists at filename.
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// isGeneratedFileName reports whether a path is a file gg generates and owns.
func isGeneratedFileName(path string) bool {
	return strings.HasSuffix(path, ggconst.SuffixGenGo)
}
