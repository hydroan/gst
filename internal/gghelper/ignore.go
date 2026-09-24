package gghelper

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/internal/ggconst"
	"golang.org/x/mod/modfile"
)

// ProjectIgnore is the set of paths gg leaves out when it reads the project's
// code: the ones the project's Git ignore rules exclude, and the ones the go
// command leaves out of a ./... pattern (see ExcludedByGo). gg check and gg
// gen read the project through the same set, so a file the project ignores
// takes part in neither: runtime artifacts such as the log directories a test
// run leaves behind fail no check, and a scratch model is generated from no
// more than it is checked. Nor do they read what the go command leaves out of
// the project's packages: a vendored package, a testdata tree, a _draft
// directory or a directory the project's go.mod ignores is left out of gg
// exactly as it is left out of go build ./....
//
// gg prune goes by neither: of the ignore rules, it follows gst.yaml's
// prune.ignore alone (see package ggprune).
type ProjectIgnore struct {
	matcher gitignore.Matcher
	// root is the absolute path of the project, against which an absolute
	// path is judged.
	root string
	// goIgnore holds the ignore directives of the project's go.mod.
	goIgnore goIgnorePatterns
}

// NewProjectIgnore loads the Git ignore rules and the go.mod ignore directives
// of the project in the working directory, where gg runs. Building it scans
// the whole worktree for ignore files, so a command builds one and shares it
// across everything it reads.
func NewProjectIgnore() ProjectIgnore {
	p := ProjectIgnore{goIgnore: readGoIgnorePatterns("go.mod")}
	if root, err := os.Getwd(); err == nil {
		p.root = root
	}
	if patterns, err := gitignore.ReadPatterns(osfs.New("."), nil); err == nil && len(patterns) > 0 {
		p.matcher = gitignore.NewMatcher(patterns)
	}
	return p
}

// Ignores reports whether the project-relative path is ignored: the
// project's Git ignore rules exclude it, or the go command leaves it or a
// directory above it out (see ExcludedByGo). For example model/_draft/item.go
// is ignored, as the go command leaves out _draft and everything below it.
func (p ProjectIgnore) Ignores(path string, isDir bool) bool {
	if p.gitIgnores(path, isDir) {
		return true
	}
	elems := strings.Split(filepath.Clean(path), string(filepath.Separator))
	for i := range elems {
		if p.ExcludedByGo(filepath.Join(elems[:i+1]...), isDir || i < len(elems)-1) {
			return true
		}
	}
	return false
}

// gitIgnores reports whether the project's Git ignore rules exclude the
// project-relative path, whatever the go command does with it.
func (p ProjectIgnore) gitIgnores(path string, isDir bool) bool {
	return p.matcher != nil && p.matcher.Match(strings.Split(path, string(filepath.Separator)), isDir)
}

// ExcludedByGo reports whether the go command leaves the file or directory at
// path out of a ./... pattern, whatever the project's Git ignore rules say,
// judging path by its own name and, for a directory, by what it holds: the
// go command leaves out a file or directory whose name begins with "." or
// "_", a directory named vendor or testdata, a directory holding a go.mod file
// of its own, whose code belongs to another module, and a directory an ignore
// directive of the project's go.mod names. A walk that prunes what
// ExcludedByGo reports leaves out everything below as well, as the go command
// does; the directories above path are the walk's to judge. For example
// .git, _draft, _draft.go, service/testdata, vendor and, with a go.mod saying
// ignore ./web, web are left out, while service and service/sample.go are
// not.
func (p ProjectIgnore) ExcludedByGo(path string, isDir bool) bool {
	name := filepath.Base(path)
	if name == "." || name == ".." {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	if !isDir {
		return false
	}
	if name == ggconst.DirVendor || name == ggconst.DirTestData {
		return true
	}
	if info, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && !info.IsDir() {
		return true
	}
	if p.root != "" && filepath.IsAbs(path) {
		rel, err := filepath.Rel(p.root, path)
		if err != nil {
			return false
		}
		path = rel
	}
	return p.goIgnore.matches(path)
}

// Walk walks root, pruning the ignored paths (see Ignores), so fn only sees
// paths the project keeps. Root itself is never pruned: a walk goes where its
// caller points it. Walk errors abort the walk instead of being delegated, so
// fn only sees paths that exist.
func (p ProjectIgnore) Walk(root string, fn func(path string, info os.FileInfo) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Every directory above path has been let through already, so the go
		// command's rules have only path itself left to judge.
		if path != root && (p.gitIgnores(path, info.IsDir()) || p.ExcludedByGo(path, info.IsDir())) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return fn(path, info)
	})
}

// goIgnorePatterns are the ignore directives of a go.mod, kept the way the go
// command keeps them to match a directory against: each one slashed at both
// ends, those starting with ./ apart from the others.
//
// The go command holds this matching in its own internal package
// (search.IgnorePatterns in cmd/go), which nothing outside the Go distribution
// can import, and golang.org/x/mod only parses the directives; so the rules
// are repeated here, one for one, to leave out the directories go leaves out.
type goIgnorePatterns struct {
	// relative holds the directives starting with ./, each naming the one
	// directory at that path from the module root.
	relative []string
	// anywhere holds the others, each naming every directory whose path from
	// the module root contains it.
	anywhere []string
}

// readGoIgnorePatterns reads the ignore directives of the go.mod at path. A
// go.mod that cannot be read or parsed declares none, as the go command takes
// it: every command that builds the project reports the file itself.
func readGoIgnorePatterns(path string) goIgnorePatterns {
	content, err := os.ReadFile(path)
	if err != nil {
		return goIgnorePatterns{}
	}
	file, err := modfile.Parse(path, content, nil)
	if err != nil {
		return goIgnorePatterns{}
	}
	var patterns goIgnorePatterns
	for _, ignore := range file.Ignore {
		if rest, isRelative := strings.CutPrefix(ignore.Path, "./"); isRelative {
			patterns.relative = append(patterns.relative, slashedDir(rest))
		} else {
			patterns.anywhere = append(patterns.anywhere, slashedDir(ignore.Path))
		}
	}
	return patterns
}

// matches reports whether an ignore directive names the directory at dir, a
// path from the module root. With the directives ignore ./web and ignore
// node_modules, it matches web, web/assets, node_modules and
// service/node_modules/lib, but not service/web.
func (g goIgnorePatterns) matches(dir string) bool {
	if dir == "" {
		return false
	}
	dir = slashedDir(dir)
	for _, pattern := range g.relative {
		if strings.HasPrefix(dir, pattern) {
			return true
		}
	}
	for _, pattern := range g.anywhere {
		if strings.Contains(dir, pattern) {
			return true
		}
	}
	return false
}

// slashedDir writes path with forward slashes and a slash at each end, the
// form both an ignore directive and the directory it is matched against take:
// web/assets becomes /web/assets/.
func slashedDir(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}
