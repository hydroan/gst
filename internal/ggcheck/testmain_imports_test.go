package ggcheck_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// testMainOf returns a TestMain file of an external test package of name
// importing the packages listed as main.go imports them.
func testMainOf(name string, imports ...string) string {
	var src strings.Builder
	fmt.Fprintf(&src, "package %s_test\n\nimport (\n\t\"testing\"\n", name)
	for _, path := range imports {
		fmt.Fprintf(&src, "\n\t_ %q", path)
	}
	src.WriteString("\n)\n\nfunc TestMain(m *testing.M) {}\n")
	return src.String()
}

func TestTestMainImportsReportsAFileMissingAPresentPackage(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	writeCheckFile(t, filepath.Join("pb", "pb.gen.go"), "package pb\n")
	writeCheckFile(t, filepath.Join("service", "record", "main_test.go"), testMainOf("record", "tmpapp/model"))

	violations := runCheck(ggcheck.TestMainImports)

	assertViolationContains(t, violations, filepath.Join("service", "record", "main_test.go"), `_ "tmpapp/pb"`)
}

func TestTestMainImportsReportsAStaleBlankImport(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	writeCheckFile(t, filepath.Join("module", "module_test.go"), testMainOf("module", "tmpapp/interceptor", "tmpapp/model"))

	violations := runCheck(ggcheck.TestMainImports)

	assertViolationContains(t, violations, filepath.Join("module", "module_test.go"), "tmpapp/interceptor")
}

// TestTestMainImportsAcceptsTheFilesMirroringMainGo pins what the check
// passes: a TestMain file importing every present optional package and no
// absent one, one of an internal test package whatever it imports, and a
// project holding no optional package at all.
func TestTestMainImportsAcceptsTheFilesMirroringMainGo(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	writeCheckFile(t, filepath.Join("service", "record", "main_test.go"), testMainOf("record", "tmpapp/model"))
	writeCheckFile(t, filepath.Join("dao", "entry", "main_test.go"), "package entry\n\nimport \"testing\"\n\nfunc TestMain(m *testing.M) {}\n")

	if violations := runCheck(ggcheck.TestMainImports); len(violations) != 0 {
		t.Fatalf("checkTestMainImports() = %v in a project holding no optional package, want none", violations)
	}

	writeCheckFile(t, filepath.Join("interceptor", "interceptor.go"), "package interceptor\n")
	writeCheckFile(t, filepath.Join("pb", "pb.gen.go"), "package pb\n")
	writeCheckFile(t, filepath.Join("service", "record", "main_test.go"), testMainOf("record", "tmpapp/interceptor", "tmpapp/model", "tmpapp/pb"))

	if violations := runCheck(ggcheck.TestMainImports); len(violations) != 0 {
		t.Fatalf("checkTestMainImports() = %v for files mirroring main.go, want none", violations)
	}
}
