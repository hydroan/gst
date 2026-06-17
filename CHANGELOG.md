<a name="unreleased"></a>
## [Unreleased]


<a name="v0.10.14"></a>
## [v0.10.14] - 2026-05-18
### Chore
- modernize config parsing and trie loops
- address lint findings from make check
- remove legacy demo controllers and captcha dependencies
- resolve golangci-lint issues
- update pkg/filetype testcase
- **deps:** upgrade dependencies to latest version
- **deps:** add godump
- **deps:** upgrade dependencies to latest version
- **deps:** remove unused github.com/goforj/godump
- **gitignore:** ignore ggshield cache directory

### Docs
- update CLAUDE.md
- require grouped placement for type and constant definitions
- reorganize module guidelines and custom API model rules
- tighten module test helper placement guidance
- add module API test conventions and rename code section
- expand module file layout scenarios for CRUD and custom IAM
- clarify per-endpoint vs per-resource module layout
- document rules for git commit suggestion requests
- document rules for git commit suggestion requests
- document rules for git commit suggestion requests
- **database:** require a new Database() call per operation chain

### Feat
- **codegen:** humanize service log.Info when DSL Filename is set
- **controller:** map ServiceError.Coder to JSON responses
- **iam:** validate account on login/me and fix account status session invalidation
- **iam:** add account-status API and align reset-password actor
- **redis:** add sorted-set helpers and rename delete API
- **response:** implement Coder and add account status codes
- **types:** add Coder and optional coder on ServiceError

### Fix
- **codegen:** avoid to generate multiple level service dir
- **controller:** pass serviceCtxAfter to DeleteManyAfter hook
- **controller:** log nil request/response as "<nil>" for zap safety
- **gg:** service path collapse and prune for custom Filename outputs
- **iam:** restrict user patch to allow-listed profile fields
- **service:** stop reusing Database handle after List queries

### Refactor
- **atomic:** use typed sync/atomic for flags and cache stats
- **iam:** split user model and service into iam/user packages
- **iam:** centralize session resolution in GetCurrentSession

### Style
- apply gofumpt formatting

### Test
- tighten TestResp checks and assertion order

### Pull Requests
- Merge pull request [#85](https://github.com/hydroan/gst/issues/85) from hydroan/dev
- Merge pull request [#84](https://github.com/hydroan/gst/issues/84) from hydroan/dev
- Merge pull request [#83](https://github.com/hydroan/gst/issues/83) from hydroan/dev
- Merge pull request [#82](https://github.com/hydroan/gst/issues/82) from hydroan/dev

### BREAKING CHANGE

Tenant types moved from internal/model/iam to
internal/model/iam/tenant; update imports accordingly.

Group types moved from internal/model/iam to
internal/model/iam/group; update imports and generated code accordingly.

POST /api/offline is no longer registered; clients
must use other session revocation APIs (e.g. admin or self-service
delete flows).

Casbin major-version upgrade; downstream code using casbin/v2
alongside this module must migrate to v3-compatible APIs.

modeliamsession.SessionRedisKey and SessionUserRedisKey were
removed/replaced; update external callers to SessionIDKey/SessionUserKey.

aichat APIs and module registration are removed from the framework.

Redis session snapshot key namespace changes from iam:session
to iam:session:id; existing session keys must be migrated or users must re-login.

- GET /api/me is removed in favor of GET /api/iam/session/current (response shape changes).
- POST /api/heartbeat is moved to POST /api/iam/session/heartbeat.
- Session payload stored in Redis changes shape; existing sessions may be incompatible.

IAM email endpoints path changed:
- /iam/email-change-cancel -> /iam/email/change-cancel
- /iam/email-change-confirm -> /iam/email/change-confirm
- /iam/email-change-request -> /iam/email/change-request
- /iam/email-change-resend -> /iam/email/change-resend
- /iam/email-password-reset-confirm -> /iam/email/password-reset-confirm
- /iam/email-password-reset-request -> /iam/email/password-reset-request
- /iam/email-verification-confirm -> /iam/email/verification-confirm
- /iam/email-verification-request -> /iam/email/verification-request
- /iam/email-verification-resend -> /iam/email/verification-resend

verification endpoints moved from
- /iam/email-verification-{request,confirm,resend}
to
- /iam/email/verification-{request,confirm,resend}


<a name="v0.10.13"></a>
## [v0.10.13] - 2026-03-08
### Build
- test ds
- test moudle/twofa
- test pkg/dbmigrate
- test module/iam, module/logmgmt, module/version

### Chore
- remove unused nolint:gosec directives (G117, G703, G704)
- update CLAUDE.md
- **client:** remove unused nolint:gosec directives
- **client:** remove unused nolint:gosec directives
- **controller:** add gosec G117 nosec for session marshal (redis storage only)
- **database:** use logger replace fmt
- **deps:** upgrade dependencies to latest version
- **deps:** upgrade dependencies to latest version
- **examples:** ignore gitguardian check
- **release:** generate CHANGELOG.md

### Ci
- add revive "dot-imports" check
- add revive "exported" check

### Feat
- **client:** add `Request` method
- **client:** add `WithCookie` option function

### Fix
- address revive linter findings (26 files)
- **authz:** use initialism IDs in var and field names for revive
- **codegen:** rename ast to codegenast to resolve revive stdlib conflict
- **gg:** use correct "air" package path
- **golangci-lint:** fix nolintlint issues
- **golangci-lint:** fix revive exported issues
- **golangci-lint:** fix gosec issues
- **golangci-lint:** fix lint issues
- **golangci-lint:** fix issues
- **lint:** exclude top-level types/ and util/ from meaningless package name check
- **middleware:** resolve golangci-lint issues
- **staticcheck:** fix issues
- **task:** call context cancel when task goroutine exits (gosec G118)

### Refactor
- **cache:** rename `TracingWrapper` -> `Wrapper`, `NewTracingWrapper` -> `NewWrapper`
- **metrics:** rename package to prommetrics to avoid stdlib conflict
- **middleware:** export RouteParamsManager type for unexported-return
- **middleware:** `RateLimiterConfig` -> `Config`, `RateLimiterOption` -> `Option`
- **middleware:** refactor ratelimiter middleware
- **response:** rename exported funcs to satisfy revive exported rule
- **service:** rename ServiceFactory to Factory, Factory to NewFactory (revive exported)
- **sse:** put context.Context as first parameter for revive
- **types:** export SSEBuilder type for unexported-return (revive)
- **types:** NewModelContext(ctx, dbctx) for revive context-as-argument

### Style
- **model:** omit redundant type in Routes declaration (revive var-declaration)

### Test
- **authz:** add menu, role, user_role register tests and fix permission/shadow
- **module:** move testResp to internal/helper.TestResp

### Pull Requests
- Merge pull request [#81](https://github.com/hydroan/gst/issues/81) from hydroan/dev


<a name="v0.10.12"></a>
## [v0.10.12] - 2026-02-12
### Build
- always use the latest golangci-lint, go tools

### Chore
- **deps:** upgrade dependencies to latest version
- **release:** generate CHANGELOG.md

### Enh
- **dbmigrate:** replace boolean by tinyint
- **dbmigrate:** ensure the generate schema is always sorted
- **dbmigrate:** remove schema "IF NOT EXISTS"
- **dbmigrate:** enhance schema migration

### Fix
- **dbmigrate:** support model custom table
- **dbmigrate:** fix migrate postgres
- **golangci-lint:** fix issues
- **golangci-lint:** fix issues

### Wip
- **dbmigrate:** add dbmigrate, support dump table schema and apply schema

### Pull Requests
- Merge pull request [#80](https://github.com/hydroan/gst/issues/80) from hydroan/dbmigrate


<a name="v0.10.11"></a>
## [v0.10.11] - 2026-02-10
### Chore
- **release:** generate CHANGELOG.md

### Enh
- **dsl:** ensure route order
- **dsl:** apply service code receiver variable name according to role name

### Feat
- **dsl:** support use "Filename" to specific generated service filename

### Fix
- **database:** make init seed inserts idempotent across DBs
- **dsl:** change the generated service struct name  depends on the dsl "Filename"

### Pull Requests
- Merge pull request [#79](https://github.com/hydroan/gst/issues/79) from hydroan/dev


<a name="v0.10.10"></a>
## [v0.10.10] - 2026-01-25
### Chore
- **release:** generate CHANGELOG.md

### Fix
- **controller:** skip request unmarshal if its a file upload request
- **controller:** use structured logger instead of normal logger to log request and response
- **database:** use correct quote for postgresql in `WithOrder`

### Pull Requests
- Merge pull request [#78](https://github.com/hydroan/gst/issues/78) from hydroan/dev


<a name="v0.10.9"></a>
## [v0.10.9] - 2026-01-17
### Chore
- **release:** generate CHANGELOG.md

### Fix
- **database:** use dialect-aware identifier quoting for WithQuery on Postgres


<a name="v0.10.8"></a>
## [v0.10.8] - 2026-01-15
### Build
- upgrade go tools to v0.41.0, golangci-lint to v2.8.0

### Chore
- **deps:** upgrade dependencies to latest version
- **release:** generate CHANGELOG.md

### Ci
- use "make install" and "make check" to avoid hardcode

### Fix
- **codegen:** avoid rewriting sibling model packages in service apply
- **gg:** stabilize generated import order to avoid router.go churn

### Pref
- fix golangci-lint "preallc" issues

### Test
- **database:** auto migrate table in TestDatabaseWithDB
- **database:** auto migrate table in TestDatabaseWithDB

### Pull Requests
- Merge pull request [#76](https://github.com/hydroan/gst/issues/76) from hydroan/dev


<a name="v0.10.7"></a>
## [v0.10.7] - 2026-01-05
### Chore
- **release:** generate CHANGELOG.md

### Database
- disable auto migrate

### Pull Requests
- Merge pull request [#75](https://github.com/hydroan/gst/issues/75) from hydroan/dev


<a name="v0.10.6"></a>
## [v0.10.6] - 2026-01-02
### Chore
- **release:** generate CHANGELOG.md

### Docs
- **dsl:** clarify Enabled function dual usage scenarios

### Refactor
- **config:** improve config name generation with snake_case
- **model:** make *Any and *Empty implements interface tyeps.Model to avoid use incorrect to cause panic

### Test
- **model:** add type alias test case for "IsEmpty" function

### Pull Requests
- Merge pull request [#74](https://github.com/hydroan/gst/issues/74) from hydroan/dev


<a name="v0.10.5"></a>
## [v0.10.5] - 2025-12-16
### Build
- make test add test for "service"

### Chg
- **aichat:** rename internal/service/aichat/chat_response.go -> internal/service/aichat/chat_handlers.go

### Chore
- **deps:** upgrade dependencies to latest version
- **examples:** go mod tidy
- **minio:** fix typo in comment and remove debug output
- **release:** generate CHANGELOG.md
- **style:** run gofumpt

### Docs
- **module:** simplify package docs and comments

### Feat
- **aichat:** refactor message feedback to submit-only API
- **aichat:** add new models for attachments, sharing, and user management
- **aichat:** add context window management with token counting
- **aichat:** add stream cancellation support
- **aichat:** auto-generate conversation title from first message
- **aichat:** add conversation delete hooks to clean up messages and feedbacks
- **aichat:** add clear conversation messages API
- **aichat:** add message regeneration support
- **aichat:** add Google Gemini provider support
- **aichat:** implement attachment and document services with storage integration
- **client:** add WithAPI option for custom API path prefix
- **client:** add Stream, StreamPrint, StreamURL, StreamPrintURL
- **minio:** enhance minio provider with comprehensive object operations
- **minio:** support multiple buckets and extract bucket ensure logic
- **openapigen:** support datatypes.JSONType
- **types:** ServiceContext add method PostForm, FormFile
- **types:** ServiceContext add method SSEvent, Stream

### Fix
- **aichat:** add validation for message feedback submission
- **aichat:** explicitly set is_active for message lifecycle
- **aichat:** fix is_active handling and add regenerate status validation
- **aichat:** prevent goroutine leak in stream handling and improve stop logic
- **aichat:** prevent nil pointer panic when accessing response.Content

### Perf
- **aichat:** optimize message query with composite index and fix null pointer issues

### Refactor
- add omitempty to response model JSON tags
- **aichat:** simplify database queries using Get() method
- **aichat:** optimize regenerate message queries and fix streaming response
- **aichat:** extract response handlers to separate file
- **aichat:** remove unnecessary schema tags from request models
- **aichat:** improve stream handling concurrency and stop message response
- **service:** includes REQ and RSP types int service key generation
- **service:** service.Register support struct and pointer to struct

### Style
- **client:** align comments in WithAPI patch test

### Test
- **aichat:** add comprehensive StreamManager tests and fix nil cancel check
- **aichat:** add comprehensive tests and fix context manager edge cases

### Pull Requests
- Merge pull request [#65](https://github.com/hydroan/gst/issues/65) from hydroan/aichat
- Merge pull request [#73](https://github.com/hydroan/gst/issues/73) from hydroan/dev


<a name="v0.10.5-beta.3"></a>
## [v0.10.5-beta.3] - 2025-12-08
### Chore
- **release:** generate CHANGEME.md

### Refactor
- **database:** combine model query and raw query in database option "WithQuery"

### Pull Requests
- Merge pull request [#72](https://github.com/hydroan/gst/issues/72) from hydroan/dev


<a name="v0.10.5-beta.2"></a>
## [v0.10.5-beta.2] - 2025-12-06
### Chore
- go mod tidy
- **build:** add "go mod tidy" in `make check`
- **ci:** use go 1.25
- **release:** generate CHANGEME.md

### Refactor
- **service:** includes REQ and RSP types int service key generation

### Test
- **database:** add test case for JSON field fuzzy query

### Pull Requests
- Merge pull request [#71](https://github.com/hydroan/gst/issues/71) from hydroan/dev


<a name="v0.10.5-beta.1"></a>
## [v0.10.5-beta.1] - 2025-12-03
### Chore
- **release:** generate CHANGEME.md

### Pull Requests
- Merge pull request [#70](https://github.com/hydroan/gst/issues/70) from hydroan/dev


<a name="v0.10.5-beta.0"></a>
## [v0.10.5-beta.0] - 2025-12-03
### Chore
- **release:** generate CHANGEME.md

### Pull Requests
- Merge pull request [#69](https://github.com/hydroan/gst/issues/69) from hydroan/dev


<a name="v0.10.4"></a>
## [v0.10.4] - 2025-12-03
### Chore
- **release:** generate CHANGEME.md

### Pull Requests
- Merge pull request [#68](https://github.com/hydroan/gst/issues/68) from hydroan/dev


<a name="v0.10.3"></a>
## [v0.10.3] - 2025-12-02
### Chore
- **release:** generate CHANGEME.md

### Pull Requests
- Merge pull request [#67](https://github.com/hydroan/gst/issues/67) from hydroan/dev


<a name="v0.10.2"></a>
## [v0.10.2] - 2025-12-02
### Chore
- **release:** generate CHANGEME.md

### Feat
- add examples/aichat to show how to use sse
- **database:** support query "remark"
- **sse:** prepare support SSE

### Refactor
- rename imports: "internalsse" to  "sse"
- **sse:** unexport consts and inline context-type
- **types:** replace map[string][]string with url.Values for "Query" fields in ControllerContext, DatabaseContext, ServiceContext

### Style
- **types:** gofumpt format

### Test
- **database:** test resources ID generated automatically in "Create" and "Update"

### Pull Requests
- Merge pull request [#66](https://github.com/hydroan/gst/issues/66) from hydroan/dev
- Merge pull request [#53](https://github.com/hydroan/gst/issues/53) from hydroan/sse


<a name="v0.10.1"></a>
## [v0.10.1] - 2025-11-27
### Chore
- **controller:** remove unused controller about column
- **release:** generate CHANGELOG.md

### Feat
- **middleware:** add "Timeout" middleware
- **middleware:** add "SecurityHeaders" middleware
- **middleware:** add "RequestSizeLimit" middleware
- **middleware:** add IP whitelist/blacklist filtering middleware
- **middleware:** add delay middleware for testing
- **middleware:** add "IAMSession" middleware
- **module:** implement lazy module registration with channel-based middleware system

### Fix
- **codegen:** generated Export hook function change input parameter name to avoid same to returns param
- **database:** avoid create two tables for one model, one table is custom table, one is default table
- **dsl:** keep import/export router registration before get to avoid route overwrite
- **middleware:** sanitize logger metrics labels
- **middleware:** add thread-safe locks to "RouteParams"

### Refactor
- move the create permissions records job from router into module/authz
- **codegen:** simplify generated code templates
- **codegen:** change module.Init() to init() for automatic initialization
- **config:** default disabled Audit for record operation
- **controller:** not support set purge mode in controller

### Pull Requests
- Merge pull request [#64](https://github.com/hydroan/gst/issues/64) from hydroan/dev
- Merge pull request [#63](https://github.com/hydroan/gst/issues/63) from hydroan/dev
- Merge pull request [#62](https://github.com/hydroan/gst/issues/62) from hydroan/dev


<a name="v0.10.0"></a>
## [v0.10.0] - 2025-11-25
### Chore
- cleanup cover file
- **release:** generate CHANGEME.md
- **release:** generate CHANGEME.md

### Refactor
- **logmgmt:** move log models and services into internal packages

### Pull Requests
- Merge pull request [#60](https://github.com/hydroan/gst/issues/60) from hydroan/dev
- Merge pull request [#59](https://github.com/hydroan/gst/issues/59) from hydroan/dev


<a name="v0.10.0-beta.6"></a>
## [v0.10.0-beta.6] - 2025-11-24
### Chg
- **model:** remove internal/model/user.go

### Chore
- **release:** generate CHANGEME.md
- **types:** reorder `Pub` method below `Route` in `Module` interface

### Docs
- **model:** update docs for `AreTypeEqual`
- **module:** correct routes about "PUT" method for modules "authz", "towfa"

### Feat
- **codegen:** add model import/package synchronization for service generation
- **model:** add models in "internal/model/iam"

### Fix
- **golangci-lint:** resolve errorcheck in "LogoutService"
- **golangci-lint:** sha256 replace md5
- **shadow:** resolve shadows declaration

### Pull Requests
- Merge pull request [#58](https://github.com/hydroan/gst/issues/58) from hydroan/dev
- Merge pull request [#57](https://github.com/hydroan/gst/issues/57) from hydroan/dev
- Merge pull request [#56](https://github.com/hydroan/gst/issues/56) from hydroan/dev
- Merge pull request [#55](https://github.com/hydroan/gst/issues/55) from hydroan/dev


<a name="v0.10.0-beta.5"></a>
## [v0.10.0-beta.5] - 2025-11-22
### Build
- add analysis tools and update Makefile

### Chg
- **logger:** move "CasbinLogger" implementation to dedicated file
- **logger:** move "GormLogger" implementation to dedicated file
- **logger:** authz logger disable "caller" field and two fields: "username", "trace_id"
- **model:** disable soft-delete for model "Menu", "Button"
- **model:** remove soft-delete for "Permission"
- **router:** re-create permission in transaction

### Chore
- **ai:** update CLAUDE.md
- **ai:** update CLAUDE.md
- **ai:** update CLAUDE.md
- **ai:** update CLAUDE.md
- **ai:** add CLAUDE.md
- **build:** adjust check sequences
- **database:** change `WithTable` and `WithTx` position
- **deps:** go mod tidy
- **model:** remove "Menu", that replace by internal/model/authz/menu.go
- **release:** generate CHANGEME.md

### Ci
- sync analysis tools with Makefile
- ensure github ci tools version match to go.mod

### Docs
- **database:** concise comments
- **database:** update docs for `WithDB` and `WithTable`
- **database:** update docs for `WithTx`
- **database:** update docs
- **database:** enhance WithQuery and QueryConfig documentation
- **middleware:** update docs for "Authz"
- **types:** update documents for `Initializer`, `StandardLogger`, `StructuredLogger`, `ZapLogger`, `Logger`

### Enh
- **database:** enhance docs and add test cases for `WithDryRun`
- **database:** enhance docs and add test cases for `WithOmit`
- **database:** enhance docs and add test cases for `WithCache`
- **database:** enhance docs and add test coverage for `WithPurge`
- **database:** enhance docs and add test coverage for `WithExclude`
- **database:** enhance docs and add test coverage for `WithOrder`
- **database:** enhance docs and add test coverage for `WithLock`
- **database:** enhance docs and add test coverage for `WithRollback`
- **database:** enhance docs and test coverage for `WithSelectRaw`
- **database:** enhance docs and test coverage for `WithCursor`
- **database:** enhance docs and test coverage for `WithDebug`
- **database:** enhance `Create` docs and test coverage
- **database:** enhance `Delete` docs and test coverage
- **database:** enhance `Update` docs and test coverage
- **database:** check passed parameter is nil in `List` and enhance test coverage
- **database:** enhance `Get` docs and its test coverage
- **database:** enhance `Last` docs and its test coverage
- **database:** enhance `Last` method and its test coverage

### Feat
- add noop RBAC
- add User model in "internal/model"
- **config:** add config "middleware"
- **database:** add `Transaction` for single model transaction
- **logger:** "Option" add feild "DisableCaller"
- **module:** "Menu" add route "PATCH_MANY" and update docs
- **module:** add "Button" module
- **module:** add "Api" module
- **module:** add menu module
- **rbac:** enable "blocked" role to support deny all request

### Fix
- **controller:** remove `List` second parameter "cache"
- **database:** ignore model fields when RawQuery is provided
- **database:** always rollback if transaction failed in `TransactionFunc` and add test cases for `TransactionFunc`
- **database:** fix UseOr to use Where() for first condition
- **database:** check query condition is nil in `WithQuery`
- **database:** improve WithQuery nil handling and documentation
- **database:** ensure state cleanup on early returns
- **database:** check count is nil in `Count`
- **database:** properly uses database context for PingContext in `Health`
- **database:** skip empty items in FuzzyMatch REGEXP pattern
- **database:** always add defaultColumns in `WithSelect` and enhance `WithSelect` test cases
- **database:** apply WithSelect lazily and respect select columns
- **database:** check nil dest in `Get`, `First`, `Last`, `Take`
- **database:** fix cache delection bug in `Delete` and `Update` options and improve docs and test coverage for `WithBatchSize`
- **database:** restrict WithIndex to MySQL SELECT queries only
- **database:** support auto migration for multiple database instances
- **database:** check "db.shouldAutoMigrate" in `prepare`
- **database:** check "id", "name", "value" in `UpdateByID`
- **database:** check empty resource before "Create", "Delete", "Update"
- **golangci-lint:** fix issues
- **module:** fix golangci-lint issues: rename module "Api" -> "API"
- **module:** forget save the rename "buttonservice" -> "ButtonService"

### Perf
- **database:** remove check empty or nil model in `Create`, `Delete` and `Update`
- **database:** remove redundant check in `Create`, `Delete` and `Update`
- **database:** add "Sync.Map" to avoid model multiple migration

### Refactor
- optimize interface documentation and remove unused cache parameters
- **database:** remove second parameter "cache" in `Get`
- **database:** remove second parameter in `List
- **database:** in `WithSelect` if columns is empty or all includes in "defaultColumns", its a no-op options
- **database:** `WithRollback` no longer returns error
- **database:** check empty parameters to returns error in "UpdateByID"
- **database:** optimize query option functions
- **database:** call `WithTable` will always disable "auto migration"
- **logger:** unify zap constructors and introduce typed Option for configuration

### Style
- **modernize:** fix modernize issues
- **modernize:** reflect.TypeOf simplified using TypeFor for package "internal/reflectmeta"
- **modernize:** reflect.TypeOf simplified using TypeFor
- **shadow:** reslove shadow issues

### Test
- add database package to test targets
- **database:** enhance test coverage for `Count`
- **database:** enhance test coverage for `Health`
- **database:** add test cases for `WithPagination`
- **database:** refactor test style
- **database:** enhance test coverage for `UpdateByID`
- **database:** add benchmark test for database
- **database:** enhance test coverage for `Cleanup`
- **database:** add test cases for `WithLimit`
- **database:** update test cases for `WithDB` and `WithTable`
- **database:** update test cases for `WithTx`
- **database:** update test cases
- **database:** add test cases for `WithExpand`
- **database:** rewrite database test cases
- **model:** fix test cases aftr remove Menu"

### Wip
- **module:** role binding permissions
- **test:** add test cases for `WithCache`

### Pull Requests
- Merge pull request [#54](https://github.com/hydroan/gst/issues/54) from hydroan/dev
- Merge pull request [#52](https://github.com/hydroan/gst/issues/52) from hydroan/dev

### BREAKING CHANGE

- WithAnd() is removed
- WithOr() is removed, use QueryConfig{UseOr: true} instead
- WithTryRun() is renamed to WithDryRun()


<a name="v0.10.0-beta.4"></a>
## [v0.10.0-beta.4] - 2025-11-10
### Chg
- move module model "Role" into internal/model/authz
- **model:** move model "Permission" into "module/authz"
- **model:** move model `Permission` register to module
- **service:** move role service into modules

### Chore
- **release:** generate CHANGEME.md

### Feat
- **module:** add authz modules
- **module:** add module "RolePermission"
- **module:** add module "UserRole"
- **module:** add "Role" module
- **module:** add module "permission"

### Fix
- **module:** register structure of service cause panic, must register sturcture pointer
- **module:** use `TrimPrefix` replace `TrimLeft` to normalize the route path

### Refactor
- move UserRole model from modules -> internal/model/authz
- move "model/log" -> "internal/model/log"
- remove "service/authz" and "service/log"
- move module models "CasinRule" and "Permission" into internal/model

### Pull Requests
- Merge pull request [#51](https://github.com/hydroan/gst/issues/51) from hydroan/dev


<a name="list"></a>
## [list] - 2025-11-08

<a name="v0.10.0-beta.3"></a>
## [v0.10.0-beta.3] - 2025-11-08
### Chore
- **release:** generate CHANGEME.md

### Feat
- **module:** add "version" module

### Pull Requests
- Merge pull request [#50](https://github.com/hydroan/gst/issues/50) from hydroan/dev


<a name="v0.10.0-beta.2"></a>
## [v0.10.0-beta.2] - 2025-11-07
### Chore
- **docs:** update modules docs for "column", "helloworld", "logmgmt"
- **release:** generate CHANGEME.md

### Feat
- **module:** add modules "twofa"

### Fix
- **lint:** fix modernize issues

### Pull Requests
- Merge pull request [#49](https://github.com/hydroan/gst/issues/49) from hydroan/dev


<a name="v0.10.0-beta.1"></a>
## [v0.10.0-beta.1] - 2025-11-06
### Build
- add test case for "model" and "util"

### Chore
- **docs:** update README.md
- **model:** remove empty line
- **release:** generate CHANGEME.md

### Docs
- **module:** update modules docs for `column`, `helloworld`, `logmgmt`
- **types:** update `ServiceContext`.`Params` comments

### Enh
- **module:** normalizes param

### Feat
- **model:** add `IsValid` to check whether the model is valid
- **module:** add column modules

### Refactor
- **model:** rename IsModelEmpty -> IsEmpty, and add more check in IsEmpty

### Pull Requests
- Merge pull request [#48](https://github.com/hydroan/gst/issues/48) from hydroan/dev


<a name="v0.10.0-beta.0"></a>
## [v0.10.0-beta.0] - 2025-11-05
### Build
- add module/helloworld tests to Makefile

### Chore
- **cursor:** update cursor projct rules
- **model:** make sure `Any` type as implementing types.Model interface
- **model:** remove unused RegisterRouters
- **plugin:** remove unsed pretty import from hellowold test
- **release:** generate CHANGEME.md
- **test:** remove unused comment

### Docs
- update README.md contains module interface
- **module:** update docs

### Feat
- Makefile add `install` to install `gg` command
- Makefile add `format`
- Makefile add `fix` to improve code quality
- Makefile add `test` and `testv` to run unit test
- add Makefile with code quality checks
- **bootstrap:** improve initialization flow with database synchronization
- **database:** add Wait function for database migration/initialization synchronization
- **model:** support register models and records at anytime
- **module:** add LoginLog, OperationLog modules
- **plugin:** support plugin system now

### Perf
- **database:** remove table creation mutex

### Refactor
- rename plugin system to module system
- **module:** rename moudle bundle from `logger` -> `logmgmt`

### Style
- **golangci-lint:** resolve issues
- **modernize:** reflect.Ptr -> reflect.Pointer
- **shadow:** reslove shadow issues

### Test
- **client:** use sqlite as backend database

### Wip
- **plugin:** change `Plugin` interface: add method Route, Param, Pub, and add more docs
- **plugin:** prepare support plugin system

### Pull Requests
- Merge pull request [#47](https://github.com/hydroan/gst/issues/47) from hydroan/dev
- Merge pull request [#45](https://github.com/hydroan/gst/issues/45) from hydroan/dev

### BREAKING CHANGE

Plugin interface renamed to Module interface


<a name="v0.9.7-beta.4"></a>
## [v0.9.7-beta.4] - 2025-10-31

<a name="v0.9.7"></a>
## [v0.9.7] - 2025-10-31
### Chore
- **release:** generate CHANGEME.md
- **release:** generate CHANGEME.md

### Enh
- **logger:** enhance GormLogger trace ID extraction for nil DatabaseContext scenarios

### Fix
- **tracing:** ensure trace_id in database logs via OTEL fallback

### Style
- **gofumpt:** gofumpt -l -w .

### Pull Requests
- Merge pull request [#44](https://github.com/hydroan/gst/issues/44) from hydroan/dev


<a name="v0.9.7-beta.3"></a>
## [v0.9.7-beta.3] - 2025-10-30
### Chore
- **docs:** update README.md
- **release:** generate CHANGEME.md

### Feat
- enhance WithQuery method with QueryConfig for better safety and flexibility

### Refactor
- **database:** remove WithQueryRaw method and integrate raw query into QueryConfig

### Pull Requests
- Merge pull request [#43](https://github.com/hydroan/gst/issues/43) from hydroan/dev

### BREAKING CHANGE

WithQueryRaw method has been removed. Use WithQuery with QueryConfig{RawQuery: "...", RawQueryArgs: [...]} instead.

WithQuery method signature changed from WithQuery(query M, fuzzyMatch ...bool) to WithQuery(query M, config ...QueryConfig)


<a name="v0.9.7-beta.2"></a>
## [v0.9.7-beta.2] - 2025-10-29
### Chore
- just format code

### Feat
- **gg:** add subcommand `watch` to watch model directory changes and automatically generates code

### Fix
- **database:** prevent data loss from empty query conditions

### Refactor
- **codegen:** extract ast strings into constants

### Style
- **gofumpt:** gofumpt -l -w .
- **golangci-lint:** resolve issues

### Pull Requests
- Merge pull request [#42](https://github.com/hydroan/gst/issues/42) from hydroan/dev
- Merge pull request [#41](https://github.com/hydroan/gst/issues/41) from hydroan/dev


<a name="v0.9.7-beta.1"></a>
## [v0.9.7-beta.1] - 2025-10-29
### Chore
- **release:** generate CHANGEME.md

### Fix
- **model:** make model.Any implements types.Model interfaces


<a name="v0.9.7-beta.0"></a>
## [v0.9.7-beta.0] - 2025-10-29
### Chore
- **codegen:** update comments

### Feat
- **model:** add model.Any used to database transactions

### Pull Requests
- Merge pull request [#40](https://github.com/hydroan/gst/issues/40) from hydroan/dev


<a name="v0.9.6"></a>
## [v0.9.6] - 2025-10-29
### Chore
- **release:** generate CHANGEME.md
- **release:** generate CHANGEME.md

### Docs
- **task:** deprecated task package in favor of cronjob

### Feat
- **controller:** add configurable request/response logging with zap
- **cronjob:** cronjob support run immediately

### Style
- **golangci-lint:** resolve issues

### Pull Requests
- Merge pull request [#39](https://github.com/hydroan/gst/issues/39) from hydroan/dev
- Merge pull request [#38](https://github.com/hydroan/gst/issues/38) from hydroan/dev
- Merge pull request [#37](https://github.com/hydroan/gst/issues/37) from hydroan/dev


<a name="v0.9.6-beta.4"></a>
## [v0.9.6-beta.4] - 2025-10-27
### Chore
- add .cursor
- **release:** generate CHANGEME.md

### Feat
- **model:** add Purge() method to support configurable delete behavior

### Fix
- **database:** set database.enablePurge in prepare to make sure support delete resources permantly
- **database:** fix nil pointer panic when calling Purge() in reset()

### Refactor
- **database:** use pointer type for enablePurge to support priority control

### Pull Requests
- Merge pull request [#36](https://github.com/hydroan/gst/issues/36) from hydroan/dev


<a name="v0.9.6-beta.3"></a>
## [v0.9.6-beta.3] - 2025-10-23
### Chore
- **release:** generate CHANGEME.md

### Feat
- add authentication marker middleware and simplify auth logic

### Pull Requests
- Merge pull request [#35](https://github.com/hydroan/gst/issues/35) from hydroan/dev


<a name="v0.9.6-beta.2"></a>
## [v0.9.6-beta.2] - 2025-10-21
### Enh
- **config:** enhance AppInfo with build metadata and runtime details

### Refactor
- **config:** rename server mode constants to remove prefix "Mode" and add "Local" mode

### Pull Requests
- Merge pull request [#34](https://github.com/hydroan/gst/issues/34) from hydroan/dev


<a name="v0.9.6-beta.1"></a>
## [v0.9.6-beta.1] - 2025-10-19
### Chore
- **release:** generate CHANGEME.md

### Docs
- **database:** update method comments

### Refactor
- **database:** update TransactionFunc to use generic `any` for tx parameter

### Pull Requests
- Merge pull request [#33](https://github.com/hydroan/gst/issues/33) from hydroan/dev


<a name="v0.9.6-beta.0"></a>
## [v0.9.6-beta.0] - 2025-10-19
### Chore
- **database:** relocate TransactionFunc 2
- **database:** relocate TransactionFunc to end of file for better organization
- **release:** generate CHANGEME.md
- **release:** generate CHANGEME.md

### Enh
- **database:** add typed LockMode for safer row-level locking in WithLock
- **database:** extend WithIndex to support hint mode and update associated docs
- **database:** add TransactionFunc and manual rollback support
- **database:** enhance WithTimeRange to support flexible time filters

### Feat
- **database:** add WithTx to allow sharing an existing transaction across multile resource types

### Refactor
- rename interface DatabaseOption[M] method WithScope to WithPagination and update associated docs and test cases

### Pull Requests
- Merge pull request [#32](https://github.com/hydroan/gst/issues/32) from hydroan/dev


<a name="v0.9.5"></a>
## [v0.9.5] - 2025-10-16
### Chore
- **release:** generate CHANGEME.md

### Fix
- **controller:** If the `REQ` type is not pointer to struct, will cause panic 2
- **controller:** If the `REQ` type is not pointer to struct, will cause panic

### Pull Requests
- Merge pull request [#31](https://github.com/hydroan/gst/issues/31) from hydroan/dev


<a name="v0.9.4"></a>
## [v0.9.4] - 2025-10-16
### Fix
- **client:** adjust `Update` and `Patch` behaviour to conform the standard RESTful API
- **controller:** record the correct table name

### Refactor
- **types:** remove HTTPVerb: Most,MostBatch,All

### Pull Requests
- Merge pull request [#30](https://github.com/hydroan/gst/issues/30) from hydroan/dev


<a name="v0.9.3"></a>
## [v0.9.3] - 2025-10-14
### Chore
- **release:** generate CHANGEME.md

### Fix
- **config:** fix setDfault() for otel

### Pull Requests
- Merge pull request [#29](https://github.com/hydroan/gst/issues/29) from hydroan/dev


<a name="v0.9.2"></a>
## [v0.9.2] - 2025-10-12
### Chore
- **release:** generate CHANGEME.md

### Feat
- prepare support audit manager in controller layer

### Fix
- **deps:** downgrade redis version in examples/demo

### Style
- **golangci-lint:** resolve issues
- **shadow:** reslove shadow issues

### Pull Requests
- Merge pull request [#28](https://github.com/hydroan/gst/issues/28) from hydroan/dev


<a name="v0.9.1-beta.2"></a>
## [v0.9.1-beta.2] - 2025-10-10
### Chore
- generate CHANGELOG.md
- update examples/demo
- **deps:** upgrade dependencies to latest version
- **deps:** downgrade github.com/redis/go-redis/v9 v9.15.0 -> v9.14.0
- **deps:** add tool `nilness`
- **deps:** add tool `shadow`
- **openapigen:** comment out unreachable code

### Ci
- add code quality checks to GitHub workflow
- golangci-lint add godoclint
- golangci-lint ignore staticcheck ST1001
- exclude staticcheck QF1008
- remove complex, noctx, unused check
- add .golangci.yml
- **lint:** ignore nilerr check

### Fix
- **debug:** resolve golangci-lint gosec issues

### Refactor
- **config:** change `Save` param: string -> io.Writer
- **model:** rename model 2
- **model:** rename model
- **model:** rename model_log -> modellog
- **model:** rename model_authz -> modelauthz
- **service:** rename package service_log -> servicelog
- **service:** rename package service_authz -> serviceauthz
- **util:** replace "go-ping/ping" by "prometheus-community/pro-bing"

### Style
- resolve shadow declarations issues
- apply golangci-lint fix
- resolve golangci-lint issues
- fix shadows declaration
- resolve shadow declarations issues
- **authn:** resolve golangci-lint staticcheck ST1003 issues
- **authz:** resolve golangci-lint issues: errcheck, gosec
- **bootstrap:** resolve golangci-lint issues: errcheck
- **cache:** modernize code style: replace reflect.TypeOf by reflect.TypeFor
- **cache:** resolve golangci-lint issues: errcheck
- **client:** resolve golangci-lint issues: staticcheck
- **cmd:** golangci-lint fix errorlint
- **cmd:** modernize code style
- **cmd:** fix shadow declarations issues
- **config:** modernize code style
- **config:** resolve golangci-lint issues
- **controller:** modernize code style
- **controller:** resolve golangci-lint checked issues
- **controller:** apply golangci-lint errcheck
- **database:** modernize code style
- **database:** resolve golangci-lint checked issues
- **dcache:** resolve golangci-lint checked issues
- **dcache:** modernize code style
- **ds:** resolve golangci-lint issues
- **ds:** resolve golangci-lint issues
- **ds:** modernize code style
- **internal:** resolve golangci-lint issues
- **logger:** modernize code style
- **logger:** resolve golangci-lint issues
- **metrics:** resolve golangci-lint issues
- **middleware:** resolve golangci-lint issues
- **middleware:** modernize code style
- **model:** resolve golangci-lint issues
- **model:** modernize code style
- **model:** resolve golangci-lint issues
- **pkg:** modernize code style
- **pkg:** resolve golangci-lint issues
- **provider:** resolve golangci-lint issues
- **provider:** modernize code style
- **response:** resolve golangci-lint issues
- **service:** resolve golangci-lint issues
- **task:** resolve golangci-lint checked issues
- **types:** modernize code style
- **types:** resolve golangci-lint issues
- **util:** resolve golangci-lint issues
- **util:** modernize code style

### Test
- **database:** TestUser.UpdateBefore add param: *types.ModelContext

### Pull Requests
- Merge pull request [#27](https://github.com/hydroan/gst/issues/27) from hydroan/dev


<a name="v0.9.1-beta.1"></a>
## [v0.9.1-beta.1] - 2025-10-08
### Chore
- generate CHANGELOG.md
- add jaeger and uptrace docker-compose.yml
- update examples/demo
- **jaeger:** update jaeger logging

### Fix
- **jaeger:** replace deprecated race.NewNoopTracerProvider with noop.NewTracerProvider

### Refactor
- migrate from Jaeger to OpenTelemetry (OTEL) tracing
- **jaeger:** support stand otlp-http and otlp-grpc and unsupport jaeger endpoint

### Pull Requests
- Merge pull request [#25](https://github.com/hydroan/gst/issues/25) from hydroan/dev

### BREAKING CHANGE

Configuration field names changed from Jaeger to OTEL


<a name="v0.9.1"></a>
## [v0.9.1] - 2025-10-05
### Chore
- generate CHANGELOG.md
- update README.md

### Refactor
- **controller:** set the default query size to 1000 in ListFactory
- **database:** remove the query limit, previous is 1000

### Pull Requests
- Merge pull request [#24](https://github.com/hydroan/gst/issues/24) from hydroan/dev


<a name="v0.9.0"></a>
## [v0.9.0] - 2025-10-01
### Chore
- **dcache:** remove redis.Range
- **deps:** upgrade dependencies to latest version
- **router:** disabledisable proxyUrl configuration in scalar template
- **test:** format dcache test code

### Feat
- prepare support distributed cache
- prepare support db migration
- **logger:** add dcache logger

### Opt
- replace package "error" to "github.com/cockroachdb/errors"

### Refactor
- rename framework name from golib to gst
- **database:** simplify table creation logic in InitDatabase
- **database:** simplify table creation logic in InitDatabase
- **dcache:** move Cache and DistributedCache interfaces to types package
- **dcache:** integrate config/logger and improve error handling

### Reverts
- Merge pull request [#18](https://github.com/hydroan/gst/issues/18) from hydroan/dbmigrate

### Pull Requests
- Merge pull request [#22](https://github.com/hydroan/gst/issues/22) from hydroan/distributed-cache
- Merge pull request [#21](https://github.com/hydroan/gst/issues/21) from hydroan/dev
- Merge pull request [#18](https://github.com/hydroan/gst/issues/18) from hydroan/dbmigrate


<a name="v0.8.0"></a>
## [v0.8.0] - 2025-09-21
### Chore
- update CHANGELOG.md
- update CHANGELOG.md
- update CHANGELOG.md
- **deps:** upgrade dependencies to latest version

### Docs
- **types:** correct interface Cache method `WithContext` comment

### Enh
- **database:** make AuthMigrate error message more descriptive

### Fix
- **controller:** correct propagate controller span context into service layer
- **database:** custom table name has more priority than default table name in database.Get
- **database:** remove the redundant id query in database.Get
- **database:** fix span context propagation to model hooks
- **dsl:** parse custom Import and Export operation in Route keyword domain

### Refactor
- **database:** change WithSelect default behavior: no columns provides will use defaultColumns
- **task:** upgrade github.com/shirou/gopsutil from v3 to v4

### Pull Requests
- Merge pull request [#16](https://github.com/hydroan/gst/issues/16) from hydroan/dev
- Merge pull request [#15](https://github.com/hydroan/gst/issues/15) from hydroan/dev
- Merge pull request [#14](https://github.com/hydroan/gst/issues/14) from hydroan/dev


<a name="v0.8.0-beta.1"></a>
## [v0.8.0-beta.1] - 2025-09-13
### Chore
- update examples/demo
- **deps:** upgrade dependencies to latest version

### Enh
- **controller:** controller span add "file" and "line"

### Feat
- **cache:** refactor tracing architecture and fix span context propagation
- **controller:** fix span context propagation and enhance tracing architecture
- **database:** add tracing for model lifecycle hooks
- **middleware:** add automatic tracing for registered middlewares
- **tracing:** integrate Jaeger distributed tracing across framework

### Fix
- **controller:** correct span relation of "Service XXXBefore hook", "Database", "Service XXXAfter hook" to sibling
- **database:** propagate trace context from database into cache operations

### Refactor
- **cache:** unify cache error handling + add tracing context support
- **types:** extrace context types into dedicated file "types/context.go"
- **types:** encapsulate context in ServiceContext and add method Context() to returns the internal context


<a name="v0.7.5"></a>
## [v0.7.5] - 2025-09-09
### Chg
- **database:** switch to idiomatic DatabaseContext.Context()

### Chore
- update CHANGELOG.md
- update examples/demo

### Docs
- **codegen:** correct comment for genServiceMethod1 phases
- **database:** improve API documentation for database manipulator

### Enh
- **database:** add and reuse ErrIDRequired in Get

### Feat
- **dsl:** normalize Endpoint format by stripping/transforming slashes
- **types:** ServiceContext add method DatabaseContext(); change NewGormContext -> DatabaseContext.Context()

### Fix
- **controller:** prevent reflect panic in patchValue when handling nil pointers
- **dsl:** extends Design.Range to internal `routes` to iterates additional routes
- **openapigen:** add fine-grained mutex protection for schema modifications in removeFieldsFromRequestBody
- **openapigen:** add mutex protection for concurrent access to global doc vairable

### Refactor
- **database:** make ctx parameter required in Database function

### Test
- **database:** add comprehensive test suite with unit and benchmark tests

### Pull Requests
- Merge pull request [#15](https://github.com/hydroan/gst/issues/15) from hydroan/dev


<a name="v0.7.4"></a>
## [v0.7.4] - 2025-09-04
### Chore
- update CHANGELOG.md
- update examples/demo
- **dsl:** cleanup unnsed comment-out code
- **style:** format code use `gofumpt`

### Feat
- **dsl:** add Route keyword for alternative API endpoints

### Fix
- **router:** remove middleware Gzip to fix error: "cannot write message to writer during serve error: flate: closed writer"

### Perf
- **database:** avoid redundant id condition query by leveraging gorm v2 primary key recognition

### Refactor
- **task:** extract cross-platform process stats into OS-specific files

### Pull Requests
- Merge pull request [#14](https://github.com/hydroan/gst/issues/14) from hydroan/dev


<a name="v0.7.3"></a>
## [v0.7.3] - 2025-09-02
### Chg
- **response:** rename CodeNotFoundRouteID -> CodeNotFoundRouteParam

### Chore
- update CHANGELOG.md
- update CHANGELOG.md
- update examples/demo
- update examples/demo
- **config:** remove Wokao

### Ci
- install gg CLI from local source instead of remote build
- install gg CLI from module path instead of local build

### Feat
- **codegen:** generate Filter and FilterRaw service methods
- **types:** add `filter_raw` phase
- **types:** extends ControllerConfig with ParamName field

### Fix
- **model:** hide `DeleteAt` field from JSON output
- **openapi:** generate OpenAPI path parameters dynamically

### Refactor
- **codegen:** statement `router.Register` add param `*types.ControllerConfig`
- **controller:** support configurable route parameter names in factories
- **dsl:** export model detection helpers for broader reuse
- **gen:** remove unused performArchitectureCheck stub
- **router:** remove RegisterWithConfig, Register add param `*types.ControllerConfig`

### Pull Requests
- Merge pull request [#13](https://github.com/hydroan/gst/issues/13) from hydroan/dev
- Merge pull request [#12](https://github.com/hydroan/gst/issues/12) from hydroan/dev


<a name="v0.7.2"></a>
## [v0.7.2] - 2025-08-30
### Chore
- update examples/demo
- add example/demo
- remove example/bench, example/simple, example/myproject
- remove example/demo
- ignore `.trae` file
- **codegen:** just change style
- **codegen:** ensure provider directory is tracked in new project scaffolding
- **codegen:** ensure dao directory is tracked in new project scaffolding

### Ci
- migrate gitguardian config
- add GitGuardian config to ignore tests and examples
- extend Go workflow with caching and demo project generation
- make sure the generated project by command `gg` compiled successfully

### Docs
- update CHANGELOG.md
- update docs
- **cache:** clarify cache initialization and expiration semantics
- **dsl:** improve Param and Public documentation for clarity and REST best practices
- **types:** improve interface documentation

### Feat
- **cache:** add `ExpirableCache` returns the `fastcache` Cache
- **dsl:** add support dynamic router paramters via `Param`
- **openapigen:** support struct-level comments for OpenAPI summary/description

### Fix
- **codegen:** use TrimSuffix instead of TrimRight when removing `.go` extension
- **config:** remove default MySQL password for security
- **openapigen:** filter dynamic route parameters from summary names
- **ristretto:** honor per-entry TTL when setting cache values

### Perf
- **router:** make openapi spec updates async during route registration
- **router:** make openapi spec updates async during route registration

### Refactor
- **bigcache:** remove unused defaultExpire and increase entry size limit
- **codegen:** simplify configx/cronjob initialization via side-effect import
- **codegen:** rename `cronjobx` -> `cronjob` and `middlewarex` -> `middleware`
- **openapigen:** endpoint segment `:id`, `/batch`, `/import` and `/export` generated by codegen command `gg`
- **openapigen:** refine summary path normalization for OpenAPI operations
- **openapigen:** improve summary naming with path + HTTP verb
- **openapigen:** enhance summary generator with path-based identifiers
- **router:** endpoint segment `:id`, `/batch`, `/import` and `/export` generated by codegen command `gg`

### Pull Requests
- Merge pull request [#11](https://github.com/hydroan/gst/issues/11) from hydroan/dev


<a name="v0.7.1"></a>
## [v0.7.1] - 2025-08-27
### Chore
- update CHANGELOG.md
- update CHANGELOG.md
- update examples
- gofumpt format
- apply gofumpt formatting fixes across repo
- apply gofumpt formatting fixes across repo
- update examples
- **codegen:** correct initialization order in generated main
- **deps:** downgrade `github.com/scylladb/gocqlx/v3` from v3.0.3 -> v3.0.1
- **deps:** upgrade dependencies to latest version
- **router:** remove debug logging from Init
- **service:** chang warn -> debug when service not found

### Ci
- add gofumpt formatting check to workflow

### Docs
- **dsl:** improve DSL documentation and commentds
- **model:** add detailed documentation for `Empty` marker type

### Feat
- **consts:** add Phase.Filename helper with tests
- **docs:** add Stoplight Elements UI
- **docs:** add Scalar API Reference UI
- **dsl:** support `Public` flag in action design
- **dsl:** add IsEmpty flag to Design for model.Empty detection
- **middlware:** add function Register to register global middlewares, RegisterAuth to register auth middlewares
- **types:** ServiceContext add Cookie()

### Fix
- **gen:** use Phase.Filename for valid service file detection
- **middleware:** dynamically set allowed origin in cors
- **model:** improve IsModelEmpty to handle empty structs
- **model:** correct `AreTypesEqual` semantics for `Empty` models
- **openapi:** skip schema registration for empty models
- **openapi:** ensure models are always registered in Components.Schemas
- **openapigen:** remove system fields from request bodies
- **openapigen:** if struct only has model.Empty field, skip generate request doc

### Perf
- **openapi:** use compact JSON for OpenAPI doc response
- **openapigen:** add concurrency safety and async schema registration

### Refactor
- **codegen:** switch middlewarex to side-effect import
- **config:** simplify Register/Get API, auto-drive config name, default to the lowcase name of struct name
- **model:** remove reflection-based request/response helpers
- **model:** implement `Empty` methods on value receiver
- **openapi:** improve schema handling for request/response types
- **openapi:** centralize schema registration with registerSchema
- **openapi:** introduce reusable request/response components
- **openapi:** generalize OpenAPI generators to accept REQ/RSP generics
- **openapigen:** simplify response spec and tag generation
- **openapigen:** make newRequestBody/newResponses generic and skip empty models
- **openapigen:** unify schema processing for requests & responses
- **openapigen:** simplify schema enrichment & field removal
- **router:** simplify OpenAPI and docs endpoints
- **router:** split API into Auth and Pub groups with middleware support
- **types:** enhance ServiceContext with Writer and cookie support
- **types:** inline helper context constructors into `types` package

### Pull Requests
- Merge pull request [#10](https://github.com/hydroan/gst/issues/10) from hydroan/dev
- Merge pull request [#9](https://github.com/hydroan/gst/issues/9) from hydroan/dev
- Merge pull request [#8](https://github.com/hydroan/gst/issues/8) from hydroan/dev
- Merge pull request [#6](https://github.com/hydroan/gst/issues/6) from hydroan/dev
- Merge pull request [#5](https://github.com/hydroan/gst/issues/5) from hydroan/dev


<a name="v0.7.0"></a>
## [v0.7.0] - 2025-08-20

<a name="v0.7.0-beta.3"></a>
## [v0.7.0-beta.3] - 2025-08-20
### Chore
- update CHANGELOG.md
- **model:** remove zap debug logging from `setID`
- **model:** assert `Empty` implements `types.Model`

### Codegen
- update testcase

### Feat
- **bootstrap:** add execution time logging for init functions
- **codegen:** support `model.Empty` in model directory
- **codegen:** support pointer and non-pointer payload/result types in service generation
- **dsl:**  add support for `model.Empty` marker structs

### Fix
- **dsl:** ensure default payload/result use pointer type & update tests

### Model
- remove test about HasRequest, HasResponse

### Refactor
- **codegen:** centralize "Code generated" comment in consts, add the "Code generated" comment in the generated file by subcommand "new"
- **dsl:** improve action parsing with pointer support and unify defaults
- **model:** redefine `Empty` as non-persistent model with inert methods
- **router:** disable automatic DB migrations during router registration


<a name="v0.7.0-beta.2"></a>
## [v0.7.0-beta.2] - 2025-08-15
### Controller
- add column.QueryColumns function

### Feat
- **config:** add config AppInfo
- **dsl:** add Service flag to control service code generation per action
- **dsl:** add Migrate() to determinate if migrate to database

### Fix
- preserve comment positions during code generation
- **codegen:** If the Init() function body is empty, it will not import any packages, affect router.go, service.go
- **dsl:** Migrate parameter type is bool
- **model:** correct Base struct soft delete field name

### Openapi3gen
- change the openapi3 api tags

### Openapigen
- read project info from config

### Refactor
- **config:** unify framework name usage via consts.FrameworkName
- **dsl:** change Migrate default to false

### Types
- remove field: request,response, add feield: GinContext

### Pull Requests
- Merge pull request [#4](https://github.com/hydroan/gst/issues/4) from hydroan/cmd/gg


<a name="v0.7.0-beta.1"></a>
## [v0.7.0-beta.1] - 2025-08-12
### Chore
- update CHANGELOG.md
- update examples
- remove comment
- **codegen:** remove ununsed testcode
- **codegen:** add testcode
- **example:** update to support custom REQ and RSP in generic type parameters

### Cmd
- **gg:** remove subcommand apply
- **gg:** add option --router

### Codegen
- ModeInfo add field: ModelFilePath

### Docs
- update README.md about service interface
- **codegen:** format service method shape examples in GoDoc style
- **docs:** fix client package link in READMD.md
- **docs:** update CHANGELOG.md
- **model:** add descriptive comments to Base struct fields

### Feat
- add Enabled flag to DSL Action for fine-grained API control
- add AreTypesEqual utility for generic type comparsion
- integrate DSL design parsing into code generation pipeline
- **codegen:** add ApplyServiceFile and ApplyServiceMethod* for updating generated service methods
- **codegen:** add ServiceMethod4 and support CRUD phase-based service generation
- **codegen:** include model imports and HTTP verb in generated router file
- **codegen:** add apply package for codegen
- **codegen:** enhance ApplyServiceFile to update service.Base generics
- **codegen:** add type declarations for all enabled DSL actions in GenerateService
- **codegen:** name return values in ServiceMethod4 and adjust generator/tests
- **codegen:** support generate Import,Export
- **codegen:** add service method shape recognition helpers and tests
- **codegen:** add function BuildRouterFile to generate router/router.go
- **codegen:** generate model/model.go
- **dsl:** add Import,Export action
- **dsl:** dsl parser
- **dsl:** add Endpoint, Enabled
- **dsl:** prepare support dsl
- **model:** add soft delete support to Base struct
- **router:** pass HTTP verb from Phase to StmtRouterRegister
- **service:** add phase-aware service registration and retrieval

### Fix
- if action enabled, the Payload and Result default to the model name
- **codegen:** update Import and Export return statements
- **codegen:** Import should add import "io"
- **controller:** correct reflect.New usage for non-pointer REQ types
- **controller:** correct error variable usage in UpdateManyFactory and PatchManyFactory
- **router:** correct route map keys for batch/import/export endpoints
- **service:** set logger during Register when available
- **types:** MethodName for Import,Export

### Refactor
- implement dual-mode processing for PatchFactory with comprehensive docs
- **codegen:** seperate the code about generate code into package "internal/codegen/gen"
- **codegen:** make some function to public
- **codegen:** fix ModelPkg2ServicePkg; remove Main; public MethodAddComments,FormatNode
- **codegen:** update service method generation for Many naming convention
- **controller:** implement dual-mode processing for UpdateManyFactory with comprehensive docs
- **controller:** update client test and enhance CreateFactory documentation
- **controller:** implement dual-mode processing for CreateManyFactory with comprehensive docs
- **controller:** implement dual-mode processing for GetFactory with comprehensive docs
- **controller:** implement dual-mode processing for ListFactory with comprehensive docs
- **controller:** implement dual-mode processing for PatchFactory
- **controller:** implement dual-mode processing for UpdateFactory with comprehensive docs
- **controller:** implement dual-mode processing for UpdateFactory
- **controller:** implement dual-mode processing for DeleteFactory with comprehensive docs
- **controller:** implement dual-mode processing for UpdateManyFactory with comprehensive docs
- **controller:** implement dual-mode processing for DeleteFactory
- **controller:** implement dual-mode processing for CreateFactory
- **controller:** implement dual-mode processing for PatchManyFactory with comprehensive docs
- **dsl:** Payload(any) -> Payload[T any](); Result(any) -> Result[T any]()
- **model:** extrace GormTime and GormStrings to separate datatype file
- **model:** remove gorm.Model embedding from Base struct
- **service:** update authz services to support custom request and response in type parameters

### Test
- **codegen:** convert tests to use internal package and unexport helpers directly
- **codegen:** enhance ApplyServiceFile tests to assert formatted output

### Util
- add Uniq, Keys, Values

### Pull Requests
- Merge pull request [#3](https://github.com/hydroan/gst/issues/3) from hydroan/cmd/gg

### BREAKING CHANGE

CreateManyFactory behavior now depends on type parameter equality

When all three generic types are identical, the factory provides automatic
batch database creation with service hooks. When types differ, it delegates
full control to the service layer for custom batch creation logic.

GetFactory behavior now depends on type parameter equality

When all three generic types are identical, the factory provides automatic
resource retrieval with rich query features. When types differ, it delegates
full control to the service layer for custom retrieval logic.

ListFactory behavior now depends on type parameter equality

When all three generic types are identical, the factory provides automatic
database listing with rich query features. When types differ, it delegates
full control to the service layer for custom listing logic.

PatchFactory behavior now depends on type parameter equality

When all three generic types are identical, the factory provides automatic
partial database updates with field-level merging. When types differ, it
delegates full control to the service layer for custom patch logic.

UpdateFactory behavior now depends on type parameter equality

When all three generic types are identical, the factory provides automatic
database operations and service hooks. When types differ, it delegates
full control to the service layer for custom update logic.

DeleteFactory behavior now depends on type parameter equality

When all three generic types are identical, the factory provides automatic
database operations and service hooks. When types differ, it delegates
full control to the service layer for custom deletion logic.

Service interface now requires REQ and RSP type parameters

- Add Request and Response interface types to support custom request/response handling
- Update Service interface to use generic types: Service[M Model, REQ Request, RSP Response]
- Add primary service methods (Create, Delete, Update, Patch, List, Get, CreateMany, etc.) that return (RSP, error)
- Update Base service implementation to support new generic signature
- Modify Register and Factory functions to accept REQ/RSP type parameters
- Maintain existing hook methods (*Before/*After) alongside new primary methods

This change enables type-safe custom request/response handling while preserving
backward compatibility through hook methods. Services can now define their own
request/response types instead of relying on the default model types.

Migration: Update service implementations to specify REQ and RSP types:
- Old: Service[MyModel]
- New: Service[MyModel, MyRequest, MyResponse]

- Rename UpdatePartial/update_partial to Patch/patch across client, controller, and service layers
- Rename Batch* methods to *Many (BatchCreate -> CreateMany, BatchDelete -> DeleteMany, etc.)
- Update operation log types and phase constants to match new naming
- Update OpenAPI generation to use new method names
- Modify service interface method signatures for consistency


<a name="v0.6.2"></a>
## [v0.6.2] - 2025-07-28
### Docs
- update CHANGELOG.md with recent changes

### Feat
- **client:** client support BatchCreate, BatchDelete, BatchUpdate, BatchUpdatePartial

### Refactor
- **client:** client operation returns *Resp, bofore is []byte
- **client:** remove ListRaw, GetRaw
- **client:** move package: pkg/client -> client

### Test
- **database:** add test and benchmark case


<a name="v0.6.1"></a>
## [v0.6.1] - 2025-07-28
### Chore
- **codegen:** remove comments

### Docs
- update README.md
- generate CHANGELOG.md

### Feat
- **openapigen:** parse struct fields doc use go ast

### Fix
- **codegen:** add proper pluralization for variable names
- **codegen:** resolve package import conflicts in service generation

### Pref
- **service:** cache service instances in controller handlers

### Refactor
- **openapigen:** rename addSchemaDescriptions -> addSchemaTitleDesc


<a name="v0.6.0"></a>
## [v0.6.0] - 2025-07-02
### Chore
- remove unused test model files
- **example:** update examples/demo

### Docs
- generate CHANGELOG.md

### Feat
- prepare support cmd/gg

### Refactor
- **codegen:** simplify service template comments


<a name="v0.5.2"></a>
## [v0.5.2] - 2025-07-01
### Chore
- update examples/myproject
- **deps:** upgrade dependencies to latest version
- **example:** update examples/simple
- **example:** update examples/demo
- **example:** update examples/myproject
- **openapigen:** remove debug print statements

### Docs
- generate CHANGELOG.md

### Feat
- **model:** add JSON unmarshalling for GormStrings type
- **model:** enhance TableColumn with lifecycle hooks
- **response:** introduce CodeInstance for flexible error code customization
- **router:** add MostBatch verb group for batch operations
- **util:** make FormatDurationSmart precision parameter optional

### Fix
- **config:** normalize config type name to lowercase
- **controller:** handle nil options in BatchCreate and BatchDelete operations
- **logger:** replace Warn with Warnz method in service factory
- **router:** correct OpenAPI endpoint for get operation

### Refactor
- util.FormatDurationSmart(time.Since(begin), 2) -> util.FormatDurationSmart(time.Since(begin))
- **controller:** change empty column name log level from warn to debug
- **controller:** remove redundant hooks after import
- **cronjob:** enhance duration formatting in logs
- **database:** improve struct field reflection with pointer unwrapping
- **router:** simplify registeration API with variadic verbs
- **task:** enhance duration formatting in logs


<a name="v0.5.1"></a>
## [v0.5.1] - 2025-05-05
### Chore
- change delay build in air config
- **cache:** rename implementation files to cache.go for consistency
- **docs:** update CHANGELOG.md
- **example:** update examples/myproject

### Docs
- update interface Cache docs

### Feat
- support cursor-based pagination
- **cache:** add cache ristretto
- **cache:** support go-cache
- **cache:** support ccache
- **cache:** support fastcache
- **controller:** use correct HTTP status code(201/204) in create and delete responses
- **response:** Code add method `WithStatus` to replace deafult http status
- **router:** add "/-/api/redoc" endpoint for Redoc API documentation
- **util:** add Marshal,Unmarshal

### Fix
- **cache:** ensure thread-safe cacheMap initialization with double-check locking

### Perf
- **cache:** replace json.Marshal/Unmarshal with util.Marshal/Unmarshal for faster Go base type serialization

### Test
- **cache:** split test case


<a name="v0.5.0"></a>
## [v0.5.0] - 2025-05-04
### Chore
- **controller:** add TODO comments for DeleteBefore and DeleteAfter hooks
- **deps:** upgrade dependencies to latest version
- **deps:** go mod tidy
- **docs:** update CHANGELOG.md
- **docs:** update CHANGELOG.md
- **example:** refactor(model): add json and schema tags to GroupRequest, GroupResponse
- **example:** update
- **example:** update
- **example:** update
- **examples:** update example myproject
- **examples:** update example myproject
- **types:** simplify parameter names in Model interface methods

### Docs
- update README.md

### Feat
- add package reflectmeta to cache reflect
- support custom request and response
- **cache:** add package cache/lrue and implements interface `Cache`
- **cache:** add package cache/freecache and implements interface `Cache`
- **cache:** add package cache/bigcache and implements interface `Cache`
- **cache:** add package cache/smap and implements interface `Cache`
- **cache:** add generic Cache[T]() shortcut using lrue backend
- **config:** increase default memcached.max_idle_conns to 100
- **config:** add cache config
- **controller:** trace and propagate service phase in ServiceContext
- **controller:** prepare support captcha
- **memcached:** provider/memcached implement interface `Cache`
- **model:** add `Empty` model and it is always invalid
- **openapigen:** support custom request and response
- **redis:** implement interface `Cache`
- **types:** add phase field and methods to ServiceContext with enhanced docs

### Fix
- **config:** avoid creating config file in test environment
- **config:** skip create temp dir during test
- **controller:** handle case when model has custom request but no custom response
- **redis:** do not log error for cache miss(redis.Nil)
- **reflectmeta:** use full type string in cache keys to avoid name collisions
- **router:** move oepnapigen.Set calls from Register* to register to fix api path in api docs
- **service:** user same service key
- **service:** use package path in service registration key to prevent collisions

### Refactor
- remove package pkg/bigcacheg
- **bootstrap:** boostrap cache, remove lru,cmap
- **cache:** change method: Remove -> Delete, Count -> Len; Set method add parameter ttl
- **database:** replace lru.Int64() with lru.Cache[int64]()
- **database:** replace lru with lrue that is a expirable lru cache
- **model:** operation log add feild `Request`,`Response` and auto create table
- **openapigen:** pass path argument to set* functions and tags generator
- **redis:** move redis package: cache/redis -> provider/redis
- **redis:** move redis package: cache/redis -> provider/redis
- **redis:** add shared redis.UniversalClient (cli) for flexible access
- **response:** change empty data representation from empty string to null
- **service:** use range
- **service:** change service hooks to handle single model for method: Create/Delete/Update/Update/UpdatePartial/Get before and after hooks
- **types:** rename Set/GetRequestBody Set/GetResponseBody -> Set/GetRequest Set/GetResponse

### Test
- **cache:** add parallel benchmark, benchmark redis and memcached
- **cache:** add benchmark testcase
- **model:** remove spew
- **redis:** correct import


<a name="v0.4.4"></a>
## [v0.4.4] - 2025-04-26
### Build
- add commitizen config
- add .air.toml

### Chg
- move setup permission ID and remark to model hook CreateBefore
- remove const "FileRbacConf"
- **authz:** basic authz remove "priority" in "policy_defination"
- **bootstrap:** boostrap service_authz, service_log
- **controller:** CreateFactory will handle empty request bodies and response 200
- **debug:** correct statsviz server log output timing
- **logger:** replace FormatDurationMilliseconds by FormatDurationSmart to format time.Duration
- **model:** setup user role id manually and update casbin_rule table when create user role succesfully
- **model:** same role name always has same id
- **model:** base model add field _notoal, its necessary for openapi generate
- **model:** table casbin_rule add field: `user` and `role` to record user and role info
- **model:** cleanup unused fields for casbin_rule
- **model:** change model user fields
- **router:** change api doc path: "/api/doc" -> "/-/api-doc"

### Chore
- **bootstrap:** correct typo in signal handling log message
- **deps:** upgrade dependencies to latest version
- **example:** upgrade golib to latest
- **example:** upgrade golib to latest
- **example:** update

### Doc
- **model:** my notes

### Docs
- generate CHANGELOG.md
- **controller:** update doc for User.Login

### Enh
- exclude ID field from OpenAPI example output
- **authz:** basic authz not depends on external rbac model file
- **controller:** improve error response with detailed error information
- **router:** automatic create table

### Feat
- support rbac system (no test)
- prepare support rbac foundations with `role`, `rolebinding` and `permission` models
- **authz:** prepare support tenant mode
- **config:** add server config for server router and circular buffer
- **consts:** add TAG_QUERY constant for query parameter tag handling
- **controller:** remove middleware `operation_log`; controller log the operation; fix UpdateFactory
- **logger:** add Casbin logger implementation
- **middleware:** add authorization logging
- **model:** add model validity check to exclude request/response types
- **service:** add package `service_log` for logger
- **service:** add package `service_authz` for rbac

### Fix
- openapi3 setupBatchExample panic cause by nil op
- openapi3 setupExample panic cause by nil op
- specify table name explicitly during AutoMigrate
- prevent overwriting existing paths in OpenAPI generator
- list resources API docs support query parameters that get from "scheme" tag
- **config:** always create tmp dir
- **controller:** prevent duplicate ID processing in delete handler
- **controller:** ensure consistent ID collection in DELETE handler
- **database:** handle unexported struct fields in structFieldToMap
- **database:** avoid obtained from unexported cuase panic
- **database:** handle models with empty ID during creation operation
- **middleware:** move authorization logging after enforcement decision

### Perf
- **database:** Create has more performance

### Refactor
- move rbac package to authn/rbac/basic directory and rename to "basic"
- move jwt package to authn directory
- **config:** move RBAC configuration from Server to Auth
- **controller:** rename package model -> model_log
- **controller:** rename package model -> model_log
- **logger:** standardize duratioin formatting to millseconds in logs
- **model:** operation_log add more OperationType for batch operation
- **model:** user add logger entries
- **model:** move rbac model to package `model_authz`
- **model:** move logger model to package `model_log`

### Style
- **middleware:** change logger style
- **model:** rename user-agent.go to user_agent.go


<a name="v0.4.3"></a>
## [v0.4.3] - 2025-04-20
### Chg
- **model:** use constant for ID field name
- **model:** make SetID function priviate

### Chore
- update example/myproject
- **deps:** upgrade dependencies to latest version

### Docs
- update CHANGELOG.md with recent changes
- add CHANGELOG.md and .chglog configuration

### Feat
- enhance project with OpenAPI support
- **config:** add pre-release and test server mode constants
- **controller:** enhance batch delete with "ids" support
- **logger:** add log entry "params", "query" for ControllerContext,ServiceContext,DatabaseContext
- **logger:** add router information to log context
- **middleware:** add RouteParams

### Fix
- **controller:** improve resource existence in GetFactory

### Refactor
- use modern go APIs(strings.SplitSeq and maps.Copy)
- **boostrap:** replace custom initFunc with func() error
- **controller:** use range-based loop syntax for numeric iterations

### Pull Requests
- Merge pull request [#2](https://github.com/hydroan/gst/issues/2) from hydroan/dev


<a name="v0.4.2"></a>
## [v0.4.2] - 2025-04-01
### Chg
- **config:** 1.Remove `DB` field from `Server` config and move it to `Database` as field `Type` 2.change config Sqlite.IsMemory default value to true
- **etcd:** replace etcd default loggeer by logger.Etcd
- **pprof:** manually control mutex and block profile rate

### Chore
- update examples
- update example/simple
- update example/demo
- go mod tidy
- update example/myproject
- **deps:** upgrade dependencies to latest version
- **deps:** go mod tidy
- **deps:** add protoc too dependencies in go.mod
- **logger:** move time encoder format to consts package
- **nats:** replace zap.() with logger.nats

### Feat
- **logger:** expose zap.Logger instance via ZapLogger() method
- **logger:** add Clean function ot ensure proper zap logger shutdown
- **provider:** add package provider/rockeetmq to support rocketmq
- **provider:** add package rethinkdb
- **provider:** add package scylla to support scylladb
- **provider:** support memcached.

### Fix
- **config:** add custom ini encoder in latest viper version

### Refactor
- **scylla:** simplify batch statements appending


<a name="v0.4.1"></a>
## [v0.4.1] - 2025-03-12
### Enh
- **bootstrap:** improve application lifecycle management

### Feat
- **middleware:** add Circuit Breaker middleware

### Fix
- **gops:** prevent gops agent capture signal and exit 1

### Refactor
- **grpc:** improve server lifecycle management
- **pprof:** improve server liftcycle management
- **router:** improve server lifecycle management
- **statsviz:** improve server liftcycle management


<a name="v0.4.0"></a>
## [v0.4.0] - 2025-03-11
### Chg
- **bootstrap:** bootstrap feishu
- **bootstrap:** bootstrap influxdb
- **bootstrap:** bootstrap grpc server
- **bootstrap:** bootstrap kafka,nats,etcd,cassandra
- **config:** rename enable_statsviz -> statsviz_enable, enable_pprof -> pprof_enable, enable_gops -> gops_enable
- **config:** change redis config: remove `host`,`port`, add `addr`,`addrs`,`cluster_mode`
- **config:** remove redis config field: `idle_timeout`, `max_conn_age`
- **controller:** Remove operation in ExportFactory, ImportFactory
- **logger:** remove Global,Internal,Job, add Cassandra,Etcd,Feishu,Influxdb,Kafka,Ldap,Minio,Nats
- **redis:** upgrade Redis client from go-redis/v8 to go-redis/v9

### Chore
- go mod tidy
- **deps:** go mod tidy
- **deps:** upgrade dependencies to latest version
- **deps:** go mod tidy
- **deps:** upgrade dependencies to latest version
- **example:** update examples/myproject
- **examples:** replace github.com/pkg/errors with github.com/cockroachdb/errors
- **examples:** update demo using latest golib
- **examples:** update example simple
- **examples:** add example/bench
- **examples:** add examples/myproject
- **examples:** update example myproject
- **examples:** update example myproject
- **examples:** update example myproject
- **logger:** clean up comments and improve function naming
- **logger:** rename initVar to readConf for better clarify
- **logger:** rename logger Visitor -> Runtime for better clarity
- **minio:** remove debug print statement
- **minio:** rename cli -> client
- **redis:** no-op
- **redis:** remove unused comment

### Docs
- **config:** update comment for setDefault method

### Enh
- **config:** update influxdb configuration
- **elastic:** improve Init and New elastic client
- **ldap:** enhance provider ldap
- **minio:** enhance provider/minio
- **redis:** support redis cluster mode
- **redis:** enhance redis configuration and security options

### Feat
- prepare support grpc
- support BatchCreate, BatchDelete, BatchUpdate, BatchUpdatePartial
- support grpc server
- prepare support grpc
- **client:** add ListRaw and GetRaw methods
- **mongo:** enhance MongoDB client configuration and connection handing
- **provider:** support cassandra
- **provider:** support influxdb
- **provider:** support influxdb
- **provider:** prepare support `influxdb`, `feishu`
- **provider:** support nats
- **provider:** support kafka
- **provider:** prepare support cassandra, etcd, kafka, nats
- **provider:** support etcd
- **provider:** support feishu
- **redis:** graceful shutdown for connection cleanup
- **task:** improve collect runtime metrics
- **util:** add TLS configuration builder function `BuildTLSConfig`

### Fix
- **boostrap:** RegisterExitHandler(cassandra.Close)
- **controller:** use consts package for parameter constants
- **elasticsearch:** check elasticsearch connection in Init, - change config field: Hosts: string -> Addrs []string
- **influxdb:** properly close client on health check failure
- **kafka:** properly close client if no available broker
- **middleware:** replace logger.Global with zap.S()
- **mongo:** prevent potential use of invalid client on connection failure
- **mqtt:** prevent potential use of invalid client on connection failure
- **nats:** properly close client on health check failure
- **provider:** RegisterExitHandler(etcd.Close)
- **redis:**  close client on connection failure to prevent resource leaks

### Perf
- **boostrap:** run handlers concurrently to improve cleanup performance
- **redis:** replace encoding/json with json-iterator for better performance, add benchmark test case

### Refactor
- reorganize cache components into `cache` directory. - move database/redis, lru, cmap into `cache` directory
- reorganize components into `provider` directory. - move elastic, ldap, minio, mongo, mqtt, minio to `provider` directory
- reorganize components into provider directory - move elastic, ldap, minio, mongo, mqtt, minio to 'provider' directory
- **bootstrap:** rename exit handlers to cleanup handlers for clarity
- **config:** split config structs into seperate files
- **config:** modularize configuration defaults and move global constants near to configuration struct
- **config:** simplify config struct names and standardize viper usage


<a name="v0.3.4"></a>
## [v0.3.4] - 2025-03-05
### Chg
- **controller:** createSession -> CreateSession

### Chore
- ignore docs
- **example:** update examples/myproject
- **example:** update example/myproject
- **example:** update examples/demo
- **example:** update examples/simple
- **router:** refine final shutdown log message

### Docs
- update README.md

### Enh
- **config:** support read custom config values from envrionment variables, the priority is: env var > config file > default values

### Feat
- support debug tools: "pprof","gops"
- support debug/statsviz
- **bootstrap:** optimize cpu utilization with automaxprocs
- **bootstrap:** add Run to boostrap server

### Fix
- **config:** support parse default for time.Duration
- **config:** correct statsviz listen address field name
- **debug:** improve pprof shutdown handing
- **debug:** improve gops shutdown handing
- **debug:** improve statsviz shutdown handing

### Refactor
- **boostrap:** replace channel with errgroup for concurrent initialization

### Style
- **router:** standardize server log message format


<a name="v0.3.3"></a>
## [v0.3.3] - 2025-03-03
### Chore
- **example:** update examples/myproject
- **examples:** update example myproject
- **logger:** update comment

### Enh
- wrap errors with stack/context for better debugging.
- **task:** support reigster task before or after bootstrap.Boostrap()

### Feat
- add package cronjob
- add package cronjob
- add package cronjob
- **config:** support for custom config registration and retrieval

### Fix
- **boostrap:** prevent multiple initialization on repeated calls
- **controller:** correct error formatting in logs


<a name="v0.3.2"></a>
## [v0.3.2] - 2025-02-26
### Chg
- **config:** remove automatic domain assignment base on mode
- **config:** rename config.Auth: TokenExpireDuration -> AccessTokenExpireDuration; add RefreshTokenExpireDuration
- **config.Auth:** rename NoneExpireUser -> NoneExpireUsername; NoneExpirePass -> NoneExpirePassword
- **config.Auth:** rename NoneExpireUser -> NoneExpireUsername; NoneExpirePass -> NoneExpirePassword
- **logger:** improve GORM slow query logging with configuable threshold
- **middleware:** delete RequestID middleware, add TraceID middleware
- **response:** remove some Code and the response data add field `request_id`
- **router:** replace RequestID by TraceID

### Chore
- update example/myproject
- update examples/myproject
- nnop
- update examples/myproject
- **deps:** upgrade dependencies to latest version

### Docs
- update README.md
- update README.md

### Enh
- **jwt:** enhance jwt token handling

### Feat
- database support clickhouse
- database support sql server
- propagate tracing context to database layer and gorm with logging support
- **config:** add Clickhouse configuration support
- **config:** add SQL Server configuration support
- **config:** add DatabaseConfig to configures sqlite/postgres/mysql connection
- **config:** add slow_query_threshold configuration for server
- **util:** add TraceID and SpanID generation functions

### Fix
- **config:** set default value for Config to support read config from environment.

### Opt
- **logger:** optimize logger "With" performance

### Refactor
- **binaryheap:** remove redundant cmp parmmeter in downMinHeap and downMaxHeap methods
- **database:** simplify batch processing logic using min()
- **errors:** replace std "errors" with "github.com/cockroachdb/errors" for better error handing.
- **errors:** replace github.com/pkg/errors with github.com/cockroachdb/errors for better error handing
- **jwt:** remove accessTokenCache and refreshTokenCache

### Test
- **splaytree:** add debug print statement in test


<a name="v0.3.1"></a>
## [v0.3.1] - 2025-02-16
### Chg
- **avltree:** update WithNodeFormatter signature
- **avltree:** WithNodeFormat(string) -> WithNodeFormatter(func(*Node[K,V])string)
- **rbtree:** update WithNodeFormatter signature
- **rbtree:** WithNodeFormat(string) -> WithNodeFormatter(func(*Node[K,V])string)
- **trie:** update WithNodeFormatter and WithKeyFormatter signatures

### Chore
- update ds/tree/READMD.md
- **binaryheap:** remove comments
- **deps:** upgrade dependencies to latest version

### Docs
- **binaryheap:** fix function comments for heap operations
- **circularbuffer:** change "NewFromSlice" comments
- **rbtree:** add comments for Inorder and Postorder traversal methods
- **trie:** add comments for WithNodeFomatter,WithKeyFormatter

### Feat
- **arraylist:** add NewFromOrderedSlice, rename NewWithOrderedElements -> NewOrdered
- **arraylist:** add NewWithOrderedElements
- **avltree:** add String method for tree visualization
- **ds:** add avltree implement in package ds/tree/avltree
- **ds:** add binary heap implement on package ds/heap/binaryheap
- **ds:** add priority queue implementation on package ds/queue/priorityqueue
- **ds:** add splay tree implement in package ds/tree/splaytree
- **ds:** add trie implementation on package ds/tree/trie
- **ds:** add circular buffer implementation in package ds/queue/circularbuffer
- **ds:** add read-black tree implement in package ds/tree/rbtree
- **ds:** add skip list implementation in package ds/list/skiplist
- **rbtree:** rename Inorder -> InorerChan, add Inorder; Preorder,Postorder,LevelOrder same like Inorder
- **rbtree:** add GetNode to retrieve tree node by key
- **types:** add ErrFuncNil for nil function error handling

### Fix
- **avltree:** fix data race condition in String()
- **avltree:** add nil check for traversal functions
- **rbtree:** fix data race condition in String()
- **rbtree:** pass options to NewWithOrderedKeys in NewFromMapWithOrderedKeys
- **rbtree:** check comparsion function in New
- **rbtree:** initial default FackLocker
- **splaytree:** add nil check for traversal functions
- **trie:** fix data race condition in String()

### Refactor
- **arrayqueue:** use IsEmpty() instead of Len() == 0 for clarity
- **avltree:** change the avltree's method return type: *Node[K,V] -> (K,V,bool)
- **ds:** centralize error variables in ds/types/errors.go
- **multimap:** replace EqualFn with cmp function for value comparsion
- **priorityqueue:** simplify Clone function by removing redundant variable
- **rbtree:** change the rbtree's method return type: *Node[K,V] -> (K,V,bool)
- **rbtree:** reuse New in NewWithOrderedKeys
- **splaytree:** rename iter -> fn in MarshalJSON for clarity

### Style
- **avltree:** simplify AVL tree constructor name
- **rbtree:** simplify red-black tree constructor name
- **splaytree:** simplify splay tree constructor name

### Test
- **avltree:** optimize benchmark tests
- **avltree:** refactor compartor usage and add TestAVLTree_String
- **rbtree:** optimize benchmark tests
- **rbtree:** adjust benchmark size from {100, 100000} to {10, 100000}

### Tests
- **circularbuffer:** add test case for json encoding


<a name="v0.3.0"></a>
## [v0.3.0] - 2025-01-29
### Chore
- rename ds/multimap/multimap_bechmark_test.go -> ds/multimap/multimap_benchmark_test.go
- **deps:** upgrade dependencies to latest version
- **linkedlist:** update comment
- **linkedlist:** clarify MergeSorted doc
- **mapset:** move MarshalJSON and UnmarshalJSON to set_encoding.go

### Docs
- **arraystack:** fix typo in NewFromMapValues
- **mapset:** fix typo in UnmarshalJSON commit

### Feat
- **arraylist:** add options method to clone arraylist properties
- **arraylist:** support concurrent safe.
- **arraystack:** support concurrency safety
- **arraystack:** add a stack based on arraylist
- **ds:** add package ds/mapset that implement datastructre "set"
- **ds:** add a queue based on linkedlist in package ds/queue/linkdqueue
- **ds:** add a queue based on array list in package ds/queue/arrayqueue
- **ds:** add a stack based on linkedlist in ds/stack/linkdstack
- **ds:** add linkedlist package under ds/list
- **ds:** add arraylist package under ds/list
- **linkedlist:** add options method to clone linkedlist properties
- **linkedstack:** support concurrency safe
- **mapset:** provides WithSorted to support makeup sorted internal element, affect method: `Slice`,`String`,`MarshalJSON`, `Range`, `Iter`
- **mapset:** support concurrent safe

### Fix
- **arraylist:** ensure the underlying array capacity is always greater than 0
- **arraystack:** not use new array stack to avoid sync.RWMutex leak
- **linkedlist:** call internal "pushBackNode" to avoid deadlock in concurrent mode
- **types:** correct spelling of FakeLocker, FackeLocker -> FakeLocker

### Refactor
- **arraylist:** replace paramater "equal" with "cmp" in List[E any]
- **arraylist:** replace manual slice construction with s.list.Values()
- **arraylist:** rename parameters: values -> elements, v -> e
- **arrayqueue:** simplify Queue initialization in New function
- **arraystack:** rename `slices` parameter to `slice` in NewFromSlice
- **ds:** move ds interface and types from types to dedicated package ds/types
- **linkedlist:** replace manual slice with s.list.Slice()
- **linkedlist:** rename `slices` to `slice` in benchmark tests
- **linkedqueue:** use IsEmpty() instead of Len() == 0 for clarity
- **linkedstack:** rename `slices` parameter to `slice` in NewFromSlice
- **mapset:** rename `slices` parameter to `slice` in NewFromSlice
- **mapset:** rename mapset.go -> set.go; rename mapset_test.go -> set_test.go
- **mapset:** rename file: set.go -> mapset.go; rename package: set -> mapset

### Style
- **arraylist:** rename type parameter T -> E

### Test
- **arraylist:** refactor arraylist benchmark test case
- **linkedlist:** rename test case name
- **linkedlist:** refactor becnhark test units
- **linkedlist:** refactor and improve benchmark tests

### Tests
- **mapset:** add unit tests for mapset

### Pull Requests
- Merge pull request [#1](https://github.com/hydroan/gst/issues/1) from hydroan/feat/ds


<a name="v0.2.3"></a>
## [v0.2.3] - 2025-01-18
### Chore
- **deps:** upgrade dependencies to latest version
- **examples:** update example myproject
- **examples:** update example demo
- **examples:** update example simple
- **router:** disply exit signal in shutdown log

### Enh
- **router:** enhance server with graceful shutdown handling

### Feat
- **config:** add configurations constants for environment variables
- **logger:** support custom console encoder for better log formatting

### Refactor
- **logger:** remove 'log_' prefix from logger config fields


<a name="v0.2.2"></a>
## [v0.2.2] - 2025-01-07
### Refactor
- move context conversion functions to types/helper package


<a name="v0.2.1"></a>
## [v0.2.1] - 2025-01-07
### Chore
- **deps:** bump go packages
- **docs:** update READMD.md
- **docs:** update READMD.md
- **example:** update examples/demo
- **example:** update examples/simple
- **examples:** update myproject

### Feat
- **looger:** add WithControllerContext and WithServiceContext methods - WithControllerContext: for controller layer context fields - WithServiceControtext: for service layer context fields
- **model:** add tag "url" for query parameter that used by Client package

### Refactor
- **controller:** simplify logging context setup


<a name="v0.2.0"></a>
## [v0.2.0] - 2025-01-06
### Chore
- **deps:** bump go packages
- **deps:** upgrade dependencies to latest version
- **docs:** update READMD.md
- **docs:** update READMD.md
- **docs:** update READMD.md
- **docs:** update READMD.md
- **docs:** update READMD.md
- **examples:** update examples
- **examples:** update examples
- **examples:** remove unused example file
- **gitignore:** add *.db to ignore list
- **testdata:** add restart policy for docker-compose.yaml

### Enh
- **config:** auto create empty config file in temp directory
- **logger:** add Protocol and Binary logger
- **tunnel:** 1. add DecodePayload. 2. update testcase. 3. update docs.

### Feat
- add tunnel package
- **config:** support read config from environment and env has more priority than config file.
- **ldap:** add ldap authentication package
- **ldap:** add ldap authentication package
- **pkg:** add version package
- **tunnel:** add NewCmd, add more testcase
- **types:** add consts package
- **util:** add IsConnClosed
- **util:** add net utility functions

### Refactor
- **mongo:** rename makeURI -> buildURI
- **tunnel:** improve DecodePayload function signature
- **tunnel:** simplify CMD
- **tunnel:** cleanup code and repalce json to msgpack
- **types:** move constants into dedicated consts package


<a name="v0.1.1"></a>
## [v0.1.1] - 2024-12-27
### Chore
- update examples/simple
- update examples/demo
- update READMD.md
- **deps:** upgrade dependencies to latest version

### Docs
- **example:** update myproject code
- **example:** update demo code

### Feat
- **elastic:** add New function to create seperate elasticsearch client.
- **minio:** add `New` function to create minio seperate client.
- **mongo:** add `New` function to create seperate mongo client instance
- **mqtt:** add `New` function to create seperate client
- **mysql:** add `New` to create seperate instance.
- **postgres:** add `New` to create seperate instance.
- **router:** add function `RegisterWithConfig` to custom controller configuration
- **sqlite:** add `New` to create seperate instance.

### Refactor
- **mysql:** rename makeDSN to buildDSN
- **postgres:** rename makeDSN to buildDSN
- **sqlite:** rename makeDSN to buildDSN

### Style
- **mongo:** format log statement in a single line


<a name="v0.1.0"></a>
## [v0.1.0] - 2024-12-26
### Chore
- update examples/demo
- update README.md
- update READMD.md
- update READMD.md
- update examples/demo
- add READMD.md for controller
- update READMD.md
- update READMD.md
- update examples/demo
- update READMD.md
- update READMD.md
- update examples/demo
- update READMD.md
- update example/demo
- update examples/demo
- update examples/simple
- update README.md
- update README.md
- update examples/simple
- update README.md
- update example/demo
- bump go pkg version to latest
- **model:** add doc for `Register` and `Register`, deprecated `RegisterRoutes`

### Enh
- **model:** model.Register() will set id before insert table records


<a name="v0.0.66"></a>
## [v0.0.66] - 2024-12-18
### Chg
- **model:** remove RegisterRoutes
- **model:** rename GetTablename -> GetTableName
- **model:** move GormScannerWrapper from `model.go` to `util.go`, add function `GetTablename`
- **model:** remove model.Base field `Error`

### Chore
- update examples
- update README.md
- update README.md
- update README.md
- update exmaples/simple
- update README.md
- disinfect
- **database:** add more logger

### Feat
- **router:** add function `Register()` to quickly register routes

### Fix
- **model:** change tablename to SnakeCase

### Rename
- model.Verb -> types.HTTPVerb


<a name="v0.0.65"></a>
## [v0.0.65] - 2024-12-09
### Add
- **model:** `SysInfo`
- **util:** `Round` make float to specified percision.

### Chg
- change `model.CreatedAt`, `model.UpdatedAt` type to *time.Time

### Chore
- update examples
- update examples
- remove old comment
- use go1.23
- **controller:** use new filetype pkg path
- **database:** update testcase
- **model:** update doc
- **model:** rename: common.go -> user-agent.go
- **model:** remove field InternalMark

### Fix
- **controller:** using default database
- **model:** sysinfo

### Opt
- **util:** replace `satori/go.uuid` by `google/uuid`

### Rename
- pkg/http_wrapper -> pkg/httpwrapper
- sizedbufferpool -> pkg/sizedbufferpool
- filetype -> pkg/filetype
- sftp -> pkg/sftp
- bufferpool -> pkg/bufferpool
- cache/bigcache -> pkg/bigcache
- net/wrapper -> pkg/http_wrapper


<a name="v0.0.64"></a>
## [v0.0.64] - 2024-12-03
### Fix
- import `context`


<a name="v0.0.63"></a>
## [v0.0.63] - 2024-11-30
### Feat
- **database:** add `WithTryRun` to skip database operation but only invoke model layer hook.

### Fix
- **controller:** `GetFactory`: model set id to support invoke `GetBefore` hook


<a name="v0.0.62"></a>
## [v0.0.62] - 2024-11-26
### Chg
- **elastic:** param add context


<a name="v0.0.61"></a>
## [v0.0.61] - 2024-11-25
### Fix
- **databasee:** WithSelect


<a name="v0.0.60"></a>
## [v0.0.60] - 2024-11-15
### Fix
- **task:** error cause exit


<a name="v0.0.59"></a>
## [v0.0.59] - 2024-11-12
### Chore
- bump go package version
- **database:** remove debug
- **elastic:** add more case

### Fix
- **elasitc:** allow size to 0 to support DSL `aggs`


<a name="v0.0.58"></a>
## [v0.0.58] - 2024-11-11
### Chore
- bump go packages
- **elastic:** add more testcase

### Enh
- **elastic:** QueryBuilder support `aggs`
- **elastic:** Document.Search support `aggs`


<a name="v0.0.57"></a>
## [v0.0.57] - 2024-11-09
### Chore
- **elastic:** add more testcase

### Enh
- **elastic:** QueryBuilder add more doc, add method `BuildForce`
- **elastic:** add `SearchNext` to searches for N next hits, add `SearchPrev` to searchs for N previous hits.
- **elastic:** improve QueryBuilder to suport complex bool query


<a name="v0.0.56"></a>
## [v0.0.56] - 2024-11-07
### Feat
- **database:** add `WithJoinRaw`, `WithSelectRaw`
- **database:** add `WithJoinRaw`, `WithSelectRaw`


<a name="v0.0.55"></a>
## [v0.0.55] - 2024-11-07
### Bugfix
- Create/Update will remove/invalide cache, feat: trace database operation cost, feat: add `WithTransaction`, `WithLock`, `WithJoin`,`WithGroup`, `WithHaving`

### Chg
- interface `Database`, `DatebaseOption` Database: add `Health` DatabaseOption: add `WithTransaction`, `WithLock`

### Chore
- bump go package
- **logger:** -

### Enh
- **elastic:** support QueryBuilder


<a name="v0.0.54"></a>
## [v0.0.54] - 2024-11-04
### Add
- **logger:** mongo logger

### Chg
- **bootstrap:** bootstrap mongo
- **config:** mqtt config

### Chore
- update example
- bump go package
- update examples

### Enh
- **mqtt:** reimplement package mqtt

### Feat
- mongo package

### Update
- **config:** add mongo config


<a name="v0.0.53"></a>
## [v0.0.53] - 2024-11-02
### Chg
- **config:** config add `enable`
- **minio:** check `enable`
- **mqtt:** check `enable`
- **rbac:** check `enable`
- **util.RunOrDie:** error exit with context


<a name="v0.0.52"></a>
## [v0.0.52] - 2024-11-02
### Chg
- bootstrap mqtt
- **boostrap:** boostrap all database
- **config:** `server` config add `db` to specific which database should use
- **database:** database only boostrap when `server.db` is meet current database
- **example:** update

### Chore
- update README.md
- update READMD.md
- bump go package

### Opt
- **logger:** more check


<a name="v0.0.51"></a>
## [v0.0.51] - 2024-11-01
### Fix
- add recover for task


<a name="v0.0.50"></a>
## [v0.0.50] - 2024-10-24
### Chore
- update StringAny


<a name="v0.0.49"></a>
## [v0.0.49] - 2024-10-24
### Chg
- replace cmap to lru

### Chore
- bump go package
- remove comment


<a name="v0.0.48"></a>
## [v0.0.48] - 2024-10-22
### Chg
- set default log to console; set controller access log to access.log
- BulkIndex -> (*document).BulkIndex

### Chore
- update examples
- change logger position
- add more log
- update example
- update examples
- update README.md
- update README.md
- update README.md
- update README.md
- update README.md
- update README.md
- update README.md
- update README.md

### Fix
- support query parameter _select


<a name="v0.0.47"></a>
## [v0.0.47] - 2024-10-16
### Add
- Contains

### Chore
- update README.md
- update examples


<a name="v0.0.46"></a>
## [v0.0.46] - 2024-10-16
### Feat
- controller support `_select` query params

### Fix
- WithSelect


<a name="v0.0.45"></a>
## [v0.0.45] - 2024-10-13
### Feat
- support using custom index to query
- support using custom index to query


<a name="v0.0.44"></a>
## [v0.0.44] - 2024-10-13
### Feat
- database support WithSelect


<a name="v0.0.43"></a>
## [v0.0.43] - 2024-10-13
### Chore
- update README.md


<a name="v0.0.42"></a>
## [v0.0.42] - 2024-10-11
### Chg
- write `access_token`, `refresh_token`

### Chore
- bump go package version
- clean code

### Feat
- support refresh token; upgrade jwt to v5


<a name="v0.0.41"></a>
## [v0.0.41] - 2024-10-10
### Chg
- change tinyint -> smallint to support postgresql
- using helper
- remove router `/api/ping`

### Chore
- upgrade gorm drivers and plugins
- update documents
- update README.md
- bump go packages
- update examples for database postgresql
- change log
- change log
- update README.md
- update example

### Feat
- support database/postgresql
- database support sqlite


<a name="v0.0.40"></a>
## [v0.0.40] - 2024-10-09
### Chore
- update README.md


<a name="v0.0.39"></a>
## [v0.0.39] - 2024-10-07
### Chore
- using const
- add doc
- remove debug output

### Feat
- support _nototal in controller layer

### Fix
- nil Rows cause panic

### Opt
- add table index for `updated_at`, `created_by`,`updated_by`


<a name="v0.0.38"></a>
## [v0.0.38] - 2024-09-30

<a name="v0.0.37"></a>
## [v0.0.37] - 2024-09-30
### Opt
- concurrently query column data from database


<a name="v0.0.36"></a>
## [v0.0.36] - 2024-09-29

<a name="v0.0.35"></a>
## [v0.0.35] - 2024-09-29

<a name="v0.0.34"></a>
## [v0.0.34] - 2024-09-28
### Fix
- using new session for batch size.

### Task
- logger add cost field


<a name="v0.0.33"></a>
## [v0.0.33] - 2024-09-22
### Fix
- use logger.Task in task package


<a name="v0.0.32"></a>
## [v0.0.32] - 2024-09-04
### Chg
- remove default middleware.RateLimiter


<a name="v0.0.31"></a>
## [v0.0.31] - 2024-08-30

<a name="v0.0.30"></a>
## [v0.0.30] - 2024-08-25

<a name="v0.0.29"></a>
## [v0.0.29] - 2024-08-24
### Chg
- default base-auth and token using config


<a name="v0.0.28"></a>
## [v0.0.28] - 2024-08-24

<a name="v0.0.27"></a>
## [v0.0.27] - 2024-08-24

<a name="v0.0.26"></a>
## [v0.0.26] - 2024-08-23

<a name="v0.0.25"></a>
## [v0.0.25] - 2024-08-23

<a name="v0.0.24"></a>
## [v0.0.24] - 2024-08-22

<a name="v0.0.23"></a>
## [v0.0.23] - 2024-08-22

<a name="v0.0.22"></a>
## [v0.0.22] - 2024-08-02

<a name="v0.0.21"></a>
## [v0.0.21] - 2024-07-24

<a name="v0.0.20"></a>
## [v0.0.20] - 2024-06-28

<a name="v0.0.19"></a>
## [v0.0.19] - 2024-06-17
### Add
- SetConfigFile


<a name="v0.0.18"></a>
## [v0.0.18] - 2024-06-17
### Add
- Cache.Init

### Feat
- add SetConfigName, SetConfigType


<a name="v0.0.17"></a>
## [v0.0.17] - 2024-06-17

<a name="v0.0.16"></a>
## [v0.0.16] - 2024-05-25

<a name="v0.0.15"></a>
## [v0.0.15] - 2024-04-05
### Fix
- using default mysql instance.

### Opt
- upgrade boostrap package.


<a name="v0.0.14"></a>
## [v0.0.14] - 2024-04-03

<a name="v0.0.13"></a>
## [v0.0.13] - 2024-03-19
### Fix
- database.WithQuery
- util.Depointer


<a name="v0.0.12"></a>
## [v0.0.12] - 2024-03-04

<a name="v0.0.11"></a>
## [v0.0.11] - 2024-03-04

<a name="v0.0.10"></a>
## [v0.0.10] - 2024-03-04
### Fix
- register -> Register


<a name="v0.0.9"></a>
## [v0.0.9] - 2024-03-04
### Fix
- service.base[M types.Model] -> service.Base[M types.Model]


<a name="v0.0.8"></a>
## [v0.0.8] - 2024-03-02

<a name="v0.0.7"></a>
## [v0.0.7] - 2024-03-02

<a name="v0.0.6"></a>
## [v0.0.6] - 2024-03-02

<a name="v0.0.5"></a>
## [v0.0.5] - 2024-03-02
### Fix
- If structure field not contains json tags, structure lowercase field name as database query condition


<a name="v0.0.4"></a>
## [v0.0.4] - 2024-03-02
### Fix
- If structure field not contains json tags, structure lowercase field name as database query condition.


<a name="v0.0.3"></a>
## [v0.0.3] - 2024-02-21
### Fix
- disable automigrate model User


<a name="v0.0.2"></a>
## [v0.0.2] - 2024-02-16

<a name="v0.0.1"></a>
## v0.0.1 - 2024-02-15

[Unreleased]: https://github.com/hydroan/gst/compare/v0.10.14...HEAD
[v0.10.14]: https://github.com/hydroan/gst/compare/v0.10.13...v0.10.14
[v0.10.13]: https://github.com/hydroan/gst/compare/v0.10.12...v0.10.13
[v0.10.12]: https://github.com/hydroan/gst/compare/v0.10.11...v0.10.12
[v0.10.11]: https://github.com/hydroan/gst/compare/v0.10.10...v0.10.11
[v0.10.10]: https://github.com/hydroan/gst/compare/v0.10.9...v0.10.10
[v0.10.9]: https://github.com/hydroan/gst/compare/v0.10.8...v0.10.9
[v0.10.8]: https://github.com/hydroan/gst/compare/v0.10.7...v0.10.8
[v0.10.7]: https://github.com/hydroan/gst/compare/v0.10.6...v0.10.7
[v0.10.6]: https://github.com/hydroan/gst/compare/v0.10.5...v0.10.6
[v0.10.5]: https://github.com/hydroan/gst/compare/v0.10.5-beta.3...v0.10.5
[v0.10.5-beta.3]: https://github.com/hydroan/gst/compare/v0.10.5-beta.2...v0.10.5-beta.3
[v0.10.5-beta.2]: https://github.com/hydroan/gst/compare/v0.10.5-beta.1...v0.10.5-beta.2
[v0.10.5-beta.1]: https://github.com/hydroan/gst/compare/v0.10.5-beta.0...v0.10.5-beta.1
[v0.10.5-beta.0]: https://github.com/hydroan/gst/compare/v0.10.4...v0.10.5-beta.0
[v0.10.4]: https://github.com/hydroan/gst/compare/v0.10.3...v0.10.4
[v0.10.3]: https://github.com/hydroan/gst/compare/v0.10.2...v0.10.3
[v0.10.2]: https://github.com/hydroan/gst/compare/v0.10.1...v0.10.2
[v0.10.1]: https://github.com/hydroan/gst/compare/v0.10.0...v0.10.1
[v0.10.0]: https://github.com/hydroan/gst/compare/v0.10.0-beta.6...v0.10.0
[v0.10.0-beta.6]: https://github.com/hydroan/gst/compare/v0.10.0-beta.5...v0.10.0-beta.6
[v0.10.0-beta.5]: https://github.com/hydroan/gst/compare/v0.10.0-beta.4...v0.10.0-beta.5
[v0.10.0-beta.4]: https://github.com/hydroan/gst/compare/list...v0.10.0-beta.4
[list]: https://github.com/hydroan/gst/compare/v0.10.0-beta.3...list
[v0.10.0-beta.3]: https://github.com/hydroan/gst/compare/v0.10.0-beta.2...v0.10.0-beta.3
[v0.10.0-beta.2]: https://github.com/hydroan/gst/compare/v0.10.0-beta.1...v0.10.0-beta.2
[v0.10.0-beta.1]: https://github.com/hydroan/gst/compare/v0.10.0-beta.0...v0.10.0-beta.1
[v0.10.0-beta.0]: https://github.com/hydroan/gst/compare/v0.9.7-beta.4...v0.10.0-beta.0
[v0.9.7-beta.4]: https://github.com/hydroan/gst/compare/v0.9.7...v0.9.7-beta.4
[v0.9.7]: https://github.com/hydroan/gst/compare/v0.9.7-beta.3...v0.9.7
[v0.9.7-beta.3]: https://github.com/hydroan/gst/compare/v0.9.7-beta.2...v0.9.7-beta.3
[v0.9.7-beta.2]: https://github.com/hydroan/gst/compare/v0.9.7-beta.1...v0.9.7-beta.2
[v0.9.7-beta.1]: https://github.com/hydroan/gst/compare/v0.9.7-beta.0...v0.9.7-beta.1
[v0.9.7-beta.0]: https://github.com/hydroan/gst/compare/v0.9.6...v0.9.7-beta.0
[v0.9.6]: https://github.com/hydroan/gst/compare/v0.9.6-beta.4...v0.9.6
[v0.9.6-beta.4]: https://github.com/hydroan/gst/compare/v0.9.6-beta.3...v0.9.6-beta.4
[v0.9.6-beta.3]: https://github.com/hydroan/gst/compare/v0.9.6-beta.2...v0.9.6-beta.3
[v0.9.6-beta.2]: https://github.com/hydroan/gst/compare/v0.9.6-beta.1...v0.9.6-beta.2
[v0.9.6-beta.1]: https://github.com/hydroan/gst/compare/v0.9.6-beta.0...v0.9.6-beta.1
[v0.9.6-beta.0]: https://github.com/hydroan/gst/compare/v0.9.5...v0.9.6-beta.0
[v0.9.5]: https://github.com/hydroan/gst/compare/v0.9.4...v0.9.5
[v0.9.4]: https://github.com/hydroan/gst/compare/v0.9.3...v0.9.4
[v0.9.3]: https://github.com/hydroan/gst/compare/v0.9.2...v0.9.3
[v0.9.2]: https://github.com/hydroan/gst/compare/v0.9.1-beta.2...v0.9.2
[v0.9.1-beta.2]: https://github.com/hydroan/gst/compare/v0.9.1-beta.1...v0.9.1-beta.2
[v0.9.1-beta.1]: https://github.com/hydroan/gst/compare/v0.9.1...v0.9.1-beta.1
[v0.9.1]: https://github.com/hydroan/gst/compare/v0.9.0...v0.9.1
[v0.9.0]: https://github.com/hydroan/gst/compare/v0.8.0...v0.9.0
[v0.8.0]: https://github.com/hydroan/gst/compare/v0.8.0-beta.1...v0.8.0
[v0.8.0-beta.1]: https://github.com/hydroan/gst/compare/v0.7.5...v0.8.0-beta.1
[v0.7.5]: https://github.com/hydroan/gst/compare/v0.7.4...v0.7.5
[v0.7.4]: https://github.com/hydroan/gst/compare/v0.7.3...v0.7.4
[v0.7.3]: https://github.com/hydroan/gst/compare/v0.7.2...v0.7.3
[v0.7.2]: https://github.com/hydroan/gst/compare/v0.7.1...v0.7.2
[v0.7.1]: https://github.com/hydroan/gst/compare/v0.7.0...v0.7.1
[v0.7.0]: https://github.com/hydroan/gst/compare/v0.7.0-beta.3...v0.7.0
[v0.7.0-beta.3]: https://github.com/hydroan/gst/compare/v0.7.0-beta.2...v0.7.0-beta.3
[v0.7.0-beta.2]: https://github.com/hydroan/gst/compare/v0.7.0-beta.1...v0.7.0-beta.2
[v0.7.0-beta.1]: https://github.com/hydroan/gst/compare/v0.6.2...v0.7.0-beta.1
[v0.6.2]: https://github.com/hydroan/gst/compare/v0.6.1...v0.6.2
[v0.6.1]: https://github.com/hydroan/gst/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/hydroan/gst/compare/v0.5.2...v0.6.0
[v0.5.2]: https://github.com/hydroan/gst/compare/v0.5.1...v0.5.2
[v0.5.1]: https://github.com/hydroan/gst/compare/v0.5.0...v0.5.1
[v0.5.0]: https://github.com/hydroan/gst/compare/v0.4.4...v0.5.0
[v0.4.4]: https://github.com/hydroan/gst/compare/v0.4.3...v0.4.4
[v0.4.3]: https://github.com/hydroan/gst/compare/v0.4.2...v0.4.3
[v0.4.2]: https://github.com/hydroan/gst/compare/v0.4.1...v0.4.2
[v0.4.1]: https://github.com/hydroan/gst/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/hydroan/gst/compare/v0.3.4...v0.4.0
[v0.3.4]: https://github.com/hydroan/gst/compare/v0.3.3...v0.3.4
[v0.3.3]: https://github.com/hydroan/gst/compare/v0.3.2...v0.3.3
[v0.3.2]: https://github.com/hydroan/gst/compare/v0.3.1...v0.3.2
[v0.3.1]: https://github.com/hydroan/gst/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/hydroan/gst/compare/v0.2.3...v0.3.0
[v0.2.3]: https://github.com/hydroan/gst/compare/v0.2.2...v0.2.3
[v0.2.2]: https://github.com/hydroan/gst/compare/v0.2.1...v0.2.2
[v0.2.1]: https://github.com/hydroan/gst/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/hydroan/gst/compare/v0.1.1...v0.2.0
[v0.1.1]: https://github.com/hydroan/gst/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/hydroan/gst/compare/v0.0.66...v0.1.0
[v0.0.66]: https://github.com/hydroan/gst/compare/v0.0.65...v0.0.66
[v0.0.65]: https://github.com/hydroan/gst/compare/v0.0.64...v0.0.65
[v0.0.64]: https://github.com/hydroan/gst/compare/v0.0.63...v0.0.64
[v0.0.63]: https://github.com/hydroan/gst/compare/v0.0.62...v0.0.63
[v0.0.62]: https://github.com/hydroan/gst/compare/v0.0.61...v0.0.62
[v0.0.61]: https://github.com/hydroan/gst/compare/v0.0.60...v0.0.61
[v0.0.60]: https://github.com/hydroan/gst/compare/v0.0.59...v0.0.60
[v0.0.59]: https://github.com/hydroan/gst/compare/v0.0.58...v0.0.59
[v0.0.58]: https://github.com/hydroan/gst/compare/v0.0.57...v0.0.58
[v0.0.57]: https://github.com/hydroan/gst/compare/v0.0.56...v0.0.57
[v0.0.56]: https://github.com/hydroan/gst/compare/v0.0.55...v0.0.56
[v0.0.55]: https://github.com/hydroan/gst/compare/v0.0.54...v0.0.55
[v0.0.54]: https://github.com/hydroan/gst/compare/v0.0.53...v0.0.54
[v0.0.53]: https://github.com/hydroan/gst/compare/v0.0.52...v0.0.53
[v0.0.52]: https://github.com/hydroan/gst/compare/v0.0.51...v0.0.52
[v0.0.51]: https://github.com/hydroan/gst/compare/v0.0.50...v0.0.51
[v0.0.50]: https://github.com/hydroan/gst/compare/v0.0.49...v0.0.50
[v0.0.49]: https://github.com/hydroan/gst/compare/v0.0.48...v0.0.49
[v0.0.48]: https://github.com/hydroan/gst/compare/v0.0.47...v0.0.48
[v0.0.47]: https://github.com/hydroan/gst/compare/v0.0.46...v0.0.47
[v0.0.46]: https://github.com/hydroan/gst/compare/v0.0.45...v0.0.46
[v0.0.45]: https://github.com/hydroan/gst/compare/v0.0.44...v0.0.45
[v0.0.44]: https://github.com/hydroan/gst/compare/v0.0.43...v0.0.44
[v0.0.43]: https://github.com/hydroan/gst/compare/v0.0.42...v0.0.43
[v0.0.42]: https://github.com/hydroan/gst/compare/v0.0.41...v0.0.42
[v0.0.41]: https://github.com/hydroan/gst/compare/v0.0.40...v0.0.41
[v0.0.40]: https://github.com/hydroan/gst/compare/v0.0.39...v0.0.40
[v0.0.39]: https://github.com/hydroan/gst/compare/v0.0.38...v0.0.39
[v0.0.38]: https://github.com/hydroan/gst/compare/v0.0.37...v0.0.38
[v0.0.37]: https://github.com/hydroan/gst/compare/v0.0.36...v0.0.37
[v0.0.36]: https://github.com/hydroan/gst/compare/v0.0.35...v0.0.36
[v0.0.35]: https://github.com/hydroan/gst/compare/v0.0.34...v0.0.35
[v0.0.34]: https://github.com/hydroan/gst/compare/v0.0.33...v0.0.34
[v0.0.33]: https://github.com/hydroan/gst/compare/v0.0.32...v0.0.33
[v0.0.32]: https://github.com/hydroan/gst/compare/v0.0.31...v0.0.32
[v0.0.31]: https://github.com/hydroan/gst/compare/v0.0.30...v0.0.31
[v0.0.30]: https://github.com/hydroan/gst/compare/v0.0.29...v0.0.30
[v0.0.29]: https://github.com/hydroan/gst/compare/v0.0.28...v0.0.29
[v0.0.28]: https://github.com/hydroan/gst/compare/v0.0.27...v0.0.28
[v0.0.27]: https://github.com/hydroan/gst/compare/v0.0.26...v0.0.27
[v0.0.26]: https://github.com/hydroan/gst/compare/v0.0.25...v0.0.26
[v0.0.25]: https://github.com/hydroan/gst/compare/v0.0.24...v0.0.25
[v0.0.24]: https://github.com/hydroan/gst/compare/v0.0.23...v0.0.24
[v0.0.23]: https://github.com/hydroan/gst/compare/v0.0.22...v0.0.23
[v0.0.22]: https://github.com/hydroan/gst/compare/v0.0.21...v0.0.22
[v0.0.21]: https://github.com/hydroan/gst/compare/v0.0.20...v0.0.21
[v0.0.20]: https://github.com/hydroan/gst/compare/v0.0.19...v0.0.20
[v0.0.19]: https://github.com/hydroan/gst/compare/v0.0.18...v0.0.19
[v0.0.18]: https://github.com/hydroan/gst/compare/v0.0.17...v0.0.18
[v0.0.17]: https://github.com/hydroan/gst/compare/v0.0.16...v0.0.17
[v0.0.16]: https://github.com/hydroan/gst/compare/v0.0.15...v0.0.16
[v0.0.15]: https://github.com/hydroan/gst/compare/v0.0.14...v0.0.15
[v0.0.14]: https://github.com/hydroan/gst/compare/v0.0.13...v0.0.14
[v0.0.13]: https://github.com/hydroan/gst/compare/v0.0.12...v0.0.13
[v0.0.12]: https://github.com/hydroan/gst/compare/v0.0.11...v0.0.12
[v0.0.11]: https://github.com/hydroan/gst/compare/v0.0.10...v0.0.11
[v0.0.10]: https://github.com/hydroan/gst/compare/v0.0.9...v0.0.10
[v0.0.9]: https://github.com/hydroan/gst/compare/v0.0.8...v0.0.9
[v0.0.8]: https://github.com/hydroan/gst/compare/v0.0.7...v0.0.8
[v0.0.7]: https://github.com/hydroan/gst/compare/v0.0.6...v0.0.7
[v0.0.6]: https://github.com/hydroan/gst/compare/v0.0.5...v0.0.6
[v0.0.5]: https://github.com/hydroan/gst/compare/v0.0.4...v0.0.5
[v0.0.4]: https://github.com/hydroan/gst/compare/v0.0.3...v0.0.4
[v0.0.3]: https://github.com/hydroan/gst/compare/v0.0.2...v0.0.3
[v0.0.2]: https://github.com/hydroan/gst/compare/v0.0.1...v0.0.2
