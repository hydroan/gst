package goast_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/hydroan/gst/internal/goast"
	"github.com/stretchr/testify/require"
)

// parseStmts parses the statements of a function body.
func parseStmts(t *testing.T, body string) []ast.Stmt {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "sample.go", "package sample\n\nfunc run() {\n"+body+"\n}\n", 0)
	require.NoError(t, err)
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	require.True(t, ok)
	return fn.Body.List
}

// TestNilChecked pins the examples in the NilChecked documentation, and a
// comparison written the other way round or with another operator.
func TestNilChecked(t *testing.T) {
	for _, tc := range []struct {
		stmt string
		want string
	}{
		{stmt: "if err != nil {}", want: "err"},
		{stmt: "if err := load(); err != nil {}", want: "err"},
		{stmt: "if err != nil && retry {}"},
		{stmt: "if errors.Is(err, io.EOF) {}"},
		{stmt: "if nil != err {}"},
		{stmt: "if err == nil {}"},
		{stmt: "if (err != nil) {}"},
	} {
		t.Run(tc.stmt, func(t *testing.T) {
			ifStmt, ok := parseStmts(t, tc.stmt)[0].(*ast.IfStmt)
			require.True(t, ok)
			checked := goast.NilChecked(ifStmt)
			if tc.want == "" {
				require.Nil(t, checked)
				return
			}
			require.NotNil(t, checked)
			require.Equal(t, tc.want, checked.Name)
		})
	}
}

func TestAlwaysLeaves(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{name: "a return", body: "log()\nreturn", want: true},
		{name: "a continue", body: "for {\ncontinue\n}", want: false},
		{name: "a panic", body: "panic(err)", want: true},
		{name: "a call of another function", body: "exit(err)", want: false},
		{name: "a return that is not last", body: "if broken {\nreturn\n}\nlog()", want: false},
		{name: "no statement", body: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, goast.AlwaysLeaves(parseStmts(t, tc.body)))
		})
	}

	loop, ok := parseStmts(t, "for {\nlog()\ncontinue\n}")[0].(*ast.ForStmt)
	require.True(t, ok)
	require.True(t, goast.AlwaysLeaves(loop.Body.List), "a branch statement leaves the list it ends")
}
