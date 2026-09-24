package gen

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
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/goast"
)

// The packages the test scaffolds import, by the names the scaffolds refer
// to them by.
const (
	scaffoldImportTesting  = "testing"
	scaffoldImportStrings  = "strings"
	scaffoldImportHTTP     = "net/http"
	scaffoldImportClient   = "github.com/hydroan/gst/client"
	scaffoldImportConfig   = "github.com/hydroan/gst/config"
	scaffoldImportSSE      = "github.com/hydroan/gst/sse"
	scaffoldImportTestutil = "github.com/hydroan/gst/testutil"
	scaffoldImportRequire  = "github.com/stretchr/testify/require"
)

// scaffoldImportNames lists the names the imports above take in a scaffold,
// which the model package of the project must not take too.
var scaffoldImportNames = []string{"testing", "strings", "http", "client", "config", "sse", "testutil", "require"}

// serviceTestDoc is the part of every service test scaffold's doc comment
// that tells how the test is written, after the line naming the route.
var serviceTestDoc = []string{
	"//",
	"// The request goes through the framework client against the test server",
	"// TestMain starts, so the route, the service and the database are exercised",
	"// together; a login is a plain cli.Post to the login route, whose session the",
	"// client's cookie jar keeps for the requests that follow. A rejection is",
	"// asserted with testutil.RequireError, and rows with the testutil.Require*",
	"// helpers.",
}

// GenerateServiceTest builds the test scaffold gg gen writes next to the
// service file target locates when it creates the file: an external test
// package holding one test named after the action, which fails until the
// project deletes its first line, and then is the example request of the
// action, typed by the action's request and response types. route is the
// route the router registers the action under, without the API prefix (see
// codegen.RouterTargetForAction). For the Create action of the model Record
// of the root model package of helloworld, registered under records, it
// generates
//
//	package record_test
//
//	import (
//		"helloworld/model"
//		"testing"
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
//	// together; a login is a plain cli.Post to the login route, whose session the
//	// client's cookie jar keeps for the requests that follow. A rejection is
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
// downloads the attachment and SSE consumes the stream.
func GenerateServiceTest(info *ModelInfo, target ServiceTargetInfo, action *dsl.Action, route string) (string, error) {
	name := serviceTestName(action)
	method := consts.HTTPVerb(action.Phase).HTTPMethod()
	doc := append([]string{
		fmt.Sprintf("// %s covers %s %s/%s, served by %s in %s.", name, method, consts.APIPathPrefix, route, action.RoleName(), filepath.Base(target.FilePath)),
	}, serviceTestDoc...)

	example := newServiceTestExample(info, action, route)
	body := []ast.Stmt{
		exprStmt(call(sel(ident("t"), "Fatal"), strLit(name+" is a scaffold: delete this line and finish the test below"))),
		EmptyLine(),
		define(idents("cli", "err"), call(sel(ident("client"), "New"), call(sel(ident("testutil"), "BaseURL")))),
		requireCall("NoError", ident("err")),
		EmptyLine(),
	}
	body = append(body, example.stmts...)

	file := &ast.File{
		Name: ast.NewIdent(target.PackageName + "_test"),
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
	return FormatNodeExtra(file)
}

// serviceTestName returns the name of the test scaffolded for action: Test
// followed by the method name of its phase, as in TestDeleteMany for a
// DeleteMany action, or by the role name of a Filename action, as in
// TestArchive for Filename("archive").
func serviceTestName(action *dsl.Action) string {
	if len(action.Filename) > 0 {
		return "Test" + action.RoleName()
	}
	return "Test" + action.Phase.MethodName()
}

// serviceTestExample is the example request of a service test scaffold: the
// statements following the client construction, and the packages they
// import beyond the ones every scaffold imports.
type serviceTestExample struct {
	stmts []ast.Stmt
	// imports maps the import path of each extra package to the name it is
	// imported under, "" for its package name.
	imports map[string]string
}

// newServiceTestExample builds the example request of action on the model
// info, registered under route: every request passes t.Context() first, an
// item route reads the row's id from a placeholder variable, a batch route
// sends client.BatchItems or
// client.BatchIDs, a List without a declared result decodes
// client.ListResult, Import uploads a file, Export downloads the attachment
// and SSE consumes the stream. The model package is imported when the
// example refers to it, under an alias when its name is one a scaffold
// import takes.
func newServiceTestExample(info *ModelInfo, action *dsl.Action, route string) *serviceTestExample {
	example := &serviceTestExample{imports: map[string]string{}}
	modelImportPath := info.ImportPath()
	modelQualifier := info.ModelPkgName
	if alias := ResolveImportConflicts(map[string]string{modelImportPath: info.ModelPkgName}, scaffoldImportNames...)[modelImportPath]; alias != "" {
		modelQualifier = alias
	}
	// modelType returns the model package type typeName declares, as the
	// scaffold refers to it, and records the import the reference needs.
	modelType := func(typeName string) ast.Expr {
		example.imports[modelImportPath] = ""
		if modelQualifier != info.ModelPkgName {
			example.imports[modelImportPath] = modelQualifier
		}
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
	case consts.PHASE_IMPORT:
		example.imports[scaffoldImportStrings] = ""
		filename := strLit(filepath.Base(filepath.Dir(route)) + ".csv")
		content := call(sel(ident("strings"), "NewReader"), strLit("name\nsample\n"))
		stmts = append(stmts,
			define(idents("envelope", "err"), call(sel(ident("cli"), "Upload"), testContext(), path, filename, content, ident("nil"))),
			requireCall("NoError", ident("err")),
			requireCall("NotNil", ident("envelope")),
		)
	case consts.PHASE_EXPORT:
		stmts = append(stmts,
			define(idents("attachment", "err"), call(sel(ident("cli"), "Download"), testContext(), path)),
			requireCall("NoError", ident("err")),
			requireCall("NotEmpty", sel(ident("attachment"), "Content")),
		)
	case consts.PHASE_SSE:
		example.imports[scaffoldImportHTTP] = ""
		example.imports[scaffoldImportSSE] = ""
		callback := &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  &ast.FieldList{List: []*ast.Field{field("event", sel(ident("sse"), "Event"))}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ident("error")}}},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{Returns(sel(ident("client"), "ErrStopStream"))}},
		}
		stmts = append(stmts,
			&ast.AssignStmt{Lhs: []ast.Expr{ident("err")}, Tok: token.ASSIGN, Rhs: []ast.Expr{call(sel(ident("cli"), "Stream"), testContext(), sel(ident("http"), "MethodGet"), path, ident("nil"), callback)}},
			requireCall("NoError", ident("err")),
		)
	default:
		args := []ast.Expr{testContext(), path}
		var rspType ast.Expr
		switch action.Phase {
		case consts.PHASE_LIST:
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
		case consts.PHASE_CREATE_MANY, consts.PHASE_UPDATE_MANY, consts.PHASE_PATCH_MANY:
			items := &ast.CompositeLit{Type: &ast.ArrayType{Elt: modelType(action.Payload)}, Elts: []ast.Expr{&ast.CompositeLit{}}}
			args = append(args, call(sel(ident("client"), "BatchItems"), items))
		case consts.PHASE_DELETE_MANY:
			usesID = true
			ids := &ast.CompositeLit{Type: &ast.ArrayType{Elt: ident("string")}, Elts: []ast.Expr{ident("id")}}
			args = append(args, call(sel(ident("client"), "BatchIDs"), ids))
		default:
			switch method := consts.HTTPVerb(action.Phase).HTTPMethod(); method {
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
		verb := clientVerbs[consts.HTTPVerb(action.Phase).HTTPMethod()]
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

// clientVerbs names the client method sending each HTTP method.
var clientVerbs = map[string]string{
	http.MethodGet:    "Get",
	http.MethodPost:   "Post",
	http.MethodPut:    "Put",
	http.MethodPatch:  "Patch",
	http.MethodDelete: "Delete",
}

// importSpecs returns the import specs of a service test scaffold: the
// packages every scaffold imports and the extra ones of the example, in an
// order the formatter regroups and sorts.
func (e *serviceTestExample) importSpecs() []ast.Spec {
	specs := []ast.Spec{
		importSpec(scaffoldImportTesting, ""),
		importSpec(scaffoldImportClient, ""),
		importSpec(scaffoldImportTestutil, ""),
		importSpec(scaffoldImportRequire, ""),
	}
	for _, importPath := range slices.Sorted(maps.Keys(e.imports)) {
		specs = append(specs, importSpec(importPath, e.imports[importPath]))
	}
	return specs
}

// routePathExpr builds the path of route under the API prefix, reading each
// parameter segment from the id variable: "/api/records" for records,
// "/api/records/" + id for records/:rec, and "/api/records/" + id + "/items"
// for records/:rec/items. usesID reports whether the path reads id.
func routePathExpr(route string) (expr ast.Expr, usesID bool) {
	literal := consts.APIPathPrefix
	for segment := range strings.SplitSeq(route, "/") {
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
	"// that keep sessions or cache entries in it. Seed plants baseline rows",
	"// through database.Database before the server serves, such as the account",
	"// the tests log in with.",
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
//	// that keep sessions or cache entries in it. Seed plants baseline rows
//	// through database.Database before the server serves, such as the account
//	// the tests log in with.
//	func TestMain(m *testing.M) {
//		testutil.Run(m, testutil.Server{
//			Database: config.DBSqlite,
//			Redis:    false,
//			Routes:   router.Init,
//			Seed:     func() error { return nil },
//		})
//	}
func GenerateServiceTestMain(modulePath, servicePkgName string) (string, error) {
	// go/printer lays comments and composite literal elements out by their
	// source positions, so the import group and the Server literal take
	// positions on successive lines of a fabricated file: the doc comment
	// of the first project import starts a line of its own, and the Server
	// literal breaks across lines, one field per line.
	fset := token.NewFileSet()
	lines := goast.NewLineSet(fset)

	specs := []ast.Spec{importSpecAt(scaffoldImportTesting, "", lines.Next())}
	for i, dir := range ggconst.ProjectImportDirs {
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
			keyValue("Routes", sel(ident("router"), "Init"), routesPos),
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
		Name: ast.NewIdent(servicePkgName + "_test"),
		Decls: []ast.Decl{
			&ast.GenDecl{Tok: token.IMPORT, Specs: specs},
			&ast.FuncDecl{
				Doc:  commentGroup(serviceTestMainDoc...),
				Name: ast.NewIdent("TestMain"),
				Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{field("m", &ast.StarExpr{X: sel(ident("testing"), "M")})}}},
				Body: &ast.BlockStmt{List: []ast.Stmt{exprStmt(call(sel(ident("testutil"), "Run"), ident("m"), server))}},
			},
		},
	}
	return FormatNodeExtraWithFileSet(file, fset)
}

// PackageDeclaresTestMain reports whether a test file in dir declares
// TestMain at package level, be it main_test.go or any other test file,
// since the test binary of a package takes its TestMain from any of them. A
// missing directory declares none. A test file that does not parse is an
// error: gg cannot tell what it declares.
func PackageDeclaresTestMain(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ggconst.PatternTestFile) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return false, errors.Wrapf(err, "reading the test file %s", path)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && goast.IsTestMainFunc(fn) {
				return true, nil
			}
		}
	}
	return false, nil
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
