package ggcheck_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestJSONTagNamingFlagsDSLActionTypeTags(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// Explicit DSL Payload and Result types carry the wire format of custom
	// actions, so their json tags must be snake_case just like model structs.
	// The result type lives in another file of the same package to cover
	// cross-file references.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Empty
}

func (Sample) Design() {
	Create(func() {
		Service()
		Payload[*SampleCreateReq]()
		Result[*SampleCreateRsp]()
	})
}

type SampleCreateReq struct {
	UserName string `+"`json:\"userName\"`"+`
	Note     string `+"`json:\"note\"`"+`
	Secret   string `+"`json:\"-\"`"+`
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "create_rsp.go"), `package sample

type SampleCreateRsp struct {
	CreatedAt string `+"`json:\"createdAt\"`"+`
	PlainName string `+"`json:\"plain_name\"`"+`
}
`)

	violations := runCheck(ggcheck.JSONTagNaming)

	if len(violations) != 2 {
		t.Fatalf("expected two action type json tag violations, got %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, filepath.Join("model", "sample", "sample.go")+": field 'UserName' json tag 'userName' should be 'user_name'") {
		t.Fatalf("expected payload type violation with file path, got %#v", violations)
	}
	if !strings.Contains(joined, filepath.Join("model", "sample", "create_rsp.go")+": field 'CreatedAt' json tag 'createdAt' should be 'created_at'") {
		t.Fatalf("expected cross-file result type violation with file path, got %#v", violations)
	}
}

func TestJSONTagNamingSkipsUnreferencedActionLikeStructs(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Empty
}

func (Sample) Design() {
	Create(func() {
		Service()
		Payload[*SampleCreateReq]()
	})
}

type SampleCreateReq struct {
	UserName string `+"`json:\"userName\"`"+`
}
`)
	// Req/Rsp-suffixed structs not referenced by any Design, such as outbound
	// DTOs mirroring an external contract, must keep their tags unchecked.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "push.go"), `package sample

type PushReq struct {
	DeviceID string `+"`json:\"deviceId\"`"+`
}

type PushRsp struct {
	PushedAt string `+"`json:\"pushedAt\"`"+`
}
`)

	violations := runCheck(ggcheck.JSONTagNaming)

	for _, violation := range violations {
		if strings.Contains(violation, "push.go") {
			t.Fatalf("unreferenced action-like struct should be skipped, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], "json tag 'userName' should be 'user_name'") {
		t.Fatalf("expected only referenced payload type violation, got %#v", violations)
	}
}

func TestJSONTagNamingFlagsModelStructTags(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import "github.com/hydroan/gst/model"

type Record struct {
	model.Base

	DisplayName string `+"`json:\"displayName\"`"+`
}
`)

	violations := runCheck(ggcheck.JSONTagNaming)

	if len(violations) != 1 {
		t.Fatalf("expected one model struct json tag violation, got %#v", violations)
	}
	if !strings.Contains(violations[0], filepath.Join("model", "record", "record.go")+": field 'DisplayName' json tag 'displayName' should be 'display_name'") {
		t.Fatalf("expected model struct violation with file path, got %#v", violations)
	}
}

// TestJSONTagNamingFlagsExportedFieldsWithoutAJSONTag pins that an exported
// field without a json key is a violation on a model struct and on an
// explicit DSL action type alike: its JSON key would be its Go name, which no
// snake_case key is, and a struct tag without a json key counts the same. An
// embedded field without a json key promotes fields carrying keys of their
// own, an unexported field never reaches JSON and json:"-" keeps a field off
// the wire, so none of those is reported; an embedded field given a json
// key is served under that key, so the key is held to snake_case like any
// other.
func TestJSONTagNamingFlagsExportedFieldsWithoutAJSONTag(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Inner struct {
	Value string `+"`json:\"value\"`"+`
}

type Sample struct {
	Untagged string
	Note     string `+"`gorm:\"column:note\"`"+`
	Hidden   string `+"`json:\"-\"`"+`
	secret   string
	Inner    `+"`json:\"innerData\"`"+`

	model.Base
}

func (Sample) Design() {
	Create(func() {
		Service()
		Payload[*SampleCreateReq]()
	})
}

type SampleCreateReq struct {
	Name  string `+"`json:\"name\"`"+`
	Count int
}
`)

	violations := runCheck(ggcheck.JSONTagNaming)

	if len(violations) != 4 {
		t.Fatalf("expected four violations, got %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	for _, want := range []string{
		filepath.Join("model", "sample", "sample.go") + `: field 'Untagged' has no json key, so its key is 'Untagged'; tag it json:"untagged", json:"-" to keep it off the wire, or unexport it`,
		filepath.Join("model", "sample", "sample.go") + `: field 'Note' has no json key, so its key is 'Note'; tag it json:"note", json:"-" to keep it off the wire, or unexport it`,
		filepath.Join("model", "sample", "sample.go") + `: embedded field 'Inner' json tag 'innerData' should be 'inner_data'`,
		filepath.Join("model", "sample", "sample.go") + `: field 'Count' has no json key, so its key is 'Count'; tag it json:"count", json:"-" to keep it off the wire, or unexport it`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q, got %#v", want, violations)
		}
	}
}
