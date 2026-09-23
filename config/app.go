package config

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hydroan/gst/consts"

	"github.com/spf13/viper"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
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

	// Build and runtime information. BuildTime is the time the configuration
	// names, or else the time of the commit Go recorded in the binary.
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

	// What the build information Go records in the binary supplies becomes the
	// default of its key, so a file or an environment variable still overrides
	// it and nothing else has to know where it came from. The git branch is not
	// among it: a deployment that reports one sets it through APP_GIT_BRANCH or
	// the configuration file.
	if info, ok := debug.ReadBuildInfo(); ok {
		a.setBuildInfo(info)
	}
	if !a.BuildTime.IsZero() {
		v.SetDefault("app.build_time", a.BuildTime)
	}
	for key, value := range map[string]string{
		"app.version":        a.Version,
		"app.git_commit":     a.GitCommit,
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

// setBuildInfo fills in what the build information Go records in the binary
// supplies: the commit, its time, whether the tree had local changes, the
// build tags and the main module version.
func (a *AppInfo) setBuildInfo(buildInfo *debug.BuildInfo) {
	for _, setting := range buildInfo.Settings {
		switch setting.Key {
		case "vcs.revision":
			a.GitCommit = setting.Value
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
				a.BuildTime = t
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

	// The version is the one Go records for the main module: a tag, or a
	// pseudo-version for a commit no tag names, "+dirty" marking local changes
	// either way. Only a tag names the tag. A build outside version control
	// records "(devel)", which is no version.
	if moduleVersion := buildInfo.Main.Version; moduleVersion != "(devel)" && moduleVersion != "" {
		a.Version = moduleVersion
		if !module.IsPseudoVersion(moduleVersion) {
			a.GitTag = semver.Canonical(moduleVersion)
		}
	}
}
