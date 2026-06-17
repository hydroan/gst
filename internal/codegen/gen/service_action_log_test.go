package gen

import (
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/types/consts"
)

func TestHumanizeDSLFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"search_source_dedup", "search source dedup"},
		{"search-source-dedup", "search source dedup"},
		{"path/to/foo_bar-baz", "foo bar baz"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := humanizeDSLFilename(tt.in); got != tt.want {
				t.Fatalf("humanizeDSLFilename(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestServiceActionLogQuoted(t *testing.T) {
	t.Parallel()
	act := &dsl.Action{Filename: "search_source_dedup"}
	if got := serviceActionLogQuoted("Common", consts.PHASE_CREATE, act); got != `"common: search source dedup"` {
		t.Fatalf("main create: got %s", got)
	}
	if got := serviceActionLogQuoted("Common", consts.PHASE_CREATE_BEFORE, act); got != `"common: search source dedup before"` {
		t.Fatalf("before hook: got %s", got)
	}
	if got := serviceActionLogQuoted("Common", consts.PHASE_CREATE_AFTER, act); got != `"common: search source dedup after"` {
		t.Fatalf("after hook: got %s", got)
	}
	act2 := &dsl.Action{Filename: "search-source-dedup"}
	if got := serviceActionLogQuoted("Common", consts.PHASE_CREATE, act2); got != `"common: search source dedup"` {
		t.Fatalf("hyphen filename: got %s", got)
	}
	if got := serviceActionLogQuoted("User", consts.PHASE_CREATE, nil); got != `"user create"` {
		t.Fatalf("no Filename: got %s", got)
	}
}
