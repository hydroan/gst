package modelinfo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/dsl"
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
						Endpoint: "users",
						Migrate:  false,
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
						Endpoint: "groups",
						Migrate:  false,
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
						Endpoint: "devices",
						Migrate:  false,
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
						Endpoint: "users",
						Migrate:  false,
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
						Endpoint: "groups",
						Migrate:  false,
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
						Endpoint: "devices",
						Migrate:  false,
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
