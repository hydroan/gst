package gen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/stretchr/testify/require"
)

// recordInfo is the model Record of the root model package of helloworld,
// the model the scaffold examples are built for.
var recordInfo = &gen.ModelInfo{
	ModulePath:   "helloworld",
	ModelPkgName: "model",
	ModelName:    "Record",
	ModelVarName: "r",
	ModelFileDir: "model",
	Design:       &dsl.Design{},
}

// recordTarget locates the service file of a default action of Record.
func recordTarget(filename string) gen.ServiceTargetInfo {
	return gen.ServiceTargetInfo{
		Dir:         "service/record",
		FilePath:    "service/record/" + filename,
		ImportPath:  "helloworld/service/record",
		PackageName: "record",
	}
}

// recordAction is a default action of Record: dsl.Parse defaults both action
// types to the starred model name.
func recordAction(phase consts.Phase) *dsl.Action {
	return &dsl.Action{Enabled: true, Service: true, Payload: "*Record", Result: "*Record", Phase: phase}
}

// TestGenerateServiceTest compares the whole file GenerateServiceTest builds
// for the example of its doc comment: the Create action of the model Record,
// registered under records.
func TestGenerateServiceTest(t *testing.T) {
	got, err := gen.GenerateServiceTest(recordInfo, recordTarget("create.go"), recordAction(consts.PHASE_CREATE), "records")
	require.NoError(t, err)
	want := `package record_test

import (
	"helloworld/model"
	"testing"

	"github.com/hydroan/gst/client"
	"github.com/hydroan/gst/testutil"
	"github.com/stretchr/testify/require"
)

// TestCreate covers POST /api/records, served by Creator in create.go.
//
// The request goes through the framework client against the test server
// TestMain starts, so the route, the service and the database are exercised
// together; a login is a plain cli.Post to the login route, whose session the
// client's cookie jar keeps for the requests that follow. A rejection is
// asserted with testutil.RequireError, and rows with the testutil.Require*
// helpers.
func TestCreate(t *testing.T) {
	t.Fatal("TestCreate is a scaffold: delete this line and finish the test below")

	cli, err := client.New(testutil.BaseURL())
	require.NoError(t, err)

	rsp, err := cli.Post[model.Record]("/api/records", &model.Record{})
	require.NoError(t, err)
	require.NotNil(t, rsp)
}
`
	require.Equal(t, want, got)
}

// TestGenerateServiceTestExamples pins the example request of every action
// shape: the verb picks the client call, the route and its parameter the
// path, and the action types the request and response types.
func TestGenerateServiceTestExamples(t *testing.T) {
	tests := []struct {
		name     string
		info     *gen.ModelInfo
		target   gen.ServiceTargetInfo
		action   *dsl.Action
		route    string
		wantDoc  string
		wantCode []string
	}{
		{
			name:    "delete_names_the_row_by_id",
			info:    recordInfo,
			target:  recordTarget("delete.go"),
			action:  recordAction(consts.PHASE_DELETE),
			route:   "records/:rec",
			wantDoc: "// TestDelete covers DELETE /api/records/:rec, served by Deleter in delete.go.",
			wantCode: []string{
				`	id := "the ID of a row the test seeded"`,
				`	rsp, err := cli.Delete[model.Record]("/api/records/"+id, nil)`,
			},
		},
		{
			name:     "update_puts_the_row",
			info:     recordInfo,
			target:   recordTarget("update.go"),
			action:   recordAction(consts.PHASE_UPDATE),
			route:    "records/:rec",
			wantDoc:  "// TestUpdate covers PUT /api/records/:rec, served by Updater in update.go.",
			wantCode: []string{`	rsp, err := cli.Put[model.Record]("/api/records/"+id, &model.Record{})`},
		},
		{
			name:     "patch_patches_the_row",
			info:     recordInfo,
			target:   recordTarget("patch.go"),
			action:   recordAction(consts.PHASE_PATCH),
			route:    "records/:rec",
			wantDoc:  "// TestPatch covers PATCH /api/records/:rec, served by Patcher in patch.go.",
			wantCode: []string{`	rsp, err := cli.Patch[model.Record]("/api/records/"+id, &model.Record{})`},
		},
		{
			name:     "get_reads_the_row",
			info:     recordInfo,
			target:   recordTarget("get.go"),
			action:   recordAction(consts.PHASE_GET),
			route:    "records/:rec",
			wantDoc:  "// TestGet covers GET /api/records/:rec, served by Getter in get.go.",
			wantCode: []string{`	rsp, err := cli.Get[model.Record]("/api/records/" + id)`},
		},
		{
			name:     "list_of_the_model_decodes_the_list_result",
			info:     recordInfo,
			target:   recordTarget("list.go"),
			action:   recordAction(consts.PHASE_LIST),
			route:    "records",
			wantDoc:  "// TestList covers GET /api/records, served by Lister in list.go.",
			wantCode: []string{`	rsp, err := cli.Get[client.ListResult[*model.Record]]("/api/records")`},
		},
		{
			name:     "list_with_a_result_decodes_the_result",
			info:     recordInfo,
			target:   recordTarget("list.go"),
			action:   &dsl.Action{Enabled: true, Service: true, Payload: dsl.PayloadEmpty, Result: "*RecordListRsp", Phase: consts.PHASE_LIST},
			route:    "records",
			wantDoc:  "// TestList covers GET /api/records, served by Lister in list.go.",
			wantCode: []string{`	rsp, err := cli.Get[model.RecordListRsp]("/api/records")`},
		},
		{
			name:     "create_many_posts_the_items",
			info:     recordInfo,
			target:   recordTarget("create_many.go"),
			action:   recordAction(consts.PHASE_CREATE_MANY),
			route:    "records/batch",
			wantDoc:  "// TestCreateMany covers POST /api/records/batch, served by ManyCreator in create_many.go.",
			wantCode: []string{`	rsp, err := cli.Post[model.Record]("/api/records/batch", client.BatchItems([]*model.Record{{}}))`},
		},
		{
			name:     "delete_many_deletes_the_ids",
			info:     recordInfo,
			target:   recordTarget("delete_many.go"),
			action:   recordAction(consts.PHASE_DELETE_MANY),
			route:    "records/batch",
			wantDoc:  "// TestDeleteMany covers DELETE /api/records/batch, served by ManyDeleter in delete_many.go.",
			wantCode: []string{`	rsp, err := cli.Delete[model.Record]("/api/records/batch", client.BatchIDs([]string{id}))`},
		},
		{
			name:     "update_many_puts_the_items",
			info:     recordInfo,
			target:   recordTarget("update_many.go"),
			action:   recordAction(consts.PHASE_UPDATE_MANY),
			route:    "records/batch",
			wantDoc:  "// TestUpdateMany covers PUT /api/records/batch, served by ManyUpdater in update_many.go.",
			wantCode: []string{`	rsp, err := cli.Put[model.Record]("/api/records/batch", client.BatchItems([]*model.Record{{}}))`},
		},
		{
			name:     "patch_many_patches_the_items",
			info:     recordInfo,
			target:   recordTarget("patch_many.go"),
			action:   recordAction(consts.PHASE_PATCH_MANY),
			route:    "records/batch",
			wantDoc:  "// TestPatchMany covers PATCH /api/records/batch, served by ManyPatcher in patch_many.go.",
			wantCode: []string{`	rsp, err := cli.Patch[model.Record]("/api/records/batch", client.BatchItems([]*model.Record{{}}))`},
		},
		{
			name:    "import_uploads_a_file",
			info:    recordInfo,
			target:  recordTarget("import.go"),
			action:  recordAction(consts.PHASE_IMPORT),
			route:   "records/import",
			wantDoc: "// TestImport covers POST /api/records/import, served by Importer in import.go.",
			wantCode: []string{
				`	envelope, err := cli.Upload("/api/records/import", "records.csv", strings.NewReader("name\nsample\n"), nil)`,
				`	require.NotNil(t, envelope)`,
			},
		},
		{
			name:    "export_downloads_the_attachment",
			info:    recordInfo,
			target:  recordTarget("export.go"),
			action:  recordAction(consts.PHASE_EXPORT),
			route:   "records/export",
			wantDoc: "// TestExport covers GET /api/records/export, served by Exporter in export.go.",
			wantCode: []string{
				`	attachment, err := cli.Download("/api/records/export")`,
				`	require.NotEmpty(t, attachment.Content)`,
			},
		},
		{
			name:    "sse_consumes_the_stream",
			info:    recordInfo,
			target:  recordTarget("sse.go"),
			action:  recordAction(consts.PHASE_SSE),
			route:   "records",
			wantDoc: "// TestSSE covers GET /api/records, served by Streamer in sse.go.",
			wantCode: []string{
				`	err = cli.Stream(http.MethodGet, "/api/records", nil, func(event sse.Event) error {`,
				`		return client.ErrStopStream`,
			},
		},
		{
			name:     "filename_action_is_named_after_its_file_and_typed_by_the_dsl",
			info:     recordInfo,
			target:   recordTarget("archive.go"),
			action:   &dsl.Action{Enabled: true, Service: true, Payload: "*RecordArchiveReq", Result: "*RecordArchiveRsp", Phase: consts.PHASE_CREATE, Filename: "archive"},
			route:    "records/archive",
			wantDoc:  "// TestArchive covers POST /api/records/archive, served by Archive in archive.go.",
			wantCode: []string{`	rsp, err := cli.Post[model.RecordArchiveRsp]("/api/records/archive", &model.RecordArchiveReq{})`},
		},
		{
			name:     "empty_request_sends_no_body_and_empty_result_decodes_any",
			info:     recordInfo,
			target:   recordTarget("ping.go"),
			action:   &dsl.Action{Enabled: true, Service: true, Payload: dsl.PayloadEmpty, Result: dsl.PayloadEmpty, Phase: consts.PHASE_CREATE, Filename: "ping"},
			route:    "records/ping",
			wantDoc:  "// TestPing covers POST /api/records/ping, served by Ping in ping.go.",
			wantCode: []string{`	rsp, err := cli.Post[any]("/api/records/ping", nil)`},
		},
		{
			name:     "value_typed_request_is_a_value_literal",
			info:     recordInfo,
			target:   recordTarget("merge.go"),
			action:   &dsl.Action{Enabled: true, Service: true, Payload: "RecordItems", Result: "RecordItems", Phase: consts.PHASE_CREATE, Filename: "merge"},
			route:    "records/merge",
			wantDoc:  "// TestMerge covers POST /api/records/merge, served by Merge in merge.go.",
			wantCode: []string{`	rsp, err := cli.Post[model.RecordItems]("/api/records/merge", model.RecordItems{})`},
		},
		{
			name: "flattened_file_takes_the_package_of_its_model",
			info: &gen.ModelInfo{
				ModulePath:   "helloworld",
				ModelPkgName: "archive",
				ModelName:    "Document",
				ModelVarName: "d",
				ModelFileDir: "model/archive",
				Design:       &dsl.Design{},
			},
			target:   gen.ServiceTargetInfo{Dir: "service/archive", FilePath: "service/archive/seal.go", ImportPath: "helloworld/service/archive", PackageName: "archive"},
			action:   &dsl.Action{Enabled: true, Service: true, Payload: "*DocumentSealReq", Result: "*DocumentSealRsp", Phase: consts.PHASE_CREATE, Filename: "seal", Flatten: true},
			route:    "archive/documents/seal",
			wantDoc:  "// TestSeal covers POST /api/archive/documents/seal, served by Seal in seal.go.",
			wantCode: []string{"package archive_test\n", `	rsp, err := cli.Post[archive.DocumentSealRsp]("/api/archive/documents/seal", &archive.DocumentSealReq{})`},
		},
		{
			name: "model_package_named_like_a_framework_import_is_aliased",
			info: &gen.ModelInfo{
				ModulePath:   "helloworld",
				ModelPkgName: "client",
				ModelName:    "Session",
				ModelVarName: "s",
				ModelFileDir: "model/client",
				Design:       &dsl.Design{},
			},
			target:   gen.ServiceTargetInfo{Dir: "service/client/session", FilePath: "service/client/session/create.go", ImportPath: "helloworld/service/client/session", PackageName: "session"},
			action:   &dsl.Action{Enabled: true, Service: true, Payload: "*Session", Result: "*Session", Phase: consts.PHASE_CREATE},
			route:    "client/sessions",
			wantDoc:  "// TestCreate covers POST /api/client/sessions, served by Creator in create.go.",
			wantCode: []string{`	model_client "helloworld/model/client"`, `	rsp, err := cli.Post[model_client.Session]("/api/client/sessions", &model_client.Session{})`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gen.GenerateServiceTest(tt.info, tt.target, tt.action, tt.route)
			require.NoError(t, err)
			require.Contains(t, got, tt.wantDoc+"\n//\n")
			for _, code := range tt.wantCode {
				require.Contains(t, got, code+"\n")
			}
		})
	}
}

// TestGenerateServiceTestMain compares the whole file GenerateServiceTestMain
// builds for the example of its doc comment: the service package record of
// the module helloworld.
func TestGenerateServiceTestMain(t *testing.T) {
	got, err := gen.GenerateServiceTestMain("helloworld", "record")
	require.NoError(t, err)
	want := `package record_test

import (
	"testing"

	// The registrations of main.go: the models, modules, services and cron
	// jobs register themselves through the init of these packages.
	_ "helloworld/component"
	_ "helloworld/configx"
	_ "helloworld/cronjob"
	_ "helloworld/leader"
	_ "helloworld/lock"
	_ "helloworld/middleware"
	_ "helloworld/model"
	_ "helloworld/module"
	"helloworld/router"
	_ "helloworld/service"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/testutil"
)

// TestMain starts the test server of this package the way main.go starts the
// application: the framework bootstraps against the backing services the
// Server declares, which come up in containers of their own, the routes are
// registered, and the server serves the tests until they are done. Database
// is the database the tests run against: config.DBSqlite needs no container,
// config.DBMySQL and config.DBPostgres run in one. Redis serves the modules
// that keep sessions or cache entries in it. Seed plants baseline rows
// through database.Database before the server serves, such as the account
// the tests log in with.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Database: config.DBSqlite,
		Redis:    false,
		Routes:   router.Init,
		Seed:     func() error { return nil },
	})
}
`
	require.Equal(t, want, got)
}

// TestPackageDeclaresTestMain pins where PackageDeclaresTestMain looks for a
// TestMain: in any test file of the directory, main_test.go or not, and only
// at package-level functions.
func TestPackageDeclaresTestMain(t *testing.T) {
	testMain := "package record_test\n\nimport \"testing\"\n\nfunc TestMain(m *testing.M) {}\n"

	t.Run("no_test_file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "create.go"), "package record\n")

		declared, err := gen.PackageDeclaresTestMain(dir)
		require.NoError(t, err)
		require.False(t, declared)
	})
	t.Run("main_test_file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "main_test.go"), testMain)

		declared, err := gen.PackageDeclaresTestMain(dir)
		require.NoError(t, err)
		require.True(t, declared)
	})
	t.Run("test_main_in_a_paired_test_file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "create_test.go"), testMain)

		declared, err := gen.PackageDeclaresTestMain(dir)
		require.NoError(t, err)
		require.True(t, declared)
	})
	t.Run("a_method_named_test_main_does_not_count", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "create_test.go"), "package record_test\n\nimport \"testing\"\n\ntype suite struct{}\n\nfunc (suite) TestMain(m *testing.M) {}\n")

		declared, err := gen.PackageDeclaresTestMain(dir)
		require.NoError(t, err)
		require.False(t, declared)
	})
	t.Run("missing_directory", func(t *testing.T) {
		declared, err := gen.PackageDeclaresTestMain(filepath.Join(t.TempDir(), "missing"))
		require.NoError(t, err)
		require.False(t, declared)
	})
	t.Run("unparsable_test_file_is_an_error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "create_test.go"), "package record_test\n\nfunc {\n")

		_, err := gen.PackageDeclaresTestMain(dir)
		require.Error(t, err)
	})
}

// writeFile writes content to path.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
