// Package gggen builds the code gg gen writes: the registration files, the
// service files and their test scaffolds, and, in its sub-packages, the
// column references, the TypeScript declarations and the protobuf
// definitions.
package gggen

import (
	"fmt"
	"go/ast"
	"strconv"
	"strings"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/modelsource"
	"github.com/stoewer/go-strcase"
)

// serviceActionLogQuoted returns a Go string literal (as used in
// ast.BasicLit.Value) for log.Info in generated service methods:
// "{model} {phase}", as in "user create" and "user create before". When the
// action names its service, uses "{model}: {name}" with the underscores of the
// name as spaces and an optional hook suffix (before/after/filter/...), as in
// "user: archive before" for Service("archive") and
// "record: archive sample items" for Service("archive_sample_items").
func serviceActionLogQuoted(modelName string, phase apiaction.Phase, action *dsl.Action) string {
	modelLower := strings.ToLower(modelName)
	phaseSnake := strings.ReplaceAll(strcase.SnakeCase(phase.Name()), "_", " ")
	if action != nil && action.ServiceName != "" {
		label := strings.ReplaceAll(action.ServiceName, "_", " ")
		ps := string(phase)
		var msg string
		switch {
		case strings.HasSuffix(ps, "_before"):
			msg = fmt.Sprintf("%s: %s before", modelLower, label)
		case strings.HasSuffix(ps, "_after"):
			msg = fmt.Sprintf("%s: %s after", modelLower, label)
		default:
			msg = fmt.Sprintf("%s: %s", modelLower, label)
		}
		return strconv.Quote(msg)
	}
	msg := fmt.Sprintf("%s %s", modelLower, phaseSnake)
	return strconv.Quote(msg)
}

// serviceFilterLogQuoted returns the Go string literal log.Info logs in a
// generated Filter hook: the message of serviceActionLogQuoted for the
// action's phase followed by " filter", as in "user list filter" for the
// List action of User, "user export filter" for its Export action and
// "user: search filter" for a List with Service("search").
func serviceFilterLogQuoted(modelName string, phase apiaction.Phase, action *dsl.Action) string {
	msg, _ := strconv.Unquote(serviceActionLogQuoted(modelName, phase, action))
	return strconv.Quote(msg + " filter")
}

// genServiceMethod1 uses AST to generate CreateBefore, CreateAfter,
// UpdateBefore, UpdateAfter, DeleteBefore, DeleteAfter, GetBefore, GetAfter,
// PatchBefore, PatchAfter methods. modelQualifier is the name the file refers
// to the model package by (see serviceModelQualifier). For the model User and
// phase apiaction.CreateBefore it generates
//
//	func (u *Creator) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create before")
//
//		return nil
//	}
func genServiceMethod1(info *modelsource.Model, modelQualifier string, action *dsl.Action, phase apiaction.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod1(
		info.ModelVarName, info.ModelName, modelQualifier, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod2 uses AST to generate ListBefore, ListAfter methods,
// referring to the model package by modelQualifier (see genServiceMethod1). For
// the model User and phase apiaction.ListBefore it generates
//
//	func (u *Lister) ListBefore(ctx *gst.ServiceContext, users *[]*model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user list before")
//
//		return nil
//	}
func genServiceMethod2(info *modelsource.Model, modelQualifier string, action *dsl.Action, phase apiaction.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod2(
		info.ModelVarName, info.ModelName, modelQualifier, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod3 uses AST to generate CreateManyBefore, CreateManyAfter,
// DeleteManyBefore, DeleteManyAfter, UpdateManyBefore, UpdateManyAfter,
// PatchManyBefore, PatchManyAfter, referring to the model package by
// modelQualifier (see genServiceMethod1). For the model User and phase
// apiaction.CreateManyBefore it generates
//
//	func (u *ManyCreator) CreateManyBefore(ctx *gst.ServiceContext, users ...*model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create many before")
//
//		return nil
//	}
func genServiceMethod3(info *modelsource.Model, modelQualifier string, action *dsl.Action, phase apiaction.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod3(
		info.ModelVarName, info.ModelName, modelQualifier, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod4 uses AST to generate Create, Delete, Update, Patch, List,
// Get, CreateMany, DeleteMany, UpdateMany, PatchMany methods, referring to the
// model package by modelQualifier (see genServiceMethod1). For the model User,
// the request and result types *User and phase apiaction.Create it generates
//
//	func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create")
//
//		return rsp, nil
//	}
func genServiceMethod4(info *modelsource.Model, modelQualifier string, action *dsl.Action, reqName, rspName string, phase apiaction.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod4(
		info.ModelVarName, modelQualifier, emptyReqPkgName(modelQualifier), reqName, rspName, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(
			ast.NewIdent("rsp"),
			ast.NewIdent("nil"),
		),
	)
}

// genServiceMethod5 uses AST to generate Import method, referring to the model
// package by modelQualifier (see genServiceMethod1). The scaffold returns a
// literal nil error: returning the never-assigned named err would fail the
// error discipline check on the very next gg run. For the model User
// it generates
//
//	func (u *Importer) Import(ctx *gst.ServiceContext, reader io.Reader) (users []*model.User, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user import")
//
//		return users, nil
//	}
func genServiceMethod5(info *modelsource.Model, modelQualifier string, action *dsl.Action, phase apiaction.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod5(
		info.ModelVarName, info.ModelName, modelQualifier, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent(pluralizeCli.Plural(strings.ToLower(info.ModelName))), ast.NewIdent("nil")),
	)
}

// genServiceMethod6 uses AST to generate Export method, referring to the model
// package by modelQualifier (see genServiceMethod1). Like the Import scaffold,
// it returns a literal nil error to keep generated code compliant with the
// error discipline check. For the model User it generates
//
//	func (u *Exporter) Export(ctx *gst.ServiceContext, users ...*model.User) (data []byte, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user export")
//
//		return data, nil
//	}
func genServiceMethod6(info *modelsource.Model, modelQualifier string, action *dsl.Action, phase apiaction.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod6(
		info.ModelVarName, info.ModelName, modelQualifier, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("data"), ast.NewIdent("nil")),
	)
}

// genServiceMethod7 uses AST to generate the SSE method scaffold. Like the
// Import scaffold, it returns a literal nil error to keep generated code
// compliant with the error discipline check; the business fills in
// the streaming callback through sse.Serve. For the model User it generates
//
//	func (u *Streamer) SSE(ctx *gst.ServiceContext) (err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user sse")
//
//		return nil
//	}
func genServiceMethod7(info *modelsource.Model, action *dsl.Action, phase apiaction.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod7(
		info.ModelVarName, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod8 uses AST to generate the Filter hook of the List and
// Export actions, referring to the model package by modelQualifier (see
// genServiceMethod1). The scaffold passes the model and the options through
// unchanged. For the model User and phase apiaction.List it generates
//
//	func (u *Lister) Filter(ctx *gst.ServiceContext, user *model.User, opts gst.QueryOptions) (*model.User, gst.QueryOptions, error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user list filter")
//
//		return user, opts, nil
//	}
func genServiceMethod8(info *modelsource.Model, modelQualifier string, action *dsl.Action, phase apiaction.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod8(
		info.ModelVarName, info.ModelName, modelQualifier, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceFilterLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent(strings.ToLower(info.ModelName)), ast.NewIdent("opts"), ast.NewIdent("nil")),
	)
}

// genServiceMethod9 uses AST to generate the Stream method scaffold of a
// Stream action, referring to the model package by modelQualifier (see
// genServiceMethod1): the method takes what the action declares (see
// serviceMethod9) and, like the SSE scaffold, returns literal nil so the
// generated code passes the error discipline check; the business
// fills in the stream. For the model Feed and a Service("watch") Stream
// declaring Payload[*FeedWatchReq] and StreamingResult[*FeedEvent] it
// generates
//
//	func (w *Watch) Stream(ctx *gst.ServiceContext, req *model.FeedWatchReq, stream *gst.ServerStream[*model.FeedEvent]) (err error) {
//		log := w.WithContext(ctx, ctx.Phase())
//		log.Info("feed: watch")
//
//		return nil
//	}
//
// and a Service("upload") Stream declaring StreamingPayload[*FeedEvent] and
// Result[*FeedUploadRsp], whose method answers a response,
//
//	func (u *Upload) Stream(ctx *gst.ServiceContext, stream *gst.ClientStream[*model.FeedEvent]) (rsp *model.FeedUploadRsp, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("feed: upload")
//
//		return rsp, nil
//	}
func genServiceMethod9(info *modelsource.Model, modelQualifier string, action *dsl.Action, phase apiaction.Phase, roleName string) *ast.FuncDecl {
	results := []ast.Expr{ast.NewIdent("nil")}
	if action.StreamingPayload && !action.StreamingResult {
		results = []ast.Expr{ast.NewIdent("rsp"), ast.NewIdent("nil")}
	}
	return serviceMethod9(
		info.ModelVarName, modelQualifier, emptyReqPkgName(modelQualifier), action.Payload, action.Result, action.StreamingPayload, action.StreamingResult, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(results...),
	)
}

// GenerateService builds the scaffold of the action's service file in package
// servicePkgName: the service struct named after the action's role and the
// methods of phase the controller calls. It returns nil when the action is
// disabled or declares no service.
//
// The controller runs a Create, Delete, Update, Patch, List or Get action, and
// their batch forms, that exchanges the database model itself through its
// default flow, the hooks around the database operation, and never calls the
// action method. For such a Create action on the model User of the root model
// package the file prints as
//
//	package user
//
//	import (
//		"helloworld/model"
//
//		"github.com/hydroan/gst"
//		"github.com/hydroan/gst/service"
//	)
//
//	type Creator struct {
//		service.Base[*model.User, *model.User, *model.User]
//	}
//
//	func (u *Creator) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create before")
//
//		return nil
//	}
//
//	func (u *Creator) CreateAfter(ctx *gst.ServiceContext, user *model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create after")
//
//		return nil
//	}
//
// A List action gets the Filter hook as well, between ListBefore and
// ListAfter, the order the controller invokes them in.
//
// An action declaring a Payload or a Result of its own, and every action of a
// model without a database table, one embedding model.Empty, is handed to the
// action method instead, and none of the hooks is called. With
// Payload[*UserCreateReq]() and Result[*UserCreateRsp]() the same action
// prints as
//
//	package user
//
//	import (
//		"helloworld/model"
//
//		"github.com/hydroan/gst"
//		"github.com/hydroan/gst/service"
//	)
//
//	type Creator struct {
//		service.Base[*model.User, *model.UserCreateReq, *model.UserCreateRsp]
//	}
//
//	func (u *Creator) Create(ctx *gst.ServiceContext, req *model.UserCreateReq) (rsp *model.UserCreateRsp, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create")
//
//		return rsp, nil
//	}
//
// An Export action holds both: the list hooks the export controller runs
// before it, on a database model, and the Export method. GenerateServiceShell
// returns the struct and the action method whatever the action exchanges.
func GenerateService(info *modelsource.Model, action *dsl.Action, phase apiaction.Phase, servicePkgName string) *ast.File {
	return generateService(info, action, phase, servicePkgName, true)
}

// GenerateServiceShell returns the shell gg module copy merges the module's
// service source onto: the struct and the action method, whatever the action
// exchanges, and none of the hook placeholders GenerateService scaffolds. The
// shell owns the signatures the source's bodies are grafted onto and the
// source owns the hooks it defines; a placeholder the source does not define
// would stay in the copy as a hook that runs. For the Create action of the
// model User above the file prints as
//
//	package user
//
//	import (
//		"helloworld/model"
//
//		"github.com/hydroan/gst"
//		"github.com/hydroan/gst/service"
//	)
//
//	type Creator struct {
//		service.Base[*model.User, *model.User, *model.User]
//	}
//
//	func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create")
//
//		return rsp, nil
//	}
func GenerateServiceShell(info *modelsource.Model, action *dsl.Action, phase apiaction.Phase, servicePkgName string) *ast.File {
	return generateService(info, action, phase, servicePkgName, false)
}

// generateService builds the service file of the action: the struct and,
// when hooks is set, the methods the controller calls for it, the hooks of
// an action exchanging a database model itself and the action method of any
// other; without hooks, the struct and the action method.
func generateService(info *modelsource.Model, action *dsl.Action, phase apiaction.Phase, servicePkgName string, hooks bool) *ast.File {
	if !action.Service {
		return nil
	}
	hooks = hooks && !info.Design.IsEmpty

	roleName := action.RoleName()

	// When the service is named, derive the receiver variable name from
	// RoleName (e.g., Archive → "a") instead of the model name
	// (e.g., Record → "r").
	if action.ServiceName != "" && len(roleName) > 0 {
		copied := *info
		copied.ModelVarName = strings.ToLower(roleName[:1])
		info = &copied
	}

	// The file refers to the model package by its package name, or by an
	// alias when a framework package the file imports takes that name, as in
	// service.Base[*model_service.Item, ...] (see serviceModelQualifier).
	qualifier := serviceModelQualifier(info, phase)

	otherPkgs := []string{}
	// A dsl.PayloadEmpty request or result type references model.Empty from
	// the gst model package, so the generated file needs its import, the
	// imports() entry "gstmodel github.com/hydroan/gst/model" when the file
	// refers to the business model package as "model" and the bare path
	// otherwise (see emptyReqPkgName).
	if isEmptyPayload(action.Payload) || isEmptyPayload(action.Result) {
		otherPkgs = append(otherPkgs, GstModelImportEntry(emptyReqPkgName(qualifier)))
	}

	decls := []ast.Decl{
		importDecl(info.ModulePath, info.ModelFileDir, qualifier, phase, otherPkgs...),
		types(qualifier, emptyReqPkgName(qualifier), info.ModelName, action.Payload, action.Result, roleName),
	}

	// The controller runs a CRUD action that exchanges the model itself
	// through its default flow, the hooks around the database operation, and
	// hands any other to the action method (see model.AreTypesEqual), so
	// only what it calls is scaffolded: a method it never calls would hold
	// logic that never runs.
	defaultFlow := hooks && action.Payload == "*"+info.ModelName && action.Result == "*"+info.ModelName
	calledMethods := func(hook func(*modelsource.Model, string, *dsl.Action, apiaction.Phase, string) *ast.FuncDecl) []ast.Decl {
		if !defaultFlow {
			return []ast.Decl{genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName)}
		}
		return []ast.Decl{hook(info, qualifier, action, phase.Before(), roleName), hook(info, qualifier, action, phase.After(), roleName)}
	}
	switch phase {
	case apiaction.Create, apiaction.Delete, apiaction.Update, apiaction.Patch, apiaction.Get:
		decls = append(decls, calledMethods(genServiceMethod1)...)
	case apiaction.List: // List hooks use genServiceMethod2, Filter genServiceMethod8
		called := calledMethods(genServiceMethod2)
		if defaultFlow {
			// Filter sits between the hooks, the order the list controller
			// invokes them in.
			called = []ast.Decl{called[0], genServiceMethod8(info, qualifier, action, phase, roleName), called[1]}
		}
		decls = append(decls, called...)
	case apiaction.CreateMany, apiaction.DeleteMany, apiaction.UpdateMany, apiaction.PatchMany: // XXXMany hooks use genServiceMethod3
		decls = append(decls, calledMethods(genServiceMethod3)...)
	case apiaction.Import:
		decls = append(decls, genServiceMethod5(info, qualifier, action, phase, roleName))
	case apiaction.SSE:
		decls = append(decls, genServiceMethod7(info, action, phase, roleName))
	case apiaction.Stream:
		decls = append(decls, genServiceMethod9(info, qualifier, action, phase, roleName))
	case apiaction.Export:
		// The export controller reuses the list pipeline before delegating to
		// Export: it invokes ListBefore, applies the service Filter hook when
		// building the query, then invokes ListAfter, so the hooks are
		// scaffolded in that order.
		if hooks {
			decls = append(decls, genServiceMethod2(info, qualifier, action, apiaction.ListBefore, roleName)) // generate list before hook
			decls = append(decls, genServiceMethod8(info, qualifier, action, phase, roleName))                // generate filter hook
			decls = append(decls, genServiceMethod2(info, qualifier, action, apiaction.ListAfter, roleName))  // generate list after hook
		}
		decls = append(decls, genServiceMethod6(info, qualifier, action, phase, roleName))
	}

	return &ast.File{
		Name:  ast.NewIdent(servicePkgName),
		Decls: decls,
	}
}
