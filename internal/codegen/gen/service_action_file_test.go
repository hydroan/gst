package gen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
)

func TestIsActionServiceSource(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	good := filepath.Join(tmp, "archive_sample_items.go")
	err := os.WriteFile(good, []byte(`package common

import (
	"example.com/mod/model/common"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type ArchiveSampleItems struct {
	service.Base[*common.Common, *common.Common, *common.Common]
}

func (a *ArchiveSampleItems) Create(ctx *gst.ServiceContext, req *common.Common) (rsp *common.Common, err error) {
	return rsp, nil
}
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if !gen.IsActionServiceSource(good) {
		t.Fatal("expected custom-filename-style service file to be recognized")
	}

	bad := filepath.Join(tmp, "helper.go")
	err = os.WriteFile(bad, []byte(`package common

func Helper() {}
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if gen.IsActionServiceSource(bad) {
		t.Fatal("expected plain helper not to be recognized")
	}

	syntaxErr := filepath.Join(tmp, "broken.go")
	err = os.WriteFile(syntaxErr, []byte(`package common

func {`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if gen.IsActionServiceSource(syntaxErr) {
		t.Fatal("expected broken parse to return false")
	}

	legacy := filepath.Join("testdata", "service", "user_create.go")
	if !gen.IsActionServiceSource(legacy) {
		t.Fatal("expected testdata service file to be recognized")
	}
}
