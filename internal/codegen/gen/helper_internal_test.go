package gen

import (
	"testing"

	"github.com/hydroan/gst/consts"
)

func TestFixCommentPosition(t *testing.T) {
	header := consts.CodeGeneratedComment()
	tests := []struct {
		name string
		code string
		want string
	}{
		{
			name: "header_on_the_package_line_moves_above_it",
			code: "package model " + header + "\nfunc init() {\n}\n",
			want: header + "\n\npackage model\n\nfunc init() {\n}\n",
		},
		{
			name: "header_right_above_the_package_clause_gets_a_blank_line",
			code: header + "\npackage model\n\nfunc init() {\n}\n",
			want: header + "\n\npackage model\n\nfunc init() {\n}\n",
		},
		{
			name: "well_placed_header_stays",
			code: header + "\n\npackage model\n\nfunc init() {\n}\n",
			want: header + "\n\npackage model\n\nfunc init() {\n}\n",
		},
		{
			name: "package_doc_stays_attached_to_the_clause",
			code: header + "\n\n// Package model registers the models.\npackage model\n\nfunc init() {\n}\n",
			want: header + "\n\n// Package model registers the models.\npackage model\n\nfunc init() {\n}\n",
		},
		{
			name: "package_clause_gets_a_blank_line_after_it",
			code: "package model\nfunc init() {\n}\n",
			want: "package model\n\nfunc init() {\n}\n",
		},
		{
			name: "code_without_package_clause_is_unchanged",
			code: "func (u *Creator) Create() {\n}",
			want: "func (u *Creator) Create() {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixCommentPosition(tt.code); got != tt.want {
				t.Errorf("fixCommentPosition(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}
