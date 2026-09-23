package ggconfig_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/ggconfig"
)

// TestRouteIgnoreRulesUnmarshal loads gen.routes.ignore entries through
// Load, which also rejects duplicate rules once they are parsed.
func TestRouteIgnoreRulesUnmarshal(t *testing.T) {
	t.Run("valid entries", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  routes:
    ignore:
      /api/signup: [POST]
      /api/iam/admin/users: [GET]
      /api/iam/admin/users/:id:
        - GET
        - DELETE
`)
		cfg, err := ggconfig.Load(dir)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got := len(cfg.Gen.Routes.Ignore); got != 4 {
			t.Fatalf("len(Ignore) = %d, want 4", got)
		}
		if cfg.Gen.Routes.Ignore[0].Method != http.MethodPost {
			t.Errorf("Ignore[0].Method = %q, want POST", cfg.Gen.Routes.Ignore[0].Method)
		}
		last := cfg.Gen.Routes.Ignore[3]
		if last.Method != http.MethodDelete || last.Raw != "DELETE /api/iam/admin/users/:id" {
			t.Errorf("Ignore[3] = %+v, want DELETE /api/iam/admin/users/:id", last)
		}
	})

	t.Run("legacy string entries are rejected", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  routes:
    ignore:
      - POST /api/signup
`)
		if _, err := ggconfig.Load(dir); err == nil {
			t.Fatal("Load() expected error for non-mapping ignore, got nil")
		}
	})

	t.Run("invalid method is rejected", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  routes:
    ignore:
      /api/signup: [TRACE]
`)
		if _, err := ggconfig.Load(dir); err == nil {
			t.Fatal("Load() expected error for invalid method, got nil")
		}
	})

	t.Run("route without methods is rejected", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  routes:
    ignore:
      /api/signup: []
`)
		if _, err := ggconfig.Load(dir); err == nil {
			t.Fatal("Load() expected error for empty method list, got nil")
		}
	})

	t.Run("duplicate route paths are rejected", func(t *testing.T) {
		// All variants collapse to the same normalized path key: exact
		// repetition, /api prefix and slash formatting, and parameter names.
		tests := []struct {
			name    string
			entries string
		}{
			{"exact repetition", "      /api/signup: [POST]\n      /api/signup: [DELETE]\n"},
			{"formatting variant", "      /api/signup: [POST]\n      /signup/: [POST]\n"},
			{"param name variant", "      /api/users/:id: [GET]\n      /api/users/:userId: [GET]\n"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				dir := writeConfig(t, "version: 1\ngen:\n  routes:\n    ignore:\n"+tt.entries)
				if _, err := ggconfig.Load(dir); err == nil {
					t.Fatal("Load() expected error for duplicate route paths, got nil")
				}
			})
		}
	})

	t.Run("duplicate methods on one path are rejected", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  routes:
    ignore:
      /api/signup: [POST, post]
`)
		if _, err := ggconfig.Load(dir); err == nil {
			t.Fatal("Load() expected error for duplicate methods, got nil")
		}
	})

	t.Run("object form with from scopes the rule", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  routes:
    ignore:
      /api/iam/admin/users:
        methods: [GET]
        from: model/iam/
`)
		cfg, err := ggconfig.Load(dir)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got := len(cfg.Gen.Routes.Ignore); got != 1 {
			t.Fatalf("len(Ignore) = %d, want 1", got)
		}
		if from := cfg.Gen.Routes.Ignore[0].From; from != "model/iam" {
			t.Errorf("Ignore[0].From = %q, want model/iam (trailing slash trimmed)", from)
		}
	})

	t.Run("object form without from matches all models", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  routes:
    ignore:
      /api/signup:
        methods: [POST]
`)
		cfg, err := ggconfig.Load(dir)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if from := cfg.Gen.Routes.Ignore[0].From; from != "" {
			t.Errorf("Ignore[0].From = %q, want empty", from)
		}
	})

	t.Run("object form with unknown field is rejected", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  routes:
    ignore:
      /api/signup:
        methods: [POST]
        form: model/iam
`)
		if _, err := ggconfig.Load(dir); err == nil {
			t.Fatal("Load() expected error for unknown field in rule object, got nil")
		}
	})

	t.Run("object form with invalid from is rejected", func(t *testing.T) {
		for _, from := range []string{`""`, `../iam`, `model//iam`} {
			dir := writeConfig(t, `version: 1
gen:
  routes:
    ignore:
      /api/signup:
        methods: [POST]
        from: `+from+`
`)
			if _, err := ggconfig.Load(dir); err == nil {
				t.Errorf("Load() expected error for from %s, got nil", from)
			}
		}
	})
}

func TestParseRouteRule(t *testing.T) {
	t.Run("valid entries", func(t *testing.T) {
		tests := []struct {
			raw          string
			wantMethod   string
			wantSegments []string
		}{
			{"POST /api/signup", "POST", []string{"signup"}},
			{"get /api/iam/admin/users", "GET", []string{"iam", "admin", "users"}},
			{"GET /api/iam/admin/users/:id", "GET", []string{"iam", "admin", "users", ":id"}},
			{"GET /api/iam/admin/users/{id}", "GET", []string{"iam", "admin", "users", ":id"}},
			{"DELETE /samples/", "DELETE", []string{"samples"}},
		}
		for _, tt := range tests {
			rule, err := ggconfig.ParseRouteRule(tt.raw)
			if err != nil {
				t.Fatalf("ParseRouteRule(%q) error = %v", tt.raw, err)
			}
			if rule.Method != tt.wantMethod {
				t.Errorf("ParseRouteRule(%q).Method = %q, want %q", tt.raw, rule.Method, tt.wantMethod)
			}
			if !reflect.DeepEqual(rule.Segments, tt.wantSegments) {
				t.Errorf("ParseRouteRule(%q).Segments = %v, want %v", tt.raw, rule.Segments, tt.wantSegments)
			}
			if rule.Raw != tt.raw {
				t.Errorf("ParseRouteRule(%q).Raw = %q, want original entry", tt.raw, rule.Raw)
			}
		}
	})

	t.Run("invalid entries", func(t *testing.T) {
		for _, raw := range []string{
			"",
			"POST",
			"POST /api/signup extra",
			"TRACE /api/signup",
			"POST api/signup",
			"POST /",
			"POST /api",
			"POST /api//signup",
		} {
			if _, err := ggconfig.ParseRouteRule(raw); err == nil {
				t.Errorf("ParseRouteRule(%q) expected error, got nil", raw)
			}
		}
	})
}

func TestNormalizeRoutePath(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"/api/iam/admin/users/:id", []string{"iam", "admin", "users", ":id"}},
		{"iam/admin/users", []string{"iam", "admin", "users"}},
		{"/signup/", []string{"signup"}},
		{"{group}/items", []string{":group", "items"}},
		{"/api", nil},
		{"", nil},
	}
	for _, tt := range tests {
		if got := ggconfig.NormalizeRoutePath(tt.path); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("NormalizeRoutePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestRouteRuleMatch(t *testing.T) {
	tests := []struct {
		name      string
		rule      string
		method    string
		routePath string
		want      bool
	}{
		{"exact static path", "POST /api/signup", "POST", "signup", true},
		{"leading slash on target", "POST /api/signup", "POST", "/signup", true},
		{"method mismatch", "POST /api/signup", "GET", "signup", false},
		{"static segment mismatch", "GET /api/iam/admin/users", "GET", "iam/admin/roles", false},
		{"segment count mismatch", "GET /api/iam/admin/users", "GET", "iam/admin/users/:id", false},
		{"param matches positionally regardless of name", "GET /api/iam/admin/users/:id", "GET", "iam/admin/users/:userId", true},
		{"brace param on target", "GET /api/iam/admin/users/:id", "GET", "iam/admin/users/{userId}", true},
		{"static does not match param segment", "GET /api/iam/admin/users/list", "GET", "iam/admin/users/:id", false},
		{"param does not match static segment", "GET /api/iam/admin/users/:id", "GET", "iam/admin/users/list", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := ggconfig.ParseRouteRule(tt.rule)
			if err != nil {
				t.Fatalf("ParseRouteRule(%q) error = %v", tt.rule, err)
			}
			if got := rule.Match(tt.method, tt.routePath); got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.method, tt.routePath, got, tt.want)
			}
		})
	}
}

func TestRouteRuleMatchesSource(t *testing.T) {
	tests := []struct {
		name          string
		from          string
		modelFilePath string
		want          bool
	}{
		{"empty from matches everything", "", "model/admin/admin.go", true},
		{"model under from dir", "model/iam", "model/iam/user/user.go", true},
		{"model outside from dir", "model/iam", "model/admin/admin.go", false},
		{"prefix must be a whole segment", "model/iam", "model/iamx/user.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := ggconfig.RouteRule{From: tt.from}
			if got := rule.MatchesSource(tt.modelFilePath); got != tt.want {
				t.Errorf("MatchesSource(%q) with From=%q = %v, want %v", tt.modelFilePath, tt.from, got, tt.want)
			}
		})
	}
}
