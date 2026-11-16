package gggen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
	"github.com/hydroan/gst/internal/modelinfo"
	"golang.org/x/tools/go/ast/astutil"
)

// The packages the test scaffolds import, by the names the scaffolds refer
// to them by.
const (
	scaffoldImportTesting  = "testing"
	scaffoldImportStrings  = "strings"
	scaffoldImportHTTP     = "net/http"
	scaffoldImportClient   = "github.com/hydroan/gst/client"
	scaffoldImportConfig   = "github.com/hydroan/gst/config"
	scaffoldImportTestutil = "github.com/hydroan/gst/testutil"
	scaffoldImportRequire  = "github.com/stretchr/testify/require"
)

// scaffoldImportNames lists the names the imports above take in a scaffold
// and the variables its statements declare, which a package the scaffold
// imports for the example must not take too.
var scaffoldImportNames = []string{"testing", "strings", "http", "client", "config", "testutil", "require", "cli", "stream", "rsp", "err", "id", "events", "event"}

// serviceTestDoc is the part of the test scaffold's doc comment that tells
// how the test is written, after the line naming the route.
var serviceTestDoc = []string{
	"//",
	"// The request goes through the framework client against the test server",
	"// TestMain starts, so the route, the service and the database are exercised",
	"// together. A route not declared Public() goes through the auth middleware the",
	"// project registers, which reads the credentials the client carries: a token",
	"// set with client.WithToken, headers set with client.WithHeader, and the",
	"// cookies earlier responses set, kept in its cookie jar. A rejection is",
	"// asserted with testutil.RequireError, and rows with the testutil.Require*",
	"// helpers.",
}

// GenerateServiceTest builds the test scaffold gg gen writes next to the
// service file target locates when it creates the file: an external test
// package holding one test named after the action, which fails until the
// project deletes its first line, and then is the example request of the
// action, typed by the action's request and response types. route is the
// path the router registers the action at, API prefix included (see
// modelinfo.RouterTargetForAction). For the Create action of the model Record
// of the root model package of helloworld, registered at /api/records, it
// generates
//
//	package record_test
//
//	import (
//		"testing"
//
//		"helloworld/model"
//
//		"github.com/hydroan/gst/client"
//		"github.com/hydroan/gst/testutil"
//		"github.com/stretchr/testify/require"
//	)
//
//	// TestCreate covers POST /api/records, served by Creator in create.go.
//	//
//	// The request goes through the framework client against the test server
//	// TestMain starts, so the route, the service and the database are exercised
//	// together. A route not declared Public() goes through the auth middleware the
//	// project registers, which reads the credentials the client carries: a token
//	// set with client.WithToken, headers set with client.WithHeader, and the
//	// cookies earlier responses set, kept in its cookie jar. A rejection is
//	// asserted with testutil.RequireError, and rows with the testutil.Require*
//	// helpers.
//	func TestCreate(t *testing.T) {
//		t.Fatal("TestCreate is a scaffold: delete this line and finish the test below")
//
//		cli, err := client.New(testutil.BaseURL())
//		require.NoError(t, err)
//
//		rsp, err := cli.Post[model.Record](t.Context(), "/api/records", &model.Record{})
//		require.NoError(t, err)
//		require.NotNil(t, rsp)
//	}
//
// The example request of every action shape is pinned by the tests of this
// function: an item route reads the row's id from a placeholder variable, a
// batch route sends client.BatchItems or client.BatchIDs, a List without a
// declared result decodes client.ListResult, Import uploads a file, Export
// downloads the attachment and SSE consumes the stream. A Stream action is
// opened over its WebSocket connection, through the client method of its
// kind: for the Stream action declaring Service("watch"), Payload and
// StreamingResult on the model Feed of the root model package, registered at
// /api/feeds/watch, the test reads
//
//	// TestWatch covers GET /api/feeds/watch, served by Watch in watch.go.
//	//
//	// The request goes through the framework client against the test server
//	// TestMain starts, so the route, the service and the database are exercised
//	// together. A route not declared Public() goes through the auth middleware the
//	// project registers, which reads the credentials the client carries: a token
//	// set with client.WithToken, headers set with client.WithHeader, and the
//	// cookies earlier responses set, kept in its cookie jar. A rejection is
//	// asserted with testutil.RequireError, and rows with the testutil.Require*
//	// helpers.
//	func TestWatch(t *testing.T) {
//		t.Fatal("TestWatch is a scaffold: delete this line and finish the test below")
//
//		cli, err := client.New(testutil.BaseURL())
//		require.NoError(t, err)
//
//		stream, err := cli.ServerStream[model.FeedEvent](t.Context(), "/api/feeds/watch", &model.FeedWatchReq{})
//		require.NoError(t, err)
//		defer stream.Close()
//		rsp, err := stream.Recv()
//		require.NoError(t, err)
//		require.NotNil(t, rsp)
//	}
//
// and a client stream sends a request then reads the answer with
// CloseAndRecv, a bidirectional one sends, reads and closes its side.
func GenerateServiceTest(info *modelinfo.Model, target modelinfo.ServiceTargetInfo, action *dsl.Action, route string) (string, error) {
	name := serviceTestName(action)
	doc := append([]string{
		fmt.Sprintf("// %s covers %s %s, served by %s in %s.", name, action.Phase.HTTPMethod(), route, action.RoleName(), filepath.Base(target.FilePath)),
	}, serviceTestDoc...)

	example := newServiceTestExample(info, action, route)
	body := []ast.Stmt{
		exprStmt(call(sel(ident("t"), "Fatal"), strLit(name+" is a scaffold: delete this line and finish the test below"))),
		EmptyLine(),
	}
	body = append(body, example.setup...)
	body = append(body, EmptyLine())
	body = append(body, example.stmts...)

	file := &ast.File{
		Name: ast.NewIdent(target.PackageName + ggconst.SuffixTestPkg),
		Decls: []ast.Decl{
			&ast.GenDecl{Tok: token.IMPORT, Specs: example.importSpecs()},
			&ast.FuncDecl{
				Doc:  commentGroup(doc...),
				Name: ast.NewIdent(name),
				Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{field("t", &ast.StarExpr{X: sel(ident("testing"), "T")})}}},
				Body: &ast.BlockStmt{List: body},
			},
		},
	}
	return gghelper.FormatFile(file, nil, info.ModulePath)
}

// serviceTestName returns the name of the test scaffolded for action: Test
// followed by the method name of its phase, as in TestDeleteMany for a
// DeleteMany action, or by the role name of a named service, as in
// TestArchive for Service("archive").
func serviceTestName(action *dsl.Action) string {
	if action.ServiceName != "" {
		return "Test" + action.RoleName()
	}
	return "Test" + action.Phase.Name()
}

// serviceTestExample is the example request of a service test scaffold: the
// statements constructing the client the request goes through, the request
// itself, and the packages they import beyond the ones every scaffold
// imports.
type serviceTestExample struct {
	setup []ast.Stmt
	stmts []ast.Stmt
	// imports maps the import path of each extra package to the name its
	// import is written with, "" for none.
	imports map[string]string
}

// newServiceTestExample builds the example request of action on the model
// info, registered under route: every request passes t.Context() first, an
// item route reads the row's id from a placeholder variable, a batch route
// sends client.BatchItems or client.BatchIDs, a List without a declared result
// decodes client.ListResult, Import uploads a file, Export downloads the
// attachment, SSE receives the first event of the stream, and a Stream is
// opened through the client method of its kind, typed by the messages of its
// sides — a server stream sends the request and reads the first answer, a
// client stream sends a request and reads the answer with CloseAndRecv, a
// bidirectional stream sends, reads and closes its side — all through the
// framework client. The model package is imported when the example refers to
// it, under an alias when its name is one the scaffold takes, and under its
// name when the path does not tell it (see importSpecName).
func newServiceTestExample(info *modelinfo.Model, action *dsl.Action, route string) *serviceTestExample {
	example := &serviceTestExample{
		setup: []ast.Stmt{
			define(idents("cli", "err"), call(sel(ident("client"), "New"), call(sel(ident("testutil"), "BaseURL")))),
			requireCall("NoError", ident("err")),
		},
		imports: map[string]string{scaffoldImportClient: ""},
	}
	modelImportPath := info.ImportPath()
	modelImportName := ResolveImportConflicts(map[string]string{modelImportPath: info.ModelPkgName}, scaffoldImportNames...)[modelImportPath]
	modelQualifier := info.ModelPkgName
	if modelImportName != "" {
		modelQualifier = modelImportName
	}
	// modelType returns the model package type typeName declares, as the
	// scaffold refers to it, and records the import the reference needs.
	modelType := func(typeName string) ast.Expr {
		example.imports[modelImportPath] = modelImportName
		return actionTypeExpr(modelQualifier, typeName)
	}
	// valueType returns the response type of the action in the value form
	// the client decodes: model.Record for *Record and Record alike, and
	// any for the dsl.PayloadEmpty sentinel.
	valueType := func(typeName string) ast.Expr {
		if isEmptyPayload(typeName) {
			return ident("any")
		}
		return modelType(strings.TrimPrefix(typeName, "*"))
	}
	// requestBody returns the request of the action as a literal of its
	// declared form: &model.Record{} for *Record, model.Records{} for the
	// value type Records, and nil for the dsl.PayloadEmpty sentinel.
	requestBody := func(typeName string) ast.Expr {
		if isEmptyPayload(typeName) {
			return ident("nil")
		}
		if structName, ok := strings.CutPrefix(typeName, "*"); ok {
			return &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: modelType(structName)}}
		}
		return &ast.CompositeLit{Type: modelType(typeName)}
	}

	// The request statements follow the declaration of the id placeholder
	// when the path or the body reads it.
	var stmts []ast.Stmt
	path, usesID := routePathExpr(route)
	switch action.Phase {
	case apiaction.Import:
		example.imports[scaffoldImportStrings] = ""
		filename := strLit(filepath.Base(filepath.Dir(route)) + ".csv")
		content := call(sel(ident("strings"), "NewReader"), strLit("name\nsample\n"))
		stmts = append(stmts,
			define(idents("envelope", "err"), call(sel(ident("cli"), "Upload"), testContext(), path, filename, content, ident("nil"))),
			requireCall("NoError", ident("err")),
			requireCall("NotNil", ident("envelope")),
		)
	case apiaction.Export:
		stmts = append(stmts,
			define(idents("attachment", "err"), call(sel(ident("cli"), "Download"), testContext(), path)),
			requireCall("NoError", ident("err")),
			requireCall("NotEmpty", sel(ident("attachment"), "Content")),
		)
	case apiaction.SSE:
		example.imports[scaffoldImportHTTP] = ""
		stmts = append(stmts,
			define(idents("events", "err"), call(sel(ident("cli"), "SSE"), testContext(), sel(ident("http"), "MethodGet"), path, ident("nil"))),
			requireCall("NoError", ident("err")),
			&ast.DeferStmt{Call: call(sel(ident("events"), "Close"))},
			define(idents("event", "err"), call(sel(ident("events"), "Recv"))),
			requireCall("NoError", ident("err")),
			requireCall("NotEmpty", sel(ident("event"), "Data")),
		)
	case apiaction.Stream:
		// message is one message of the request side as the client sends it,
		// and answered the statements reading one of the response side.
		message := &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: valueType(action.Payload)}}
		answered := func(recv string) []ast.Stmt {
			return []ast.Stmt{
				define(idents("rsp", "err"), call(sel(ident("stream"), recv))),
				requireCall("NoError", ident("err")),
				requireCall("NotNil", ident("rsp")),
			}
		}
		var open ast.Expr
		var exchange []ast.Stmt
		switch {
		case action.StreamingPayload && action.StreamingResult:
			open = call(&ast.IndexListExpr{X: sel(ident("cli"), clientBidiStream), Indices: []ast.Expr{valueType(action.Payload), valueType(action.Result)}}, testContext(), path)
			exchange = append(exchange, requireCall("NoError", call(sel(ident("stream"), "Send"), message)))
			exchange = append(exchange, answered("Recv")...)
			exchange = append(exchange, requireCall("NoError", call(sel(ident("stream"), "CloseSend"))))
		case action.StreamingPayload:
			open = call(&ast.IndexListExpr{X: sel(ident("cli"), clientClientStream), Indices: []ast.Expr{valueType(action.Payload), valueType(action.Result)}}, testContext(), path)
			exchange = append(exchange, requireCall("NoError", call(sel(ident("stream"), "Send"), message)))
			exchange = append(exchange, answered("CloseAndRecv")...)
		default:
			open = call(&ast.IndexExpr{X: sel(ident("cli"), clientServerStream), Index: valueType(action.Result)}, testContext(), path, requestBody(action.Payload))
			exchange = answered("Recv")
		}
		stmts = append(stmts,
			define(idents("stream", "err"), open),
			requireCall("NoError", ident("err")),
			&ast.DeferStmt{Call: call(sel(ident("stream"), "Close"))},
		)
		stmts = append(stmts, exchange...)
	default:
		args := []ast.Expr{testContext(), path}
		var rspType ast.Expr
		switch action.Phase {
		case apiaction.List:
			// The example sends no query parameter: pagination and the
			// other query capabilities are opted into per model, and a
			// parameter the model did not opt into is rejected.
			// A List declaring no result answers the standard list
			// envelope of the model; one declaring a result answers it.
			if isEmptyPayload(action.Payload) {
				rspType = valueType(action.Result)
			} else {
				rspType = &ast.IndexExpr{X: sel(ident("client"), "ListResult"), Index: modelType(action.Result)}
			}
		case apiaction.CreateMany, apiaction.UpdateMany, apiaction.PatchMany:
			items := &ast.CompositeLit{Type: &ast.ArrayType{Elt: modelType(action.Payload)}, Elts: []ast.Expr{&ast.CompositeLit{}}}
			args = append(args, call(sel(ident("client"), "BatchItems"), items))
		case apiaction.DeleteMany:
			usesID = true
			ids := &ast.CompositeLit{Type: &ast.ArrayType{Elt: ident("string")}, Elts: []ast.Expr{ident("id")}}
			args = append(args, call(sel(ident("client"), "BatchIDs"), ids))
		default:
			switch method := action.Phase.HTTPMethod(); method {
			case http.MethodGet:
			case http.MethodDelete:
				// The default Delete action reads the row from the path and
				// takes no body; a DELETE action declaring its own request
				// type sends it.
				if isEmptyPayload(action.Payload) || action.Payload == "*"+info.ModelName {
					args = append(args, ident("nil"))
				} else {
					args = append(args, requestBody(action.Payload))
				}
			default:
				args = append(args, requestBody(action.Payload))
			}
		}
		if rspType == nil {
			rspType = valueType(action.Result)
		}
		verb := clientVerbs[action.Phase.HTTPMethod()]
		request := &ast.CallExpr{Fun: &ast.IndexExpr{X: sel(ident("cli"), verb), Index: rspType}, Args: args}
		stmts = append(stmts,
			define(idents("rsp", "err"), request),
			requireCall("NoError", ident("err")),
			requireCall("NotNil", ident("rsp")),
		)
	}
	if usesID {
		example.stmts = append(example.stmts, define(idents("id"), strLit("the ID of a row the test seeded")))
	}
	example.stmts = append(example.stmts, stmts...)
	return example
}

// The client methods opening the stream of a Stream action, one for each kind
// of stream, which they are named after.
const (
	clientServerStream = "ServerStream"
	clientClientStream = "ClientStream"
	clientBidiStream   = "BidiStream"
)

// clientVerbs names the client method sending each HTTP method.
var clientVerbs = map[string]string{
	http.MethodGet:    "Get",
	http.MethodPost:   "Post",
	http.MethodPut:    "Put",
	http.MethodPatch:  "Patch",
	http.MethodDelete: "Delete",
}

// importSpecs returns the import specs of a service test scaffold: the
// packages every scaffold imports and the ones of the example, in an order
// the formatter regroups and sorts.
func (e *serviceTestExample) importSpecs() []ast.Spec {
	specs := []ast.Spec{
		importSpec(scaffoldImportTesting, ""),
		importSpec(scaffoldImportTestutil, ""),
		importSpec(scaffoldImportRequire, ""),
	}
	for _, importPath := range slices.Sorted(maps.Keys(e.imports)) {
		specs = append(specs, importSpec(importPath, e.imports[importPath]))
	}
	return specs
}

// routePathExpr builds the path of route, the one the router registers the
// action at, reading each parameter segment from the id variable:
// "/api/records" for /api/records, "/api/records/" + id for
// /api/records/:rec, and "/api/records/" + id + "/items" for
// /api/records/:rec/items. usesID reports whether the path reads id.
func routePathExpr(route string) (expr ast.Expr, usesID bool) {
	literal := ""
	for segment := range strings.SplitSeq(strings.TrimPrefix(route, "/"), "/") {
		if !strings.HasPrefix(segment, ":") {
			literal += "/" + segment
			continue
		}
		expr = concat(concat(expr, strLit(literal+"/")), ident("id"))
		literal = ""
		usesID = true
	}
	if literal != "" {
		expr = concat(expr, strLit(literal))
	}
	return expr, usesID
}

// serviceTestMainDoc is the doc comment of a scaffolded TestMain.
var serviceTestMainDoc = []string{
	"// TestMain starts the test server of this package the way main.go starts the",
	"// application: the framework bootstraps against the backing services the",
	"// Server declares, which come up in containers of their own, the routes are",
	"// registered, and the server serves the tests until they are done. Database",
	"// is the database the tests run against: config.DBSqlite needs no container,",
	"// config.DBMySQL and config.DBPostgres run in one. Redis serves the modules",
	"// that keep sessions or cache entries in it. Seed plants the baseline rows the",
	"// tests start from through database.Database before the server serves.",
}

// GenerateServiceTestMain builds the main_test.go gg gen writes into the
// service package servicePkgName of the module modulePath along with the
// package's first test scaffold, when no test file of the package declares
// TestMain yet. It links the packages main.go imports
// (ggconst.ProjectImportDirs), so the test server registers what the
// application registers, and declares the test server with the framework
// defaults spelled out, a no-op Seed among them, for the project to edit.
// For the package record of helloworld it generates
//
//	package record_test
//
//	import (
//		"testing"
//
//		// The registrations of main.go: the models, modules, services and cron
//		// jobs register themselves through the init of these packages.
//		_ "helloworld/component"
//		_ "helloworld/configx"
//		_ "helloworld/cronjob"
//		_ "helloworld/leader"
//		_ "helloworld/lock"
//		_ "helloworld/middleware"
//		_ "helloworld/model"
//		_ "helloworld/module"
//		"helloworld/router"
//		_ "helloworld/service"
//
//		"github.com/hydroan/gst/config"
//		"github.com/hydroan/gst/testutil"
//	)
//
//	// TestMain starts the test server of this package the way main.go starts the
//	// application: the framework bootstraps against the backing services the
//	// Server declares, which come up in containers of their own, the routes are
//	// registered, and the server serves the tests until they are done. Database
//	// is the database the tests run against: config.DBSqlite needs no container,
//	// config.DBMySQL and config.DBPostgres run in one. Redis serves the modules
//	// that keep sessions or cache entries in it. Seed plants the baseline rows the
//	// tests start from through database.Database before the server serves.
//	func TestMain(m *testing.M) {
//		testutil.Run(m, testutil.Server{
//			Database: config.DBSqlite,
//			Redis:    false,
//			Routes:   router.Init,
//			Seed:     func() error { return nil },
//		})
//	}
func GenerateServiceTestMain(modulePath, servicePkgName string, extraDirs ...string) (string, error) {
	// go/printer lays comments and composite literal elements out by their
	// source positions, so the import group and the Server literal take
	// positions on successive lines of a fabricated file: the doc comment
	// of the first project import starts a line of its own, and the Server
	// literal breaks across lines, one field per line.
	fset := token.NewFileSet()
	lines := goast.NewLineSet(fset)

	specs := []ast.Spec{importSpecAt(scaffoldImportTesting, "", lines.Next())}
	for i, dir := range slices.Sorted(slices.Values(slices.Concat(ggconst.ProjectImportDirs, extraDirs))) {
		var doc *ast.CommentGroup
		if i == 0 {
			// A blank line sets the project imports apart from "testing".
			lines.Next()
			doc = commentGroup(
				"// The registrations of main.go: the models, modules, services and cron",
				"// jobs register themselves through the init of these packages.",
			)
			doc.List[0].Slash = lines.Next()
			doc.List[1].Slash = lines.Next()
		}
		name := "_"
		if dir == ggconst.DirRouter {
			name = ""
		}
		spec := importSpecAt(modulePath+"/"+dir, name, lines.Next())
		spec.Doc = doc
		specs = append(specs, spec)
	}
	specs = append(specs, importSpecAt(scaffoldImportConfig, "", lines.Next()), importSpecAt(scaffoldImportTestutil, "", lines.Next()))

	lbrace, databasePos, redisPos, routesPos, seedPos, rbrace := lines.Next(), lines.Next(), lines.Next(), lines.Next(), lines.Next(), lines.Next()
	server := &ast.CompositeLit{
		Type:   sel(ident("testutil"), "Server"),
		Lbrace: lbrace,
		Elts: []ast.Expr{
			keyValue("Database", sel(ident("config"), "DBSqlite"), databasePos),
			keyValue("Redis", ident("false"), redisPos),
			keyValue("Routes", sel(ident(ggconst.PkgRouter), ggconst.FuncInit2), routesPos),
			// The function literal keeps its one-line body when its header
			// is positioned on the line of its key.
			keyValue("Seed", &ast.FuncLit{
				Type: &ast.FuncType{Func: seedPos, Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: ident("error")}}}},
				Body: &ast.BlockStmt{List: []ast.Stmt{Returns(ident("nil"))}},
			}, seedPos),
		},
		Rbrace: rbrace,
	}

	file := &ast.File{
		Name: ast.NewIdent(servicePkgName + ggconst.SuffixTestPkg),
		Decls: []ast.Decl{
			&ast.GenDecl{Tok: token.IMPORT, Specs: specs},
			&ast.FuncDecl{
				Doc:  commentGroup(serviceTestMainDoc...),
				Name: ast.NewIdent(ggconst.FuncTestMain),
				Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{field("m", &ast.StarExpr{X: sel(ident("testing"), "M")})}}},
				Body: &ast.BlockStmt{List: []ast.Stmt{exprStmt(call(sel(ident("testutil"), "Run"), ident("m"), server))}}, //gst:literal testutil.Run, not bootstrap.Run
			},
		},
	}
	return gghelper.FormatFile(file, fset, modulePath)
}

// TestMainFile returns the path of the test file in dir declaring TestMain
// at package level, be it main_test.go or any other test file, since the
// test binary of a package takes its TestMain from any of them: for
// service/record/main_test.go declaring one, that path; "" when no test
// file of dir declares one, a missing directory included. A test file that
// does not parse is an error: gg cannot tell what it declares.
func TestMainFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ggconst.PatternTestFile) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return "", errors.Wrapf(err, "reading the test file %s", path)
		}
		if goast.DeclaresTestMain(file) {
			return path, nil
		}
	}
	return "", nil
}

// SyncTestMainImports makes file, a TestMain file of the module modulePath
// parsed through fset with its comments, import the optional project
// packages the way main.go does, present being the directories the project
// holds (see gghelper.OptionalImportDrift): a missing import is added as a
// blank import in the group of the file's other imports of the module (see
// insertBlankImport), and a stale blank import is dropped with its comment;
// the rest of the file stays as it is. It reports whether file changed. The
// file is laid out by its positions, so go/format prints it with the groups
// it had, the added import sorted into its group. The scaffold
// GenerateServiceTestMain writes for a project holding neither package,
// synced for one holding interceptor and pb, is the scaffold it writes for
// that project, its project imports reading
//
//	_ "helloworld/component"
//	_ "helloworld/configx"
//	_ "helloworld/cronjob"
//	_ "helloworld/interceptor"
//	_ "helloworld/leader"
//	_ "helloworld/lock"
//	_ "helloworld/middleware"
//	_ "helloworld/model"
//	_ "helloworld/module"
//	_ "helloworld/pb"
//	"helloworld/router"
//	_ "helloworld/service"
func SyncTestMainImports(fset *token.FileSet, file *ast.File, modulePath string, present []string) bool {
	missing, stale := gghelper.OptionalImportDrift(file, modulePath, present)
	for _, importPath := range missing {
		insertBlankImport(fset, file, modulePath, importPath)
	}
	for _, importPath := range stale {
		// astutil keeps the comments of a deleted import unless it was the
		// last one of its declaration; they go with the import here.
		spec := goast.FindImportSpec(file, importPath)
		file.Comments = slices.DeleteFunc(file.Comments, func(group *ast.CommentGroup) bool { return group == spec.Doc || group == spec.Comment })
		astutil.DeleteNamedImport(fset, file, "_", importPath)
	}
	return len(missing)+len(stale) > 0
}

// insertBlankImport adds the blank import of importPath to the first import
// declaration of file, parsed through fset, at the line of the file's last
// import of the module modulePath, so that go/format sorts it into the
// group of the module's imports, or two lines below the last import, in a
// group of its own, when the file imports nothing of the module: the lone
// "testing" of
//
//	import "testing"
//
// becomes
//
//	import (
//		"testing"
//
//		_ "helloworld/pb"
//	)
//
// astutil.AddNamedImport places an import beside the one sharing the longest
// path prefix, which is "testing" for the first import of the module, and
// the formatters group by the dot of a domain, which a module path need not
// have. A TestMain file imports testing, so the declaration is there; one
// without any import gets the import declared on its own (see
// goast.InsertImportSpec).
func insertBlankImport(fset *token.FileSet, file *ast.File, modulePath, importPath string) {
	var decl *ast.GenDecl
	var last, anchor *ast.ImportSpec
	var at int
	for _, d := range file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT || len(gen.Specs) == 0 {
			continue
		}
		decl, at = gen, len(gen.Specs)
		for i, spec := range gen.Specs {
			imp, ok := spec.(*ast.ImportSpec)
			if !ok {
				continue
			}
			last = imp
			if path, err := strconv.Unquote(imp.Path.Value); err == nil && (path == modulePath || strings.HasPrefix(path, modulePath+"/")) {
				anchor, at = imp, i+1
			}
		}
		break
	}
	if last == nil {
		goast.InsertImportSpec(file, importSpecAt(importPath, "_", token.NoPos))
		return
	}
	pos := last.Path.ValuePos
	if anchor != nil {
		pos = anchor.Path.ValuePos
	} else {
		lines := fset.File(pos)
		pos = lines.LineStart(min(lines.Line(pos)+2, lines.LineCount()))
	}
	spec := importSpecAt(importPath, "_", pos)
	decl.Specs = slices.Insert(decl.Specs, at, ast.Spec(spec))
	if !decl.Lparen.IsValid() {
		decl.Lparen = decl.Specs[0].Pos()
	}
	file.Imports = append(file.Imports, spec)
}

// The AST builders of the scaffolds.

func ident(name string) *ast.Ident { return ast.NewIdent(name) }

func idents(names ...string) []ast.Expr {
	exprs := make([]ast.Expr, len(names))
	for i, name := range names {
		exprs[i] = ident(name)
	}
	return exprs
}

func sel(x ast.Expr, name string) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: x, Sel: ident(name)}
}

func call(fun ast.Expr, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: fun, Args: args}
}

func strLit(value string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(value)}
}

// concat joins x and y with +, or returns y alone when x is nil.
func concat(x, y ast.Expr) ast.Expr {
	if x == nil {
		return y
	}
	return &ast.BinaryExpr{X: x, Op: token.ADD, Y: y}
}

func exprStmt(x ast.Expr) *ast.ExprStmt { return &ast.ExprStmt{X: x} }

// testContext builds the t.Context() argument every example request passes
// first: the context of the test bounds the request.
func testContext() *ast.CallExpr { return call(sel(ident("t"), "Context")) }

// define builds the short variable declaration of lhs from rhs.
func define(lhs []ast.Expr, rhs ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: []ast.Expr{rhs}}
}

// requireCall builds the require.<name>(t, args...) assertion.
func requireCall(name string, args ...ast.Expr) *ast.ExprStmt {
	return exprStmt(call(sel(ident("require"), name), append([]ast.Expr{ident("t")}, args...)...))
}

func field(name string, typ ast.Expr) *ast.Field {
	return &ast.Field{Names: []*ast.Ident{ident(name)}, Type: typ}
}

// keyValue builds the key: value element of a composite literal, with the
// key at pos.
func keyValue(key string, value ast.Expr, pos token.Pos) *ast.KeyValueExpr {
	return &ast.KeyValueExpr{Key: &ast.Ident{Name: key, NamePos: pos}, Value: value}
}

// importSpec builds the import of importPath under name, "" for the package
// name and "_" for a blank import.
func importSpec(importPath, name string) *ast.ImportSpec {
	return importSpecAt(importPath, name, token.NoPos)
}

// importSpecAt is importSpec with the import positioned at pos.
func importSpecAt(importPath, name string, pos token.Pos) *ast.ImportSpec {
	spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(importPath), ValuePos: pos}}
	if name != "" {
		spec.Name = &ast.Ident{Name: name, NamePos: pos}
	}
	return spec
}

func commentGroup(lines ...string) *ast.CommentGroup {
	group := &ast.CommentGroup{List: make([]*ast.Comment, len(lines))}
	for i, line := range lines {
		group.List[i] = &ast.Comment{Text: line}
	}
	return group
}
