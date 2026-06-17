package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/spf13/cobra"
)

type modelRoute struct {
	Model  string
	Req    string
	Rsp    string
	Source string
	Scope  string
	Path   string
	Method string
	Phase  string
	Param  string
}

type modelRoutesPrintOptions struct {
	Detail bool
	Filter string
	Scope  string
	color  bool
}

type modelRoutesCommandOptions struct {
	detail bool
	model  bool
	scope  string
}

var routesCmd = newRoutesCmd()

func newRoutesCmd() *cobra.Command {
	opts := new(modelRoutesCommandOptions)
	cmd := &cobra.Command{
		Use:   "routes [filter]",
		Short: "print current route hierarchy",
		Long: `Print the current route hierarchy.
This command analyzes generated router registrations and displays routes grouped
by model source files so developers can understand model and route structure.

Optional filter parameter can match model names, source files, route paths, or phases.
For example: 'gg routes conversation' will show router routes related to conversation.
Use --model to display routes grouped by model source files.
Use --scope auth or --scope pub to display only authenticated or public routes.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := ""
			if len(args) > 0 {
				filter = args[0]
			}
			return runModelRoutes(cmd.OutOrStdout(), filter, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.detail, "detail", false, "show request, response, param, and source details")
	cmd.Flags().BoolVar(&opts.model, "model", false, "group routes by model source files")
	cmd.Flags().StringVar(&opts.scope, "scope", "", "filter routes by scope: auth, pub, public")
	return cmd
}

func runModelRoutes(w io.Writer, filter string, opts *modelRoutesCommandOptions) error {
	routerFile := filepath.Join(routerDir, "router.go")
	routes, err := parseModelRoutesFromProject(routerFile, modelDir)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		fmt.Fprintln(w, "No routes found. Please run 'gg gen' first to generate routes.")
		return nil
	}

	scope, err := normalizeRouteScope(opts.scope)
	if err != nil {
		return err
	}
	if scope != "" {
		routes = filterRoutesByScope(routes, scope)
		if len(routes) == 0 {
			fmt.Fprintf(w, "No routes found matching scope: %s\n", opts.scope)
			return nil
		}
	}

	if filter != "" {
		routes = filterModelRoutes(routes, filter)
		if len(routes) == 0 {
			fmt.Fprintf(w, "No routes found matching filter: %s\n", filter)
			return nil
		}
	}

	printOpts := modelRoutesPrintOptions{
		Detail: opts.detail,
		Filter: filter,
		Scope:  scope,
	}
	if opts.model {
		printModelRoutes(w, routes, printOpts)
		return nil
	}
	printRouterRoutes(w, routes, printOpts)
	return nil
}

// parseModelRoutesFromProject parses generated router registrations and links them to model files.
func parseModelRoutesFromProject(routerFile, modelRoot string) ([]modelRoute, error) {
	if !fileExists(routerFile) {
		return nil, errors.Newf("router file not found: %s. Please run 'gg gen' first", routerFile)
	}

	modelSources, err := scanModelSources(modelRoot)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routerFile, nil, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse router file %s", routerFile)
	}

	routes := make([]modelRoute, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isRouterRegisterCall(call) {
			return true
		}

		route, ok := modelRouteFromCall(call, modelSources)
		if ok {
			routes = append(routes, route)
		}
		return true
	})
	return routes, nil
}

func modelRouteFromCall(call *ast.CallExpr, modelSources map[string]string) (modelRoute, bool) {
	index, ok := call.Fun.(*ast.IndexListExpr)
	if !ok || len(index.Indices) != 3 || len(call.Args) < 4 {
		return modelRoute{}, false
	}

	model := exprString(index.Indices[0])
	req := exprString(index.Indices[1])
	rsp := exprString(index.Indices[2])
	path, ok := stringLiteralValue(call.Args[1])
	if !ok {
		return modelRoute{}, false
	}
	phase, ok := selectorName(call.Args[3])
	if !ok {
		return modelRoute{}, false
	}

	return modelRoute{
		Model:  model,
		Req:    req,
		Rsp:    rsp,
		Source: lookupModelSource(modelSources, model),
		Scope:  routeScope(call.Args[0]),
		Path:   path,
		Method: routePhaseMethod(phase),
		Phase:  phase,
		Param:  routeParamName(call.Args[2]),
	}, true
}

func isRouterRegisterCall(call *ast.CallExpr) bool {
	index, ok := call.Fun.(*ast.IndexListExpr)
	if !ok {
		return false
	}
	selector, ok := index.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Register" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "router"
}

func scanModelSources(modelRoot string) (map[string]string, error) {
	sources := make(map[string]string)
	typeOnly := make(map[string]string)
	duplicates := make(map[string]bool)

	if _, err := os.Stat(modelRoot); os.IsNotExist(err) {
		return sources, nil
	}

	sourceBase := filepath.Dir(modelRoot)
	err := filepath.WalkDir(modelRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return errors.Wrapf(err, "failed to parse model file %s", path)
		}

		rel, err := filepath.Rel(sourceBase, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				key := file.Name.Name + "." + typeSpec.Name.Name
				sources[key] = rel
				if existing, ok := typeOnly[typeSpec.Name.Name]; ok && existing != rel {
					duplicates[typeSpec.Name.Name] = true
					continue
				}
				typeOnly[typeSpec.Name.Name] = rel
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for name, source := range typeOnly {
		if !duplicates[name] {
			sources[name] = source
		}
	}
	return sources, nil
}

func lookupModelSource(sources map[string]string, model string) string {
	key := strings.TrimPrefix(model, "*")
	if source, ok := sources[key]; ok {
		return source
	}
	if _, typeName, ok := splitQualifiedType(key); ok {
		if source, ok := sources[typeName]; ok {
			return source
		}
	}
	return ""
}

func filterModelRoutes(routes []modelRoute, filter string) []modelRoute {
	filter = strings.ToLower(strings.Trim(filter, "/"))
	if filter == "" {
		return routes
	}

	filtered := make([]modelRoute, 0, len(routes))
	for _, route := range routes {
		fields := []string{
			route.Model,
			route.Req,
			route.Rsp,
			route.Source,
			route.Scope,
			route.Path,
			route.Phase,
			route.Param,
		}
		for _, field := range fields {
			if strings.Contains(strings.ToLower(strings.Trim(field, "/")), filter) {
				filtered = append(filtered, route)
				break
			}
		}
	}
	return filtered
}

func printModelRoutes(w io.Writer, routes []modelRoute, opts modelRoutesPrintOptions) {
	opts.color = opts.color || routeColorEnabled(w)

	fmt.Fprintf(w, "%s %s\n", routeText(opts.color, clioutput.StyleInfo, "%s", clioutput.SymbolSection), routeText(opts.color, clioutput.StyleBold, "Model Routes"))
	fmt.Fprintf(w, "  %s models: %d, routes: %d, public: %d, auth: %d\n", routeText(opts.color, clioutput.StyleMuted, "%s", clioutput.SymbolItem), countRouteModels(routes), len(routes), countRouteScope(routes, "public"), countRouteScope(routes, "auth"))
	if opts.Filter != "" {
		fmt.Fprintf(w, "  %s filter: %s\n", routeText(opts.color, clioutput.StyleMuted, "%s", clioutput.SymbolItem), opts.Filter)
	}
	if opts.Scope != "" {
		fmt.Fprintf(w, "  %s scope: %s\n", routeText(opts.color, clioutput.StyleMuted, "%s", clioutput.SymbolItem), opts.Scope)
	}
	fmt.Fprintln(w)

	if len(routes) == 0 {
		fmt.Fprintln(w, "No routes found.")
		return
	}

	printModelRouteSourceTree(w, groupModelRoutesBySource(routes), opts)
}

func printRouterRoutes(w io.Writer, routes []modelRoute, opts modelRoutesPrintOptions) {
	opts.color = opts.color || routeColorEnabled(w)

	fmt.Fprintf(w, "%s %s\n", routeText(opts.color, clioutput.StyleInfo, "%s", clioutput.SymbolSection), routeText(opts.color, clioutput.StyleBold, "Router Routes"))
	fmt.Fprintf(w, "  %s models: %d, routes: %d, public: %d, auth: %d\n", routeText(opts.color, clioutput.StyleMuted, "%s", clioutput.SymbolItem), countRouteModels(routes), len(routes), countRouteScope(routes, "public"), countRouteScope(routes, "auth"))
	if opts.Filter != "" {
		fmt.Fprintf(w, "  %s filter: %s\n", routeText(opts.color, clioutput.StyleMuted, "%s", clioutput.SymbolItem), opts.Filter)
	}
	if opts.Scope != "" {
		fmt.Fprintf(w, "  %s scope: %s\n", routeText(opts.color, clioutput.StyleMuted, "%s", clioutput.SymbolItem), opts.Scope)
	}
	fmt.Fprintln(w)

	if len(routes) == 0 {
		fmt.Fprintln(w, "No routes found.")
		return
	}

	printRouterRouteModels(w, groupRoutesByModel(routes), "", opts)
}

type modelRouteSourceGroup struct {
	source string
	routes []modelRoute
}

func groupModelRoutesBySource(routes []modelRoute) []modelRouteSourceGroup {
	groupMap := make(map[string][]modelRoute)
	for _, route := range routes {
		source := trimModelSourceRoot(route.Source)
		if source == "" {
			source = "unknown"
		}
		groupMap[source] = append(groupMap[source], route)
	}

	sources := make([]string, 0, len(groupMap))
	for source := range groupMap {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool {
		return routeSourceSortKey(sources[i]) < routeSourceSortKey(sources[j])
	})

	groups := make([]modelRouteSourceGroup, 0, len(sources))
	for _, source := range sources {
		groups = append(groups, modelRouteSourceGroup{
			source: source,
			routes: groupMap[source],
		})
	}
	return groups
}

type modelRouteSourceNode struct {
	name     string
	group    *modelRouteSourceGroup
	children map[string]*modelRouteSourceNode
}

type orderedModelRoutes struct {
	model  string
	routes []modelRoute
}

func printModelRouteSourceTree(w io.Writer, groups []modelRouteSourceGroup, opts modelRoutesPrintOptions) {
	root := &modelRouteSourceNode{children: make(map[string]*modelRouteSourceNode)}
	for i := range groups {
		insertModelRouteSource(root, &groups[i])
	}
	printModelRouteSourceNodes(w, sortedModelRouteSourceNodes(root.children), "", opts)
}

func insertModelRouteSource(root *modelRouteSourceNode, group *modelRouteSourceGroup) {
	node := root
	for segment := range strings.SplitSeq(filepath.ToSlash(group.source), "/") {
		if segment == "" {
			continue
		}
		if node.children == nil {
			node.children = make(map[string]*modelRouteSourceNode)
		}
		child := node.children[segment]
		if child == nil {
			child = &modelRouteSourceNode{
				name:     segment,
				children: make(map[string]*modelRouteSourceNode),
			}
			node.children[segment] = child
		}
		node = child
	}
	node.group = group
}

func printModelRouteSourceNodes(w io.Writer, nodes []*modelRouteSourceNode, prefix string, opts modelRoutesPrintOptions) {
	for i, node := range nodes {
		isLast := i == len(nodes)-1
		connector := "├─"
		childPrefix := prefix + "│  "
		if isLast {
			connector = "└─"
			childPrefix = prefix + "   "
		}

		if node.group != nil {
			models := groupRoutesByModel(node.group.routes)
			if len(models) == 1 {
				fmt.Fprintf(w, "%s%s %s  %s\n", prefix, connector, node.name, modelRouteLabel(models[0], opts.color))
				printRouteLines(w, models[0].routes, childPrefix, opts)
			} else {
				fmt.Fprintf(w, "%s%s %s  (%d models)\n", prefix, connector, node.name, len(models))
				printModelRouteGroup(w, *node.group, childPrefix, opts)
			}
		} else {
			fmt.Fprintf(w, "%s%s %s/\n", prefix, connector, node.name)
		}
		if len(node.children) > 0 {
			printModelRouteSourceNodes(w, sortedModelRouteSourceNodes(node.children), childPrefix, opts)
		}
	}
}

func printModelRouteGroup(w io.Writer, group modelRouteSourceGroup, prefix string, opts modelRoutesPrintOptions) {
	models := groupRoutesByModel(group.routes)
	for i, model := range models {
		modelConnector := "├─"
		modelChildPrefix := prefix + "│  "
		if i == len(models)-1 {
			modelConnector = "└─"
			modelChildPrefix = prefix + "   "
		}
		fmt.Fprintf(w, "%s%s %s\n", prefix, modelConnector, modelRouteLabel(model, opts.color))
		printRouteLines(w, model.routes, modelChildPrefix, opts)
	}
}

func groupRoutesByModel(routes []modelRoute) []orderedModelRoutes {
	indexes := make(map[string]int)
	models := make([]orderedModelRoutes, 0)
	for _, route := range routes {
		index, ok := indexes[route.Model]
		if !ok {
			index = len(models)
			indexes[route.Model] = index
			models = append(models, orderedModelRoutes{model: route.Model})
		}
		models[index].routes = append(models[index].routes, route)
	}
	return models
}

func printRouterRouteModels(w io.Writer, models []orderedModelRoutes, prefix string, opts modelRoutesPrintOptions) {
	for i, model := range models {
		modelConnector := "├─"
		modelChildPrefix := prefix + "│  "
		if i == len(models)-1 {
			modelConnector = "└─"
			modelChildPrefix = prefix + "   "
		}
		fmt.Fprintf(w, "%s%s %s\n", prefix, modelConnector, modelRouteLabel(model, opts.color))
		printRouteLines(w, model.routes, modelChildPrefix, opts)
	}
}

func printRouteLines(w io.Writer, routes []modelRoute, prefix string, opts modelRoutesPrintOptions) {
	for j, route := range routes {
		routeConnector := "├─"
		routeChildPrefix := prefix + "│  "
		if j == len(routes)-1 {
			routeConnector = "└─"
			routeChildPrefix = prefix + "   "
		}
		fmt.Fprintf(w, "%s%s %s /%s", prefix, routeConnector, routeMethodColumn(route.Method, opts.color), route.Path)
		if !opts.Detail {
			fmt.Fprintln(w)
			continue
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%sphase: %s\n", routeChildPrefix, route.Phase)
		if route.Param != "" {
			fmt.Fprintf(w, "%sparam: %s\n", routeChildPrefix, route.Param)
		}
		fmt.Fprintf(w, "%sscope: %s\n", routeChildPrefix, route.Scope)
		fmt.Fprintf(w, "%sreq:   %s\n", routeChildPrefix, route.Req)
		fmt.Fprintf(w, "%srsp:   %s\n", routeChildPrefix, route.Rsp)
		if route.Source != "" {
			fmt.Fprintf(w, "%ssource: %s\n", routeChildPrefix, route.Source)
		}
	}
}

func modelRouteLabel(model orderedModelRoutes, colorize bool) string {
	return fmt.Sprintf("%s %s", model.model, routeScopeBadge(modelRouteScope(model.routes), colorize))
}

func routeMethodColumn(method string, colorize bool) string {
	style := clioutput.StylePlain
	switch method {
	case "GET":
		style = clioutput.StyleInfo
	case "POST":
		style = clioutput.StyleBlue
	case "PUT", "PATCH":
		style = clioutput.StyleMagenta
	case "DELETE":
		style = clioutput.StyleError
	}
	return routeText(colorize, style, "%-6s", method)
}

func routeScopeBadge(scope string, colorize bool) string {
	style := clioutput.StyleMuted
	switch scope {
	case "auth":
		style = clioutput.StyleBlue
	case "public":
		style = clioutput.StyleMagenta
	}
	return routeText(colorize, style, "[%s]", scope)
}

func routeText(colorize bool, style clioutput.Style, format string, args ...any) string {
	if !colorize {
		return fmt.Sprintf(format, args...)
	}
	return clioutput.Text(style, format, args...)
}

func routeColorEnabled(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func filterRoutesByScope(routes []modelRoute, scope string) []modelRoute {
	filtered := make([]modelRoute, 0)
	for _, route := range routes {
		if route.Scope == scope {
			filtered = append(filtered, route)
		}
	}
	return filtered
}

func normalizeRouteScope(scope string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "":
		return "", nil
	case "auth":
		return "auth", nil
	case "pub", "public":
		return "public", nil
	default:
		return "", errors.New("scope must be auth, pub, or public")
	}
}

func trimModelSourceRoot(source string) string {
	source = filepath.ToSlash(source)
	prefix := strings.Trim(filepath.ToSlash(modelDir), "/")
	if prefix == "" || prefix == "." {
		return source
	}
	if source == prefix {
		return ""
	}
	return strings.TrimPrefix(source, prefix+"/")
}

func sortedModelRouteSourceNodes(children map[string]*modelRouteSourceNode) []*modelRouteSourceNode {
	nodes := make([]*modelRouteSourceNode, 0, len(children))
	for _, child := range children {
		nodes = append(nodes, child)
	}
	sort.Slice(nodes, func(i, j int) bool {
		left := modelRouteSourceNodeSortKey(nodes[i])
		right := modelRouteSourceNodeSortKey(nodes[j])
		if left != right {
			return left < right
		}
		return nodes[i].name < nodes[j].name
	})
	return nodes
}

func modelRouteSourceNodeSortKey(node *modelRouteSourceNode) string {
	name := strings.TrimSuffix(node.name, ".go")
	kind := "1"
	if node.group != nil {
		kind = "0"
	}
	return name + "\x00" + kind
}

func modelRouteScope(routes []modelRoute) string {
	scopes := make(map[string]struct{})
	for _, route := range routes {
		if route.Scope != "" {
			scopes[route.Scope] = struct{}{}
		}
	}
	if len(scopes) == 1 {
		for scope := range scopes {
			return scope
		}
	}
	return "mixed"
}

func routeSourceSortKey(source string) string {
	if source == "" {
		return "zzzzzz/unknown"
	}
	return source
}

func countRouteModels(routes []modelRoute) int {
	models := make(map[string]struct{})
	for _, route := range routes {
		models[route.Model] = struct{}{}
	}
	return len(models)
}

func countRouteScope(routes []modelRoute, scope string) int {
	count := 0
	for _, route := range routes {
		if route.Scope == scope {
			count++
		}
	}
	return count
}

func routeScope(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "unknown"
	}
	name, ok := selectorName(call.Fun)
	if !ok {
		return "unknown"
	}
	switch name {
	case "Pub":
		return "public"
	case "Auth":
		return "auth"
	default:
		return strings.ToLower(name)
	}
}

func routeParamName(expr ast.Expr) string {
	unary, ok := expr.(*ast.UnaryExpr)
	if !ok {
		return ""
	}
	lit, ok := unary.X.(*ast.CompositeLit)
	if !ok {
		return ""
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "ParamName" {
			continue
		}
		value, ok := stringLiteralValue(kv.Value)
		if ok {
			return value
		}
	}
	return ""
}

func routePhaseMethod(phase string) string {
	switch phase {
	case "Create", "CreateMany", "Import", "Export":
		return "POST"
	case "Delete", "DeleteMany":
		return "DELETE"
	case "Update", "UpdateMany":
		return "PUT"
	case "Patch", "PatchMany":
		return "PATCH"
	case "List", "Get":
		return "GET"
	default:
		return ""
	}
}

func selectorName(expr ast.Expr) (string, bool) {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	return selector.Sel.Name, true
}

func stringLiteralValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

func exprString(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return "*" + exprString(value.X)
	case *ast.SelectorExpr:
		return exprString(value.X) + "." + value.Sel.Name
	case *ast.IndexExpr:
		return exprString(value.X) + "[" + exprString(value.Index) + "]"
	case *ast.IndexListExpr:
		indices := make([]string, 0, len(value.Indices))
		for _, index := range value.Indices {
			indices = append(indices, exprString(index))
		}
		return exprString(value.X) + "[" + strings.Join(indices, ", ") + "]"
	default:
		return ""
	}
}

func splitQualifiedType(value string) (string, string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return "", value, false
	}
	return parts[0], parts[1], true
}
