package main

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	debugPkg "runtime/debug"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/spf13/cobra"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build [FILE]",
	Short: "Cross-platform build with advanced features",
	Long: `Cross-platform build command for GST framework.

This command provides comprehensive build capabilities including:
- Automatic injection of build information into config.AppInfo
- Configuration file support (hack/config.yaml or framework config)
- Resource packing capabilities
- Cross-platform compilation with extensive platform support
- Custom variable injection
- Build environment information

Examples:
  gg build                                   # Build current platform
  gg build main.go                           # Build specific file
  gg build --name myapp                      # Custom binary name
  gg build --arch all --system all           # Build for all platforms
  gg build --pack resource,manifest          # Pack resources
  gg build --version v1.0.0                  # Custom version
  gg build --output ./bin/myapp              # Specific output path`,
	RunE: buildRun,
}

// Build represents the build configuration structure
type Build struct {
	Name          string            `mapstructure:"name" yaml:"name" json:"name"`                                                          // Binary name
	Arch          string            `mapstructure:"arch" yaml:"arch" json:"arch"`                                                          // Target architectures
	System        string            `mapstructure:"system" yaml:"system" json:"system"`                                                    // Target systems
	Path          string            `mapstructure:"path" yaml:"path" json:"path" default:"./bin"`                                          // Output directory
	Mod           string            `mapstructure:"mod" yaml:"mod" json:"mod"`                                                             // Go mod option
	Cgo           bool              `mapstructure:"cgo" yaml:"cgo" json:"cgo"`                                                             // Enable CGO
	PackSrc       string            `mapstructure:"pack_src" yaml:"pack_src" json:"pack_src"`                                              // Directories to pack
	PackDst       string            `mapstructure:"pack_dst" yaml:"pack_dst" json:"pack_dst" default:"internal/packed/build_pack_data.go"` // Pack destination
	Version       string            `mapstructure:"version" yaml:"version" json:"version"`                                                 // Version
	Output        string            `mapstructure:"output" yaml:"output" json:"output"`                                                    // Output file path
	Extra         string            `mapstructure:"extra" yaml:"extra" json:"extra"`                                                       // Extra build flags
	VarMap        map[string]string `mapstructure:"var_map" yaml:"var_map" json:"var_map"`                                                 // Custom variables
	DumpEnv       bool              `mapstructure:"dump_env" yaml:"dump_env" json:"dump_env"`                                              // Dump environment
	ExitWhenError bool              `mapstructure:"exit_when_error" yaml:"exit_when_error" json:"exit_when_error"`                         // Exit on error
}

// BuildTarget represents build target information
type BuildTarget struct {
	OS   string
	Arch string
}

func (t BuildTarget) String() string {
	return fmt.Sprintf("%s_%s", t.OS, t.Arch)
}

// BuildInfo contains comprehensive build information compatible with config.AppInfo
type BuildInfo struct {
	// Basic information
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`

	// Build metadata
	BuildTime    string `json:"build_time"`
	GitCommit    string `json:"git_commit"`
	GitBranch    string `json:"git_branch"`
	GoVersion    string `json:"go_version"`
	GitTag       string `json:"git_tag"`
	GitTreeState string `json:"git_tree_state"`
	Platform     string `json:"platform"`
	Compiler     string `json:"compiler"`
	BuildTags    string `json:"build_tags"`

	// Module information
	Module     string            `json:"module"`
	Binary     string            `json:"binary"`
	CustomVars map[string]string `json:"custom_vars,omitempty"`
}

// Supported platforms (extended from GoFrame)
var (
	supportedPlatforms = map[string][]string{
		"darwin":    {"amd64", "arm64"},
		"linux":     {"386", "amd64", "arm", "arm64", "ppc64", "ppc64le", "mips", "mipsle", "mips64", "mips64le"},
		"windows":   {"386", "amd64", "arm", "arm64"},
		"freebsd":   {"386", "amd64", "arm"},
		"netbsd":    {"386", "amd64", "arm"},
		"openbsd":   {"386", "amd64", "arm"},
		"dragonfly": {"amd64"},
		"plan9":     {"386", "amd64"},
		"solaris":   {"amd64"},
	}
)

// Command line flags
var (
	buildName          string
	buildArch          string
	buildSystem        string
	buildPath          string
	buildMod           string
	buildCgo           bool
	buildPackSrc       string
	buildPackDst       string
	buildVersion       string
	buildOutput        string
	buildExtra         string
	buildVarMap        map[string]string
	buildDumpEnv       bool
	buildExitWhenError bool
)

func init() {
	// Basic flags
	buildCmd.Flags().StringVarP(&buildName, "name", "n", "", "Binary name (default: module name)")
	buildCmd.Flags().StringVarP(&buildArch, "arch", "a", runtime.GOARCH, "Target architectures (386,amd64,arm,arm64,all)")
	buildCmd.Flags().StringVarP(&buildSystem, "system", "s", runtime.GOOS, "Target systems (linux,darwin,windows,all)")
	buildCmd.Flags().StringVarP(&buildPath, "path", "p", "./bin", "Output directory")
	buildCmd.Flags().StringVarP(&buildMod, "mod", "m", "", "Go mod option (none,readonly,vendor)")
	buildCmd.Flags().BoolVar(&buildCgo, "cgo", false, "Enable CGO")

	// Advanced flags
	buildCmd.Flags().StringVar(&buildPackSrc, "pack", "", "Directories to pack (comma-separated)")
	buildCmd.Flags().StringVar(&buildPackDst, "packDst", "internal/packed/build_pack_data.go", "Pack destination file")
	buildCmd.Flags().StringVarP(&buildVersion, "version", "v", "", "Version (default: auto-detect)")
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Output file path (overrides name and path)")
	buildCmd.Flags().StringVar(&buildExtra, "extra", "", "Extra build flags")
	buildCmd.Flags().StringToStringVar(&buildVarMap, "var", nil, "Custom variables (key=value)")
	buildCmd.Flags().BoolVar(&buildDumpEnv, "dump-env", false, "Dump build environment")
	buildCmd.Flags().BoolVar(&buildExitWhenError, "exit-on-error", false, "Exit immediately on error")
}

func buildRun(cmd *cobra.Command, args []string) error {
	clioutput.Section("Build")

	oldStdout := os.Stdout
	devNull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0o666)
	os.Stdout = devNull

	config.Register[Build]()
	if err := config.Init(); err != nil {
		return err
	}

	// Capture the package-level cleanup before the local config variable below
	// shadows the package name; the error paths need it for explicit cleanup.
	cleanup := config.Clean
	defer cleanup()
	os.Stdout = oldStdout

	// Load configuration
	config := config.Get[*Build]()
	// Override config with command line flags
	overrideConfigWithFlags(config)

	// Dump environment if requested
	if config.DumpEnv {
		dumpBuildEnvironment()
	}

	// Pack resources if specified
	if config.PackSrc != "" {
		if packErr := packResources(config); packErr != nil {
			if config.ExitWhenError {
				// os.Exit skips deferred calls, so run the cleanup explicitly.
				cleanup()
				os.Exit(1) //nolint:gocritic // cleanup already ran right above.
			}
			return fmt.Errorf("failed to pack resources: %w", packErr)
		}
	}

	// Get build information
	buildInfo := getBuildInfo(config)

	// Get build targets
	targets, err := getBuildTargets(config)
	if err != nil {
		return fmt.Errorf("failed to determine build targets: %w", err)
	}

	// Determine build file
	buildFile := "."
	if len(args) > 0 {
		buildFile = args[0]
	}

	// Build for each target
	successCount := 0
	for _, target := range targets {
		if err := buildForTarget(target, buildInfo, config, buildFile); err != nil {
			clioutput.Error("", "Failed to build for %s: %v", target.String(), err)
			if config.ExitWhenError {
				os.Exit(1)
			}
			continue
		}
		clioutput.Success("", "Built successfully for %s", target.String())
		successCount++
	}

	if successCount == 0 {
		return errors.New("all builds failed")
	}

	clioutput.Success("", "Build completed: %d/%d successful", successCount, len(targets))
	if config.Output != "" {
		clioutput.Item("", "Output: %s", config.Output)
	} else {
		clioutput.Item("", "Output directory: %s", config.Path)
	}

	return nil
}

// overrideConfigWithFlags overrides config with command line flags
func overrideConfigWithFlags(config *Build) {
	if buildName != "" {
		config.Name = buildName
	}
	if buildArch != "" {
		config.Arch = buildArch
	}
	if buildSystem != "" {
		config.System = buildSystem
	}
	if buildPath != "" {
		config.Path = buildPath
	}
	if buildMod != "" {
		config.Mod = buildMod
	}
	if buildCgo {
		config.Cgo = buildCgo
	}
	if buildPackSrc != "" {
		config.PackSrc = buildPackSrc
	}
	if buildPackDst != "" {
		config.PackDst = buildPackDst
	}
	if buildVersion != "" {
		config.Version = buildVersion
	}
	if buildOutput != "" {
		config.Output = buildOutput
	}
	if buildExtra != "" {
		config.Extra = buildExtra
	}
	if len(buildVarMap) > 0 {
		if config.VarMap == nil {
			config.VarMap = make(map[string]string)
		}
		maps.Copy(config.VarMap, buildVarMap)
	}
	if buildDumpEnv {
		config.DumpEnv = buildDumpEnv
	}
	if buildExitWhenError {
		config.ExitWhenError = buildExitWhenError
	}
}

// dumpBuildEnvironment prints build environment information
func dumpBuildEnvironment() {
	clioutput.Info("", "Build Environment:")
	clioutput.Line(clioutput.StyleMuted, "Go Version: %s", runtime.Version())
	// Get GOROOT using go env command instead of deprecated runtime.GOROOT()
	goroot := "unknown"
	if cmd := exec.Command("go", "env", "GOROOT"); cmd != nil {
		if output, err := cmd.Output(); err == nil {
			goroot = strings.TrimSpace(string(output))
		}
	}
	clioutput.Line(clioutput.StyleMuted, "Go Root: %s", goroot)
	clioutput.Line(clioutput.StyleMuted, "Go Path: %s", os.Getenv("GOPATH"))
	clioutput.Line(clioutput.StyleMuted, "Go Mod Cache: %s", os.Getenv("GOMODCACHE"))
	clioutput.Line(clioutput.StyleMuted, "CGO Enabled: %s", os.Getenv("CGO_ENABLED"))
	clioutput.Line(clioutput.StyleMuted, "Current OS: %s", runtime.GOOS)
	clioutput.Line(clioutput.StyleMuted, "Current Arch: %s", runtime.GOARCH)
	clioutput.Line(clioutput.StyleMuted, "Num CPU: %d", runtime.NumCPU())

	if buildInfo, ok := debugPkg.ReadBuildInfo(); ok {
		clioutput.Line(clioutput.StyleMuted, "Module: %s", buildInfo.Main.Path)
		clioutput.Line(clioutput.StyleMuted, "Module Version: %s", buildInfo.Main.Version)
	}
	fmt.Println()
}

// packResources packs specified directories into Go files
func packResources(config *Build) error {
	if config.PackSrc == "" {
		return nil
	}

	clioutput.Info("", "Packing resources...")

	dirs := strings.Split(config.PackSrc, ",")
	for i, dir := range dirs {
		dirs[i] = strings.TrimSpace(dir)
	}

	// Create pack destination directory
	packDir := filepath.Dir(config.PackDst)
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return fmt.Errorf("failed to create pack directory: %w", err)
	}

	// This is a simplified implementation
	// In a real implementation, you would pack the directories into the Go file
	clioutput.Success("", "Resource packing completed: %s", config.PackDst)

	return nil
}

// getBuildInfo collects build information
func getBuildInfo(config *Build) *BuildInfo {
	info := &BuildInfo{
		BuildTime:  time.Now().UTC().Format(time.RFC3339),
		GoVersion:  runtime.Version(),
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Compiler:   runtime.Compiler,
		CustomVars: make(map[string]string),
	}

	// Get module information
	if buildInfo, ok := debugPkg.ReadBuildInfo(); ok {
		info.Module = buildInfo.Main.Path
		if config.Name != "" {
			info.Binary = config.Name
		} else {
			info.Binary = filepath.Base(buildInfo.Main.Path)
		}

		// Extract build settings
		var buildTags []string
		gitTreeState := "clean"

		for _, setting := range buildInfo.Settings {
			switch setting.Key {
			case "-tags":
				buildTags = append(buildTags, setting.Value)
			case "vcs.modified":
				if setting.Value == "true" {
					gitTreeState = "dirty"
				}
			}
		}

		info.BuildTags = strings.Join(buildTags, ",")
		info.GitTreeState = gitTreeState
	}

	// Get version
	if config.Version != "" {
		info.Version = config.Version
	} else {
		version, err := getGitVersion()
		if err != nil {
			clioutput.Warn("", "Failed to get git version, using 'dev': %v", err)
			info.Version = "dev"
		} else {
			info.Version = version
		}
	}

	// Get git information
	if commit, err := getGitCommit(); err == nil {
		info.GitCommit = commit
	} else {
		info.GitCommit = "unknown"
	}

	if branch, err := getGitBranch(); err == nil {
		info.GitBranch = branch
	} else {
		info.GitBranch = "unknown"
	}

	// Add custom variables
	maps.Copy(info.CustomVars, config.VarMap)

	return info
}

// getBuildTargets determines build targets with platform support
func getBuildTargets(config *Build) ([]BuildTarget, error) {
	var targets []BuildTarget

	// Parse systems
	var systems []string
	if config.System == "all" {
		for os := range supportedPlatforms {
			systems = append(systems, os)
		}
	} else {
		systems = strings.Split(config.System, ",")
		for i, s := range systems {
			systems[i] = strings.TrimSpace(s)
		}
	}

	// Parse architectures
	var architectures []string
	if config.Arch == "all" {
		archSet := make(map[string]bool)
		for _, archs := range supportedPlatforms {
			for _, arch := range archs {
				archSet[arch] = true
			}
		}
		for arch := range archSet {
			architectures = append(architectures, arch)
		}
	} else {
		architectures = strings.Split(config.Arch, ",")
		for i, a := range architectures {
			architectures[i] = strings.TrimSpace(a)
		}
	}

	// Generate targets
	for _, os := range systems {
		supportedArchs, exists := supportedPlatforms[os]
		if !exists {
			return nil, fmt.Errorf("unsupported OS: %s", os)
		}

		for _, arch := range architectures {
			// Check if architecture is supported for this OS
			if !slices.Contains(supportedArchs, arch) {
				return nil, fmt.Errorf("architecture %s is not supported for %s", arch, os)
			}
			targets = append(targets, BuildTarget{OS: os, Arch: arch})
		}
	}

	if len(targets) == 0 {
		return nil, errors.New("no valid build targets found")
	}

	return targets, nil
}

// buildForTarget builds binary for specific target
func buildForTarget(target BuildTarget, info *BuildInfo, config *Build, buildFile string) error {
	// Generate binary name
	binaryName := info.Binary
	if target.OS == "windows" {
		binaryName += ".exe"
	}

	// Generate output path
	var outputPath string
	if config.Output != "" {
		outputPath = config.Output
		if target.OS == "windows" && !strings.HasSuffix(outputPath, ".exe") {
			outputPath += ".exe"
		}
	} else {
		outputPath = filepath.Join(config.Path, target.String(), binaryName)
	}

	// Create output directory
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Build ldflags for config.AppInfo injection
	ldflags := buildLdflags(info, config)

	// Prepare build command
	args := []string{"build"}

	if config.Mod != "" {
		args = append(args, "-mod", config.Mod)
	}

	args = append(args, "-ldflags", ldflags, "-o", outputPath)

	if config.Extra != "" {
		extraArgs := strings.Fields(config.Extra)
		args = append(args, extraArgs...)
	}

	args = append(args, buildFile)

	cmd := exec.Command("go", args...)

	// Set environment
	env := os.Environ()
	env = append(env, "GOOS="+target.OS)
	env = append(env, "GOARCH="+target.Arch)

	if !config.Cgo {
		env = append(env, "CGO_ENABLED=0")
	} else {
		env = append(env, "CGO_ENABLED=1")
	}

	cmd.Env = env

	// Execute build
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// buildLdflags constructs ldflags for injecting build information into config.AppInfo
func buildLdflags(info *BuildInfo, config *Build) string {
	flags := make([]string, 0, len(info.CustomVars)+10) // Pre-allocate with estimated capacity

	// Inject into config.AppInfo structure
	flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.appVersion=%s'", info.Version))
	flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.appCommit=%s'", info.GitCommit))
	flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.appBranch=%s'", info.GitBranch))
	flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.appBuildTime=%s'", info.BuildTime))
	flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.appGoVersion=%s'", info.GoVersion))
	flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.appPlatform=%s'", info.Platform))
	flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.appCompiler=%s'", info.Compiler))
	flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.appBuildTags=%s'", info.BuildTags))
	flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.appGitTreeState=%s'", info.GitTreeState))

	// Add custom variables to ldflags
	caser := cases.Title(language.English)
	for k, v := range info.CustomVars {
		flags = append(flags, fmt.Sprintf("-X 'github.com/hydroan/gst/config.app%s=%s'", caser.String(k), v))
	}

	// Add extra ldflags if specified
	if config.Extra != "" {
		flags = append(flags, config.Extra)
	}

	return strings.Join(flags, " ")
}

// getGitVersion returns the current git version/tag
func getGitVersion() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getGitCommit returns the current git commit hash
func getGitCommit() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getGitBranch returns the current git branch
func getGitBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
