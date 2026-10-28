package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestConfigListNamesTheSectionsTheDefaultsHold pins that gg config list
// names, in order, the top-level sections gg config defaults prints, the
// names gg config defaults <section> takes.
func TestConfigListNamesTheSectionsTheDefaultsHold(t *testing.T) {
	t.Chdir(t.TempDir())

	listed, err := runConfigCommand(t, "list")
	require.NoError(t, err)
	defaults, err := runConfigCommand(t, "defaults")
	require.NoError(t, err)

	var sections []string
	for line := range strings.SplitSeq(defaults, "\n") {
		if name, ok := strings.CutPrefix(line, "["); ok && !strings.Contains(name, ".") {
			sections = append(sections, strings.TrimSuffix(name, "]"))
		}
	}
	require.NotEmpty(t, sections)
	require.Equal(t, strings.Join(sections, "\n")+"\n", listed)
}

// TestConfigDefaultsPrintsTheSectionAskedForInTheFormatAskedFor pins gg
// config defaults <section> --format: that one section, in that format, with
// the framework's default values whatever the environment sets, and the
// environment left as it was; a section the configuration does not have is
// refused by its name.
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

	_, err = runConfigCommand(t, "defaults", "nosuch")
	require.ErrorContains(t, err, `unknown configuration section "nosuch"; run 'gg config list' to see available sections`)
}

// TestConfigConvertReadsEveryFormatTheSameWay pins gg config convert to INI
// from each format it reads: the same configuration in JSON, YAML and TOML,
// a number, a boolean, a list and a nested section among its values, comes
// out as the same INI, sections and keys sorted, the nested section under
// its dotted name and the list joined with commas.
func TestConfigConvertReadsEveryFormatTheSameWay(t *testing.T) {
	t.Chdir(t.TempDir())
	inputs := map[string]string{
		"config.json": `{"server": {"port": 8080, "debug": true, "trusted_proxies": ["127.0.0.1", "::1"]}, "logger": {"level": "info", "http_body": {"enabled": false}}}`,
		"config.yaml": "server:\n  port: 8080\n  debug: true\n  trusted_proxies: [\"127.0.0.1\", \"::1\"]\nlogger:\n  level: info\n  http_body:\n    enabled: false\n",
		"config.toml": "[server]\nport = 8080\ndebug = true\ntrusted_proxies = [\"127.0.0.1\", \"::1\"]\n\n[logger]\nlevel = \"info\"\n\n[logger.http_body]\nenabled = false\n",
	}
	want := "[logger]\nlevel = info\n\n[logger.http_body]\nenabled = false\n\n[server]\ndebug = true\nport = 8080\ntrusted_proxies = 127.0.0.1,::1\n"
	for name, content := range inputs {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(name, []byte(content), 0o600))

			out, err := runConfigCommand(t, "convert", name, "--to", "ini")
			require.NoError(t, err)
			require.Equal(t, want, out)
		})
	}
}

// TestConfigConvertKeepsTheDefaultsThroughEveryFormat pins that the
// framework's defaults converted from INI through YAML, JSON and TOML back
// to INI come back as the file they started as, the nested section included.
func TestConfigConvertKeepsTheDefaultsThroughEveryFormat(t *testing.T) {
	t.Chdir(t.TempDir())
	defaults, err := runConfigCommand(t, "defaults")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("config.ini", []byte(defaults), 0o600))

	for _, step := range [][2]string{{"config.ini", "config.yaml"}, {"config.yaml", "config.json"}, {"config.json", "config.toml"}, {"config.toml", "back.ini"}} {
		_, err = runConfigCommand(t, "convert", step[0], step[1])
		require.NoError(t, err, "%s to %s", step[0], step[1])
	}
	back, err := os.ReadFile("back.ini")
	require.NoError(t, err)
	require.Equal(t, defaults, string(back))
}

// TestConfigConvertKeepsASectionWrittenBeforeItsParent pins that the order
// of the sections of an INI file does not matter: a nested section written
// before its parent ends up beside the parent's own keys.
func TestConfigConvertKeepsASectionWrittenBeforeItsParent(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("config.ini", []byte("[logger.http_body]\nenabled = false\n\n[logger]\nlevel = info\n"), 0o600))

	out, err := runConfigCommand(t, "convert", "config.ini", "--to", "json")
	require.NoError(t, err)
	require.JSONEq(t, `{"logger": {"level": "info", "http_body": {"enabled": "false"}}}`, out)
}

// TestConfigConvertRefusesWhatItCannotDecide pins the refusals of gg config
// convert — no output named either way, an output named both ways, an
// output whose name tells no format, a format gg does not write, and an
// output that exists while --force is not given, which it leaves as it was
// — and that --force overwrites that output.
func TestConfigConvertRefusesWhatItCannotDecide(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("config.json", []byte(`{"server": {"port": 8080}}`), 0o600))

	for args, want := range map[string]string{
		"config.json": "output file or --to format is required",
		"config.json out.yaml --output other.yaml": "output file specified twice; use either positional output or --output",
		"config.json out":                          "cannot infer config format from out; use --from or --to",
		"config.json out.conf":                     `unsupported config format "conf"; supported formats: ini, json, toml, yaml`,
	} {
		_, err := runConfigCommand(t, append([]string{"convert"}, strings.Fields(args)...)...)
		require.ErrorContains(t, err, want, args)
	}

	require.NoError(t, os.WriteFile("out.yaml", []byte("kept: true\n"), 0o600))
	_, err := runConfigCommand(t, "convert", "config.json", "out.yaml")
	require.ErrorContains(t, err, "output file out.yaml already exists; use --force to overwrite it")
	kept, err := os.ReadFile("out.yaml")
	require.NoError(t, err)
	require.Equal(t, "kept: true\n", string(kept))

	_, err = runConfigCommand(t, "convert", "config.json", "out.yaml", "--force")
	require.NoError(t, err)
	written, err := os.ReadFile("out.yaml")
	require.NoError(t, err)
	var doc map[string]map[string]any
	require.NoError(t, yaml.Unmarshal(written, &doc), string(written))
	require.Equal(t, "8080", fmt.Sprint(doc["server"]["port"]))
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
