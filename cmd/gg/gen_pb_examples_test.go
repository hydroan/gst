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

	"github.com/stretchr/testify/require"
)

// pbDocExampleFuncs are the functions of internal/gggen/pb whose doc comments
// quote what the generator writes, every example a passage of a golden file
// under testdata/pb/golden, and the model it writes it for a passage of the
// model sources gen_pb_test.go writes (see TestGenRunMatchesGolden).
var pbDocExampleFuncs = []string{
	"Generate", "buildMessage", "declareService", "rpcMessages", "customRequest",
	"customResponse", "queryFields", "descriptor", "handlerFile", "serviceType",
	"actionCalls", "handler", "streamHandler", "toProto", "fromProto",
	"conversionFuncs", "registrationFiles",
}

// TestPBDocExamplesComeFromTheGoldenFiles pins every indented code block in
// the doc comments of pbDocExampleFuncs to the golden files and the model
// sources behind them: each block, its lines trimmed and blank lines
// dropped, must appear as a contiguous run of lines in one of them, so an
// example that drifts from what the generator writes fails here instead of
// misleading a reader.
func TestPBDocExamplesComeFromTheGoldenFiles(t *testing.T) {
	golden := goldenFileLines(t, filepath.Join("testdata", "pb", "golden"))
	fixture, err := os.ReadFile("gen_pb_test.go")
	require.NoError(t, err)
	// The model sources are raw string literals, so they spell a backtick as
	// a single quote and the test writing them turns it back.
	golden["gen_pb_test.go"] = trimmedLines(strings.Split(strings.ReplaceAll(string(fixture), "'", "`"), "\n"))
	pbDir := filepath.Join("..", "..", "internal", "gggen", "pb")
	entries, err := os.ReadDir(pbDir)
	require.NoError(t, err)
	fset := token.NewFileSet()
	found := make(map[string]bool, len(pbDocExampleFuncs))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(pbDir, entry.Name()), nil, parser.ParseComments)
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
	for _, name := range pbDocExampleFuncs {
		if !found[name] {
			t.Errorf("the pb package declares no function %s with a doc comment", name)
		}
	}
}

// goldenFileLines reads every file under dir into its trimmed, non-blank
// lines, keyed by path.
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
		files[path] = trimmedLines(strings.Split(string(data), "\n"))
		return nil
	}))
	require.NotEmpty(t, files)
	return files
}

// docCodeBlocks returns the code blocks of a doc comment text, the runs of
// tab-indented lines, each as its trimmed, non-blank lines. A blank line
// inside a run keeps the run going, as it does inside a generated file.
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
			if block := trimmedLines(current); len(block) > 0 {
				blocks = append(blocks, block)
			}
			current = nil
		}
	}
	if block := trimmedLines(current); len(block) > 0 {
		blocks = append(blocks, block)
	}
	return blocks
}

// trimmedLines returns lines trimmed of surrounding whitespace, the blank
// ones dropped.
func trimmedLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// goldenHolds reports whether block appears as a contiguous run of lines in
// one of the files.
func goldenHolds(golden map[string][]string, block []string) bool {
	for _, lines := range golden {
		for start := 0; start+len(block) <= len(lines); start++ {
			if slices.Equal(lines[start:start+len(block)], block) {
				return true
			}
		}
	}
	return false
}
