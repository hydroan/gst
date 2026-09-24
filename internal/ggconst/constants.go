// Package ggconst holds the conventions the framework's tooling shares: the
// import paths of the framework packages, the names of the files, packages
// and fields gg generates and reads, and the patterns it recognizes them by.
// The dsl parser, the syntax tree helpers of goast, the code generator and
// the gg commands all read them from here, so a convention is spelled once.
package ggconst

// Import paths
const (
	// Framework import paths
	//nolint:godoclint
	ImportPathGst       = "github.com/hydroan/gst"
	ImportPathModel     = "github.com/hydroan/gst/model"
	ImportPathService   = "github.com/hydroan/gst/service"
	ImportPathRouter    = "github.com/hydroan/gst/router"
	ImportPathConsts    = "github.com/hydroan/gst/consts"
	ImportPathBootstrap = "github.com/hydroan/gst/bootstrap"
	ImportPathUtil      = "github.com/hydroan/gst/util"
	ImportPathAPIDoc    = "github.com/hydroan/gst/apidoc"

	// ImportPathIO is the standard library package the Import method of a
	// generated service file reads its input through.
	ImportPathIO = "io"
)

// File patterns and extensions
const (
	ExtensionGo     = ".go"
	PatternTestFile = "_test.go"
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

// Directory names. The project directories are spelled here alone: gg reads
// and writes them, and ProjectImportDirs below lists the ones a generated
// main.go imports.
const (
	DirVendor   = "vendor"
	DirTestData = "testdata"

	DirComponent  = "component"
	DirConfigx    = "configx"
	DirCronjob    = "cronjob"
	DirDAO        = "dao"
	DirLeader     = "leader"
	DirLock       = "lock"
	DirMiddleware = "middleware"
	DirModel      = "model"
	DirModule     = "module"
	DirRouter     = "router"
	DirService    = "service"
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

// Cache file
const (
	CacheFileName = ".gg_cache.json"
)

// ProjectImportDirs lists the project packages a generated main.go imports,
// in import-path order. The migration program imports the same list: its
// model set is whatever those packages' initialisers registered, so it has to
// link exactly what the service links. Declared once so the two cannot drift.
// Every directory here is a scaffold file gg new creates and gg gen restores
// when missing, so a project always has the package main.go imports.
var ProjectImportDirs = []string{
	DirComponent,
	DirConfigx,
	DirCronjob,
	DirLeader,
	DirLock,
	DirMiddleware,
	DirModel,
	DirModule,
	DirRouter,
	DirService,
}

// Bootstrap method names
const (
	BootstrapBootstrap = "Bootstrap"
	BootstrapRun       = "Run"
	RouterInit         = "Init"
	ModuleInit         = "Init"
)
