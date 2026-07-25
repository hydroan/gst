package constants

// Import paths
const (
	// Framework import paths
	//nolint:godoclint
	ImportPathGst       = "github.com/hydroan/gst"
	ImportPathModel     = "github.com/hydroan/gst/model"
	ImportPathService   = "github.com/hydroan/gst/service"
	ImportPathRouter    = "github.com/hydroan/gst/router"
	ImportPathTypes     = "github.com/hydroan/gst/types"
	ImportPathConsts    = "github.com/hydroan/gst/types/consts"
	ImportPathBootstrap = "github.com/hydroan/gst/bootstrap"
	ImportPathUtil      = "github.com/hydroan/gst/util"
	ImportPathAPIDoc    = "github.com/hydroan/gst/apidoc"

	// ModelPackagePath is the package path for comparison
	ModelPackagePath = `"github.com/hydroan/gst/model"`
)

// File patterns and extensions
const (
	ExtensionGo     = ".go"
	PatternTestFile = "_test.go"
	PrefixIgnore    = "_"
)

// FileModeGenerated is the permission gg writes project files with.
//
// Generated sources are ordinary source files and must be as readable as the
// hand-written ones next to them: a stricter mode leaves the package with
// mixed permissions and breaks any reader that is not the generating user,
// such as a CI job or a container build running as another account. The
// process umask still tightens this as configured.
const FileModeGenerated = 0o644

// Generated file names.
//
// SuffixGenGo marks every file gg generates and fully owns. A file carrying
// this suffix is framework-managed: gg rewrites it, and deletes it once the
// source it was generated from is gone. Project code never has to clean up
// after the generator.
const (
	SuffixGenGo = ".gen.go"

	FileModelGen   = "model" + SuffixGenGo
	FileAPIDocGen  = "apidoc" + SuffixGenGo
	FileServiceGen = "service" + SuffixGenGo
	FileRouterGen  = "router" + SuffixGenGo
	// FileMain keeps its conventional name: main.go is the entry point every
	// Go toolchain and IDE expects.
	FileMain = "main.go"
)

// Directory names
const (
	DirVendor   = "vendor"
	DirTestData = "testdata"
	DirModel    = "model"
	DirService  = "service"
	DirRouter   = "router"
)

// Package names
const (
	PkgMain      = "main"
	PkgModel     = "model"
	PkgService   = "service"
	PkgRouter    = "router"
	PkgModule    = "module"
	PkgBootstrap = "bootstrap"
)

// Model field names
const (
	FieldBase     = "Base"
	FieldAutoBase = "AutoBase"
	FieldEmpty    = "Empty"
)

// Function names
const (
	FuncInit     = "init"
	FuncMain     = "main"
	FuncInit2    = "Init"
	FuncRegister = "Register"
	FuncRunOrDie = "RunOrDie"
)

// Prefix for model package conversion
const (
	PrefixModel         = "model"
	PrefixService       = "service"
	SeparatorUnderscore = "_"
)

// Cache file
const (
	CacheFileName = ".gg_cache.json"
)

// Project subdirectories for main.go imports
const (
	SubDirConfigx    = "configx"
	SubDirCronjob    = "cronjob"
	SubDirMiddleware = "middleware"
	SubDirModel      = "model"
	SubDirModule     = "module"
	SubDirService    = "service"
	SubDirRouter     = "router"
)

// Bootstrap method names
const (
	BootstrapBootstrap = "Bootstrap"
	BootstrapRun       = "Run"
	RouterInit         = "Init"
	ModuleInit         = "Init"
)
