// Package unsupported declares fixture types whose JSON shape the TypeScript
// generator cannot describe. Rejected carries a field of every such form.
package unsupported

import (
	"math"
	"net/netip"

	dashed "github.com/hydroan/gst/internal/codegen/gen/ts/fixture/a-b"
	underscored "github.com/hydroan/gst/internal/codegen/gen/ts/fixture/a_b"
	prelude "github.com/hydroan/gst/internal/codegen/gen/ts/fixture/gst"
	"github.com/hydroan/gst/internal/codegen/gen/ts/fixture/mode"
)

// Rejected is a route type the generator refuses.
type Rejected struct {
	Custom  Custom                `json:"custom"`
	Speaker Speaker               `json:"speaker"`
	Updates chan int              `json:"updates"`
	Box     Box[string]           `json:"box"`
	Pair    Pair[string]          `json:"pair"`
	ByKey   map[Key]string        `json:"by_key"`
	Hosts   map[netip.Addr]string `json:"hosts"`
	Quoted  string                `json:"it's"` //nolint:staticcheck // the malformed name is the form under test.
	Format  string                `json:"format,format:RFC3339"`
	Kind    class                 `json:"kind"`
	Huge    Huge                  `json:"huge"`
	Mode    mode.Mode             `json:"mode"`

	Dashed      dashed.Item      `json:"dashed"`
	Underscored underscored.Item `json:"underscored"`
	Prelude     prelude.Item     `json:"prelude"`

	*hidden
}

// Custom encodes itself.
type Custom struct{}

// MarshalJSON encodes Custom as a fixed string.
func (Custom) MarshalJSON() ([]byte, error) {
	return []byte(`"custom"`), nil
}

// Speaker is an interface with methods.
type Speaker interface {
	Speak() string
}

// Box is a generic type of the project.
type Box[T any] struct {
	Value T `json:"value"`
}

// Pair is a generic alias of the project.
type Pair[T any] = Box[T]

// Key is a struct used as a map key.
type Key struct {
	ID int
}

// class is named after a reserved word of TypeScript.
type class string

// Huge is an enum with a constant no JavaScript number holds exactly.
type Huge uint64

const HugeMax Huge = math.MaxUint64

// ModeExtra is a constant of mode.Mode declared outside the mode package.
const ModeExtra mode.Mode = "extra"

// hidden is an unexported struct embedded through a pointer.
type hidden struct {
	Note string `json:"note"`
}
