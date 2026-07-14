package gen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/types/consts"
)

func TestApplyServiceFileWithModelSyncForcesCanonicalServiceStruct(t *testing.T) {
	modelInfo := &ModelInfo{
		ModulePath:   "helloworld",
		ModelFileDir: "model",
		ModelPkgName: "model",
		ModelName:    "User",
	}
	exportAction := &dsl.Action{
		Enabled: true,
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.PHASE_EXPORT,
	}

	tests := []struct {
		name        string
		code        string
		action      *dsl.Action
		wantChanged bool
		want        []string // substrings that must appear in the rewritten file
		wantAbsent  []string // substrings that must not appear in the rewritten file
	}{
		{
			// A hand edit replaced the service.Base embedding with another
			// service struct and dropped the service import; the struct body is
			// generated code, so it is forced back to the canonical single
			// embedding and the extra field is discarded.
			name: "forces_body_with_foreign_embedding",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst/types"
)

type Exporter struct {
	// service.Base[*model.User, *model.User, *model.User]
	Lister
}

func (e *Exporter) Export(ctx *types.ServiceContext, users ...*model.User) (data []byte, err error) {
	return data, err
}
`,
			action:      exportAction,
			wantChanged: true,
			want: []string{
				"service.Base[*model.User, *model.User, *model.User]",
				`"github.com/hydroan/gst/service"`,
			},
			wantAbsent: []string{"Lister"},
		},
		{
			// A malformed service.Base embedding (wrong arity) is replaced by
			// the canonical one instead of gaining a duplicate next to it.
			name: "forces_body_with_malformed_embedding",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Exporter struct {
	service.Base[*model.User]
}

func (e *Exporter) Export(ctx *types.ServiceContext, users ...*model.User) (data []byte, err error) {
	return data, err
}
`,
			action:      exportAction,
			wantChanged: true,
			want: []string{
				"service.Base[*model.User, *model.User, *model.User]",
			},
		},
		{
			// A struct that was deleted entirely is regenerated, so gg gen
			// always converges on a registrable service struct.
			name: "restores_deleted_struct",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst/types"
)

func exportHeaders(users ...*model.User) []string { return nil }

var _ = types.ServiceContext{}
`,
			action:      exportAction,
			wantChanged: true,
			want: []string{
				"type Exporter struct",
				"service.Base[*model.User, *model.User, *model.User]",
				`"github.com/hydroan/gst/service"`,
			},
		},
		{
			// A struct that already has the canonical body needs no rewrite.
			name: "keeps_canonical_struct_untouched",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Exporter struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (e *Exporter) Export(ctx *types.ServiceContext, users ...*model.User) (data []byte, err error) {
	return data, err
}
`,
			action:      exportAction,
			wantChanged: false,
			want: []string{
				"service.Base[*model.User, *model.User, *model.User]",
			},
		},
		{
			// With Filename set, a canonical struct still carrying the old role
			// name belongs to the rename path, not the force-rewrite path.
			name: "skips_rewrite_when_filename_rename_applies",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, err
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Service:  true,
				Payload:  "*User",
				Result:   "*User",
				Filename: "upload",
				Phase:    consts.PHASE_CREATE,
			},
			wantChanged: true,
			want: []string{
				"type Upload struct",
				"service.Base[*model.User, *model.User, *model.User]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			changed := ApplyServiceFileWithModelSync(file, tt.action, "user", modelInfo)
			if changed != tt.wantChanged {
				t.Errorf("ApplyServiceFileWithModelSync changed = %v, want %v", changed, tt.wantChanged)
			}

			got, err := FormatNodeExtra(file)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("Expected to find %q in rewritten code, but got:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("Expected %q to be removed from rewritten code, but got:\n%s", absent, got)
				}
			}
			if count := strings.Count(got, "service.Base["); count != 1 {
				t.Errorf("Expected exactly one service.Base embedding, found %d in:\n%s", count, got)
			}
		})
	}
}
