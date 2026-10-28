package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/testutil/golist"
	"github.com/stretchr/testify/require"
)

// pbDocExampleFuncs are the functions of internal/pbschema and
// internal/gggen/pb whose doc comments quote what the generator writes, every
// example a passage of a golden file under testdata/pb/golden, and the model
// it writes it for a passage of the model sources gen_pb_test.go writes (see
// TestGenRunWritesTheProtobufDefinitionsOfGRPCModels).
var pbDocExampleFuncs = []string{
	"Generate", "buildMessage", "declareService", "rpcMessages", "patchRequest",
	"customRequest", "customResponse", "queryFields", "fileDescriptor",
	"handlerFile", "serviceType", "actionCalls", "handler", "streamHandler",
	"toProto", "fromProto", "elementFromProto", "conversionFuncs",
	"registrationFiles",
}

// pbDocExamplePackages are the packages pbDocExampleFuncs are declared in.
var pbDocExamplePackages = []string{
	ggconst.ImportPathGst + "/internal/pbschema",
	ggconst.ImportPathGst + "/internal/gggen/pb",
}

// TestPBDocExamplesComeFromTheGoldenFiles pins every indented code block in
// the doc comments of pbDocExampleFuncs to the golden files and the model
// sources behind them: each block must appear as a contiguous run of lines in
// one of them, indented as a whole by any amount, its blank lines and the
// indentation of each line against the others included, so an example that
// drifts from what the generator writes, down to the blank line between two
// import groups or a line a level too deep, fails here instead of misleading a
// reader.
func TestPBDocExamplesComeFromTheGoldenFiles(t *testing.T) {
	golden := goldenFileLines(t, filepath.Join("testdata", "pb", "golden"))
	fixture, err := os.ReadFile("gen_pb_test.go")
	require.NoError(t, err)
	// The model sources are raw string literals, so they spell a backtick as
	// a single quote and the test writing them turns it back.
	golden["gen_pb_test.go"] = sourceLines(strings.Split(strings.ReplaceAll(string(fixture), "'", "`"), "\n"))
	fset := token.NewFileSet()
	found := make(map[string]bool, len(pbDocExampleFuncs))
	for _, pkg := range golist.Packages(t, pbDocExamplePackages...) {
		for _, name := range pkg.GoFiles(t) {
			file, err := parser.ParseFile(fset, filepath.Join(pkg.Dir, name), nil, parser.ParseComments)
			require.NoError(t, err)
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Doc == nil || !slices.Contains(pbDocExampleFuncs, fn.Name.Name) {
					continue
				}
				found[fn.Name.Name] = true
				for i, block := range docCodeBlocks(fn.Doc.Text()) {
					if !goldenHolds(golden, block) {
						t.Errorf("%s: code block %d of the doc comment of %s is in no golden file; it starts with %q", fset.Position(fn.Pos()), i+1, fn.Name.Name, block[0])
					}
				}
			}
		}
	}
	for _, name := range pbDocExampleFuncs {
		if !found[name] {
			t.Errorf("neither package declares a function %s with a doc comment", name)
		}
	}
}

// goldenFileLines reads every file under dir into its lines (see
// sourceLines), keyed by path.
func goldenFileLines(t *testing.T, dir string) map[string][]string {
	t.Helper()
	files := make(map[string][]string)
	require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = sourceLines(strings.Split(string(data), "\n"))
		return nil
	}))
	require.NotEmpty(t, files)
	return files
}

// docCodeBlocks returns the code blocks of a doc comment text, the runs of
// tab-indented lines, each as its lines (see sourceLines) with the
// indentation they share removed (see dedented). A blank line inside a run
// keeps the run going, as it does inside a generated file.
func docCodeBlocks(text string) [][]string {
	var blocks [][]string
	var current []string
	for line := range strings.SplitSeq(text, "\n") {
		switch {
		case strings.HasPrefix(line, "\t"):
			current = append(current, line)
		case strings.TrimSpace(line) == "":
			if current != nil {
				current = append(current, line)
			}
		default:
			if block := dedented(sourceLines(current)); len(block) > 0 {
				blocks = append(blocks, block)
			}
			current = nil
		}
	}
	if block := dedented(sourceLines(current)); len(block) > 0 {
		blocks = append(blocks, block)
	}
	return blocks
}

// sourceLines returns lines trimmed of trailing whitespace, the blank lines at
// either end dropped and every run of blank lines inside kept as one: "\ta",
// "", "", "\t\tb", "" gives "\ta", "", "\t\tb".
func sourceLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// dedented returns lines with the indentation every line but a blank one
// starts with removed: "\ta", "", "\t\tb" gives "a", "", "\tb".
func dedented(lines []string) []string {
	common, found := "", false
	for _, line := range lines {
		if line == "" {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if !found {
			common, found = indent, true
			continue
		}
		for !strings.HasPrefix(indent, common) {
			common = common[:len(common)-1]
		}
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = strings.TrimPrefix(line, common)
	}
	return out
}

// goldenHolds reports whether block appears as a contiguous run of lines in
// one of the files, indented as a whole by any amount (see indentedRun).
func goldenHolds(golden map[string][]string, block []string) bool {
	for _, lines := range golden {
		for start := 0; start+len(block) <= len(lines); start++ {
			if indentedRun(lines[start:start+len(block)], block) {
				return true
			}
		}
	}
	return false
}

// indentedRun reports whether run is block behind one shared indentation:
// "\t\ta", "", "\t\t\tb" is "a", "", "\tb" behind two tabs, with a blank
// line of block blank in run as well.
func indentedRun(run, block []string) bool {
	indent, found := "", false
	for i, line := range block {
		if line == "" {
			if run[i] != "" {
				return false
			}
			continue
		}
		if !found {
			prefix, ok := strings.CutSuffix(run[i], line)
			if !ok || strings.TrimLeft(prefix, " \t") != "" {
				return false
			}
			indent, found = prefix, true
		}
		if run[i] != indent+line {
			return false
		}
	}
	return true
}
