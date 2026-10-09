package ggcheck_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestActionTypeUniquenessReportsATypeNameTwoActionsDeclare pins the rule
// that an explicit action type name belongs to one action: a Result, a
// Payload and a streaming Payload declaring a name again report the action
// declaring it first; an alias of the shared type under a name of its own,
// an alias into another package, another package's own name and the actions
// declaring no type pass.
func TestActionTypeUniquenessReportsATypeNameTwoActionsDeclare(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	samplePath := filepath.Join("model", "sample", "sample.go")
	writeCheckFile(t, filepath.Join(projectDir, samplePath), `package sample

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"

	"tmpapp/model/shared"
)

type Sample struct {
	model.Base
}

type SampleReq struct {
	Name string
}

type SampleRsp struct {
	Name string
}

// SampleGetRsp is the shape of SampleRsp under the Get's own name.
type SampleGetRsp = SampleRsp

// SampleSyncRsp is another package's shape under the sync's own name.
type SampleSyncRsp = shared.SyncRsp

type SampleFeedRsp struct {
	Name string
}

func (Sample) Design() {
	dsl.Endpoint("samples")
	dsl.Create(func() {
		dsl.Payload[*SampleReq]()
		dsl.Result[*SampleRsp]()
	})
	dsl.Get(func() {
		dsl.Result[*SampleGetRsp]()
	})
	dsl.List(func() {
		dsl.Result[*SampleRsp]()
	})
	dsl.Delete(func() {})
	dsl.Route("samples/sync", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Payload[*SampleReq]()
			dsl.Result[*SampleSyncRsp]()
		})
	})
	dsl.Route("samples/feed", func() {
		dsl.Stream(func() {
			dsl.Service("feed")
			dsl.StreamingPayload[*SampleReq]()
			dsl.StreamingResult[*SampleFeedRsp]()
		})
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "shared", "shared.go"), `package shared

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Shared struct {
	model.Empty
}

type SyncRsp struct {
	Done bool
}

func (Shared) Design() {
	dsl.Route("shared/sync", func() {
		dsl.Create(func() {
			dsl.Service()
			dsl.Result[*SyncRsp]()
		})
	})
}
`)

	violations := runCheck(ggcheck.ActionTypeUniqueness)

	want := []string{
		samplePath + ": List action on samples declares Result[*SampleRsp], which the Create action on samples declares as Result[*SampleRsp] already; each action names request and response types of its own",
		samplePath + ": Create action on samples/sync declares Payload[*SampleReq], which the Create action on samples declares as Payload[*SampleReq] already; each action names request and response types of its own",
		samplePath + ": Stream action on samples/feed declares StreamingPayload[*SampleReq], which the Create action on samples declares as Payload[*SampleReq] already; each action names request and response types of its own",
	}
	slices.Sort(violations)
	slices.Sort(want)
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want %#v", violations, want)
	}
}
