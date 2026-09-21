package gen_test

import (
	"maps"
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
)

func TestResolveImportConflicts(t *testing.T) {
	tests := []struct {
		name     string
		imports  map[string]string
		reserved []string
		want     map[string]string
	}{
		{
			name:    "no_imports",
			imports: nil,
			want:    map[string]string{},
		},
		{
			name: "distinct_names_need_no_alias",
			imports: map[string]string{
				"helloworld/service/record": "record",
				"helloworld/service/item":   "item",
			},
			want: map[string]string{
				"helloworld/service/record": "",
				"helloworld/service/item":   "",
			},
		},
		{
			name: "shared_name_is_aliased_with_its_parent_dir",
			imports: map[string]string{
				"helloworld/service/sample/record":  "record",
				"helloworld/service/archive/record": "record",
			},
			want: map[string]string{
				"helloworld/service/sample/record":  "sample_record",
				"helloworld/service/archive/record": "archive_record",
			},
		},
		{
			name: "only_the_conflicting_imports_are_aliased",
			imports: map[string]string{
				"helloworld/service/sample/record":  "record",
				"helloworld/service/archive/record": "record",
				"helloworld/service/item":           "item",
			},
			want: map[string]string{
				"helloworld/service/sample/record":  "sample_record",
				"helloworld/service/archive/record": "archive_record",
				"helloworld/service/item":           "",
			},
		},
		{
			name: "aliases_sharing_the_last_two_segments_grow_until_they_differ",
			imports: map[string]string{
				"helloworld/service/sample/record/item":  "item",
				"helloworld/service/archive/record/item": "item",
			},
			want: map[string]string{
				"helloworld/service/sample/record/item":  "sample_record_item",
				"helloworld/service/archive/record/item": "archive_record_item",
			},
		},
		{
			name: "alias_grows_past_an_unaliased_import_of_the_same_name",
			imports: map[string]string{
				"helloworld/service/record_item":        "record_item",
				"helloworld/service/sample/record/item": "item",
				"helloworld/service/archive/item":       "item",
			},
			want: map[string]string{
				"helloworld/service/record_item":        "",
				"helloworld/service/sample/record/item": "sample_record_item",
				"helloworld/service/archive/item":       "archive_item",
			},
		},
		{
			// Underscores inside segments can make two different paths join
			// to the same name at every length; a number tells them apart.
			name: "aliases_that_cannot_grow_apart_take_a_number",
			imports: map[string]string{
				"helloworld/service/x/y_z": "y_z",
				"helloworld/service/v/y_z": "y_z",
				"helloworld/service/x_y/z": "z",
				"helloworld/service/w/z":   "z",
			},
			want: map[string]string{
				"helloworld/service/x/y_z": "helloworld_service_x_y_z",
				"helloworld/service/v/y_z": "v_y_z",
				"helloworld/service/x_y/z": "helloworld_service_x_y_z2",
				"helloworld/service/w/z":   "w_z",
			},
		},
		{
			name: "alias_replaces_characters_an_identifier_cannot_hold",
			imports: map[string]string{
				"helloworld/service/my-dir/item": "item",
				"helloworld/service/other/item":  "item",
			},
			want: map[string]string{
				"helloworld/service/my-dir/item": "my_dir_item",
				"helloworld/service/other/item":  "other_item",
			},
		},
		{
			name: "alias_grown_into_a_dotted_host_stays_an_identifier",
			imports: map[string]string{
				"example.com/x/item": "item",
				"example.org/x/item": "item",
			},
			want: map[string]string{
				"example.com/x/item": "example_com_x_item",
				"example.org/x/item": "example_org_x_item",
			},
		},
		{
			name: "alias_starting_with_a_digit_takes_a_leading_underscore",
			imports: map[string]string{
				"helloworld/service/2fa/item":   "item",
				"helloworld/service/other/item": "item",
			},
			want: map[string]string{
				"helloworld/service/2fa/item":   "_2fa_item",
				"helloworld/service/other/item": "other_item",
			},
		},
		{
			name:     "name_a_framework_import_takes_is_aliased",
			imports:  map[string]string{"helloworld/service/sample/service": "service"},
			reserved: []string{"service", "consts"},
			want: map[string]string{
				"helloworld/service/sample/service": "sample_service",
			},
		},
		{
			name:     "alias_that_cannot_grow_past_a_framework_name_takes_a_number",
			imports:  map[string]string{"service": "service"},
			reserved: []string{"service", "consts"},
			want: map[string]string{
				"service": "service2",
			},
		},
		{
			// The package name, not the last path segment, decides a clash.
			name: "shared_name_under_distinct_last_segments_is_aliased",
			imports: map[string]string{
				"helloworld/service/sample/record_item": "recorditem",
				"helloworld/service/archive/recorditem": "recorditem",
			},
			want: map[string]string{
				"helloworld/service/sample/record_item": "sample_record_item",
				"helloworld/service/archive/recorditem": "archive_recorditem",
			},
		},
		{
			name: "distinct_names_under_a_shared_last_segment_need_no_alias",
			imports: map[string]string{
				"helloworld/service/sample/item":  "record",
				"helloworld/service/archive/item": "entry",
			},
			want: map[string]string{
				"helloworld/service/sample/item":  "",
				"helloworld/service/archive/item": "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gen.ResolveImportConflicts(tt.imports, tt.reserved...); !maps.Equal(got, tt.want) {
				t.Errorf("ResolveImportConflicts(%v, %q) = %v, want %v", tt.imports, tt.reserved, got, tt.want)
			}
		})
	}
}
