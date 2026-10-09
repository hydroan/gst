package ggcheck_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestActionTypeFormStructAndSliceForms(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// A struct action type must use the pointer form, while a slice or map
	// action type (declared through a named alias) must use the value form.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	Update(func() {
		Service()
		Payload[*SampleUpdateReq]()
		Result[SampleUpdateRsp]()
	})
	Patch(func() {
		Service()
		Payload[SamplePatchReq]()
		Result[*SamplePatchRsp]()
	})
}

type SampleUpdateReq struct {
	Name string `+"`json:\"name\"`"+`
}

type SampleUpdateRsp = []*Sample

type SamplePatchReq struct {
	Name string `+"`json:\"name\"`"+`
}

type SamplePatchRsp = []*Sample
`)

	violations := runCheck(ggcheck.ActionTypeForm)

	if len(violations) != 2 {
		t.Fatalf("expected two form violations, got %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "Patch action declares Payload[SamplePatchReq] with the value form; a struct action type must use the pointer form Payload[*SamplePatchReq]") {
		t.Fatalf("expected value-form struct violation, got %#v", violations)
	}
	if !strings.Contains(joined, "Patch action declares Result[*SamplePatchRsp] with the pointer form; a slice or map action type must use the value form Result[SamplePatchRsp]") {
		t.Fatalf("expected pointer-form slice violation, got %#v", violations)
	}
}

// TestActionTypeFormCoversTheStreamingSides pins that StreamingPayload and
// StreamingResult are held to the same forms as Payload and Result, and are
// named as themselves in the violation.
func TestActionTypeFormCoversTheStreamingSides(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	GRPC()
	Stream(func() {
		Service("watch")
		StreamingResult[SampleWatchRsp]()
	})
}

type SampleWatchRsp struct {
	Name string `+"`json:\"name\"`"+`
}
`)

	violations := runCheck(ggcheck.ActionTypeForm)

	if len(violations) != 1 {
		t.Fatalf("violations = %#v, want one", violations)
	}
	if want := "Stream action declares StreamingResult[SampleWatchRsp] with the value form; a struct action type must use the pointer form StreamingResult[*SampleWatchRsp]"; !strings.Contains(violations[0], want) {
		t.Fatalf("violation = %q, want it to contain %q", violations[0], want)
	}
}

func TestActionTypeFormEmptyStructPairRule(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// Empty struct action types are the delegation marker for actions that
	// carry no data: they are allowed only when both sides of the action are
	// empty (an omitted side counts as empty). An empty struct paired with a
	// real data type must be removed because omitting it defaults the side to
	// *model.Empty.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	Delete(func() {
		Service()
		Payload[*SampleDeleteReq]()
		Result[*SampleDeleteRsp]()
	})
	Update(func() {
		Service()
		Payload[*SampleUpdateReq]()
	})
	Create(func() {
		Service()
		Payload[*SampleCreateReq]()
		Result[*SampleCreateRsp]()
	})
}

type SampleDeleteReq struct{}

type SampleDeleteRsp struct{}

type SampleUpdateReq struct{}

type SampleCreateReq struct{}

type SampleCreateRsp struct {
	Name string `+"`json:\"name\"`"+`
}
`)

	violations := runCheck(ggcheck.ActionTypeForm)

	for _, violation := range violations {
		if strings.Contains(violation, "SampleDeleteReq") || strings.Contains(violation, "SampleDeleteRsp") || strings.Contains(violation, "SampleUpdateReq") {
			t.Fatalf("empty struct with an empty peer side should be allowed, got violations: %#v", violations)
		}
	}
	if len(violations) != 1 || !strings.Contains(violations[0], "Create action declares Payload[*SampleCreateReq] whose type is an empty struct; remove the declaration so the framework defaults this side to *model.Empty") {
		t.Fatalf("expected single empty-struct removal violation, got %#v", violations)
	}
}

func TestActionTypeFormRejectsUnsupportedArguments(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// The type argument must be a named type declared in the same model
	// package: slice literals, cross-package selectors, and undeclared names
	// are rejected instead of being silently dropped by the parser.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"example.com/external"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	Create(func() {
		Service()
		Payload[[]SampleItem]()
		Result[*external.CreateRsp]()
	})
	Update(func() {
		Service()
		Payload[*SampleMissingReq]()
	})
}

type SampleItem struct {
	Name string `+"`json:\"name\"`"+`
}
`)

	violations := runCheck(ggcheck.ActionTypeForm)

	if len(violations) != 3 {
		t.Fatalf("expected three violations, got %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	if strings.Count(joined, "type argument must be a named type declared in the same model package") != 2 {
		t.Fatalf("expected two unsupported-argument violations, got %#v", violations)
	}
	if !strings.Contains(joined, "Update action declares Payload[*SampleMissingReq] but the type is not declared in the model package") {
		t.Fatalf("expected undeclared-type violation, got %#v", violations)
	}
}

func TestActionTypeFormRejectsInterfacePayloads(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// A request body decodes into no interface with methods, whether the
	// interface declares them itself or embeds an interface that does, and
	// whether Payload names it by value or through a pointer. An interface
	// without methods holds any JSON value, and a Result is only encoded, so
	// neither is a violation.
	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	"fmt"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	Create(func() {
		Service()
		Payload[SampleCreateReq]()
		Result[SampleCreateRsp]()
	})
	Update(func() {
		Service()
		Payload[*SampleUpdateReq]()
	})
	Patch(func() {
		Service()
		Payload[SamplePatchReq]()
	})
	Delete(func() {
		Service()
		Payload[SampleDeleteReq]()
	})
}

type SampleCreateReq interface {
	Bind()
}

type SampleCreateRsp interface {
	Render()
}

type SampleUpdateReq interface {
	fmt.Stringer
}

type SamplePatchReq interface {
	SampleBinder
}

type SampleBinder interface {
	Bind()
}

type SampleDeleteReq interface {
	SampleAny
}

type SampleAny = interface{}
`)

	violations := runCheck(ggcheck.ActionTypeForm)

	if len(violations) != 3 {
		t.Fatalf("expected three interface violations, got %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	for _, want := range []string{
		"Create action declares Payload[SampleCreateReq] whose type is an interface with methods, which no request body decodes into; declare a struct type and use the pointer form Payload[*SampleCreateReq]",
		"Update action declares Payload[*SampleUpdateReq] whose type is an interface with methods, which no request body decodes into; declare a struct type and use the pointer form Payload[*SampleUpdateReq]",
		"Patch action declares Payload[SamplePatchReq] whose type is an interface with methods, which no request body decodes into; declare a struct type and use the pointer form Payload[*SamplePatchReq]",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected violation %q, got %#v", want, violations)
		}
	}
}

func TestActionTypeFormAllowsDefaultCRUDAndEmptySides(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	// Default CRUD actions keep the model type on both sides, and a GET
	// action with a declared Result carries the *model.Empty sentinel as its
	// request side; neither shape is a violation.
	writeCheckFile(t, filepath.Join(projectDir, "model", "record", "record.go"), `package record

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Record struct {
	model.Base
}

func (Record) Design() {
	Migrate()
	Create(func() {})
	List(func() {
		Service()
		Result[*RecordListRsp]()
	})
}

type RecordListRsp struct {
	Items []*Record `+"`json:\"items\"`"+`
}
`)

	violations := runCheck(ggcheck.ActionTypeForm)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

// TestActionTypeFormRejectsInterfaceStreamingPayloads pins that the
// request side of a Stream action is held to what a request body decodes
// into the way Payload is: a StreamingPayload naming an interface with
// methods is reported, under its own keyword.
func TestActionTypeFormRejectsInterfaceStreamingPayloads(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Sample struct {
	model.Base
}

func (Sample) Design() {
	GRPC()
	Route("samples/chat", func() {
		Stream(func() {
			Service("chat")
			StreamingPayload[SampleChatReq]()
			StreamingResult[SampleChatRsp]()
		})
	})
}

type SampleChatReq interface {
	Bind()
}

type SampleChatRsp interface {
	Render()
}
`)

	violations := runCheck(ggcheck.ActionTypeForm)

	want := "Stream action declares StreamingPayload[SampleChatReq] whose type is an interface with methods, which no request body decodes into; declare a struct type and use the pointer form StreamingPayload[*SampleChatReq]"
	if len(violations) != 1 || !strings.Contains(violations[0], want) {
		t.Fatalf("expected the streaming payload interface to be reported once as %q, got %#v", want, violations)
	}
}

// TestActionTypeFormRefusesATypeDefinedOverANamedType pins that an action
// type declared as a defined type over another named type, of the package or
// of another, is refused for an alias, which shares the shape under
// the action's own name, or a struct of the action's own; the alias passes.
func TestActionTypeFormRefusesATypeDefinedOverANamedType(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "model", "sample", "sample.go"), `package sample

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"

	"tmpapp/model/shared"
)

type Sample struct {
	model.Base
}

type SampleRsp struct {
	Name string `+"`json:\"name\"`"+`
}

// SampleGetRsp is defined over SampleRsp: a type of its own with the same
// fields, which an alias is for.
type SampleGetRsp SampleRsp

// SampleSyncRsp is defined over another package's type.
type SampleSyncRsp shared.SyncRsp

// SampleListRsp is an alias of SampleRsp: the shape under the List's own name.
type SampleListRsp = SampleRsp

func (Sample) Design() {
	Endpoint("samples")
	Create(func() {
		Result[*SampleRsp]()
	})
	Get(func() {
		Result[*SampleGetRsp]()
	})
	List(func() {
		Result[*SampleListRsp]()
	})
	Route("samples/sync", func() {
		Create(func() {
			Service()
			Result[*SampleSyncRsp]()
		})
	})
}
`)

	violations := runCheck(ggcheck.ActionTypeForm)

	want := []string{
		"model/sample/sample.go: Get action declares Result[*SampleGetRsp] whose type is defined over SampleRsp; share the shape through an alias, type SampleGetRsp = SampleRsp, or declare a struct type of its own",
		"model/sample/sample.go: Create action declares Result[*SampleSyncRsp] whose type is defined over shared.SyncRsp; share the shape through an alias, type SampleSyncRsp = shared.SyncRsp, or declare a struct type of its own",
	}
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want %#v", violations, want)
	}
}
