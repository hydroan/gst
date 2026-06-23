package dsl

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/hydroan/gst/types/consts"
)

// Parse analyzes a Go source file and extracts DSL design information from models.
// It looks for structs that have a Design() method and parses the DSL calls within that method.
//
// The parser identifies models by finding structs that embed either model.Base or model.Empty,
// then locates their Design() method and analyzes the DSL function calls to build the Design configuration.
//
// Parameters:
//   - file: The parsed AST file node to analyze
//   - endpoint: Optional endpoint override - if provided, will overwrite the default endpoint for enabled designs
//
// Returns:
//   - map[string]*Design: A map where keys are model names and values are their parsed Design configurations
//
// Example usage:
//
//	designs := Parse(fileNode, "custom-endpoint")
//	for modelName, design := range designs {
//		fmt.Printf("Model %s has endpoint: %s\n", modelName, design.Endpoint)
//	}
//
// The parser supports various DSL patterns:
//   - Global settings: Enabled(), Endpoint("path"), Migrate(true)
//   - Action configuration: Create().Payload[Type].Result[Type]
//   - Service and visibility: Service(true), Public(false)
func Parse(file *ast.File, endpoint string) map[string]*Design {
	designBase, designEmpty := parse(file)

	// If struct contains model.Base and model.Empty, then remove it from designEmpty.
	// model.Base has more priority than model.Empty.
	for name := range designBase {
		delete(designEmpty, name)
	}

	m := make(map[string]*Design)
	for name, fnDecl := range designBase {
		design := parseDesign(fnDecl)
		m[name] = design
	}
	for name, fnDecl := range designEmpty {
		design := parseDesign(fnDecl)
		// the struct has field model.Empty always should be not migrated,
		// and mark `IsEmpty` field to true
		design.Migrate = false
		design.IsEmpty = true
		m[name] = design
	}

	// Set default values for Design.
	// Declared actions default to enabled when parsed by parseAction.
	// Missing actions are initialized here and remain disabled by default.
	// Service defaults to false for both declared and missing actions.
	for name, design := range m {
		// Default endpoint is the lower case of the model name.
		if len(design.Endpoint) == 0 {
			design.Endpoint = strings.ToLower(name)
		}

		if design.Create == nil {
			design.Create = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.Delete == nil {
			design.Delete = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.Update == nil {
			design.Update = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.Patch == nil {
			design.Patch = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.List == nil {
			design.List = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.Get == nil {
			design.Get = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.CreateMany == nil {
			design.CreateMany = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.DeleteMany == nil {
			design.DeleteMany = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.UpdateMany == nil {
			design.UpdateMany = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.PatchMany == nil {
			design.PatchMany = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.Import == nil {
			design.Import = &Action{Payload: starName(name), Result: starName(name)}
		}
		if design.Export == nil {
			design.Export = &Action{Payload: starName(name), Result: starName(name)}
		}

		initDefaultAction(name, design.Create)
		initDefaultAction(name, design.Delete)
		initDefaultAction(name, design.Update)
		initDefaultAction(name, design.Patch)
		initDefaultAction(name, design.List)
		initDefaultAction(name, design.Get)
		initDefaultAction(name, design.CreateMany)
		initDefaultAction(name, design.DeleteMany)
		initDefaultAction(name, design.UpdateMany)
		initDefaultAction(name, design.PatchMany)
		initDefaultAction(name, design.Import)
		initDefaultAction(name, design.Export)
		for _, actions := range design.routes {
			for _, action := range actions {
				initDefaultAction(name, action)
			}
		}

		if len(endpoint) > 0 && design.Enabled {
			design.Endpoint = endpoint
		}

		m[name] = design
	}

	return m
}

// initDefaultAction initializes default payload and result values for an enabled action.
// If the action is enabled but has empty Payload or Result fields, they are set to
// the pointer type of the model name (e.g., "*User" for model "User").
//
// Parameters:
//   - modelName: The name of the model (e.g., "User")
//   - action: The action to initialize defaults for
//
// This function only modifies enabled actions. For disabled actions, the Payload
// and Result fields remain unchanged.
func initDefaultAction(modelName string, action *Action) {
	if action.Enabled {
		if len(action.Payload) == 0 {
			action.Payload = starName(modelName)
		}
		if len(action.Result) == 0 {
			action.Result = starName(modelName)
		}
	}
}

// parse analyzes an AST file to find all models and their Design method declarations.
// It identifies models by looking for structs that embed model.Base or model.Empty,
// then searches for their corresponding Design() method declarations.
//
// Parameters:
//   - file: The AST file node to analyze
//
// Returns:
//   - First map: Models with model.Base embedding (modelName -> Design method AST node)
//   - Second map: Models with model.Empty embedding (modelName -> Design method AST node)
//
// If a model doesn't have a Design() method, the corresponding value in the map is nil.
// This allows the caller to generate default design configurations for such models.
func parse(file *ast.File) (map[string]*ast.FuncDecl, map[string]*ast.FuncDecl) {
	designBase := make(map[string]*ast.FuncDecl)
	designEmpty := make(map[string]*ast.FuncDecl)
	if file == nil {
		return designBase, designEmpty
	}

	modelBase := FindAllModelBase(file)
	modelEmpty := FindAllModelEmpty(file)
	// Every model should always has a *ast.FuncDecl,
	// If model has no "Design" method, then the value is nil.
	// It's convenient to generate a default design for the model.
	for _, model := range modelBase {
		designBase[model] = nil
	}
	for _, model := range modelEmpty {
		designEmpty[model] = nil
	}

	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn != nil {
			if fn.Name == nil || len(fn.Name.Name) == 0 {
				continue
			}

			// Check if the model has method "Design"
			if fn.Name.Name != "Design" {
				continue
			}
			// Check if the method receiver name is the model name.
			if fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			var recvName string
			switch t := fn.Recv.List[0].Type.(type) {
			case *ast.Ident:
				if t != nil {
					recvName = t.Name
				}
			case *ast.StarExpr:
				if ident, ok := t.X.(*ast.Ident); ok && ident != nil {
					recvName = ident.Name
				}
			}
			if slices.Contains(modelBase, recvName) {
				designBase[recvName] = fn
			}
			if slices.Contains(modelEmpty, recvName) {
				designEmpty[recvName] = fn
			}
		}
	}

	return designBase, designEmpty
}

// parseDesign parses a Design method's AST declaration and extracts the DSL configuration.
// It analyzes the function body to find DSL calls like Enabled(), Endpoint(), Migrate(),
// and action configurations like Create().Payload[Type].Result[Type].
//
// Parameters:
//   - fn: The AST function declaration for the Design() method
//
// Returns:
//   - *Design: The parsed design configuration with default values applied
//
// If fn is nil or has no body, returns a default Design with Enabled=true and Migrate=false.
// The parser recognizes various DSL patterns and converts them into the Design structure.
func parseDesign(fn *ast.FuncDecl) *Design {
	defaults := &Design{Enabled: true, Migrate: false}
	// model don't have "Design" method, so returns the default design values.
	if fn == nil || fn.Body == nil || len(fn.Body.List) == 0 {
		return defaults
	}
	stmts := fn.Body.List

	for _, stmt := range stmts {
		callExpr, ok := stmt.(*ast.ExprStmt)
		if !ok || callExpr == nil || callExpr.X == nil {
			continue
		}
		call, ok := callExpr.X.(*ast.CallExpr)
		if !ok || call == nil || call.Fun == nil || len(call.Args) == 0 {
			continue
		}
		var funcName string
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun == nil {
				continue
			}
			funcName = fun.Name
		case *ast.SelectorExpr:
			if fun == nil || fun.Sel == nil {
				continue
			}
			funcName = fun.Sel.Name
		default:
			continue
		}
		if !is(funcName) {
			continue
		}

		// Parse "Enabled()".
		if funcName == "Enabled" && len(call.Args) == 1 {
			if arg, ok := call.Args[0].(*ast.Ident); ok && arg != nil {
				defaults.Enabled = arg.Name == "true"
			}
		}

		// Parse "Endpoint()".
		if funcName == "Endpoint" && len(call.Args) == 1 {
			if arg, ok := call.Args[0].(*ast.BasicLit); ok && arg != nil && arg.Kind == token.STRING {
				defaults.Endpoint = trimQuote(arg.Value)
				defaults.Endpoint = strings.TrimLeft(defaults.Endpoint, "/")
				defaults.Endpoint = strings.ReplaceAll(defaults.Endpoint, "/", "-")
			}
		}

		// Parse "Migrate()".
		if funcName == "Migrate" && len(call.Args) == 1 {
			if arg, ok := call.Args[0].(*ast.Ident); ok && arg != nil {
				defaults.Migrate = arg.Name == "true"
			}
		}

		// Parse "Param()".
		if funcName == "Param" && len(call.Args) == 1 {
			if arg, ok := call.Args[0].(*ast.BasicLit); ok && arg != nil && arg.Kind == token.STRING {
				defaults.Param = trimQuote(arg.Value)
				defaults.Param = strings.TrimFunc(defaults.Param, func(r rune) bool {
					return r == ' ' || r == '{' || r == '}' || r == '[' || r == ']' || r == ':'
				})
				defaults.Param = ":" + defaults.Param
			}
		}

		// Parse "Route()".
		// Example:
		//
		// Route("/config/apps", func() {
		// 	List(func() {
		// 		Service(true)
		// 	})
		// 	Get(func() {
		// 		Service(true)
		// 	})
		// })
		if funcName == "Route" && len(call.Args) == 2 {
			var route string
			if arg, ok := call.Args[0].(*ast.BasicLit); ok && arg != nil && arg.Kind == token.STRING {
				route = trimQuote(arg.Value)
				route = strings.TrimLeft(route, "/")
			}
			if len(route) > 0 {
				if defaults.routes == nil {
					defaults.routes = make(map[string][]*Action)
				}
				if flit, ok := call.Args[1].(*ast.FuncLit); ok && flit != nil && flit.Body != nil {
					for _, stmt := range flit.Body.List {
						expr, ok := stmt.(*ast.ExprStmt)
						if !ok || expr == nil {
							continue
						}
						call_, ok := expr.X.(*ast.CallExpr)
						if !ok || call_ == nil || call_.Fun == nil || len(call_.Args) == 0 {
							continue
						}

						var funName_ string
						switch fun := call_.Fun.(type) {
						case *ast.Ident:
							if fun == nil {
								continue
							}
							funName_ = fun.Name
						case *ast.SelectorExpr:
							if fun == nil || fun.Sel == nil {
								continue
							}
							funName_ = fun.Sel.Name
						default:
							continue
						}
						if !is(funName_) {
							continue
						}

						if act, e := parseAction(consts.PHASE_CREATE, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_DELETE, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_UPDATE, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_PATCH, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_LIST, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_GET, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_CREATE_MANY, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_DELETE_MANY, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_UPDATE_MANY, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_PATCH_MANY, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_IMPORT, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
						if act, e := parseAction(consts.PHASE_EXPORT, funName_, call_.Args[0]); e {
							defaults.routes[route] = append(defaults.routes[route], act)
						}
					}
				}
			}
		}

		if act, e := parseAction(consts.PHASE_CREATE, funcName, call.Args[0]); e {
			defaults.Create = act
		}
		if act, e := parseAction(consts.PHASE_DELETE, funcName, call.Args[0]); e {
			defaults.Delete = act
		}
		if act, e := parseAction(consts.PHASE_UPDATE, funcName, call.Args[0]); e {
			defaults.Update = act
		}
		if act, e := parseAction(consts.PHASE_PATCH, funcName, call.Args[0]); e {
			defaults.Patch = act
		}
		if act, e := parseAction(consts.PHASE_LIST, funcName, call.Args[0]); e {
			defaults.List = act
		}
		if act, e := parseAction(consts.PHASE_GET, funcName, call.Args[0]); e {
			defaults.Get = act
		}
		if act, e := parseAction(consts.PHASE_CREATE_MANY, funcName, call.Args[0]); e {
			defaults.CreateMany = act
		}
		if act, e := parseAction(consts.PHASE_DELETE_MANY, funcName, call.Args[0]); e {
			defaults.DeleteMany = act
		}
		if act, e := parseAction(consts.PHASE_UPDATE_MANY, funcName, call.Args[0]); e {
			defaults.UpdateMany = act
		}
		if act, e := parseAction(consts.PHASE_PATCH_MANY, funcName, call.Args[0]); e {
			defaults.PatchMany = act
		}
		if act, e := parseAction(consts.PHASE_IMPORT, funcName, call.Args[0]); e {
			defaults.Import = act
		}
		if act, e := parseAction(consts.PHASE_EXPORT, funcName, call.Args[0]); e {
			defaults.Export = act
		}

	}

	return defaults
}

// parseAction parses DSL configuration from an action function's body.
// It extracts Payload, Result types and configuration flags (Enabled, Service, Public)
// from the function literal passed to action methods like Create(), Update(), etc.
//
// Parameters:
//   - phase: The expected phase to match (e.g., consts.PHASE_CREATE, consts.PHASE_LIST)
//   - funcName: The actual function name being called
//   - args: The function call arguments, expected to contain a function literal
//
// Returns:
//   - actionResult: Parsed configuration including payload/result types and flags
//   - bool: true if parsing was successful, false otherwise
//
// The function parses DSL calls within the action function body:
//   - Enabled(true/false): Sets whether the action is enabled. Declared actions default to enabled.
//   - Service(true/false): Sets whether to generate service layer code
//   - Public(true/false): Sets whether the API endpoint is public
//   - Filename("name"): Sets a custom filename for the generated service file
//   - Payload[Type]: Sets the request payload type
//   - Result[Type]: Sets the response result type
//
// Example usage in DSL:
//
//	Create(func() {
//	    Service(true)
//	    Payload[CreateUserRequest]
//	    Result[*User]
//	})
func parseAction(phase consts.Phase, funcName string, expr ast.Expr) (*Action, bool) {
	var payload string
	var result string
	enabled := true     // declared actions are enabled by default
	var service bool    // default to false
	var public bool     // default to false
	var filename string // default to ""
	var flatten bool    // default to false

	if phase.MethodName() != funcName {
		return nil, false
	}
	flit, ok := expr.(*ast.FuncLit)
	if !ok {
		return nil, false
	}
	if flit == nil || flit.Body == nil {
		return nil, false
	}

	for _, stmt := range flit.Body.List {
		if expr, ok := stmt.(*ast.ExprStmt); ok && expr != nil {
			if call, ok := expr.X.(*ast.CallExpr); ok && call != nil && call.Fun != nil {

				// Parse Enabled(true)/Enabled(false)
				var isEnabledCall bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					// anonymous import: Enabled(true)
					if fun != nil && fun.Name == "Enabled" {
						isEnabledCall = true
					}
				case *ast.SelectorExpr:
					// non-anonymous import: dsl.Enabled(true)
					if fun != nil && fun.Sel != nil && fun.Sel.Name == "Enabled" {
						isEnabledCall = true
					}
				}
				if isEnabledCall && len(call.Args) > 0 && call.Args[0] != nil {
					if identExpr, ok := call.Args[0].(*ast.Ident); ok && identExpr != nil {
						// check the argument of Enabled() is true.
						enabled = identExpr.Name == "true"
					}
				}

				// Parse Service(true)/Service(false)
				var isServiceCall bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					// anonymous import: Service(true)
					if fun != nil && fun.Name == "Service" {
						isServiceCall = true
					}
				case *ast.SelectorExpr:
					// non-anonymous import: dsl.Service(true)
					if fun != nil && fun.Sel != nil && fun.Sel.Name == "Service" {
						isServiceCall = true
					}
				}
				if isServiceCall && len(call.Args) > 0 && call.Args[0] != nil {
					if identExpr, ok := call.Args[0].(*ast.Ident); ok && identExpr != nil {
						// check the argument of Service() is true.
						service = identExpr.Name == "true"
					}
				}

				// Parse Public(true)/Public(false)
				var isPublicCall bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					// anonymous import: Public(false)
					if fun != nil && fun.Name == "Public" {
						isPublicCall = true
					}
				case *ast.SelectorExpr:
					// non-anonymous import: dsl.Public(false)
					if fun != nil && fun.Sel != nil && fun.Sel.Name == "Public" {
						isPublicCall = true
					}
				}

				if isPublicCall && len(call.Args) > 0 && call.Args[0] != nil {
					if identExpr, ok := call.Args[0].(*ast.Ident); ok && identExpr != nil {
						// check the argument of Public() is true.
						public = identExpr.Name == "true"
					}
				}

				// Parse Filename("upload")/Filename("parse")
				var isFilenameCall bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					// anonymous import: Filename("upload")
					if fun != nil && fun.Name == "Filename" {
						isFilenameCall = true
					}
				case *ast.SelectorExpr:
					// non-anonymous import: dsl.Filename("upload")
					if fun != nil && fun.Sel != nil && fun.Sel.Name == "Filename" {
						isFilenameCall = true
					}
				}
				if isFilenameCall && len(call.Args) > 0 && call.Args[0] != nil {
					if arg, ok := call.Args[0].(*ast.BasicLit); ok && arg != nil && arg.Kind == token.STRING {
						filename = trimQuote(arg.Value)
					}
				}

				// Parse Flatten()
				var isFlattenCall bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					// anonymous import: Flatten()
					if fun != nil && fun.Name == "Flatten" {
						isFlattenCall = true
					}
				case *ast.SelectorExpr:
					// non-anonymous import: dsl.Flatten()
					if fun != nil && fun.Sel != nil && fun.Sel.Name == "Flatten" {
						isFlattenCall = true
					}
				}
				if isFlattenCall {
					flatten = true
				}

				// Parse Payload[User] or Result[*User].
				if indexExpr, ok := call.Fun.(*ast.IndexExpr); ok && indexExpr != nil {
					var isPayload bool
					var isResult bool
					var funcName string
					switch x := indexExpr.X.(type) {
					case *ast.Ident:
						// anonymous import: Payload[User]
						if x != nil {
							funcName = x.Name
						}
					case *ast.SelectorExpr:
						// non-anonymous import: dsl.Payload[User]
						if x != nil && x.Sel != nil {
							funcName = x.Sel.Name
						}
					}
					switch funcName {
					case "Payload":
						isPayload = true
					case "Result":
						isResult = true
					}
					if isPayload {
						if ident, ok := indexExpr.Index.(*ast.Ident); ok && ident != nil { // Payload[User]
							payload = ident.Name
						} else if starExpr, ok := indexExpr.Index.(*ast.StarExpr); ok && starExpr != nil { // Payload[*User]
							if ident, ok := starExpr.X.(*ast.Ident); ok && ident != nil {
								payload = "*" + ident.Name
							}
						}
					}
					if isResult {
						if ident, ok := indexExpr.Index.(*ast.Ident); ok && ident != nil { // Result[User]
							result = ident.Name
						} else if starExpr, ok := indexExpr.Index.(*ast.StarExpr); ok && starExpr != nil { // Result[*User]
							if ident, ok := starExpr.X.(*ast.Ident); ok && ident != nil {
								result = "*" + ident.Name
							}
						}
					}
				}
			}
		}
	}

	return &Action{
		Payload:  payload,
		Result:   result,
		Enabled:  enabled,
		Service:  service,
		Public:   public,
		Filename: filename,
		Flatten:  flatten,
		Phase:    phase,
	}, true
}

// // actionResult holds the parsed configuration for a single DSL action.
// // It contains all the settings that can be configured for an API action
// // through the DSL, including type information and behavioral flags.
// type actionResult struct {
// 	// payload is the name of the request payload type (e.g., "CreateUserRequest")
// 	payload string
// 	// result is the name of the response result type (e.g., "User" or "*User")
// 	result string
// 	// enabled indicates whether this action should generate API endpoints
// 	enabled bool
// 	// service indicates whether to generate service layer code for this action
// 	service bool
// 	// public indicates whether the generated API endpoint should be publicly accessible
// 	public bool
// }

// FindAllModelBase finds all struct types that embed model.Base as an anonymous field.
// It searches for structs containing anonymous fields of type "model.Base" or aliased versions
// like "pkgmodel.Base" where pkgmodel is an import alias for the model package.
//
// Parameters:
//   - file: The AST file to search in
//
// Returns:
//   - []string: Names of all struct types that embed model.Base
//
// This function is used to identify models that should have full database functionality,
// as opposed to lightweight models that embed model.Empty.
// FindAllModelBase finds all struct types that embed model.Base as an anonymous field
func FindAllModelBase(file *ast.File) []string {
	names := make([]string, 0)
	if file == nil {
		return names
	}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl == nil || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec == nil {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType == nil || structType.Fields == nil {
				continue
			}
			for _, field := range structType.Fields.List {
				if IsModelBase(file, field) {
					names = append(names, typeSpec.Name.Name)
					break
				}
			}
		}
	}

	return names
}

// FindAllModelEmpty finds all struct types that embed model.Empty as an anonymous field.
// It searches for structs containing anonymous fields of type "model.Empty" or aliased versions
// like "pkgmodel.Empty" where pkgmodel is an import alias for the model package.
//
// Parameters:
//   - file: The AST file to search in
//
// Returns:
//   - []string: Names of all struct types that embed model.Empty
//
// This function is used to identify lightweight models that typically don't require
// database migration and have simplified API generation.
// FindAllModelEmpty finds all struct types that embed model.Empty as an anonymous field
func FindAllModelEmpty(file *ast.File) []string {
	names := make([]string, 0)
	if file == nil {
		return names
	}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl == nil || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec == nil {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType == nil || structType.Fields == nil {
				continue
			}
			for _, field := range structType.Fields.List {
				if IsModelEmpty(file, field) {
					names = append(names, typeSpec.Name.Name)
					break
				}
			}
		}
	}

	return names
}

// IsModelBase checks if a struct field is an anonymous embedding of model.Base.
// It handles various import patterns including direct imports, aliased imports,
// and dot imports of the model package.
//
// Parameters:
//   - file: The AST file containing import information
//   - field: The struct field to check
//
// Returns:
//   - bool: true if the field is an anonymous model.Base embedding
//
// Supported import patterns:
//   - import "github.com/hydroan/gst/model"
//   - import pkgmodel "github.com/hydroan/gst/model"
//   - import . "github.com/hydroan/gst/model"
//
// Example field patterns that return true:
//   - model.Base (with standard import)
//   - pkgmodel.Base (with aliased import)
//   - Base (with dot import)
//
// IsModelBase checks if a struct field is an anonymous embedding of model.Base
func IsModelBase(file *ast.File, field *ast.Field) bool {
	// Not anonymouse field.
	if file == nil || field == nil || len(field.Names) != 0 {
		return false
	}

	aliasNames := []string{"model"}
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		if imp.Path.Value == consts.IMPORT_PATH_MODEL {
			if imp.Name != nil && !slices.Contains(aliasNames, imp.Name.Name) {
				aliasNames = append(aliasNames, imp.Name.Name)
			}
		}
	}

	switch t := field.Type.(type) {
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return slices.Contains(aliasNames, ident.Name) && t.Sel.Name == "Base"
		}
	case *ast.Ident:
		return t.Name == "Base"
	}

	return false
}

// IsModelEmpty checks if a struct field is an anonymous embedding of model.Empty.
// It handles various import patterns including direct imports, aliased imports,
// and dot imports of the model package.
//
// Parameters:
//   - file: The AST file containing import information
//   - field: The struct field to check
//
// Returns:
//   - bool: true if the field is an anonymous model.Empty embedding
//
// Supported import patterns:
//   - import "github.com/hydroan/gst/model"
//   - import pkgmodel "github.com/hydroan/gst/model"
//   - import . "github.com/hydroan/gst/model"
//
// Example field patterns that return true:
//   - model.Empty (with standard import)
//   - pkgmodel.Empty (with aliased import)
//   - Empty (with dot import)
//
// IsModelEmpty checks if a struct field is an anonymous embedding of model.Empty
func IsModelEmpty(file *ast.File, field *ast.Field) bool {
	// Not anonymouse field.
	if file == nil || field == nil || len(field.Names) != 0 {
		return false
	}

	aliasNames := []string{"model"}
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		if imp.Path.Value == consts.IMPORT_PATH_MODEL {
			if imp.Name != nil && !slices.Contains(aliasNames, imp.Name.Name) {
				aliasNames = append(aliasNames, imp.Name.Name)
			}
		}
	}

	switch t := field.Type.(type) {
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return slices.Contains(aliasNames, ident.Name) && t.Sel.Name == "Empty"
		}
	case *ast.Ident:
		return t.Name == "Empty"
	}

	return false
}

// starName converts a type name to its pointer equivalent.
// If the name already starts with '*', it removes any existing '*' prefix first
// to avoid double pointers, then adds a single '*' prefix.
//
// Parameters:
//   - name: The type name to convert (e.g., "User", "*User")
//
// Returns:
//   - string: The pointer type name (e.g., "*User")
//
// Examples:
//   - starName("User") returns "*User"
//   - starName("*User") returns "*User"
//   - starName("") returns ""
func starName(name string) string {
	if len(name) == 0 {
		return ""
	}

	return "*" + strings.TrimPrefix(name, `*`)
}
