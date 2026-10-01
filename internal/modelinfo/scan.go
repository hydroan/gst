package modelinfo

import (
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/gghelper"
)

// ScannedModels is what ScanModels read: the models of the project, their
// routes resolved, the module path they were read under, and how the ignore
// rules of gst.yaml applied to their routes and to the models.
type ScannedModels struct {
	Module       string
	Models       []*Model
	RouteIgnores RouteIgnoreResult
	ModelIgnores ModelIgnoreResult
}

// ScanModels reads the models of the project of module path module the way
// gg gen reads them, so that every command and check reading the models
// reads the same models on the same routes: the models FindModels finds
// under modelDir, their routes resolved by ResolveRoutes with the route
// ignore rules of cfg, the matched actions dropped, and the model ignore
// rules of cfg applied by ApplyModelIgnores last, once the actions are what
// they will be. A model file FindModels refuses stops the scan with its
// error.
func ScanModels(module, modelDir string, ignore gghelper.ProjectIgnore, cfg *ggconfig.Config) (ScannedModels, error) {
	models, err := FindModels(module, modelDir, ignore)
	if err != nil {
		return ScannedModels{}, err
	}
	routeIgnores := ResolveRoutes(models, cfg.Gen.Routes.Ignore)
	modelIgnores := ApplyModelIgnores(models, cfg.Gen.Models.Ignore)
	return ScannedModels{Module: module, Models: models, RouteIgnores: routeIgnores, ModelIgnores: modelIgnores}, nil
}
