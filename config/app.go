package config

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hydroan/gst/internal/i18n"
	"github.com/hydroan/gst/internal/types"
	"github.com/spf13/viper"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

const (
	APP_NAME        = "APP_NAME"
	APP_VERSION     = "APP_VERSION"
	APP_DESCRIPTION = "APP_DESCRIPTION"
	APP_LANGUAGE    = "APP_LANGUAGE"
	APP_BUILD_TIME  = "APP_BUILD_TIME"
	APP_GIT_COMMIT  = "APP_GIT_COMMIT"
	APP_GIT_BRANCH  = "APP_GIT_BRANCH"
)

// AppInfo represents application metadata and build information
// This struct contains essential project information that can be used
// for version reporting, monitoring, and application identification
type AppInfo struct {
	// Basic application information
	Name        string `json:"name" mapstructure:"name" ini:"name" yaml:"name"`
	Version     string `json:"version" mapstructure:"version" ini:"version" yaml:"version"`
	Description string `json:"description" mapstructure:"description" ini:"description" yaml:"description"`

	// Language is the language the process answers its clients in, a BCP 47
	// language tag with a region or a script or without: zh, zh-CN and
	// zh-Hans name Chinese, zh-TW, zh-HK and zh-Hant name Chinese in the
	// traditional script, en-US names English. The sentences a client is
	// answered with that a translation into it is registered for, the
	// framework's own refusals and the descriptions of the fields a request
	// is refused on among them, are answered in it; any other sentence is
	// answered as it is written. It is English by default, the language the
	// sentences are written in, and a tag naming a language the framework
	// does not speak, English and Chinese in either script being the ones it
	// does, fails the boot.
	Language string `json:"language" mapstructure:"language" ini:"language" yaml:"language"`

	// Build and runtime information. BuildTime is the time the configuration
	// names, or else the time of the commit Go recorded in the binary.
	BuildTime    time.Time `json:"build_time" mapstructure:"build_time" ini:"build_time" yaml:"build_time"`
	GitCommit    string    `json:"git_commit" mapstructure:"git_commit" ini:"git_commit" yaml:"git_commit"`
	GitBranch    string    `json:"git_branch" mapstructure:"git_branch" ini:"git_branch" yaml:"git_branch"`
	GitTag       string    `json:"git_tag" mapstructure:"git_tag" ini:"git_tag" yaml:"git_tag"`
	GitTreeState string    `json:"git_tree_state" mapstructure:"git_tree_state" ini:"git_tree_state" yaml:"git_tree_state"`
	BuildTags    []string  `json:"build_tags" mapstructure:"build_tags" ini:"build_tags" yaml:"build_tags"`
}

// setDefaults sets default values for AppInfo configuration
func (a *AppInfo) setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", types.FrameworkName)
	v.SetDefault("app.version", "")
	v.SetDefault("app.description", fmt.Sprintf("A Go application built with %s framework", types.FrameworkName))
	v.SetDefault("app.language", i18n.English)

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
