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
		{
			name:    "aliases_sharing_the_last_two_segments_grow_until_they_differ",
			imports: []string{"helloworld/service/sample/record/item", "helloworld/service/archive/record/item"},
			want: map[string]string{
				"helloworld/service/sample/record/item":  "sample_record_item",
				"helloworld/service/archive/record/item": "archive_record_item",
			},
		},
		{
			name:    "alias_grows_past_an_unaliased_import_of_the_same_name",
			imports: []string{"helloworld/service/record_item", "helloworld/service/sample/record/item", "helloworld/service/archive/item"},
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
			imports: []string{
				"helloworld/service/x/y_z",
				"helloworld/service/v/y_z",
				"helloworld/service/x_y/z",
				"helloworld/service/w/z",
			},
			want: map[string]string{
				"helloworld/service/x/y_z": "helloworld_service_x_y_z",
				"helloworld/service/v/y_z": "v_y_z",
				"helloworld/service/x_y/z": "helloworld_service_x_y_z2",
				"helloworld/service/w/z":   "w_z",
			},
		},
		{
			name:    "alias_replaces_characters_an_identifier_cannot_hold",
			imports: []string{"helloworld/service/my-dir/item", "helloworld/service/other/item"},
			want: map[string]string{
				"helloworld/service/my-dir/item": "my_dir_item",
				"helloworld/service/other/item":  "other_item",
			},
		},
		{
			name:    "alias_grown_into_a_dotted_host_stays_an_identifier",
			imports: []string{"example.com/x/item", "example.org/x/item"},
			want: map[string]string{
				"example.com/x/item": "example_com_x_item",
				"example.org/x/item": "example_org_x_item",
			},
		},
		{
			name:    "alias_starting_with_a_digit_takes_a_leading_underscore",
			imports: []string{"helloworld/service/2fa/item", "helloworld/service/other/item"},
			want: map[string]string{
				"helloworld/service/2fa/item":   "_2fa_item",
				"helloworld/service/other/item": "other_item",
			},
		},
		{
			name:    "a_repeated_import_needs_no_alias",
			imports: []string{"helloworld/service/record", "helloworld/service/record"},
			want: map[string]string{
				"helloworld/service/record": "",
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
