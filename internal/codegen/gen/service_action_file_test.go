package gen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsActionServiceSource(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	good := filepath.Join(tmp, "search_source_dedup.go")
	err := os.WriteFile(good, []byte(`package common

import (
	"example.com/mod/model/common"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type SearchSourceDedup struct {
	service.Base[*common.Common, *common.Common, *common.Common]
}

func (s *SearchSourceDedup) Create(ctx *types.ServiceContext, req *common.Common) (rsp *common.Common, err error) {
	return rsp, nil
}
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if !IsActionServiceSource(good) {
		t.Fatal("expected custom-filename-style service file to be recognized")
	}

	bad := filepath.Join(tmp, "helper.go")
	err = os.WriteFile(bad, []byte(`package common

func Helper() {}
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if IsActionServiceSource(bad) {
		t.Fatal("expected plain helper not to be recognized")
	}

	syntaxErr := filepath.Join(tmp, "broken.go")
	err = os.WriteFile(syntaxErr, []byte(`package common

func {`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if IsActionServiceSource(syntaxErr) {
		t.Fatal("expected broken parse to return false")
	}

	legacy := filepath.Join("testdata", "service", "user_create.go")
	if !IsActionServiceSource(legacy) {
		t.Fatal("expected testdata service file to be recognized")
	}
}
