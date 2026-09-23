package ggconfig_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/ggconfig"
)

func TestModelIgnoreRulesUnmarshal(t *testing.T) {
	t.Run("valid entries", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  models:
    ignore:
      Profile:
        from: model/iam
      Widget:
      Gadget: {}
`)
		cfg, err := ggconfig.Load(dir)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		want := ggconfig.ModelIgnoreRules{
			{Name: "Profile", From: "model/iam", Raw: "Profile"},
			{Name: "Widget", Raw: "Widget"},
			{Name: "Gadget", Raw: "Gadget"},
		}
		if !reflect.DeepEqual(cfg.Gen.Models.Ignore, want) {
			t.Fatalf("Ignore = %+v, want %+v", cfg.Gen.Models.Ignore, want)
		}
	})

	t.Run("invalid entries", func(t *testing.T) {
		for name, content := range map[string]string{
			"not a mapping":       "version: 1\ngen:\n  models:\n    ignore: [Profile]\n",
			"unexported name":     "version: 1\ngen:\n  models:\n    ignore:\n      profile:\n",
			"invalid identifier":  "version: 1\ngen:\n  models:\n    ignore:\n      \"My-Model\":\n",
			"duplicate name":      "version: 1\ngen:\n  models:\n    ignore:\n      Profile:\n      Profile:\n", //nolint:dupword // the duplicate key is the invalid input under test
			"unknown value field": "version: 1\ngen:\n  models:\n    ignore:\n      Profile:\n        source: model/iam\n",
			"value not a mapping": "version: 1\ngen:\n  models:\n    ignore:\n      Profile: model/iam\n",
			"empty from":          "version: 1\ngen:\n  models:\n    ignore:\n      Profile:\n        from: \"\"\n",
			"escaping from":       "version: 1\ngen:\n  models:\n    ignore:\n      Profile:\n        from: ../outside\n",
		} {
			t.Run(name, func(t *testing.T) {
				dir := writeConfig(t, content)
				if _, err := ggconfig.Load(dir); err == nil {
					t.Fatalf("Load() expected error, got nil")
				}
			})
		}
	})
}

func TestModelRuleMatchesSource(t *testing.T) {
	tests := []struct {
		name string
		from string
		path string
		want bool
	}{
		{"empty from matches everything", "", "model/admin/user.go", true},
		{"prefix match", "model/iam", "model/iam/user/user.go", true},
		{"exact dir match", "model/iam", "model/iam", true},
		{"different dir", "model/iam", "model/admin/user.go", false},
		{"prefix is not a path boundary", "model/iam", "model/iamx/user.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := ggconfig.ModelRule{Name: "User", From: tt.from}
			if got := rule.MatchesSource(tt.path); got != tt.want {
				t.Errorf("MatchesSource(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
