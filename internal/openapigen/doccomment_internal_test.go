package openapigen

import "testing"

func TestOpenAPIDocCommentRemovesExactGoDocSubject(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		comment string
		want    string
	}{
		{
			name:    "Chinese copula",
			symbol:  "Label",
			comment: "Label 是字段标签。",
			want:    "字段标签。",
		},
		{
			name:    "Chinese question phrase",
			symbol:  "Enabled",
			comment: "Enabled 是否启用。",
			want:    "是否启用。",
		},
		{
			name:    "English copula",
			symbol:  "Name",
			comment: "Name is the option name.",
			want:    "The option name.",
		},
		{
			name:    "English plural copula",
			symbol:  "Options",
			comment: "Options are available choices.",
			want:    "Available choices.",
		},
		{
			name:    "tab symbol boundary",
			symbol:  "Name",
			comment: "Name\tis the display name.",
			want:    "The display name.",
		},
		{
			name:    "wrapped English copula",
			symbol:  "Name",
			comment: "Name is\nthe display name.",
			want:    "The display name.",
		},
		{
			name:    "English verb",
			symbol:  "GroupRoles",
			comment: "GroupRoles binds groups to their roles.",
			want:    "Binds groups to their roles.",
		},
		{
			name:    "ASCII colon",
			symbol:  "Name",
			comment: "Name: display name.",
			want:    "Display name.",
		},
		{
			name:    "full width colon",
			symbol:  "Name",
			comment: "Name：显示名称。",
			want:    "显示名称。",
		},
		{
			name:    "multiple lines",
			symbol:  "Model",
			comment: "Model is the model summary.\n\nMore details.",
			want:    "The model summary.\n\nMore details.",
		},
		{
			name:    "similar identifier",
			symbol:  "ID",
			comment: "IDCard is the identity card.",
			want:    "IDCard is the identity card.",
		},
		{
			name:    "comment without subject",
			symbol:  "Name",
			comment: "显示名称。",
			want:    "显示名称。",
		},
		{
			name:    "subject only",
			symbol:  "Name",
			comment: "Name",
			want:    "Name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := openAPIDocComment(tt.symbol, tt.comment); got != tt.want {
				t.Fatalf("openAPIDocComment(%q, %q) = %q, want %q", tt.symbol, tt.comment, got, tt.want)
			}
		})
	}
}
