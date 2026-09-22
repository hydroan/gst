package ts

import (
	"flag"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/golden")

// fixtureModule is the module path the fixture packages stand in for a
// project module under.
const fixtureModule = "github.com/hydroan/gst/internal/codegen/gen/ts/fixture"

var (
	sampleRoot   = TypeRef{PkgPath: fixtureModule + "/model/sample", Name: "Sample"}
	rejectedRoot = TypeRef{PkgPath: fixtureModule + "/unsupported", Name: "Rejected"}
)

// TestGenerateMatchesGoldenFiles holds the files generated from Sample against
// testdata/golden; run it with -update to rewrite them. The golden files carry
// the examples of the Generate, render, writeInterface, property, expr, ref,
// builtin, structural, value, files, enumBody, enumZero and constantLiteral doc
// comments.
func TestGenerateMatchesGoldenFiles(t *testing.T) {
	files, err := generateFixture(t, sampleRoot)
	require.NoError(t, err)

	golden := filepath.Join("testdata", "golden")
	if *update {
		require.NoError(t, os.RemoveAll(golden))
		writeFiles(t, golden, files)
	}
	got := make(map[string]string, len(files))
	for _, f := range files {
		got[f.Path] = f.Content
	}
	require.Equal(t, readFiles(t, golden), got)
}

func TestGenerateIsDeterministic(t *testing.T) {
	first, err := generateFixture(t, sampleRoot)
	require.NoError(t, err)

	for range 20 {
		again, againErr := generateFixture(t, sampleRoot)
		require.NoError(t, againErr)
		require.Equal(t, first, again)
	}
}

func TestGenerateReportsTypesWithoutAJSONShape(t *testing.T) {
	_, err := generateFixture(t, rejectedRoot)
	var diagnostics *DiagnosticsError
	require.ErrorAs(t, err, &diagnostics)

	subjects := make([]string, 0, len(diagnostics.Diagnostics))
	for _, d := range diagnostics.Diagnostics {
		// A problem with a whole package, such as its output file name, has no
		// position; every other one points into the fixture.
		if d.Pos.IsValid() {
			require.Equal(t, filepath.Join("fixture", "unsupported", "unsupported.go"), d.Pos.Filename, d.String())
		}
		subjects = append(subjects, strings.TrimPrefix(d.Subject, fixtureModule+"/"))
	}
	require.ElementsMatch(t, []string{
		"a_b",
		"gst",
		"unsupported.Custom",
		"unsupported.HugeMax",
		"unsupported.ModeExtra",
		"unsupported.Rejected.Format",
		"unsupported.Rejected.Quoted",
		"unsupported.Rejected.box",
		"unsupported.Rejected.by_key",
		"unsupported.Rejected.hidden",
		"unsupported.Rejected.hosts",
		"unsupported.Rejected.pair",
		"unsupported.Rejected.updates",
		"unsupported.Speaker",
		"unsupported.class",
	}, subjects)
}

func TestGenerateReportsRootsItCannotFind(t *testing.T) {
	_, err := generateFixture(
		t,
		TypeRef{PkgPath: fixtureModule + "/model/sample", Name: "Missing"},
		TypeRef{PkgPath: "example.com/elsewhere", Name: "Sample"},
	)
	var diagnostics *DiagnosticsError
	require.ErrorAs(t, err, &diagnostics)

	require.Contains(t, diagnostics.Error(), "the package declares no type Missing")
	require.Contains(t, diagnostics.Error(), "the package is not part of module "+fixtureModule)
}

func TestGenerateNamesThePreludeAfterTheApplication(t *testing.T) {
	tests := map[string]struct{ appName, file string }{
		"an unset name falls back to the framework": {"", "gst.ts"},
		"a plain name": {"shop", "shop.ts"},
		"a name with characters a file cannot hold": {"Sample Shop / v2", "Sample_Shop___v2.ts"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := fixtureConfig(sampleRoot)
			cfg.AppName = tt.appName
			files, err := newGenerator(cfg, loadFixture(t)).generate()
			require.NoError(t, err)

			paths := make([]string, 0, len(files))
			for _, f := range files {
				paths = append(paths, f.Path)
			}
			require.Contains(t, paths, tt.file)
		})
	}
}

func TestGenerateWritesNothingWhenNoRouteDeclaresAType(t *testing.T) {
	files, err := newGenerator(fixtureConfig(), loadFixture(t)).generate()
	require.NoError(t, err)
	require.Empty(t, files)
}

// typescriptImage is the Node.js image the compiler runs in, and
// typescriptVersion the compiler release it installs.
const (
	typescriptImage   = "node:22-alpine"
	typescriptVersion = "5.9.2"
)

// TestGeneratedFilesCompile compiles the generated files, together with code
// that uses them, under the compiler configurations frontends commonly build
// with: a strict bundler setup, native Node.js modules, a loose CommonJS setup,
// and isolated declaration emit.
func TestGeneratedFilesCompile(t *testing.T) {
	files, err := generateFixture(t, sampleRoot)
	require.NoError(t, err)

	dir := t.TempDir()
	writeFiles(t, dir, files)
	extra := map[string]string{"consumer.ts": consumerSource, "package.json": `{"type": "module"}`}
	maps.Copy(extra, compilerConfigs)
	for name, content := range extra {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}

	var containerFiles []testcontainers.ContainerFile
	for rel := range readFiles(t, dir) {
		containerFiles = append(containerFiles, testcontainers.ContainerFile{
			HostFilePath:      filepath.Join(dir, filepath.FromSlash(rel)),
			ContainerFilePath: "/work/" + rel,
			FileMode:          0o644,
		})
	}
	script := "npm install --silent --no-audit --no-fund --prefix /opt/tsc typescript@" + typescriptVersion +
		` && cd /work && for config in tsconfig.*.json; do echo "tsc -p $config" && /opt/tsc/node_modules/.bin/tsc -p "$config" || exit 1; done`

	ctx := t.Context()
	compiler, err := testcontainers.Run(ctx, typescriptImage,
		testcontainers.WithFiles(containerFiles...),
		testcontainers.WithCmd("sh", "-c", script),
		testcontainers.WithWaitStrategy(wait.ForExit().WithExitTimeout(5*time.Minute)),
	)
	testcontainers.CleanupContainer(t, compiler)
	require.NoError(t, err)

	state, err := compiler.State(ctx)
	require.NoError(t, err)
	logs, err := compiler.Logs(ctx)
	require.NoError(t, err)
	output, err := io.ReadAll(logs)
	require.NoError(t, err)
	require.NoError(t, logs.Close())
	require.Zerof(t, state.ExitCode, "the TypeScript compiler rejected the generated files:\n%s", output)
}

// compilerConfigs are the compiler configurations TestGeneratedFilesCompile
// runs, keyed by file name.
var compilerConfigs = map[string]string{
	"tsconfig.bundler.json": `{
  "compilerOptions": {
    "strict": true, "noEmit": true, "target": "ES2022", "module": "ESNext", "moduleResolution": "bundler",
    "isolatedModules": true, "verbatimModuleSyntax": true, "erasableSyntaxOnly": true,
    "exactOptionalPropertyTypes": true, "noUncheckedIndexedAccess": true, "noUnusedLocals": true
  },
  "include": ["*.ts"]
}`,
	"tsconfig.nodenext.json": `{
  "compilerOptions": {"strict": true, "noEmit": true, "module": "NodeNext", "moduleResolution": "NodeNext"},
  "include": ["*.ts"]
}`,
	"tsconfig.commonjs.json": `{
  "compilerOptions": {"strict": false, "noEmit": true, "module": "CommonJS", "moduleResolution": "node"},
  "include": ["*.ts"]
}`,
	"tsconfig.declarations.json": `{
  "compilerOptions": {
    "strict": true, "declaration": true, "emitDeclarationOnly": true, "isolatedDeclarations": true,
    "outDir": "declarations", "module": "ESNext", "moduleResolution": "bundler"
  },
  "include": ["*.ts"],
  "exclude": ["consumer.ts"]
}`,
}

// consumerSource uses the generated declarations the way a frontend does. Each
// @ts-expect-error line must fail to compile, or the compiler reports the
// directive as unused.
const consumerSource = `import type { Envelope, IDsPayload, ItemsPayload, ListResult } from "./gst.js";
import type { Record, State } from "./record.js";
import type { Entry, Kind, Level, Permission, Sample, Status } from "./sample.js";

export const record: Record = { title: "first", state: 0 };

// @ts-expect-error a required key cannot be left out
export const incomplete: Record = { title: "first" };

export const entry: Entry = { title: "second", parent: record, state: 1 };

export const status: Status = "active";

// @ts-expect-error a value no constant declares is refused
export const unknownStatus: Status = "deleted";

export const state: State = 1;
export const kind: Kind = "";
export const level: Level = 2;
export const permission: Permission = 3;

export const listed: Envelope<ListResult<Record>> = {
  code: 0,
  data: { items: [record], total: 1 },
  msg: "success",
  trace_id: "trace",
};

export const batch: ItemsPayload<Record> = { items: [record] };
export const removal: IDsPayload = { ids: ["1"] };

export function statusOf(sample: Sample): Status | "" {
  return sample.status;
}
`

// fixtureLoad type-checks the fixture packages once per test binary.
var fixtureLoad = sync.OnceValues(func() (*loaded, error) {
	return load(fixtureConfig(sampleRoot, rejectedRoot))
})

// generateFixture renders the fixture declarations reachable from roots.
func generateFixture(t *testing.T, roots ...TypeRef) ([]File, error) {
	t.Helper()

	return newGenerator(fixtureConfig(roots...), loadFixture(t)).generate()
}

// loadFixture returns the type-checked fixture packages.
func loadFixture(t *testing.T) *loaded {
	t.Helper()

	l, err := fixtureLoad()
	require.NoError(t, err)
	return l
}

// fixtureConfig configures a run over the fixture packages from roots, rooted
// at the fixture model directory the way a project's run is rooted at its own.
func fixtureConfig(roots ...TypeRef) Config {
	return Config{Dir: ".", ModulePath: fixtureModule, RootPath: fixtureModule + "/model", Roots: roots}
}

// writeFiles writes generated files under dir.
func writeFiles(t *testing.T, dir string, files []File) {
	t.Helper()

	for _, f := range files {
		target := filepath.Join(dir, filepath.FromSlash(f.Path))
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o750))
		require.NoError(t, os.WriteFile(target, []byte(f.Content), 0o600))
	}
}

// readFiles reads every file under dir, keyed by slash-separated path.
func readFiles(t *testing.T, dir string) map[string]string {
	t.Helper()

	files := make(map[string]string)
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(content)
		return nil
	}))
	return files
}
