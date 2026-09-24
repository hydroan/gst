package gen

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/goast"
)

// GenerateServiceTest builds the test scaffold gg gen writes next to the
// service file target locates when it creates the file: an external test
// package holding one test named after the action, which fails until the
// project replaces it with the test of the route. route is the route the
// router registers the action under, without the API prefix (see
// codegen.RouterTargetForAction). For the List action of the model Record
// registered under records, whose service file is service/record/list.go,
// it generates
//
//	package record_test
//
//	import "testing"
//
//	// TestList covers GET /api/records, served by Lister in list.go.
//	func TestList(t *testing.T) {
//		t.Fatal("TestList is a scaffold: replace it with the test of GET /api/records")
//	}
func GenerateServiceTest(target ServiceTargetInfo, action *dsl.Action, route string) (string, error) {
	name := serviceTestName(action)
	// The verbs are spelled like the phases (consts.Create is create), so a
	// phase converts to the verb the router registers it as.
	endpoint := consts.HTTPVerb(action.Phase).HTTPMethod() + " " + consts.APIPathPrefix + "/" + route
	src := fmt.Sprintf(`package %s_test

import "testing"

// %s covers %s, served by %s in %s.
func %s(t *testing.T) {
	t.Fatal(%q)
}
`, target.PackageName, name, endpoint, action.RoleName(), filepath.Base(target.FilePath), name, name+" is a scaffold: replace it with the test of "+endpoint)
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return "", errors.Wrap(err, "format the service test scaffold")
	}
	return string(formatted), nil
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

// GenerateServiceTestMain builds the main_test.go gg gen writes into the
// service package servicePkgName of the module modulePath along with the
// package's first test scaffold, when no test file of the package declares
// TestMain yet. It links the packages main.go imports
// (ggconst.ProjectImportDirs), so the test server registers what the
// application registers, and runs the tests on the default test server,
// which needs no container. For the package record of helloworld it
// generates
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
//		"github.com/hydroan/gst/testutil"
//	)
//
//	// TestMain starts the test server of this package the way main.go starts the
//	// application. Declare what the tests need on the Server, such as Database
//	// or Redis.
//	func TestMain(m *testing.M) {
//		testutil.Run(m, testutil.Server{
//			Routes: router.Init,
//		})
//	}
func GenerateServiceTestMain(modulePath, servicePkgName string) (string, error) {
	var imports strings.Builder
	for _, dir := range ggconst.ProjectImportDirs {
		if dir == ggconst.DirRouter {
			fmt.Fprintf(&imports, "\t%q\n", modulePath+"/"+dir)
			continue
		}
		fmt.Fprintf(&imports, "\t_ %q\n", modulePath+"/"+dir)
	}
	src := fmt.Sprintf(`package %s_test

import (
	"testing"

	// The registrations of main.go: the models, modules, services and cron
	// jobs register themselves through the init of these packages.
%s
	%q
)

// TestMain starts the test server of this package the way main.go starts the
// application. Declare what the tests need on the Server, such as Database
// or Redis.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Routes: router.Init,
	})
}
`, servicePkgName, imports.String(), ggconst.ImportPathTestutil)
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return "", errors.Wrap(err, "format the main_test.go scaffold")
	}
	return string(formatted), nil
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
