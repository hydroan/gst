package modelinfo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/kr/pretty"
)

var defaultImportModelSource = `
package model

import "github.com/hydroan/gst/model"

type User struct {
	Name  string
	Age   int
	Email string

	model.Base
}

type Group struct {
	Name    string
	Members []User

	model.Base
}

type GroupUser struct {
	GroupId int
	UserId  int
}

type Device struct {
	Name string

	model.AutoBase
}
	`

var namedImportModelSource = `
package model

import model_auth "github.com/hydroan/gst/model"

type User struct {
	Name  string
	Age   int
	Email string

	model_auth.Base
}

type Group struct {
	Name    string
	Members []User

	model_auth.Base
}

type GroupUser struct {
	GroupId int
	UserId  int
}

type Device struct {
	Name string

	model_auth.AutoBase
}
	`

func TestFindModelsInFile(t *testing.T) {
	// The model files live in a directory of their own: a go.mod written into
	// the package directory would briefly turn it into a module of its own.
	dir := t.TempDir()
	t.Chdir(dir)
	modulePath := "github.com/hydroan/gst"
	if err := os.WriteFile("go.mod", []byte("module "+modulePath), 0o600); err != nil {
		t.Fatal(err)
	}

	modelDir := filepath.Join(dir, "model")
	if err := os.MkdirAll(modelDir, 0o750); err != nil {
		t.Fatal(err)
	}

	filename1 := filepath.Join(modelDir, "user.go")
	filename2 := filepath.Join(modelDir, "user2.go")
	if err := os.WriteFile(filename1, []byte(defaultImportModelSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename2, []byte(namedImportModelSource), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		modulePath string
		modelDir   string
		filename   string
		want       []*Model
	}{
		{
			name:       "default",
			modulePath: modulePath,
			modelDir:   modelDir,
			filename:   filename1,
			want: []*Model{
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
					ModelFilePath: filename1,
					ModelPkgName:  "model",
					ModelName:     "User",
					ModelVarName:  "u",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "users",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*User", Result: "*User"},
						Delete:     &dsl.Action{Payload: "*User", Result: "*User"},
						Update:     &dsl.Action{Payload: "*User", Result: "*User"},
						Patch:      &dsl.Action{Payload: "*User", Result: "*User"},
						List:       &dsl.Action{Payload: "*User", Result: "*User"},
						Get:        &dsl.Action{Payload: "*User", Result: "*User"},
						CreateMany: &dsl.Action{Payload: "*User", Result: "*User"},
						DeleteMany: &dsl.Action{Payload: "*User", Result: "*User"},
						UpdateMany: &dsl.Action{Payload: "*User", Result: "*User"},
						PatchMany:  &dsl.Action{Payload: "*User", Result: "*User"},
						Import:     &dsl.Action{Payload: "*User", Result: "*User"},
						Export:     &dsl.Action{Payload: "*User", Result: "*User"},
						SSE:        &dsl.Action{Payload: "*User", Result: "*User"},
						Stream:     &dsl.Action{Payload: "*User", Result: "*User"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
					ModelFilePath: filename1,
					ModelPkgName:  "model",
					ModelName:     "Group",
					ModelVarName:  "g",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "groups",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Delete:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Update:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Patch:      &dsl.Action{Payload: "*Group", Result: "*Group"},
						List:       &dsl.Action{Payload: "*Group", Result: "*Group"},
						Get:        &dsl.Action{Payload: "*Group", Result: "*Group"},
						CreateMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						DeleteMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						UpdateMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						PatchMany:  &dsl.Action{Payload: "*Group", Result: "*Group"},
						Import:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Export:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						SSE:        &dsl.Action{Payload: "*Group", Result: "*Group"},
						Stream:     &dsl.Action{Payload: "*Group", Result: "*Group"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
					ModelFilePath: filename1,
					ModelPkgName:  "model",
					ModelName:     "Device",
					ModelVarName:  "d",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "devices",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Delete:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Update:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Patch:      &dsl.Action{Payload: "*Device", Result: "*Device"},
						List:       &dsl.Action{Payload: "*Device", Result: "*Device"},
						Get:        &dsl.Action{Payload: "*Device", Result: "*Device"},
						CreateMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						DeleteMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						UpdateMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						PatchMany:  &dsl.Action{Payload: "*Device", Result: "*Device"},
						Import:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Export:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						SSE:        &dsl.Action{Payload: "*Device", Result: "*Device"},
						Stream:     &dsl.Action{Payload: "*Device", Result: "*Device"},
					},
				},
			},
		},
		{
			name:       "named",
			modulePath: modulePath,
			modelDir:   modelDir,
			filename:   filename2,
			want: []*Model{
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
					ModelFilePath: filename2,
					ModelPkgName:  "model",
					ModelName:     "User",
					ModelVarName:  "u",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "users",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*User", Result: "*User"},
						Delete:     &dsl.Action{Payload: "*User", Result: "*User"},
						Update:     &dsl.Action{Payload: "*User", Result: "*User"},
						Patch:      &dsl.Action{Payload: "*User", Result: "*User"},
						List:       &dsl.Action{Payload: "*User", Result: "*User"},
						Get:        &dsl.Action{Payload: "*User", Result: "*User"},
						CreateMany: &dsl.Action{Payload: "*User", Result: "*User"},
						DeleteMany: &dsl.Action{Payload: "*User", Result: "*User"},
						UpdateMany: &dsl.Action{Payload: "*User", Result: "*User"},
						PatchMany:  &dsl.Action{Payload: "*User", Result: "*User"},
						Import:     &dsl.Action{Payload: "*User", Result: "*User"},
						Export:     &dsl.Action{Payload: "*User", Result: "*User"},
						SSE:        &dsl.Action{Payload: "*User", Result: "*User"},
						Stream:     &dsl.Action{Payload: "*User", Result: "*User"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
					ModelFilePath: filename2,
					ModelPkgName:  "model",
					ModelName:     "Group",
					ModelVarName:  "g",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "groups",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Delete:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Update:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Patch:      &dsl.Action{Payload: "*Group", Result: "*Group"},
						List:       &dsl.Action{Payload: "*Group", Result: "*Group"},
						Get:        &dsl.Action{Payload: "*Group", Result: "*Group"},
						CreateMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						DeleteMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						UpdateMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						PatchMany:  &dsl.Action{Payload: "*Group", Result: "*Group"},
						Import:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Export:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						SSE:        &dsl.Action{Payload: "*Group", Result: "*Group"},
						Stream:     &dsl.Action{Payload: "*Group", Result: "*Group"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
					ModelFilePath: filename2,
					ModelPkgName:  "model",
					ModelName:     "Device",
					ModelVarName:  "d",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "devices",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Delete:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Update:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Patch:      &dsl.Action{Payload: "*Device", Result: "*Device"},
						List:       &dsl.Action{Payload: "*Device", Result: "*Device"},
						Get:        &dsl.Action{Payload: "*Device", Result: "*Device"},
						CreateMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						DeleteMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						UpdateMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						PatchMany:  &dsl.Action{Payload: "*Device", Result: "*Device"},
						Import:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Export:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						SSE:        &dsl.Action{Payload: "*Device", Result: "*Device"},
						Stream:     &dsl.Action{Payload: "*Device", Result: "*Device"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findModelsInFile(tt.modulePath, tt.modelDir, tt.filename)
			if err != nil {
				t.Fatalf("findModelsInFile() failed: %v", err)
			}
			var got2 []Model
			var want2 []Model
			for _, v := range got {
				got2 = append(got2, *v)
			}
			for _, v := range tt.want {
				want2 = append(want2, *v)
			}
			if !reflect.DeepEqual(got2, want2) {
				t.Errorf("findModelsInFile() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got2), pretty.Sprintf("% #v", want2))
			}
		})
	}
}
