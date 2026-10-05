package ggprune

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggconst"
)

// ScanPBFiles lists the files gg gen writes under pbDir, the directory the
// protobuf definitions, the Go files serving them and the Go files compiled
// from them go to, in walk order: pb/pb.gen.go, pb/record/item.proto,
// pb/record.gen.go, pb/record.pb.go, pb/record.proto and
// pb/record_grpc.pb.go for a project serving Record and Item over gRPC. Only
// the .proto, .gen.go and .pb.go files are listed, the kinds gg gen writes.
// Like ScanServiceFiles it reads the directory whole, the directory being
// gg's: a file the project's Git ignore rules cover, or one below a
// directory the go command leaves out, is listed like any other. A missing
// directory lists nothing.
func ScanPBFiles(pbDir string) ([]string, error) {
	var files []string
	if _, err := os.Stat(pbDir); os.IsNotExist(err) {
		return files, nil
	}
	err := filepath.Walk(pbDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".proto") || strings.HasSuffix(info.Name(), ggconst.SuffixGenGo) || strings.HasSuffix(info.Name(), ".pb.go")) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// PlanPBFiles works out which of the existing files under pb/ prune deletes:
// the ones gg gen would not write now, generated being the paths it writes,
// slash-separated as pb.Generate and pb.Compile hand them out. Of existing
// [pb/record.proto pb/record.pb.go pb/note.proto pb/legacy/item.proto] and
// generated [pb/record.proto pb/record.pb.go], the plan deletes pb/note.proto
// and, with a gst.yaml prune.ignore entry pb/legacy in protect, keeps
// pb/legacy/item.proto under Ignored. The files are the models' mirror,
// rewritten by every gg gen, so unlike a service file nothing in them is the
// project's to keep, but for the numbers and names a definition reserves:
// gg gen deletes the stale Go files itself, as it writes, and leaves the
// definitions to prune, which warns of the reservations going with them.
func PlanPBFiles(existing, generated []string, protect ggconfig.PruneConfig) FilePlan {
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
