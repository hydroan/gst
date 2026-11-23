package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/stretchr/testify/require"
)

// TestRoutesRunListsTheModelsAsATreeWithTheirRoutes pins the tree gg routes
// prints for a project, read from its model declarations with no gg gen run
// before: the models the way their files nest, a model under the one the
// file named after its directory declares and under the directory where
// there is no such file, each with what backs and serves it; under each its
// routes, the columns after the path lined up across the whole tree and no
// line trailing spaces, with the service file of an action declaring
// Service(), the public mark and the rpc of an action a model served over
// gRPC carries there; a model declaring no route; then the counts, and the
// framework modules whose routes are not among them. It is the example
// printRouteTree gives.
func TestRoutesRunListsTheModelsAsATreeWithTheirRoutes(t *testing.T) {
	chdirToRoutedProject(t)

	var out bytes.Buffer
	require.NoError(t, routesRun(&out, nil))
	require.Equal(t, `Audit  model/audit.go  table
Ping  model/ping.go
└── GET     /api/pings                          List    service/ping/list.go        public
Sample  model/sample.go  table  grpc
├── POST    /api/samples                        Create  service/sample/create.go            rpc CreateSample
├── GET     /api/samples                        List                                public  rpc ListSample
├── GET     /api/samples/export                 Export  service/sample/export.go
├── GET     /api/samples/:sample                Get                                         rpc GetSample
└── Item  model/sample/item.go  table
    └── DELETE  /api/samples/:sample/items/:id  Delete
tool/
└── Label  model/tool/label.go
    └── GET     /api/tool/labels                List    service/tool/label/list.go

5 models, 7 routes (2 public)
  Not listed: the routes of the framework modules module/ registers at run time (helloworld); the running service lists every route at /docs/index.html
`, out.String())
}

// TestRoutesRunKeepsWhatTheTextsMatch pins the texts of gg routes: a text
// keeps the routes whose line or whose model's line holds it, letter case
// aside, a mark being a word of the line like any other; a text with a slash
// keeps the routes serving it as an address as well; several texts keep the
// routes all of them match; a model declaring no route is kept by its own
// line; the models and the directories above a model kept stay, without
// routes; and texts keeping nothing say so.
func TestRoutesRunKeepsWhatTheTextsMatch(t *testing.T) {
	chdirToRoutedProject(t)
	notListed := "  Not listed: the routes of the framework modules module/ registers at run time (helloworld); the running service lists every route at /docs/index.html\n"

	for _, tt := range []struct {
		name  string
		texts []string
		want  string
	}{
		{name: "the name of a model keeps its routes, under the model above it", texts: []string{"ITEM"}, want: `Sample  model/sample.go  table  grpc
└── Item  model/sample/item.go  table
    └── DELETE  /api/samples/:sample/items/:id  Delete

1 model, 1 route (0 public), matching ITEM
`},
		{name: "a mark keeps the routes carrying it", texts: []string{"public"}, want: `Ping  model/ping.go
└── GET  /api/pings    List  service/ping/list.go  public
Sample  model/sample.go  table  grpc
└── GET  /api/samples  List                        public  rpc ListSample

2 models, 2 routes (2 public), matching public
`},
		{name: "an address keeps the routes serving it", texts: []string{"http://localhost:8080/api/samples/7?_expand=items"}, want: `Sample  model/sample.go  table  grpc
└── GET  /api/samples/:sample  Get  rpc GetSample

1 model, 1 route (0 public), matching http://localhost:8080/api/samples/7?_expand=items
`},
		{name: "several texts keep the routes all of them match", texts: []string{"grpc", "service/"}, want: `Sample  model/sample.go  table  grpc
├── POST  /api/samples         Create  service/sample/create.go  rpc CreateSample
└── GET   /api/samples/export  Export  service/sample/export.go

1 model, 2 routes (0 public), matching grpc service/
`},
		{name: "a model declaring no route is kept by its own line", texts: []string{"audit"}, want: `Audit  model/audit.go  table

1 model, 0 routes (0 public), matching audit
`},
		{name: "a model under a directory stays under it", texts: []string{"label"}, want: `tool/
└── Label  model/tool/label.go
    └── GET  /api/tool/labels  List  service/tool/label/list.go

1 model, 1 route (0 public), matching label
`},
		{name: "texts keeping nothing", texts: []string{"sample", "nothing"}, want: "No model or route matches sample nothing\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, routesRun(&out, tt.texts))
			require.Equal(t, tt.want+notListed, out.String())
		})
	}
}

// TestRoutesRunOfAProjectWithoutModels pins what gg routes says for a
// project declaring no model yet, and that a directory holding no model
// directory is refused.
func TestRoutesRunOfAProjectWithoutModels(t *testing.T) {
	writeGoMod(t, t.TempDir(), "tmpapp")
	var out bytes.Buffer
	require.EqualError(t, routesRun(&out, nil), "model dir not found: model")

	writeProjectFile(t, filepath.Join(ggconst.DirModel, "doc.go"), "package model\n")
	require.NoError(t, routesRun(&out, nil))
	require.Equal(t, "No model yet: declare the first one in a file under model/\n", out.String())
}

// TestKeepRoutes pins the examples keepRoutes gives.
func TestKeepRoutes(t *testing.T) {
	list := listedRoute{Method: "GET", Path: "/api/records", Action: "List"}
	remove := listedRoute{Method: "DELETE", Path: "/api/records/:id", Action: "Delete"}
	items := listedRoute{Method: "GET", Path: "/api/items", Action: "List"}
	record := listedModel{Name: "Record", File: "model/record.go", Routes: []listedRoute{list, remove}}
	item := listedModel{Name: "Item", File: "model/item.go", Routes: []listedRoute{items}}
	models := []listedModel{record, item}
	only := func(m listedModel, routes ...listedRoute) listedModel {
		m.Routes = routes
		return m
	}

	require.Equal(t, models, keepRoutes(models, nil))
	require.Equal(t, []listedModel{record}, keepRoutes(models, []string{"record"}))
	require.Equal(t, []listedModel{only(record, list), item}, keepRoutes(models, []string{"get"}))
	require.Equal(t, []listedModel{only(record, remove)}, keepRoutes(models, []string{"/api/records/7"}))
	require.Equal(t, []listedModel{item}, keepRoutes(models, []string{"get", "/api/items"}))
	require.Equal(t, []listedModel{record, item}, models, "the models given keep their routes")
}

// TestRouteTreeNestsTheModelsTheWayTheirFilesNest pins where routeTree puts
// a model: under the model of the file named after its directory, level
// after level; under a directory of its own where no file is named after the
// directory, or the file declares several models, which then stand side by
// side ahead of it. A model not listed is in the tree only above one that
// is, and then without its routes.
func TestRouteTreeNestsTheModelsTheWayTheirFilesNest(t *testing.T) {
	served := []listedRoute{{Method: "GET", Path: "/api/served", Action: "List"}}
	models := []listedModel{
		{Name: "Sample", File: "model/sample.go", Routes: served},
		{Name: "Item", File: "model/sample/item.go", Routes: served},
		{Name: "Entry", File: "model/sample/item/entry.go", Routes: served},
		{Name: "Box", File: "model/store.go", Routes: served},
		{Name: "Shelf", File: "model/store.go", Routes: served},
		{Name: "Slot", File: "model/store/slot.go", Routes: served},
		{Name: "Label", File: "model/tool/label.go", Routes: served},
	}
	// outline writes the tree a node a line, each under the node above it by
	// a space: a model by its name and the number of its routes, in brackets
	// when it stands above the models listed, a directory by its path.
	var outline func(nodes []*routeNode, indent string) []string
	outline = func(nodes []*routeNode, indent string) []string {
		var lines []string
		for _, node := range nodes {
			switch {
			case node.model == nil:
				lines = append(lines, indent+node.key+"/")
			case node.above:
				lines = append(lines, fmt.Sprintf("%s(%s %d)", indent, node.model.Name, len(node.model.Routes)))
			default:
				lines = append(lines, fmt.Sprintf("%s%s %d", indent, node.model.Name, len(node.model.Routes)))
			}
			lines = append(lines, outline(node.children, indent+" ")...)
		}
		return lines
	}

	require.Equal(t, []string{
		"Sample 1",
		" Item 1",
		"  Entry 1",
		"Box 1",
		"Shelf 1",
		"store/",
		" Slot 1",
		"tool/",
		" Label 1",
	}, outline(routeTree(models, models), ""))

	require.Equal(t, []string{
		"(Sample 0)",
		" (Item 0)",
		"  Entry 1",
		"store/",
		" Slot 1",
	}, outline(routeTree(models, []listedModel{models[2], models[5]}), ""))
	require.Empty(t, routeTree(models, nil))
}

// TestPrintRouteTreeDrawsItsExample pins the example printRouteTree gives:
// the routes of a model under it and the models nested under it after them,
// a directory by its name, the columns after the path lined up across the
// levels.
func TestPrintRouteTreeDrawsItsExample(t *testing.T) {
	models := []listedModel{
		{Name: "Sample", File: "model/sample.go", Table: true, GRPC: true, Routes: []listedRoute{
			{Method: "POST", Path: "/api/samples", Action: "Create", Service: "service/sample/create.go", RPC: "CreateSample"},
			{Method: "GET", Path: "/api/samples", Action: "List", Public: true, RPC: "ListSample"},
		}},
		{Name: "Item", File: "model/sample/item.go", Table: true, Routes: []listedRoute{
			{Method: "DELETE", Path: "/api/samples/:sample/items/:id", Action: "Delete"},
		}},
		{Name: "Label", File: "model/tool/label.go", Routes: []listedRoute{
			{Method: "GET", Path: "/api/tool/labels", Action: "List", Service: "service/tool/label/list.go"},
		}},
	}

	var out bytes.Buffer
	printRouteTree(&out, routeTree(models, models))
	require.Equal(t, `Sample  model/sample.go  table  grpc
├── POST    /api/samples                        Create  service/sample/create.go            rpc CreateSample
├── GET     /api/samples                        List                                public  rpc ListSample
└── Item  model/sample/item.go  table
    └── DELETE  /api/samples/:sample/items/:id  Delete
tool/
└── Label  model/tool/label.go
    └── GET     /api/tool/labels                List    service/tool/label/list.go
`, out.String())

	out.Reset()
	printRouteTree(&out, nil)
	require.Empty(t, out.String())
}

// TestServesAddress pins the examples servesAddress gives.
func TestServesAddress(t *testing.T) {
	const pattern = "/api/samples/:sample/items"
	for address, want := range map[string]bool{
		"/api/samples/7/items":                              true,
		"http://localhost:8080/api/samples/7/items?_page=2": true,
		"/api/samples/7/items/":                             true,
		"/api/samples/7":                                    false,
		"/api/samples/7/items/9":                            false,
		"/api/samples//items":                               false,
		"/api/others/7/items":                               false,
	} {
		require.Equal(t, want, servesAddress(pattern, address), address)
	}
}

// TestRegisteredFrameworkModules pins the example registeredFrameworkModules
// gives, and that a project without a module package registers none.
func TestRegisteredFrameworkModules(t *testing.T) {
	t.Chdir(t.TempDir())
	modules, err := registeredFrameworkModules()
	require.NoError(t, err)
	require.Empty(t, modules)

	writeProjectFile(t, filepath.Join(ggconst.DirModule, "module.go"), `package module

import (
	"github.com/hydroan/gst/module/helloworld"
	"github.com/hydroan/gst/module/iam"
)

func init() {
	iam.Register()
	helloworld.Register()
}
`)
	// A test file imports a module package for its helpers, and registers
	// nothing the service serves.
	writeProjectFile(t, filepath.Join(ggconst.DirModule, "module_test.go"), "package module\n\nimport _ \"github.com/hydroan/gst/module/mfa\"\n")
	modules, err = registeredFrameworkModules()
	require.NoError(t, err)
	require.Equal(t, []string{"helloworld", "iam"}, modules)
}

// chdirToRoutedProject moves the test into a project of its own, the module
// tmpapp, declaring five models: Sample, with a table, served over gRPC as
// well, its routes of four kinds — an action with service code, a public
// one, one only HTTP carries and a plain one; Item, nested under it; Audit,
// a table and no route; Ping, an action with no table; and Label, in a
// directory no model file is named after. Its module package registers the
// framework module helloworld.
func chdirToRoutedProject(t *testing.T) {
	t.Helper()
	writeGoMod(t, t.TempDir(), "tmpapp")

	writeProjectFile(t, filepath.Join(ggconst.DirModel, "sample.go"), `package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	Name string `+"`json:\"name\" pb:\"1\"`"+`
	model.Base
}

func (Sample) Design() {
	Migrate()
	GRPC()
	Endpoint("samples")
	Param("sample")
	Create(func() {
		Service()
	})
	List(func() {
		Public()
	})
	Get(func() {})
	Export(func() {
		Service()
	})
}
`)
	writeProjectFile(t, filepath.Join(ggconst.DirModel, "sample", "item.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Item struct {
	model.Base
}

func (Item) Design() {
	Migrate()
	Endpoint("items")
	Delete(func() {})
}
`)
	writeProjectFile(t, filepath.Join(ggconst.DirModel, "audit.go"), `package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Audit struct {
	model.Base
}

func (Audit) Design() {
	Migrate()
}
`)
	writeProjectFile(t, filepath.Join(ggconst.DirModel, "ping.go"), `package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Ping struct {
	model.Empty
}

func (Ping) Design() {
	List(func() {
		Public()
		Service()
		Result[*PingRsp]()
	})
}

type PingRsp struct{}
`)
	writeProjectFile(t, filepath.Join(ggconst.DirModel, "tool", "label.go"), `package tool

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Label struct {
	model.Empty
}

func (Label) Design() {
	Endpoint("labels")
	List(func() {
		Service()
		Result[*LabelRsp]()
	})
}

type LabelRsp struct{}
`)
	writeProjectFile(t, filepath.Join(ggconst.DirModule, "module.go"), `package module

import "github.com/hydroan/gst/module/helloworld"

func init() {
	helloworld.Register()
}
`)
}
