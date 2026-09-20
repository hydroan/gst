package config

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hydroan/gst/consts"

	"github.com/spf13/viper"
)

const (
	APP_NAME        = "APP_NAME"
	APP_VERSION     = "APP_VERSION"
	APP_DESCRIPTION = "APP_DESCRIPTION"
	APP_AUTHOR      = "APP_AUTHOR"
	APP_EMAIL       = "APP_EMAIL"
	APP_HOMEPAGE    = "APP_HOMEPAGE"
	APP_LICENSE     = "APP_LICENSE"
	APP_BUILD_TIME  = "APP_BUILD_TIME"
	APP_GIT_COMMIT  = "APP_GIT_COMMIT"
	APP_GIT_BRANCH  = "APP_GIT_BRANCH"
	APP_GO_VERSION  = "APP_GO_VERSION"
)

// AppInfo represents application metadata and build information
// This struct contains essential project information that can be used
// for version reporting, monitoring, and application identification
type AppInfo struct {
	// Basic application information
	Name        string `json:"name" mapstructure:"name" ini:"name" yaml:"name"`
	Version     string `json:"version" mapstructure:"version" ini:"version" yaml:"version"`
	Description string `json:"description" mapstructure:"description" ini:"description" yaml:"description"`
	Author      string `json:"author" mapstructure:"author" ini:"author" yaml:"author"`
	Email       string `json:"email" mapstructure:"email" ini:"email" yaml:"email"`
	Homepage    string `json:"homepage" mapstructure:"homepage" ini:"homepage" yaml:"homepage"`
	License     string `json:"license" mapstructure:"license" ini:"license" yaml:"license"`

	// Build and runtime information
	BuildTime    time.Time `json:"build_time" mapstructure:"build_time" ini:"build_time" yaml:"build_time"`
	GitCommit    string    `json:"git_commit" mapstructure:"git_commit" ini:"git_commit" yaml:"git_commit"`
	GitBranch    string    `json:"git_branch" mapstructure:"git_branch" ini:"git_branch" yaml:"git_branch"`
	GoVersion    string    `json:"go_version" mapstructure:"go_version" ini:"go_version" yaml:"go_version"`
	GitTag       string    `json:"git_tag" mapstructure:"git_tag" ini:"git_tag" yaml:"git_tag"`
	GitTreeState string    `json:"git_tree_state" mapstructure:"git_tree_state" ini:"git_tree_state" yaml:"git_tree_state"`
	Platform     string    `json:"platform" mapstructure:"platform" ini:"platform" yaml:"platform"`
	Compiler     string    `json:"compiler" mapstructure:"compiler" ini:"compiler" yaml:"compiler"`
	BuildTags    []string  `json:"build_tags" mapstructure:"build_tags" ini:"build_tags" yaml:"build_tags"`
}

// setDefault sets default values for AppInfo configuration
func (a *AppInfo) setDefault(v *viper.Viper) {
	v.SetDefault("app.name", consts.FrameworkName)
	v.SetDefault("app.version", "")
	v.SetDefault("app.description", fmt.Sprintf("A Go application built with %s framework", consts.FrameworkName))
	v.SetDefault("app.license", "MIT")
	v.SetDefault("app.go_version", runtime.Version())
	v.SetDefault("app.platform", runtime.GOOS+"/"+runtime.GOARCH)
	v.SetDefault("app.compiler", runtime.Compiler)

	// What the build linked in first, then what the module records for
	// whatever the build did not carry.
	a.setLinkedBuildInfo()
	a.setBuildInfo()

	// The linked values are the defaults of their keys, so a file or an
	// environment variable still overrides them and nothing else has to know
	// where they came from.
	if !a.BuildTime.IsZero() {
		v.SetDefault("app.build_time", a.BuildTime)
	}
	for key, value := range map[string]string{
		"app.version":        a.Version,
		"app.git_commit":     a.GitCommit,
		"app.git_branch":     a.GitBranch,
		"app.go_version":     a.GoVersion,
		"app.platform":       a.Platform,
		"app.compiler":       a.Compiler,
		"app.git_tree_state": a.GitTreeState,
	} {
		if value != "" {
			v.SetDefault(key, value)
		}
	}
	if len(a.BuildTags) > 0 {
		v.SetDefault("app.build_tags", a.BuildTags)
	}
}

// The build information a build links in. gg build sets them with the
// linker's -X, which only writes string variables, so they are strings here
// and parsed into the typed fields below. A build without gg leaves them
// empty and the values come from runtime/debug instead.
var (
	appVersion      string
	appCommit       string
	appBranch       string
	appBuildTime    string
	appGoVersion    string
	appPlatform     string
	appCompiler     string
	appBuildTags    string
	appGitTreeState string
)

// setLinkedBuildInfo applies what the build linked in. It runs before the
// runtime/debug fallback and before the configuration is read, so a value the
// build carries wins over what the module records, and a configuration file
// or an environment variable still wins over both.
func (a *AppInfo) setLinkedBuildInfo() {
	if appVersion != "" {
		a.Version = appVersion
	}
	if appCommit != "" {
		a.GitCommit = appCommit
	}
	if appBranch != "" {
		a.GitBranch = appBranch
	}
	if appBuildTime != "" {
		if t, err := time.Parse(time.RFC3339, appBuildTime); err == nil {
			a.BuildTime = t
		}
	}
	if appGoVersion != "" {
		a.GoVersion = appGoVersion
	}
	if appPlatform != "" {
		a.Platform = appPlatform
	}
	if appCompiler != "" {
		a.Compiler = appCompiler
	}
	if appBuildTags != "" {
		a.BuildTags = strings.Split(appBuildTags, ",")
	}
	if appGitTreeState != "" {
		a.GitTreeState = appGitTreeState
	}
}

// setBuildInfo attempts to extract build information from runtime/debug
func (a *AppInfo) setBuildInfo() {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	// Set platform and compiler information
	a.Platform = runtime.GOOS + "/" + runtime.GOARCH
	a.Compiler = runtime.Compiler

	// Extract version control information from build settings
	for _, setting := range buildInfo.Settings {
		switch setting.Key {
		case "vcs.revision":
			if a.GitCommit == "" {
				a.GitCommit = setting.Value
			}
		case "vcs.time":
			if a.BuildTime.IsZero() {
				if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
					a.BuildTime = t
				}
			}
		case "vcs.modified":
			if setting.Value == "true" {
				a.GitTreeState = "dirty"
			} else {
				a.GitTreeState = "clean"
			}
		case "-tags":
			// Extract build tags if available
			if setting.Value != "" {
				// Split tags by comma and clean up
				tags := make([]string, 0)
				for tag := range strings.SplitSeq(setting.Value, ",") {
					if trimmed := strings.TrimSpace(tag); trimmed != "" {
						tags = append(tags, trimmed)
					}
				}
				a.BuildTags = tags
			}
		}
	}

	// Use module version if available and no custom version is set
	if a.Version == "dev" && buildInfo.Main.Version != "(devel)" && buildInfo.Main.Version != "" {
		a.Version = buildInfo.Main.Version
		a.GitTag = buildInfo.Main.Version
	}
}
