package ggcheck

import (
	"fmt"
	"os"
	"sort"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/hydroan/gst/internal/modelinspect"
)

// IndexDeclarations holds the models' Indexes() declarations to what the
// configured database accepts.
var IndexDeclarations = Check{
	Name: "Index declarations",
	Rule: "on a project configured for MySQL, the Indexes() declarations of every model must be ones MySQL accepts: the fields exist, no column repeats, no declaration duplicates a struct tag index, and every column can be indexed, which a string field without a size cannot, being a longtext column; give it a size (gorm:\"size:191\") or a bounded type",
	run:  checkIndexDeclarations,
}

// checkIndexDeclarations reports the index declarations the configured
// database refuses. Only MySQL refuses column types an index cannot cover, so
// the check runs when the project's configuration, read the way the service
// and gg migrate read it, names MySQL as the database, and otherwise has
// nothing to report. The column type a field gets is gorm's to decide from the
// field's tags and type, so the check asks the models themselves through the
// inspection gg gen also runs (see modelinspect.Inspect), built from the
// project as it is: a project whose model packages do not compile, its
// generated column files out of date among the causes, cannot be asked and is
// reported as such rather than passed. Models of copyable framework modules
// are skipped, since copied module code is owned by the framework repository.
func checkIndexDeclarations(ignore gghelper.ProjectIgnore) []string {
	if _, err := os.Stat(ggconst.DirModel); os.IsNotExist(err) {
		return nil
	}
	if err := config.Init(); err != nil {
		return []string{fmt.Sprintf("index declarations could not be checked: loading the configuration: %v", err)}
	}
	defer config.Clean()
	if config.App.Database.Type != config.DBMySQL {
		return nil
	}

	scanned, err := scanModels(ignore)
	if err != nil {
		return []string{fmt.Sprintf("index declarations could not be checked: %v", err)}
	}
	owned, err := copyableModuleOwners()
	if err != nil {
		return []string{fmt.Sprintf("listing copyable framework modules: %v", err)}
	}
	inspected, ok, err := modelinspect.Inspect(scanned.Module, ggconst.DirModel, scanned.Models, ignore, nil)
	if err != nil {
		return []string{fmt.Sprintf("index declarations could not be checked: %v", err)}
	}
	if !ok {
		return nil
	}

	declared := make(map[string]*modelinfo.Model, len(scanned.Models))
	for _, m := range scanned.Models {
		declared[m.ImportPath()+"."+m.ModelName] = m
	}
	var violations []string
	for _, model := range inspected {
		if model.IndexViolation == "" {
			continue
		}
		// A model the scan knows is reported under the file declaring it; one
		// registered from elsewhere, a framework module's, under its package.
		origin := model.PkgPath
		if m, found := declared[model.PkgPath+"."+model.Name]; found {
			if moduleOwnedPath(owned, ggconst.DirModel, m.ModelFilePath) {
				continue
			}
			origin = m.ModelFilePath
		}
		violations = append(violations, origin+": "+model.IndexViolation)
	}
	sort.Strings(violations)
	return violations
}
