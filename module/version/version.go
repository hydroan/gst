package versionmod

import (
	"runtime"
	"time"

	"github.com/hydroan/gst/config"
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var startTime = time.Now()

var _ types.Module[*Version, *Version, *VersionRsp] = (*VersionModule)(nil)

type VersionModule struct{}

func (*VersionModule) Service() types.Service[*Version, *Version, *VersionRsp] {
	return &VersionService{}
}

func (*VersionModule) Route() string { return "version" }
func (*VersionModule) Param() string { return "" }
func (*VersionModule) Pub() bool     { return true }

// Version represents the backend version information for frontend update detection
type Version struct {
	model.Empty
}

// Design defines the API routes for version checking
func (Version) Design() {
	Route("version", func() {
		List(func() {
			Enabled(true)
			Service(true)
			Public(true) // Allow public access for version checking
			Result[*VersionRsp]()
		})
	})
}

// VersionRsp contains version information returned to frontend
type VersionRsp struct {
	Version     string `json:"version"`     // Backend version string (semantic version)
	BuildTime   int64  `json:"build_time"`  // Build timestamp (Unix timestamp)
	GitCommit   string `json:"git_commit"`  // Git commit hash (short hash)
	GitBranch   string `json:"git_branch"`  // Git branch name
	GoVersion   string `json:"go_version"`  // Go compiler version
	Environment string `json:"environment"` // Environment (dev/staging/prod)
	Uptime      int64  `json:"uptime"`      // Server uptime in seconds
	Timestamp   int64  `json:"timestamp"`   // Current server timestamp
}

type VersionService struct {
	service.Base[*Version, *Version, *VersionRsp]
}

// List returns version information including build details and runtime info
func (l *VersionService) List(ctx *types.ServiceContext, req *Version) (rsp *VersionRsp, err error) {
	appInfo := config.App.AppInfo

	// Calculate uptime in seconds
	uptime := int64(time.Since(startTime).Seconds())

	version := appInfo.Version
	if len(version) == 0 {
		version = appInfo.GitCommit
	}

	return &VersionRsp{
		Version:     version,
		BuildTime:   startTime.Unix(),
		GitCommit:   appInfo.GitCommit,
		GitBranch:   appInfo.GitBranch,
		GoVersion:   runtime.Version(),
		Environment: string(config.App.Server.Mode),
		Uptime:      uptime,
		Timestamp:   time.Now().Unix(),
	}, nil
}
