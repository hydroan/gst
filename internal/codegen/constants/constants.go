package constants

// Import paths
const (
	// Framework import paths
	//nolint:godoclint
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
	FieldBase  = "Base"
	FieldEmpty = "Empty"
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
