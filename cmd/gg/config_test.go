package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestConfigDefaultsPrintsTheSectionAskedForInTheFormatAskedFor pins gg
// config defaults <section> --format: that one section, in that format, with
// the framework's default values whatever the environment sets, and the
// environment left as it was; a section the configuration does not have is
// refused with the names of the sections it has.
func TestConfigDefaultsPrintsTheSectionAskedForInTheFormatAskedFor(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("SERVER_PORT", "9999")

	out, err := runConfigCommand(t, "defaults", "server", "--format", "yaml")
	require.NoError(t, err)
	var doc map[string]map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(out), &doc), out)
	require.Len(t, doc, 1, out)
	require.Equal(t, "8080", fmt.Sprint(doc["server"]["port"]))
	require.Equal(t, "9999", os.Getenv("SERVER_PORT"))

	all, err := runConfigCommand(t, "defaults")
	require.NoError(t, err)
	var sections []string
	for _, name := range iniSectionNames(all) {
		if !strings.Contains(name, ".") {
			sections = append(sections, name)
		}
	}
	require.Contains(t, sections, "server")
	_, err = runConfigCommand(t, "defaults", "nosuch")
	require.EqualError(t, err, `unknown configuration section "nosuch"; the sections are `+strings.Join(sections, ", "))
}

// TestConfigDefaultsHoldTheSameConfigurationInEveryFormat pins the formats
// of gg config defaults. The INI lists the sections, and the keys of each, in
// alphabetical order, a section nested in another under its dotted name;
// JSON, TOML and YAML hold the same keys and values, the nested section as a
// map under its parent's key. A section printed alone comes with the
// sections nested in it.
func TestConfigDefaultsHoldTheSameConfigurationInEveryFormat(t *testing.T) {
	t.Chdir(t.TempDir())

	printed, err := runConfigCommand(t, "defaults")
	require.NoError(t, err)
	names := iniSectionNames(printed)
	require.True(t, slices.IsSorted(names), names)

	// The tree the other formats must hold, read off the INI line by line.
	want := make(map[string]any)
	var section map[string]any
	var keys []string
	for line := range strings.SplitSeq(strings.TrimSpace(printed), "\n") {
		if name, ok := strings.CutPrefix(line, "["); ok {
			require.True(t, slices.IsSorted(keys), keys)
			keys = nil
			section = want
			for part := range strings.SplitSeq(strings.TrimSuffix(name, "]"), ".") {
				child, ok := section[part].(map[string]any)
				if !ok {
					child = make(map[string]any)
					section[part] = child
				}
				section = child
			}
			continue
		}
		if key, value, ok := strings.Cut(line, " = "); ok {
			section[key] = value
			keys = append(keys, key)
		}
	}
	// The keys of the last section, which no section follows.
	require.True(t, slices.IsSorted(keys), keys)

	for format, decode := range map[string]func([]byte, any) error{"json": json.Unmarshal, "toml": toml.Unmarshal, "yaml": yaml.Unmarshal} {
		out, runErr := runConfigCommand(t, "defaults", "--format", format)
		require.NoError(t, runErr, format)
		var got map[string]any
		require.NoError(t, decode([]byte(out), &got), format)
		require.Equal(t, want, got, format)
	}

	// The first section that holds another, and what it is printed with.
	nested := slices.IndexFunc(names, func(name string) bool { return strings.Contains(name, ".") })
	require.GreaterOrEqual(t, nested, 0, names)
	parent, _, _ := strings.Cut(names[nested], ".")
	var family []string
	for _, name := range names {
		if name == parent || strings.HasPrefix(name, parent+".") {
			family = append(family, name)
		}
	}
	alone, err := runConfigCommand(t, "defaults", parent)
	require.NoError(t, err)
	require.Equal(t, family, iniSectionNames(alone))
}

// iniSectionNames returns the names of the sections of an INI file, in the
// order the file has them.
func iniSectionNames(content string) []string {
	var names []string
	for line := range strings.SplitSeq(content, "\n") {
		if name, ok := strings.CutPrefix(line, "["); ok {
			names = append(names, strings.TrimSuffix(name, "]"))
		}
	}
	return names
}

// runConfigCommand runs gg config with args on a command tree of its own, so
// that no flag one run parses reaches the next, and returns what it printed.
func runConfigCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newConfigCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	err := cmd.Execute()
	return out.String(), err
}
