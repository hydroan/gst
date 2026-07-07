// Package ggconfig loads the project-level gst configuration file (gst.yaml)
// that gg commands consume at build time. It is unrelated to the runtime
// configuration managed by the config package.
package ggconfig

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"gopkg.in/yaml.v3"
)

// FileName is the name of the project-level gst configuration file,
// located next to go.mod in a business project.
const FileName = "gst.yaml"

// currentVersion is the only gst.yaml schema version supported by this build.
const currentVersion = 1

// Config is the project-level gst configuration.
type Config struct {
	// Version is the gst.yaml schema version. Must be 1.
	Version int `yaml:"version"`

	// Gen configures gg gen behavior.
	Gen GenConfig `yaml:"gen"`
}

// GenConfig configures gg gen behavior.
type GenConfig struct {
	// Routes configures route generation behavior.
	Routes GenRoutesConfig `yaml:"routes"`
}

// GenRoutesConfig configures route generation behavior.
type GenRoutesConfig struct {
	// Ignore lists routes excluded from code generation, each entry in the
	// form "METHOD /api/path". A matched action is treated as if it were
	// not declared in the model Design: no router registration, no service
	// registration, and no service file generation.
	Ignore []RouteRule `yaml:"ignore"`
}

// RouteRule is a single parsed "METHOD /api/path" ignore entry.
type RouteRule struct {
	// Method is the upper-cased HTTP method.
	Method string

	// Segments is the normalized route path split into segments. Parameter
	// segments keep their ":name" form but match positionally by shape.
	Segments []string

	// Raw preserves the original entry for error and log output.
	Raw string
}

// allowedRuleMethods lists the HTTP methods the generated routes can use.
var allowedRuleMethods = map[string]struct{}{
	"GET":    {},
	"POST":   {},
	"PUT":    {},
	"PATCH":  {},
	"DELETE": {},
}

// UnmarshalYAML parses a "METHOD /api/path" scalar entry into a RouteRule.
func (r *RouteRule) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err != nil {
		return errors.Wrap(err, "route rule must be a string")
	}
	rule, err := ParseRouteRule(raw)
	if err != nil {
		return err
	}
	*r = rule
	return nil
}

// ParseRouteRule parses a "METHOD /api/path" entry into a RouteRule.
func ParseRouteRule(raw string) (RouteRule, error) {
	fields := strings.Fields(raw)
	if len(fields) != 2 {
		return RouteRule{}, errors.Newf("invalid route rule %q: want format \"METHOD /api/path\"", raw)
	}

	method := strings.ToUpper(fields[0])
	if _, ok := allowedRuleMethods[method]; !ok {
		return RouteRule{}, errors.Newf("invalid route rule %q: unsupported method %q", raw, fields[0])
	}

	if !strings.HasPrefix(fields[1], "/") {
		return RouteRule{}, errors.Newf("invalid route rule %q: path must start with \"/\"", raw)
	}
	segments := NormalizeRoutePath(fields[1])
	if len(segments) == 0 {
		return RouteRule{}, errors.Newf("invalid route rule %q: empty path", raw)
	}
	if slices.Contains(segments, "") {
		return RouteRule{}, errors.Newf("invalid route rule %q: empty path segment", raw)
	}

	return RouteRule{Method: method, Segments: segments, Raw: raw}, nil
}

// Match reports whether the rule matches the given HTTP method and route
// path. The route path is normalized the same way as the rule path, and
// parameter segments (":name" or "{name}") match positionally regardless
// of the parameter name.
func (r RouteRule) Match(method, routePath string) bool {
	if r.Method != strings.ToUpper(method) {
		return false
	}

	segments := NormalizeRoutePath(routePath)
	if len(segments) != len(r.Segments) {
		return false
	}
	for i, want := range r.Segments {
		got := segments[i]
		wantParam := strings.HasPrefix(want, ":")
		gotParam := strings.HasPrefix(got, ":")
		if wantParam != gotParam {
			return false
		}
		if !wantParam && want != got {
			return false
		}
	}
	return true
}

// NormalizeRoutePath splits a route path into normalized segments: the
// "/api" prefix and surrounding slashes are stripped, and "{name}"
// parameter segments are converted to the ":name" form. It returns nil
// when no segments remain.
//
// The "api" prefix strip is applied to rule paths and generated route
// paths alike, which assumes no generated endpoint has a literal "api"
// first segment (generated routes never carry the "/api" prefix; the
// runtime router group adds it).
func NormalizeRoutePath(path string) []string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "api" {
		return nil
	}
	path = strings.TrimPrefix(path, "api/")
	if path == "" {
		return nil
	}

	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			segments[i] = ":" + strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
		}
	}
	return segments
}

// Load reads the gst.yaml file from dir. A missing file is not an error
// and yields a configuration with only defaults, so projects without a
// gst.yaml keep the current gg behavior.
func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Version: currentVersion}, nil
		}
		return nil, errors.Wrapf(err, "failed to read %s", path)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	cfg := new(Config)
	if err := decoder.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, errors.Wrapf(err, "failed to parse %s", path)
	}
	if cfg.Version != currentVersion {
		return nil, errors.Newf("%s: unsupported version %d, want %d", path, cfg.Version, currentVersion)
	}
	if err := validateIgnoreRules(cfg.Gen.Routes.Ignore); err != nil {
		return nil, errors.Wrapf(err, "%s: gen.routes.ignore", path)
	}
	return cfg, nil
}

// validateIgnoreRules rejects duplicate ignore rules. Duplicates are
// compared by normalized method and path so that formatting variants of
// the same route are still reported.
func validateIgnoreRules(rules []RouteRule) error {
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		key := rule.dedupKey()
		if _, ok := seen[key]; ok {
			return errors.Newf("duplicate rule %q", rule.Raw)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// dedupKey returns the rule identity used for duplicate detection. Parameter
// segments are collapsed to ":" because Match treats parameter names as
// interchangeable, so rules differing only in parameter names target the
// same routes. Segments are always non-empty here: ParseRouteRule rejects
// empty segments before a RouteRule is constructed.
func (r RouteRule) dedupKey() string {
	segments := make([]string, len(r.Segments))
	for i, segment := range r.Segments {
		if strings.HasPrefix(segment, ":") {
			segments[i] = ":"
		} else {
			segments[i] = segment
		}
	}
	return r.Method + " /" + strings.Join(segments, "/")
}
