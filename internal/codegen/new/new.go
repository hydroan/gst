//nolint:predeclared
package new

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"

	"github.com/cockroachdb/errors"
)

// scaffoldFiles returns, by path, the files a project holds so that every
// package a generated main.go imports exists: the first versions of the
// packages the project maintains, and the first versions of the files gg gen
// owns, which carry the generated suffix. A project must compile before its
// first generation, so the generators build those with nothing to register
// and gg gen later overwrites the same files; built by the same code, the
// scaffold and the generated version cannot drift.
func scaffoldFiles() (map[string]string, error) {
	files := map[string]string{
		"component/component.go":   componentContent,
		"configx/configx.go":       configxContent,
		"cronjob/cronjob.go":       cronjobContent,
		"leader/leader.go":         leaderContent,
		"lock/lock.go":             lockContent,
		"middleware/middleware.go": middlewareContent,
		"module/module.go":         moduleContent,
		"dao/.gitkeep":             "",
		"provider/.gitkeep":        "",
	}
	var err error
	if files["model/"+ggconst.FileModelGen], err = gen.BuildModelFile(ggconst.PkgModel, nil); err != nil {
		return nil, errors.Wrap(err, "build model/model.gen.go")
	}
	if files["model/"+ggconst.FileAPIDocGen], err = gen.BuildAPIDocFile(ggconst.PkgModel, gen.APIDocEntries{}); err != nil {
		return nil, errors.Wrap(err, "build model/apidoc.gen.go")
	}
	if files["service/"+ggconst.FileServiceGen], err = gen.BuildServiceFile(ggconst.PkgService, nil); err != nil {
		return nil, errors.Wrap(err, "build service/service.gen.go")
	}
	if files["router/"+ggconst.FileRouterGen], err = gen.BuildRouterFile(ggconst.PkgRouter, "", nil); err != nil {
		return nil, errors.Wrap(err, "build router/router.gen.go")
	}
	return files, nil
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
	scaffold, err := scaffoldFiles()
	if err != nil {
		return nil, err
	}
	// The lint configuration is the project's to tune, so gg gen never
	// restores it: it is a project file, not part of the scaffold.
	scaffold[".golangci.yml"] = golangciLintContent
	paths := slices.Sorted(maps.Keys(scaffold))
	files := make([]ProjectFile, 0, len(paths)+3)
	for _, path := range paths {
		files = append(files, ProjectFile{Path: path, Content: scaffold[path]})
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

// EnsureFileExists writes every scaffold file missing from the project in the
// working directory, among them the empty first versions of the files gg gen
// owns, and returns the paths it created, sorted.
func EnsureFileExists() ([]string, error) {
	scaffold, err := scaffoldFiles()
	if err != nil {
		return nil, err
	}

	var created []string
	for _, file := range slices.Sorted(maps.Keys(scaffold)) {
		if gghelper.FileExists(file) {
			continue
		}
		if err := gghelper.EnsureParentDir(file); err != nil {
			return created, err
		}
		if err := os.WriteFile(file, []byte(scaffold[file]), ggconst.FileModeGenerated); err != nil {
			return created, err
		}
		created = append(created, file)
	}
	return created, nil
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
