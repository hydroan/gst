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

	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/stoewer/go-strcase"
)

// serviceActionLogQuoted returns a Go string literal (as used in ast.BasicLit.Value) for
// log.Info in generated service methods: "{model} {phase}", as in
// "user create" and "user create before". When the action names its service, uses
// "{model}: {name}" with the underscores of the name as spaces and an optional
// hook suffix (before/after/filter/...), as in "user: archive before" for
// Service("archive") and "record: archive sample items" for
// Service("archive_sample_items").
func serviceActionLogQuoted(modelName string, phase consts.Phase, action *dsl.Action) string {
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
func serviceFilterLogQuoted(modelName string, phase consts.Phase, action *dsl.Action) string {
	msg, _ := strconv.Unquote(serviceActionLogQuoted(modelName, phase, action))
	return strconv.Quote(msg + " filter")
}

// genServiceMethod1 uses AST to generate CreateBefore,CreateAfter,UpdateBefore,UpdateAfter,
// DeleteBefore,DeleteAfter,GetBefore,GetAfter,PatchBefore,PatchAfter methods. modelQualifier is
// the name the file refers to the model package by (see serviceModelQualifier). For the model
// User and phase consts.CreateBefore it generates
//
//	func (u *Creator) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create before")
//
//		return nil
//	}
func genServiceMethod1(info *modelinfo.Model, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod1(
		info.ModelVarName, info.ModelName, modelQualifier, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod2 uses AST to generate ListBefore, ListAfter methods, referring to the
// model package by modelQualifier (see genServiceMethod1). For the model User and phase
// consts.ListBefore it generates
//
//	func (u *Lister) ListBefore(ctx *gst.ServiceContext, users *[]*model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user list before")
//
//		return nil
//	}
func genServiceMethod2(info *modelinfo.Model, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod2(
		info.ModelVarName, info.ModelName, modelQualifier, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod3 uses AST to generate CreateManyBefore, CreateManyAfter,
// DeleteManyBefore, DeleteManyAfter, UpdateManyBefore, UpdateManyAfter, PatchManyBefore, PatchManyAfter,
// referring to the model package by modelQualifier (see genServiceMethod1). For the model User
// and phase consts.CreateManyBefore it generates
//
//	func (u *ManyCreator) CreateManyBefore(ctx *gst.ServiceContext, users ...*model.User) error {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create many before")
//
//		return nil
//	}
func genServiceMethod3(info *modelinfo.Model, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod3(
		info.ModelVarName, info.ModelName, modelQualifier, phase, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(ast.NewIdent("nil")),
	)
}

// genServiceMethod4 uses AST to generate Create,Delete,Update,Patch,List,Get,CreateMany,DeleteMany,UpdateMany,PatchMany methods,
// referring to the model package by modelQualifier (see genServiceMethod1). For the model User,
// the request and result types *User and phase consts.Create it generates
//
//	func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user create")
//
//		return rsp, nil
//	}
func genServiceMethod4(info *modelinfo.Model, modelQualifier string, action *dsl.Action, reqName, rspName string, phase consts.Phase, roleName string) *ast.FuncDecl {
	return serviceMethod4(
		info.ModelVarName, modelQualifier, reqName, rspName, phase, roleName,
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
func genServiceMethod5(info *modelinfo.Model, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
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
func genServiceMethod6(info *modelinfo.Model, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
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
// the streaming callback through ctx.SSE. For the model User it generates
//
//	func (u *Streamer) SSE(ctx *gst.ServiceContext) (err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user sse")
//
//		return nil
//	}
func genServiceMethod7(info *modelinfo.Model, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
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
// unchanged. For the model User and phase consts.List it generates
//
//	func (u *Lister) Filter(ctx *gst.ServiceContext, user *model.User, opts gst.QueryOptions) (*model.User, gst.QueryOptions, error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("user list filter")
//
//		return user, opts, nil
//	}
func genServiceMethod8(info *modelinfo.Model, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
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
//	func (w *Watch) Stream(ctx *gst.ServiceContext, req *model.FeedWatchReq, stream *grpc.ServerStream[*model.FeedEvent]) (err error) {
//		log := w.WithContext(ctx, ctx.Phase())
//		log.Info("feed: watch")
//
//		return nil
//	}
//
// and a Service("upload") Stream declaring StreamingPayload[*FeedEvent] and
// Result[*FeedUploadRsp], whose method answers a response,
//
//	func (u *Upload) Stream(ctx *gst.ServiceContext, stream *grpc.ClientStream[*model.FeedEvent]) (rsp *model.FeedUploadRsp, err error) {
//		log := u.WithContext(ctx, ctx.Phase())
//		log.Info("feed: upload")
//
//		return rsp, nil
//	}
func genServiceMethod9(info *modelinfo.Model, modelQualifier string, action *dsl.Action, phase consts.Phase, roleName string) *ast.FuncDecl {
	results := []ast.Expr{ast.NewIdent("nil")}
	if action.StreamingPayload && !action.StreamingResult {
		results = []ast.Expr{ast.NewIdent("rsp"), ast.NewIdent("nil")}
	}
	return serviceMethod9(
		info.ModelVarName, modelQualifier, action.Payload, action.Result, action.StreamingPayload, action.StreamingResult, roleName,
		StmtLogWithContext(info.ModelVarName),
		StmtLogInfo(serviceActionLogQuoted(info.ModelName, phase, action)),
		EmptyLine(),
		Returns(results...),
	)
}

// GenerateService builds the scaffold of the action's service file in package
// servicePkgName: the service struct named after the action's role and the
// methods of phase. It returns nil when the action is disabled or declares no
// service. For a Create action on the model User of the root model package,
// a database model, the file prints as
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
// A List or Export action on a database model gets the Filter hook as well,
// between ListBefore and ListAfter, the order the controllers invoke them
// in. A model without a database table, one embedding model.Empty, gets no
// hooks at all.
func GenerateService(info *modelinfo.Model, action *dsl.Action, phase consts.Phase, servicePkgName string) *ast.File {
	if !action.Service {
		return nil
	}

	roleName := action.RoleName()

	// When the service is named, derive the receiver variable name from RoleName
	// (e.g., Archive → "a") instead of the model name (e.g., Record → "r").
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
		types(qualifier, info.ModelName, action.Payload, action.Result, roleName),
	}

	// add methods
	switch phase {
	case consts.Create:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Hook generation logic based on model.Empty field presence:
		//
		// Models WITHOUT hooks (contains model.Empty):
		// 	type Group struct {
		// 		model.Empty
		// 	}
		//
		// Models WITH hooks (does not contain model.Empty):
		// 	type Group struct {
		// 		Name string
		// 		model.Base
		// 	}
		//
		// Only generate before/after hooks for non-empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate create before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate create after hook
		}
	case consts.Delete:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate delete before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate delete after hook
		}
	case consts.Update:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate update before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate update after hook
		}
	case consts.Patch:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate patch before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate patch after hook
		}
	case consts.List: // List hooks use genServiceMethod2, Filter genServiceMethod8
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod2(info, qualifier, action, phase.Before(), roleName)) // generate list before hook
			decls = append(decls, genServiceMethod8(info, qualifier, action, phase, roleName))          // generate filter hook
			decls = append(decls, genServiceMethod2(info, qualifier, action, phase.After(), roleName))  // generate list after hook
		}
	case consts.Get:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.Before(), roleName)) // generate get before hook
			decls = append(decls, genServiceMethod1(info, qualifier, action, phase.After(), roleName))  // generate get after hook
		}
	case consts.CreateMany: // XXXMany hooks use genServiceMethod3
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.Before(), roleName)) // generate create many before hook
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.After(), roleName))  // generate create many after hook
		}
	case consts.DeleteMany:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.Before(), roleName)) // generate delete many before hook
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.After(), roleName))  // generate delete many after hook
		}
	case consts.UpdateMany:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.Before(), roleName)) // generate update many before hook
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.After(), roleName))  // generate update many after hook
		}
	case consts.PatchMany:
		decls = append(decls, genServiceMethod4(info, qualifier, action, action.Payload, action.Result, phase, roleName))
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.Before(), roleName)) // generate patch many before hook
			decls = append(decls, genServiceMethod3(info, qualifier, action, phase.After(), roleName))  // generate patch many after hook
		}
	case consts.Import:
		decls = append(decls, genServiceMethod5(info, qualifier, action, phase, roleName))
	case consts.SSE:
		decls = append(decls, genServiceMethod7(info, action, phase, roleName))
	case consts.Stream:
		decls = append(decls, genServiceMethod9(info, qualifier, action, phase, roleName))
	case consts.Export:
		// The export controller reuses the list pipeline before delegating to
		// Export: it invokes ListBefore, applies the service Filter hook when
		// building the query, then invokes ListAfter, so the hooks are
		// scaffolded in that order.
		// Skip generating hooks for empty models
		if !info.Design.IsEmpty {
			decls = append(decls, genServiceMethod2(info, qualifier, action, consts.ListBefore, roleName)) // generate list before hook
			decls = append(decls, genServiceMethod8(info, qualifier, action, phase, roleName))             // generate filter hook
			decls = append(decls, genServiceMethod2(info, qualifier, action, consts.ListAfter, roleName))  // generate list after hook
		}
		decls = append(decls, genServiceMethod6(info, qualifier, action, phase, roleName))
	}

	return &ast.File{
		Name:  ast.NewIdent(servicePkgName),
		Decls: decls,
	}
}
