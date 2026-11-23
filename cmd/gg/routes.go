package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2/tree"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/spf13/cobra"
)

var routesCmd = &cobra.Command{
	Use:   "routes [text]...",
	Short: "List the models of the project and the routes they declare",
	Long: `List the models of the project and the routes they declare, read from the
model declarations the way gg gen reads them, so the list is the one gg gen
registers whether or not it has run.

The models are laid out as trees, the way their files nest and their routes
with them: the models of model/sample/ under the model of model/sample.go,
a directory no file is named after standing for itself. A model is listed with its file, and with table
when it has one and grpc when it is served over gRPC as well; each route
under it with its method, path and action, the service file holding its
logic when the action declares Service(), public when it takes no
authentication, and the name of its rpc.

Each text keeps the routes whose line, or whose model's line, holds it,
letter case aside; several texts keep the routes holding all of them. A
text with a slash is read as an address too, parameters filled in, and keeps
the routes serving it. The models above a model kept stay, without routes,
to say where it stands.`,
	Example: `  gg routes
  gg routes record                    the model, file, path or service names record
  gg routes public                    the routes taking no authentication
  gg routes /api/records/42/items     the routes serving this address
  gg routes delete /api/records/42    the one deleting it`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return routesRun(cmd.OutOrStdout(), args)
	},
}

// listedModel is a model of the project the way gg routes lists it: where it
// is declared, what it is backed and served by, and the routes it declares.
type listedModel struct {
	Name   string // Record
	File   string // model/sample/record.go
	Table  bool   // the model declares Migrate()
	GRPC   bool   // the model declares GRPC()
	Routes []listedRoute
}

// listedRoute is one route of a model the way gg routes lists it: an action
// registered under a method and a path.
type listedRoute struct {
	Method  string // GET
	Path    string // /api/samples/:sample/records
	Action  string // List
	Service string // service/sample/record/list.go, "" for an action declaring no Service()
	Public  bool
	RPC     string // ListRecord, "" for an action gRPC does not serve
}

// The words gg routes marks a model and a route with, which a text of the
// command line matches like any other word of a line.
const (
	routeMarkTable  = "table"
	routeMarkGRPC   = "grpc"
	routeMarkPublic = "public"
	routeMarkRPC    = "rpc"
)

// routesRun lists the models of the project in the working directory and
// their routes to w, the ones texts keep (see keepRoutes), as a tree (see
// printRouteTree), and ends on how many there are. The framework modules
// the project registers at run time declare no model of the project, so
// their routes are not among them, which the last line says, naming the
// modules.
func routesRun(w io.Writer, texts []string) error {
	modulePath, err := gghelper.ModulePath(".")
	if err != nil {
		return err
	}
	if !gghelper.FileExists(ggconst.DirModel) {
		return errors.Newf("model dir not found: %s", ggconst.DirModel)
	}
	projectCfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	scanned, err := modelinfo.ScanModels(modulePath, ggconst.DirModel, gghelper.NewProjectIgnore(), projectCfg)
	if err != nil {
		return err
	}
	models := listModels(scanned.Models)
	if len(models) == 0 {
		fmt.Fprintf(w, "No model yet: declare the first one in a file under %s/\n", ggconst.DirModel)
		return nil
	}

	listed := keepRoutes(models, texts)
	printRouteTree(w, routeTree(models, listed))

	routes, public := 0, 0
	for _, m := range listed {
		routes += len(m.Routes)
		for _, r := range m.Routes {
			if r.Public {
				public++
			}
		}
	}
	if len(listed) == 0 {
		fmt.Fprintf(w, "No model or route matches %s\n", strings.Join(texts, " "))
	} else {
		summary := fmt.Sprintf("%s, %s (%d public)", counted(len(listed), "model"), counted(routes, "route"), public) //gst:literal the word for what is counted
		if len(texts) > 0 {
			summary += ", matching " + strings.Join(texts, " ")
		}
		fmt.Fprintf(w, "\n%s\n", summary)
	}

	modules, err := registeredFrameworkModules()
	if err != nil {
		return err
	}
	if len(modules) > 0 {
		fmt.Fprintf(w, "  %s\n", clioutput.Text(clioutput.StyleMuted,
			"Not listed: the routes of the framework modules %s/ registers at run time (%s); the running service lists every route at /docs/index.html",
			ggconst.DirModule, strings.Join(modules, ", ")))
	}
	return nil
}

// listModels returns the models gg routes lists for the models of a scan,
// in the order of their files, a file before the directory named after it,
// model/sample.go before model/sample/item.go: each with the routes its
// design declares, in the order gg gen registers them, a model declaring
// none included. The path of a route is the one the router registers (see
// modelinfo.RouterTargetForAction), its service file the one gg gen writes
// for an action declaring Service() (see modelinfo.ServiceTarget), and its
// rpc the one the protobuf definition of a model declaring GRPC() carries
// (see modelinfo.RPCName), which an action only HTTP carries has none of.
func listModels(models []*modelinfo.Model) []listedModel {
	listed := make([]listedModel, 0, len(models))
	for _, m := range models {
		entry := listedModel{Name: m.ModelName, File: m.ModelFilePath, Table: m.Design.Migrate, GRPC: m.Design.GRPC}
		m.Design.Range(func(declared string, action *dsl.Action) {
			registered, _ := modelinfo.RouterTargetForAction(declared, m.Design, action)
			r := listedRoute{Method: action.Phase.HTTPMethod(), Path: registered, Action: action.Phase.Name(), Public: action.Public}
			if action.Service {
				r.Service = modelinfo.ServiceTarget(m, action, ggconst.DirModel, ggconst.DirService).FilePath
			}
			if m.Design.GRPC && !dsl.HTTPOnlyAction(action.Phase.Name()) {
				r.RPC = modelinfo.RPCName(m, declared, action)
			}
			entry.Routes = append(entry.Routes, r)
		})
		listed = append(listed, entry)
	}
	slices.SortStableFunc(listed, func(a, b listedModel) int {
		return strings.Compare(strings.TrimSuffix(a.File, ggconst.ExtensionGo), strings.TrimSuffix(b.File, ggconst.ExtensionGo))
	})
	return listed
}

// keepRoutes returns the models and routes texts keep, all of them for no
// text. A route is kept when every text matches it: its line or the line of
// its model holds the text, letter case aside, or the text is an address the
// route serves (see servesAddress). A model is kept with the routes kept of
// it, and one declaring no route when its own line holds every text. With
// the models Record of model/record.go, routes GET /api/records and DELETE
// /api/records/:id, and Item of model/item.go, route GET /api/items:
//
//	record            keeps Record with both routes
//	get               keeps Record with GET /api/records, Item with its route
//	/api/records/7    keeps Record with DELETE /api/records/:id
//	get /api/items    keeps Item with its route
func keepRoutes(models []listedModel, texts []string) []listedModel {
	if len(texts) == 0 {
		return models
	}
	var kept []listedModel
	for _, m := range models {
		header := strings.ToLower(strings.Join(m.headerCells(), " "))
		matches := func(line string, served func(address string) bool) bool {
			for _, text := range texts {
				lower := strings.ToLower(text)
				if !strings.Contains(header, lower) && !strings.Contains(line, lower) && !served(text) {
					return false
				}
			}
			return true
		}
		routes := slices.DeleteFunc(slices.Clone(m.Routes), func(r listedRoute) bool {
			return !matches(strings.ToLower(strings.Join(r.cells(), " ")), func(address string) bool {
				return strings.Contains(address, "/") && servesAddress(r.Path, address)
			})
		})
		if len(routes) > 0 || len(m.Routes) == 0 && matches("", func(string) bool { return false }) {
			m.Routes = routes
			kept = append(kept, m)
		}
	}
	return kept
}

// servesAddress reports whether the route registered at pattern serves
// address: a path, or a URL whose path is taken, in which every segment is
// the pattern's own or stands where the pattern has a parameter.
// /api/samples/:sample/items serves /api/samples/7/items,
// http://localhost:8080/api/samples/7/items?_page=2 and
// /api/samples/7/items/, and neither /api/samples/7 nor /api/samples//items.
func servesAddress(pattern, address string) bool {
	if parsed, err := url.Parse(address); err == nil && parsed.Path != "" {
		address = parsed.Path
	}
	segments := strings.Split(strings.Trim(address, "/"), "/")
	patternSegments := strings.Split(strings.Trim(pattern, "/"), "/")
	if len(segments) != len(patternSegments) {
		return false
	}
	for i, segment := range segments {
		if segment == "" || segment != patternSegments[i] && !strings.HasPrefix(patternSegments[i], ":") {
			return false
		}
	}
	return true
}

// routeNode is a node of the tree gg routes prints: a model with the routes
// listed of it, a model listed for the models under it alone, or a directory
// of the model directory.
type routeNode struct {
	// key is the path under the model directory the node stands for, a model
	// by its file less the extension: sample, sample/item.
	key string
	// model is nil for a directory.
	model *listedModel
	// above marks a model none of whose routes is listed: it is in the tree
	// for the models under it.
	above    bool
	children []*routeNode
}

// routeTree arranges the models to list, kept, the way their files nest,
// which is how their routes nest (see modelinfo.ResolveRoutes): the models
// of a directory under the model the file named after the directory
// declares, model/sample/item.go under the model of model/sample.go. Where
// no file is named after a directory, or the file declares several models,
// the directory is a node of its own. A model of models that is not among
// kept is in the tree only above one that is, and then without routes. It
// returns the nodes at the top, each level in the order of its paths.
func routeTree(models, kept []listedModel) []*routeNode {
	stem := func(m *listedModel) string {
		return strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(m.File), ggconst.DirModel+"/"), ggconst.ExtensionGo)
	}
	// The model a directory's models go under: the one model of the file
	// named after the directory.
	declared := make(map[string]int, len(models))
	for i := range models {
		declared[models[i].File]++
	}
	owners := make(map[string]*listedModel, len(models))
	for i := range models {
		if declared[models[i].File] == 1 {
			owners[stem(&models[i])] = &models[i]
		}
	}

	root := new(routeNode)
	nodes := map[string]*routeNode{"": root}
	// under returns the node the models of the directory key go under,
	// adding it to the tree, with the nodes above it, when no model listed
	// put it there.
	var under func(key string) *routeNode
	under = func(key string) *routeNode {
		if node, ok := nodes[key]; ok {
			return node
		}
		node := &routeNode{key: key}
		if owner, ok := owners[key]; ok {
			withoutRoutes := *owner
			withoutRoutes.Routes = nil
			node.model, node.above = &withoutRoutes, true
		}
		nodes[key] = node
		parent := under(parentKey(key))
		parent.children = append(parent.children, node)
		return node
	}
	// A model comes after the model its directory is named after, the models
	// being in the order of their files, so that one is in the tree by then
	// when it is listed.
	for i := range kept {
		key := stem(&kept[i])
		node := &routeNode{key: key, model: &kept[i]}
		if declared[kept[i].File] == 1 {
			nodes[key] = node
		}
		parent := under(parentKey(key))
		parent.children = append(parent.children, node)
	}
	for _, node := range nodes {
		slices.SortStableFunc(node.children, func(a, b *routeNode) int { return strings.Compare(a.key, b.key) })
	}
	return root.children
}

// parentKey returns the key of the directory holding the node of key: sample
// for sample/item, and "" for sample, the model directory itself.
func parentKey(key string) string {
	if dir := path.Dir(key); dir != "." {
		return dir
	}
	return ""
}

// printRouteTree prints the nodes to w, each the root of a tree of its
// own, one after the other: a model on a line of its own, its routes under
// it and after them the models nested under it; the columns after the path
// lined up across all the trees, a column no route has anything in left
// out, and no line trailing spaces. The branches are drawn by lipgloss.
//
//	Sample  model/sample.go  table  grpc
//	├── POST    /api/samples                        Create  service/sample/create.go            rpc CreateSample
//	├── GET     /api/samples                        List                                public  rpc ListSample
//	└── Item  model/sample/item.go  table
//	    └── DELETE  /api/samples/:sample/items/:id  Delete
//	tool/
//	└── Label  model/tool/label.go
//	    └── GET     /api/tool/labels                List    service/tool/label/list.go
func printRouteTree(w io.Writer, nodes []*routeNode) {
	// The widths of the columns, and how far right the path reaching
	// furthest ends: a path starts further right the deeper its model is
	// nested, by the width of a branch a level, and the columns after it
	// start in one place all the same.
	var widths []int
	pathEnd := 0
	// level counts the branches a line stands behind: none for a node at the
	// top, one more for each level under it.
	var measure func(node *routeNode, level int)
	measure = func(node *routeNode, level int) {
		if node.model != nil {
			for _, r := range node.model.Routes {
				for i, cell := range r.cells() {
					if i == len(widths) {
						widths = append(widths, 0)
					}
					widths[i] = max(widths[i], len(cell))
				}
				pathEnd = max(pathEnd, (level+1)*treeBranchWidth+len(r.Path))
			}
		}
		for _, child := range node.children {
			measure(child, level+1)
		}
	}
	for _, node := range nodes {
		measure(node, 0)
	}

	muted := func(text string) string { return clioutput.Text(clioutput.StyleMuted, "%s", text) }
	// branch returns what lipgloss draws for the node at level: its line,
	// and under it the lines of its routes and of the models nested under it.
	var branch func(node *routeNode, level int) any
	branch = func(node *routeNode, level int) any {
		var label string
		var routes []listedRoute
		switch {
		case node.model == nil:
			label = muted(path.Base(node.key) + "/")
		case node.above:
			label = muted(strings.Join(node.model.headerCells(), routeCellGap))
		default:
			header := node.model.headerCells()
			label = clioutput.Text(clioutput.StyleBold, "%s", header[0]) + routeCellGap + muted(strings.Join(header[1:], routeCellGap))
			routes = node.model.Routes
		}

		var under []any
		for _, r := range routes {
			cells := r.cells()
			// A line ends on its last cell holding something, so no line
			// trails the padding of the empty ones after it.
			last := 0
			for i, cell := range cells {
				if cell != "" {
					last = i
				}
			}
			styles := []clioutput.Style{routeMethodStyle(r.Method), clioutput.StylePlain, clioutput.StylePlain, clioutput.StyleMuted, clioutput.StyleWarn, clioutput.StyleMuted}
			var line strings.Builder
			for i, cell := range cells[:last+1] {
				if widths[i] == 0 {
					continue
				}
				line.WriteString(clioutput.Text(styles[i], "%s", cell))
				switch i {
				case last:
				case routePathCell:
					line.WriteString(strings.Repeat(" ", pathEnd-(level+1)*treeBranchWidth-len(cell)) + routeCellGap)
				default:
					line.WriteString(strings.Repeat(" ", widths[i]-len(cell)) + routeCellGap)
				}
			}
			under = append(under, line.String())
		}
		for _, child := range node.children {
			under = append(under, branch(child, level+1))
		}
		if len(under) == 0 {
			return label
		}
		return tree.Root(label).Child(under...)
	}
	for _, node := range nodes {
		for line := range strings.SplitSeq(fmt.Sprint(branch(node, 0)), "\n") {
			fmt.Fprintln(w, strings.TrimRight(line, " "))
		}
	}
}

// routePathCell is the cell of a route's line holding its path (see
// listedRoute.cells), routeCellGap what stands between two cells, and
// treeBranchWidth how far right lipgloss moves a line for each level of the
// tree: the width of the branch tree.DefaultEnumerator draws and of the
// space after it.
const (
	routePathCell   = 1
	routeCellGap    = "  "
	treeBranchWidth = 4
)

// headerCells returns the words of the line of a model: its name, its file,
// and the marks of what it is backed and served by.
func (m listedModel) headerCells() []string {
	cells := []string{m.Name, m.File}
	if m.Table {
		cells = append(cells, routeMarkTable)
	}
	if m.GRPC {
		cells = append(cells, routeMarkGRPC)
	}
	return cells
}

// cells returns the columns of the line of a route: method, path, action,
// service file, the public mark and the rpc, each "" where the route has
// none.
func (r listedRoute) cells() []string {
	cells := []string{r.Method, r.Path, r.Action, r.Service, "", ""}
	if r.Public {
		cells[4] = routeMarkPublic
	}
	if r.RPC != "" {
		cells[5] = routeMarkRPC + " " + r.RPC
	}
	return cells
}

// routeMethodStyle is the style gg routes prints an HTTP method in.
func routeMethodStyle(method string) clioutput.Style {
	switch method {
	case http.MethodGet:
		return clioutput.StyleInfo
	case http.MethodPost:
		return clioutput.StyleBlue
	case http.MethodPut, http.MethodPatch:
		return clioutput.StyleMagenta
	case http.MethodDelete:
		return clioutput.StyleError
	}
	return clioutput.StylePlain
}

// registeredFrameworkModules returns the names of the framework modules the
// module package of the project imports, sorted: the modules gg module add
// registers, whose routes the framework registers at run time. A module
// copied into the project is not among them: its models are the project's
// own, and listed. For a module package holding
//
//	import (
//		"github.com/hydroan/gst/module/helloworld"
//		"github.com/hydroan/gst/module/iam"
//	)
//
// it returns helloworld and iam.
func registeredFrameworkModules() ([]string, error) {
	sources, err := filepath.Glob(filepath.Join(ggconst.DirModule, "*"+ggconst.ExtensionGo))
	if err != nil {
		return nil, err
	}
	var modules []string
	for _, source := range sources {
		if strings.HasSuffix(source, ggconst.PatternTestFile) {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.ImportsOnly)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse %s", source)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if path.Dir(importPath) == frameworkModuleTree {
				modules = append(modules, path.Base(importPath))
			}
		}
	}
	slices.Sort(modules)
	return slices.Compact(modules), nil
}
