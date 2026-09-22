//nolint:predeclared
package new

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"

	"github.com/cockroachdb/errors"
)

var requiredFileContentMap = map[string]string{
	"component/component.go":   componentContent,
	"configx/configx.go":       configxContent,
	"cronjob/cronjob.go":       cronjobContent,
	"leader/leader.go":         leaderContent,
	"lock/lock.go":             lockContent,
	"middleware/middleware.go": middlewareContent,
	// These three are the empty first versions of files gg gen owns, so they
	// carry the generated suffix: a project must compile before its first
	// generation, and the generator then overwrites the same file.
	"model/" + ggconst.FileModelGen:     modelContent,
	"service/" + ggconst.FileServiceGen: serviceContent,
	"module/module.go":                  moduleContent,
	"router/" + ggconst.FileRouterGen:   routerContent,
	"dao/.gitkeep":                      "",
	"provider/.gitkeep":                 "",
}

var projectFileContentMap = newProjectFileContentMap()

func newProjectFileContentMap() map[string]string {
	files := make(map[string]string, len(requiredFileContentMap)+1)
	maps.Copy(files, requiredFileContentMap)
	files[".golangci.yml"] = golangciLintContent
	return files
}

// ProjectFile is one file a new project starts with: Content goes to Path,
// relative to the project directory.
type ProjectFile struct {
	Path    string
	Content string
}

// ProjectFiles returns the files gg new writes into a new project, in the
// order it writes them: the scaffold sorted by path, then main.go, .gitignore
// and config.ini.example. projectName is the module path the project is
// created under; its last element names the application in the example
// configuration, as sampleapp does for example.com/sampleapp.
func ProjectFiles(projectName string) ([]ProjectFile, error) {
	paths := slices.Sorted(maps.Keys(projectFileContentMap))
	files := make([]ProjectFile, 0, len(paths)+3)
	for _, path := range paths {
		files = append(files, ProjectFile{Path: path, Content: projectFileContentMap[path]})
	}

	// main.go is the same file gg gen keeps regenerating, built by the same
	// generator so the scaffold and the generated version cannot drift.
	mainFile, err := gen.BuildMainFile(projectName)
	if err != nil {
		return nil, err
	}
	return append(files,
		ProjectFile{Path: ggconst.FileMain, Content: mainFile},
		ProjectFile{Path: ".gitignore", Content: gitignoreContent},
		ProjectFile{Path: "config.ini.example", Content: templateConfig(filepath.Base(projectName))},
	), nil
}

// ============================================================
// helpers
// ============================================================

func EnsureFileExists() ([]string, error) {
	files := make([]string, 0, len(requiredFileContentMap))
	for file := range requiredFileContentMap {
		files = append(files, file)
	}
	sort.Strings(files)

	var created []string
	for _, file := range files {
		content := requiredFileContentMap[file]
		if _, err := os.Stat(file); err != nil && errors.Is(err, os.ErrNotExist) {
			if err := createFile(file, content); err != nil {
				return created, err
			}
			created = append(created, file)
		}
	}
	return created, nil
}

func createFile(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), ggconst.FileModeGenerated)
}

// templateConfig renders the config.ini.example of a new project whose
// application is named appName.
func templateConfig(appName string) string {
	return fmt.Sprintf(`[app]
name = %s
description = A Go application built with gst framework

[server]
; The operational endpoints — /-/healthz, /-/readyz, /metrics, /openapi.json
; and /docs — are served on this port without authentication, by design: their
; readers hold no account here. Keep them off your public address. Under Kubernetes, route only the /api prefix from the
; Ingress and let the metrics scraper reach the pod port directly.
mode = dev
listen =
port = 8080

[auth]
; Required before mounting middleware.JwtAuth: every token this application
; issues is signed with this key and every token it accepts is verified against
; it. It has no default, because a key shipped with the framework is a key every
; deployment shares and anyone holding the framework source can compute.
jwt_secret =

[database]
type = sqlite
auto_migrate = true

[sqlite]
path = ./data.db
database = main
is_memory = true
enabled = true

[mysql]
host = 127.0.0.1
port = 3306
database =
username = root
password =
enabled = true

[postgres]
host = 127.0.0.1
port = 5432
database =
username = postgres
password =
sslmode = disable
enabled = true

[redis]
enabled = false
addr = 127.0.0.1:6379
db = 0
password =
namespace = %s
`, appName, appName)
}
