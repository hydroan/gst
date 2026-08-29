// Package ggmodule implements the behavior behind the gg module command
// family: discovering framework modules (list), registering a module import in
// the project's module/module.go (add, remove), and materializing a module
// into the project as project-owned source (copy).
//
// # Module catalog
//
// The catalog is derived from the framework source tree, never maintained as a
// static list: every module/<name>/ directory whose register.go declares func
// Register is a module. Two capabilities are computed per module:
//
//   - Addable: Register takes no arguments, so gg module add can write the
//     zero-argument pkg.Register() init call itself.
//   - Copyable: module/<name>/module.json exists. The manifest is the
//     explicit copy contract; without it gg module copy refuses the module.
//
// add and copy are mutually exclusive per module: copy refuses to run while
// module/module.go still imports the framework module package or calls its
// Register anywhere. That check is deliberately broader than the zero-argument
// init call add itself writes, because anything that looks like a registration
// has to stop the copy.
//
// # Copy pipeline
//
// Copy is a strict two-phase design. BuildCopyPlan is a pure preflight that
// computes every final output byte and every rule violation without touching
// the project; CopyExecution.Run applies a checked plan. The command layer
// between them shows the plan and asks for consent:
//
//	gg module copy <name> [--force] [--yes]
//	        |
//	        v
//	BuildCopyPlan ---- any rule fails ----> error, project untouched
//	        |
//	        v
//	plan preview: files to write, stale files that will be deleted
//	        |
//	        v
//	confirm prompt (y/N; --yes skips) ---- no ----> canceled
//	        |
//	        v
//	CopyExecution.Run
//	  1. write model files            SKIP | UPDATE | CREATE
//	  2. prune stale files            DELETE (model, service, middleware
//	                                  files + their register calls)
//	  3. run gg gen                   (see "gg gen during copy")
//	  4. write action service files   (merged onto generated shells)
//	  5. write helper service files
//	  6. write middleware files       (ownership marker prepended)
//	  7. reconcile middleware
//	     registrations               (AST edit, middleware/middleware.go)
//	        |
//	        v
//	print module.json postNotes
//
// Write statuses: CREATE is a new file, UPDATE overwrote a differing
// preexisting file (only reachable with --force), SKIP left an identical file
// untouched, DELETE removed a stale file.
//
// Run does not roll back: a failure after the first write or delete leaves the
// partial copy in place and the command prints the manual cleanup path.
// Re-running a completed copy is idempotent — every write becomes SKIP and the
// prune finds nothing.
//
// # Copy planning (BuildCopyPlan)
//
// Planning walks the stages below in order; any failed rule aborts the whole
// copy before a single write:
//
//	validate <name>: a bare catalog name, no paths
//	require ./go.mod, read the project module path
//	locate the framework root (./internal/gst, then ".", then parents;
//	a root is a directory whose go.mod declares module github.com/hydroan/gst)
//	require module/<name>/, internal/model/<name>/, internal/service/<name>/
//	load and validate module/<name>/module.json
//	refuse a module that is still add-registered
//	        |
//	        v
//	MODEL PLANNING
//	  mirror internal/model/<name>/** (skip test files, dotfiles,
//	  vendor/, testdata/, excluded sources) ----------> model files
//	  fail if a mirrored file references an excluded one
//	  non-exempt leftover target files ---------------> stale models
//	        |
//	        v
//	ACTION PLANNING (DSL-driven)
//	  every Design() action with Service():
//	    source internal/service/<name>/<ServiceFilename()>
//	    target service/<name>/<ServiceFilename()>
//	  each source must declare a service struct
//	        |
//	        v
//	HELPER DISCOVERY (type-informed closure, whole files)
//	  seeds: action files + manifest includeSourceFiles
//	  a referenced tree file joins the copy; a blank-imported
//	  tree package joins wholesale
//	  fail on references to excluded files or to orphan
//	  service-struct files
//	        |
//	        v
//	SERVICE CONTENT
//	  per action target: generate the gg gen shell and merge
//	  the source onto it ----------------------------> service files
//	  normalize discovered helpers ------------------> helper files
//	  non-exempt leftover target files --------------> stale services
//	        |
//	        v
//	MIDDLEWARE PLANNING (manifest-declared only)
//	  validate middleware/*.go source and its handler
//	  normalize, prepend ownership marker -----------> middleware files
//	  marker-owned targets outside the manifest -----> stale middleware
//	        |
//	        v
//	CONFLICT CHECK
//	  a preexisting target whose planned content differs
//	  requires --force; identical content needs nothing
//
// The conflict rule is re-checked at write time, so a file that changed
// between plan and execution still cannot be overwritten without --force.
//
// # Manifest (module.json)
//
// module/<name>/module.json is the copy contract. All fields live under the
// "copy" key:
//
//   - excludeSourceFiles: framework-root relative source files that copy
//     skips entirely — not copied, not planned as models or actions. An
//     excluded action service file drops its actions from the copy, and a
//     previously copied instance of an excluded file counts as stale in the
//     target project and is pruned. A file that copied sources still
//     reference cannot be excluded: the model-tree reference check and the
//     helper closure both fail the copy at preflight, naming the referencing
//     file, instead of shipping a package that cannot compile. (References
//     from copied service files into excluded model files are outside both
//     walks and surface through the copied project's build.)
//   - includeSourceFiles: files under internal/service/<name>/ that copy
//     must always carry as helper files even when no action references
//     them, for hook implementations reached only from project-owned
//     assembly code (a login second-factor verifier, a login observer).
//     Entries must exist, must not be test files, must not also be excluded,
//     and must not declare a service struct — action service files are
//     copied through their DSL actions only.
//   - middleware: manifest-declared middleware copies, see "Middleware copy
//     rules".
//   - postNotes: free-form lines printed after a successful copy, for the
//     manual follow-up steps the copy cannot automate.
//
// Manifest paths are validated framework-root relative paths; absolute paths
// and ".." escapes are rejected.
//
// # Model copy rules
//
// Model copy is a mirror: every Go source file under internal/model/<name>,
// including nested subpackages, is copied to model/<name>/ at the same
// relative path. Skipped are test files, dotfiles, files under vendor/ and
// testdata/ directories, and manifest-excluded sources. When the manifest
// excludes model files, the model tree is type-checked and a mirrored file
// referencing an excluded one fails the copy at preflight.
//
// Each copied model file is rewritten, not copied verbatim:
//
//   - The package clause becomes the target directory name (sanitized to a
//     valid identifier), so internal/model/copytest package modelcopytest
//     becomes model/copytest package copytest.
//   - Imports of the same module's internal model tree
//     (github.com/hydroan/gst/internal/model/<name>...) are rewritten to
//     the project's model/<name> tree. Local import names follow the new
//     path base, with deterministic aliases when names collide, and every
//     package-qualified selector is retargeted. A rewritten qualifier that
//     a same-file declaration would shadow fails the copy with a rename
//     instruction for the module source.
//   - Imports of the module's internal service tree are deliberately NOT
//     rewritten in model files: a model importing service code is an
//     architecture violation, and the next rule turns it into a copy error
//     instead of hiding it in generated project code.
//   - Any surviving github.com/hydroan/gst/internal/... import fails the
//     copy: such an import compiles inside the framework but never in the
//     consumer project.
//
// # Service copy rules
//
// The service layer is not a mirror. Two kinds of files are planned.
//
// Action service files carry the business logic of DSL-declared actions. For
// every model whose Design() declares an action with Service(), the source is
// the framework file at gen.ServiceTarget under internal/service/<name>/, and
// the target is the same gen.ServiceTarget mapping under service/<name>/ —
// exactly where a later gg gen would regenerate it. Rules:
//
//   - The source file must exist and declare at least one service struct (a
//     struct embedding service.Base); hook-only files are fine, the
//     action's main method is not required.
//   - Actions whose source file is manifest-excluded are skipped.
//   - Actions sharing one ServiceFilename() merge into one target file.
//
// A target action file is produced by merging the framework source onto a
// freshly generated gg gen service shell:
//
//   - The shell owns the package clause, the import layout, the service
//     struct identity and the generated action method signatures.
//   - The source owns method bodies, hook methods, receiver helper methods,
//     ordinary declarations and comments. Bodies are grafted onto the
//     generated signatures with receiver and parameter/result names renamed
//     to the generated ones. Methods without a generated signature are
//     carried over whole, with their receiver variable renamed to the
//     shell's receiver name so the merged struct keeps one consistent
//     receiver and passes receiver-naming linters.
//   - Every source service struct collapses onto the single generated
//     target struct; one method name declared on two source structs fails
//     the copy, because the collapse would make them collide.
//   - Source imports merge into the shell. Imports stranded by the struct
//     replacement are dropped, but only when a source qualifier proves the
//     import's local package name (an import path base is not always the
//     package name). A stranded framework-owned or project-owned import
//     that survives fails the copy; third-party leftovers are left to the
//     compiler.
//   - The same import rewrites, shadowing check and internal-import guard
//     as model files apply, with service-tree imports rewritten too.
//
// Helper service files are whole files copied because action code needs them.
// They are discovered by one type-informed closure over the whole service
// tree, seeded by the action files and the manifest includeSourceFiles
// (which are helper output themselves — no other channel copies them):
//
//   - When a selected file references a top-level object declared in
//     another file of the tree, that whole file joins the copy. The walk
//     uses go/packages type information over every package of the tree at
//     once, so references cross package boundaries, the framework service
//     tree must type-check, and the granularity is the file, never the
//     single symbol.
//   - A blank import of a tree subpackage carries no identifier the walk
//     could follow, but it is an explicit request for the package's init
//     side effects, so every file of that package joins wholesale — minus
//     manifest-excluded files and service-struct files.
//   - A reference to a manifest-excluded file fails the copy (see
//     "Manifest"). A reference to a file that declares a service struct
//     without being copied by any DSL action fails the copy too: action
//     files are copied only through their actions, so shared code must live
//     in helper files.
//
// Helper files are normalized like model files, with service-tree imports
// rewritten as well.
//
// # Middleware copy rules
//
// Middleware is copied only when module.json declares it; nothing is
// discovered. Per manifest entry:
//
//   - sourceFile must match middleware/*.go (framework middleware package,
//     non-test). The target is always the project middleware directory with
//     the same filename; target paths are not configurable.
//   - handler must be a Go identifier declared as a top-level function of
//     that name in the source file.
//   - scope selects the registration call: "global" wires
//     middleware.Register(Handler()), "auth" wires
//     middleware.RegisterAuth(Handler()).
//
// The file is normalized like a helper file, because middleware may
// legitimately import copied model/service packages, and then an ownership
// marker line is prepended:
//
//	// Managed by gg module copy (module <name>). Removing the module removes this file.
//
// The marker is the ownership proof behind middleware pruning: the middleware
// directory is shared with project-owned handlers and other modules' copies,
// so only a file carrying this module's marker may ever be deleted by this
// module's copy. Files copied before the marker existed show up as a --force
// overwrite on their next copy, which upgrades them into prune management.
//
// Registration is a reconciliation of middleware/middleware.go against the
// manifest, scoped to the handlers this module owns (the top-level functions
// of its marker-carrying middleware files, taken from both the pre-copy disk
// content and the freshly written targets so renamed handlers stay
// attributable):
//
//   - a register call whose module-owned handler no longer matches a
//     declared (handler, scope) pair is dropped — a scope change or a
//     handler rename retires its old call this way;
//   - every declared pair is ensured;
//   - calls naming handlers the module does not own are never touched.
//
// The edit is AST-based: it works inside init functions, reuses an existing
// framework middleware import alias, preserves comments and existing init
// work, creates a minimal package file when the project has none, and skips
// the write when nothing changes.
//
// # Stale-file prune rules
//
// Copy keeps the copied directories in sync with the framework module source:
// after planning, files an older copy produced that the current plan no
// longer does are stale, and the execution deletes them. Staleness is decided
// per directory kind:
//
//   - model/<name>/ and service/<name>/ mirror the module source, so every
//     Go file present there that the plan does not produce is stale. Two
//     kinds of project-owned files are exempt and never enter the lists:
//     test files (_test.go — module tests are not copied, so tests inside
//     copied packages are project assets) and generated files (any file
//     carrying a Go-convention "// Code generated ... DO NOT EDIT." header
//     before the package clause belongs to its generator — gg gen Cols
//     files in particular).
//   - middleware/ is shared, so membership needs positive proof: a file is
//     stale only when it carries this module's ownership marker while the
//     manifest no longer declares it. The registration file
//     middleware.go is project infrastructure and is never pruned. When a
//     stale middleware file is deleted, the middleware.Register and
//     middleware.RegisterAuth calls naming its top-level functions are
//     removed from middleware/middleware.go in the same step, because they
//     would break the build the moment the file is gone.
//
// The preview printed before the confirmation prompt lists exactly the files
// the prune will delete, so answering yes consents to the deletions; --force
// plays no role here, it only gates content overwrites. The prune runs after
// the model writes and before gg gen: a stale model file still carries
// Design() DSL that gen would faithfully regenerate registrations for, and a
// stale service file referencing a removed model would surface as a fresh
// project-check violation during the copy-time gen run. Every deletion is
// path-checked against its owning directory, and a listed file that is
// already gone counts as pruned.
//
// # gg gen during copy
//
// Between the model writes and the service writes, the command runs the same
// generator as gg gen with two adjustments: prune and clean-orphans stay
// disabled, because a copy must not become a cleanup pass over user service
// files, and project checks are scoped to a baseline snapshot taken before
// the copy, so pre-existing violations in the project do not block copying an
// unrelated module while violations introduced by the copied module still
// fail it.
//
// # Path safety
//
// Module names must be bare catalog names, so commands cannot address
// arbitrary filesystem paths. Manifest paths are cleaned and confined to the
// framework root: absolute paths and ".." escapes are rejected. Every write
// and every deletion re-verifies its target against the owning project
// directory (model, service, or middleware) before touching the filesystem.
package ggmodule
