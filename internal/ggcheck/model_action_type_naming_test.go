package ggcheck_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestModelActionTypeNamingFlagsSuffixlessPayloadAndResultTypes pins the
// naming rule of explicit DSL action types: a Payload type ends with Req and a
// Result type with Rsp, while the model's own type is exempt.
func TestModelActionTypeNamingFlagsSuffixlessPayloadAndResultTypes(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	source := `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

type SampleCreateReq struct{}

type SampleCreateRsp struct{}

type SampleUpdateInput struct{}

type SampleUpdateOutput struct{}

func (Sample) Design() {
	dsl.Endpoint("samples")
	dsl.Create(func() {
		dsl.Payload[*SampleCreateReq]()
		dsl.Result[*SampleCreateRsp]()
	})
	dsl.Update(func() {
		dsl.Payload[*SampleUpdateInput]()
		dsl.Result[*SampleUpdateOutput]()
	})
	dsl.Get(func() {
		dsl.Result[*Sample]()
	})
}
`
	path := filepath.Join("model", "sample", "sample.go")
	writeCheckFile(t, filepath.Join(projectDir, path), source)

	violations := runCheck(ggcheck.ModelActionTypeNaming)

	want := []string{
		fmt.Sprintf("%s:%d: Payload type 'SampleUpdateInput' should end with Req", path, sourceLine(t, source, "dsl.Payload[*SampleUpdateInput]()")),
		fmt.Sprintf("%s:%d: Result type 'SampleUpdateOutput' should end with Rsp", path, sourceLine(t, source, "dsl.Result[*SampleUpdateOutput]()")),
	}
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want %#v", violations, want)
	}
}
