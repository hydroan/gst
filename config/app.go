package config

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hydroan/gst/types/consts"
)

const (
	APP_NAME        = "APP_NAME"        //nolint:staticcheck
	APP_VERSION     = "APP_VERSION"     //nolint:staticcheck
	APP_DESCRIPTION = "APP_DESCRIPTION" //nolint:staticcheck
	APP_AUTHOR      = "APP_AUTHOR"      //nolint:staticcheck
	APP_EMAIL       = "APP_EMAIL"       //nolint:staticcheck
	APP_HOMEPAGE    = "APP_HOMEPAGE"    //nolint:staticcheck
	APP_LICENSE     = "APP_LICENSE"     //nolint:staticcheck
	APP_BUILD_TIME  = "APP_BUILD_TIME"  //nolint:staticcheck
	APP_GIT_COMMIT  = "APP_GIT_COMMIT"  //nolint:staticcheck
	APP_GIT_BRANCH  = "APP_GIT_BRANCH"  //nolint:staticcheck
	APP_GO_VERSION  = "APP_GO_VERSION"  //nolint:staticcheck
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
func (a *AppInfo) setDefault() {
	cv.SetDefault("app.name", consts.FrameworkName)
	cv.SetDefault("app.version", "")
	cv.SetDefault("app.description", fmt.Sprintf("A Go application built with %s framework", consts.FrameworkName))
	cv.SetDefault("app.license", "MIT")
	cv.SetDefault("app.go_version", runtime.Version())
	cv.SetDefault("app.platform", runtime.GOOS+"/"+runtime.GOARCH)
	cv.SetDefault("app.compiler", runtime.Compiler)

	// Try to get build info from runtime
	a.setBuildInfo()
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
