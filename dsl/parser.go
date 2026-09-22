package dsl

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/goast"
	"github.com/stoewer/go-strcase"
)

var pluralizeCli = pluralize.NewClient()

// Parse analyzes a Go source file and extracts DSL design information from models.
// It looks for structs that have a Design() method and parses the DSL calls within that method.
//
// The parser identifies models by finding structs that embed either model.Base or model.Empty,
// then locates their Design() method and analyzes the DSL function calls to build the Design configuration.
//
// Parameters:
//   - file: The parsed AST file node to analyze
//
// Returns:
//   - map[string]*Design: A map where keys are model names and values are their parsed Design configurations
//
// Example usage:
//
//	designs := Parse(fileNode)
//	for modelName, design := range designs {
//		fmt.Printf("Model %s has endpoint: %s\n", modelName, design.Endpoint)
//	}
//
// The parser supports various DSL patterns:
//   - Global settings: Enabled(), Endpoint("path"), Migrate()
//   - Action configuration: Create(func() { Payload[Type](); Result[Type]() })
//   - Service and visibility: Service(), Public()
func Parse(file *ast.File) map[string]*Design {
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
		// Default endpoint is the pluralized snake_case form of the model name,
		// matching the RESTful convention and the default table naming,
		// e.g. "SampleRecord" becomes "sample_records".
		if len(design.Endpoint) == 0 {
			design.Endpoint = strcase.SnakeCase(pluralizeCli.Plural(name))
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
		if design.SSE == nil {
			design.SSE = &Action{Payload: starName(name), Result: starName(name)}
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
		initDefaultAction(name, design.SSE)
		for _, actions := range design.routes {
			for _, action := range actions {
				initDefaultAction(name, action)
			}
		}

		m[name] = design
	}

	return m
}

// initDefaultAction initializes default payload and result values for an enabled action.
//
// With neither side declared both default to the pointer type of the model
// name (e.g., "*User" for model "User"), which keeps the built-in CRUD
// controller path active. Once the developer declares one side explicitly the
// action is a custom contract, and the framework never guesses the model as
// the other half: the missing side defaults to PayloadEmpty (*model.Empty).
//
// List and Get actions handle HTTP GET requests without a request body. When
// such an action declares Result it is delegated to a custom service method,
// so its Payload is fixed to PayloadEmpty (*model.Empty); custom services
// read query parameters from ServiceContext.Query().
//
// Parameters:
//   - modelName: The name of the model (e.g., "User")
//   - action: The action to initialize defaults for
//
// This function only modifies enabled actions. For disabled actions, the Payload
// and Result fields remain unchanged.
func initDefaultAction(modelName string, action *Action) {
	if !action.Enabled {
		return
	}
	if isGetVerbPhase(action.Phase) && len(action.Result) > 0 {
		action.Payload = PayloadEmpty
	}
	switch {
	case len(action.Payload) == 0 && len(action.Result) == 0:
		action.Payload = starName(modelName)
		action.Result = starName(modelName)
	case len(action.Payload) == 0:
		action.Payload = PayloadEmpty
	case len(action.Result) == 0:
		action.Result = PayloadEmpty
	}
}

// isGetVerbPhase reports whether the phase's generated route handles an HTTP
// GET request, which carries no request body.
func isGetVerbPhase(phase consts.Phase) bool {
	return phase == consts.PHASE_LIST || phase == consts.PHASE_GET
}

// isFixedContractPhase reports whether the phase delegates to a fixed service
// method signature that never binds Payload or Result types: the Import
// controller reads the uploaded multipart form file through
// Import(ctx, io.Reader) and the Export controller writes the bytes returned
// by Export(ctx, ...M) as a file attachment.
func isFixedContractPhase(phase consts.Phase) bool {
	return phase == consts.PHASE_IMPORT || phase == consts.PHASE_EXPORT
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
// and action configurations like Create(func() { Payload[Type](); Result[Type]() }).
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
		if !ok || call == nil || call.Fun == nil {
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

		// Parse "Migrate()". Declaring the marker enables database migration.
		if funcName == "Migrate" && len(call.Args) == 0 {
			defaults.Migrate = true
		}

		// The remaining DSL calls all carry at least one argument.
		if len(call.Args) == 0 {
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
			if value, ok := stringLiteral(call.Args[0]); ok {
				defaults.Endpoint = strings.TrimLeft(value, "/")
				defaults.Endpoint = strings.ReplaceAll(defaults.Endpoint, "/", "-")
			}
		}

		// Parse "Param()".
		if funcName == "Param" && len(call.Args) == 1 {
			if value, ok := stringLiteral(call.Args[0]); ok {
				defaults.Param = strings.TrimFunc(value, func(r rune) bool {
					return r == ' ' || r == '{' || r == '}' || r == '[' || r == ']' || r == ':'
				})
				defaults.Param = ":" + defaults.Param
			}
		}

		// Parse "Route()".
		// Example:
		//
		// Route("/archive/items", func() {
		// 	List(func() {
		// 		Service()
		// 	})
		// 	Get(func() {
		// 		Service()
		// 	})
		// })
		if funcName == "Route" && len(call.Args) == 2 {
			var route string
			if value, ok := stringLiteral(call.Args[0]); ok {
				route = strings.TrimLeft(value, "/")
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
						if act, e := parseAction(consts.PHASE_SSE, funName_, call_.Args[0]); e {
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
		if act, e := parseAction(consts.PHASE_SSE, funcName, call.Args[0]); e {
			defaults.SSE = act
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
//   - *Action: Parsed configuration including payload/result types and flags
//   - bool: true if parsing was successful, false otherwise
//
// The function parses DSL calls within the action function body:
//   - Enabled(true/false): Sets whether the action is enabled. Declared actions default to enabled.
//   - Service(): Marks the action for custom service generation and registration
//   - Public(): Marks the API endpoint as public
//   - Exact(): Registers the action route exactly as declared
//   - Filename("name"): Sets a custom filename for the generated service file
//   - Payload[Type]: Sets the request payload type
//   - Result[Type]: Sets the response result type
//
// Example usage in DSL:
//
//	Create(func() {
//	    Service()
//	    Payload[CreateUserRequest]()
//	    Result[*User]()
//	})
func parseAction(phase consts.Phase, funcName string, expr ast.Expr) (*Action, bool) {
	var payload string
	var result string
	enabled := true     // declared actions are enabled by default
	var service bool    // default to false
	var public bool     // default to false
	var exact bool      // default to false
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

				// Parse Service().
				var isServiceCall bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					// anonymous import: Service()
					if fun != nil && fun.Name == "Service" {
						isServiceCall = true
					}
				case *ast.SelectorExpr:
					// non-anonymous import: dsl.Service()
					if fun != nil && fun.Sel != nil && fun.Sel.Name == "Service" {
						isServiceCall = true
					}
				}
				if isServiceCall && len(call.Args) == 0 {
					service = true
				}

				// Parse Public().
				var isPublicCall bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					// anonymous import: Public()
					if fun != nil && fun.Name == "Public" {
						isPublicCall = true
					}
				case *ast.SelectorExpr:
					// non-anonymous import: dsl.Public()
					if fun != nil && fun.Sel != nil && fun.Sel.Name == "Public" {
						isPublicCall = true
					}
				}

				if isPublicCall && len(call.Args) == 0 {
					public = true
				}

				// Parse Exact().
				var isExactCall bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					// anonymous import: Exact()
					if fun != nil && fun.Name == "Exact" {
						isExactCall = true
					}
				case *ast.SelectorExpr:
					// non-anonymous import: dsl.Exact()
					if fun != nil && fun.Sel != nil && fun.Sel.Name == "Exact" {
						isExactCall = true
					}
				}

				if isExactCall && len(call.Args) == 0 {
					exact = true
				}

				// Parse Filename("archive").
				var isFilenameCall bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					// anonymous import: Filename("archive")
					if fun != nil && fun.Name == "Filename" {
						isFilenameCall = true
					}
				case *ast.SelectorExpr:
					// non-anonymous import: dsl.Filename("archive")
					if fun != nil && fun.Sel != nil && fun.Sel.Name == "Filename" {
						isFilenameCall = true
					}
				}
				if isFilenameCall && len(call.Args) > 0 && call.Args[0] != nil {
					if value, ok := stringLiteral(call.Args[0]); ok {
						filename = value
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
					// List and Get handle HTTP GET requests without a request
					// body, and Import and Export delegate to fixed service
					// method signatures that never bind Payload or Result
					// types. Declarations invalid for the phase are rejected
					// by Validate and discarded here so downstream code never
					// sees them.
					if isPayload && !isGetVerbPhase(phase) && !isFixedContractPhase(phase) {
						if ident, ok := indexExpr.Index.(*ast.Ident); ok && ident != nil { // Payload[User]
							payload = ident.Name
						} else if starExpr, ok := indexExpr.Index.(*ast.StarExpr); ok && starExpr != nil { // Payload[*User]
							if ident, ok := starExpr.X.(*ast.Ident); ok && ident != nil {
								payload = "*" + ident.Name
							}
						}
					}
					if isResult && !isFixedContractPhase(phase) {
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
		Exact:    exact,
		Filename: filename,
		Flatten:  flatten,
		Phase:    phase,
	}, true
}

// FindAllModelBase finds all struct types that embed a database base model
// (model.Base or model.AutoBase) as an anonymous field, the embedding
// recognized as IsModelBase recognizes it.
//
// Parameters:
//   - file: The AST file to search in
//
// Returns:
//   - []string: Names of all struct types that embed a database base model
//
// This function is used to identify models that should have full database functionality,
// as opposed to lightweight models that embed model.Empty.
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

// FindAllModelEmpty finds all struct types that embed model.Empty as an
// anonymous field, the embedding recognized as IsModelEmpty recognizes it.
//
// Parameters:
//   - file: The AST file to search in
//
// Returns:
//   - []string: Names of all struct types that embed model.Empty
//
// This function is used to identify lightweight models that typically don't require
// database migration and have simplified API generation.
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

// modelBaseNames are the database base model type names recognized by
// IsModelBase. Base uses a UUIDv7 string primary key and AutoBase uses an
// auto-increment integer primary key; both mark a struct as a database model.
var modelBaseNames = []string{ggconst.FieldBase, ggconst.FieldAutoBase}

// IsModelBase reports whether a struct field embeds a database base model,
// model.Base or model.AutoBase, by value. The qualifier must be a name file
// imports the framework model package under, and a bare name counts only
// when the file dot-imports the package (see goast.ImportedNames). With
//
//	import (
//		"example.com/app/model"
//		gstmodel "github.com/hydroan/gst/model"
//	)
//
// the fields gstmodel.Base and gstmodel.AutoBase report true, while
// model.Base, another package's type, and a bare Base, a type of the file's
// own package, report false; under a dot import of the framework model
// package the bare Base and AutoBase report true.
func IsModelBase(file *ast.File, field *ast.Field) bool {
	return embedsModelType(file, field, modelBaseNames...)
}

// IsModelEmpty reports whether a struct field embeds model.Empty by value,
// under the same import rules as IsModelBase: with
//
//	import gstmodel "github.com/hydroan/gst/model"
//
// the field gstmodel.Empty reports true, and a bare Empty reports true only
// under a dot import of the framework model package.
func IsModelEmpty(file *ast.File, field *ast.Field) bool {
	return embedsModelType(file, field, ggconst.FieldEmpty)
}

// embedsModelType reports whether field is an anonymous value embedding of
// one of the framework model package's types typeNames, referred to the way
// file imports the package.
func embedsModelType(file *ast.File, field *ast.Field, typeNames ...string) bool {
	if file == nil || field == nil || len(field.Names) != 0 {
		return false
	}
	return goast.ImportedNames(file, ggconst.ImportPathModel, ggconst.PkgModel).Refers(field.Type, typeNames...)
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
