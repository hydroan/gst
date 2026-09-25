package ggprune

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/ggconfig"
)

// ScanProtoFiles lists the .proto files under pbDir, the directory gg gen
// writes the protobuf definitions to, in walk order: pb/record.proto and
// pb/record/item.proto for a project serving Record and Item over gRPC. Like
// ScanServiceFiles it reads the directory whole, the directory being gg's: a
// file the project's Git ignore rules cover, or one below a directory the go
// command leaves out, is listed like any other. A missing directory lists
// nothing.
func ScanProtoFiles(pbDir string) ([]string, error) {
	var files []string
	if _, err := os.Stat(pbDir); os.IsNotExist(err) {
		return files, nil
	}
	err := filepath.Walk(pbDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".proto") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// PlanProtoFiles works out which of the existing .proto files prune
// deletes: the ones gg gen would not write now, generated being the paths it
// writes, slash-separated as pb.Generate hands them out. Of existing
// [pb/record.proto pb/note.proto pb/legacy/item.proto] and generated
// [pb/record.proto], the plan deletes pb/note.proto and, with a gst.yaml
// prune.ignore entry pb/legacy in protect, keeps pb/legacy/item.proto under
// Ignored. The definitions are the models' mirror, rewritten by every gg
// gen, so unlike a service file nothing in them is the project's to keep.
func PlanProtoFiles(existing, generated []string, protect ggconfig.PruneConfig) FilePlan {
	current := make([]string, 0, len(generated))
	for _, path := range generated {
		current = append(current, filepath.Clean(filepath.FromSlash(path)))
	}
	var plan FilePlan
	for _, path := range existing {
		if slices.Contains(current, filepath.Clean(path)) {
			continue
		}
		if protect.Ignores(path) {
			plan.Ignored = append(plan.Ignored, path)
			continue
		}
		plan.Delete = append(plan.Delete, path)
	}
	return plan
}
