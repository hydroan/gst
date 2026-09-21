package gen_test

import (
	"maps"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
)

func TestResolveImportConflicts(t *testing.T) {
	tests := []struct {
		name    string
		imports []string
		want    map[string]string
	}{
		{
			name:    "no_imports",
			imports: nil,
			want:    map[string]string{},
		},
		{
			name:    "distinct_base_names_need_no_alias",
			imports: []string{"helloworld/service/record", "helloworld/service/item"},
			want: map[string]string{
				"helloworld/service/record": "",
				"helloworld/service/item":   "",
			},
		},
		{
			name:    "shared_base_name_is_aliased_with_its_parent_dir",
			imports: []string{"helloworld/service/sample/record", "helloworld/service/archive/record"},
			want: map[string]string{
				"helloworld/service/sample/record":  "sample_record",
				"helloworld/service/archive/record": "archive_record",
			},
		},
		{
			name:    "only_the_conflicting_imports_are_aliased",
			imports: []string{"helloworld/service/sample/record", "helloworld/service/archive/record", "helloworld/service/item"},
			want: map[string]string{
				"helloworld/service/sample/record":  "sample_record",
				"helloworld/service/archive/record": "archive_record",
				"helloworld/service/item":           "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gen.ResolveImportConflicts(tt.imports); !maps.Equal(got, tt.want) {
				t.Errorf("ResolveImportConflicts(%q) = %v, want %v", tt.imports, got, tt.want)
			}
		})
	}
}
