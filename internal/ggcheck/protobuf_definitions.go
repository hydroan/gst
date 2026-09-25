package ggcheck

import (
	"fmt"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gggen/pb"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
)

// ProtobufDefinitions holds the models declaring GRPC() to what gg gen
// requires of their protobuf definitions.
var ProtobufDefinitions = Check{
	Name: "Protobuf definitions",
	Rule: "models declaring GRPC(), and the types their actions reach, must be describable in protobuf with a valid pb tag on every field, and their definitions must keep the contract committed under pb/: the checks gg gen runs before writing",
	run:  checkProtobufDefinitions,
}

// checkProtobufDefinitions derives the protobuf definitions the way gg gen
// does (see pb.Generate), writing nothing, and reports each diagnostic it
// raises, so the project learns at check time what would stop its
// generation: a type protobuf cannot describe, a field without a pb tag or
// with a number another field holds, a name clashing with a generated
// message, a model left with nothing to serve, a definition breaking the
// contract committed under pb/. The models are read the way gg gen reads
// them, the gst.yaml route and model ignores applied, since the definitions
// reflect them. A project without a model declaring GRPC() has nothing to
// derive and costs nothing here.
func checkProtobufDefinitions(ignore gghelper.ProjectIgnore) []string {
	var violations []string
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return violations
	}
	cfg, err := ggconfig.Load(".")
	if err != nil {
		return append(violations, fmt.Sprintf("loading gst.yaml: %v", err))
	}
	modulePath, err := gghelper.ModulePath()
	if err != nil {
		return append(violations, fmt.Sprintf("reading the module path: %v", err))
	}
	allModels, err := modelinfo.FindModels(modulePath, ggconst.DirModel, ignore)
	if err != nil {
		return append(violations, fmt.Sprintf("scanning model designs: %v", err))
	}
	modelinfo.ResolveRoutes(allModels, cfg.Gen.Routes.Ignore)
	modelinfo.ApplyModelIgnores(allModels, cfg.Gen.Models.Ignore)

	_, err = pb.Generate(pb.Config{Dir: ".", ModulePath: modulePath, Models: allModels})
	var diagnostics *pb.DiagnosticsError
	switch {
	case errors.As(err, &diagnostics):
		for _, d := range diagnostics.Diagnostics {
			violations = append(violations, d.String())
		}
	case err != nil:
		violations = append(violations, fmt.Sprintf("deriving the protobuf definitions: %v", err))
	}
	return violations
}
