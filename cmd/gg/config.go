package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

type configFileFormat string

const (
	configFormatINI  configFileFormat = "ini"
	configFormatJSON configFileFormat = "json"
	configFormatTOML configFileFormat = "toml"
	configFormatYAML configFileFormat = "yaml"
)

var configCmd = newConfigCmd()

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print the framework's default configuration",
		Args:  cobra.NoArgs,
		RunE:  showHelp,
	}

	cmd.AddCommand(newConfigDefaultsCmd())
	return cmd
}

func newConfigDefaultsCmd() *cobra.Command {
	format := string(configFormatINI)
	cmd := &cobra.Command{
		Use:   "defaults [section]",
		Short: "Print the default configuration, all of it or one section",
		Long: `Print the framework's default configuration on standard output.

With a section, only that top-level section is printed, with the sections
nested in it. The output is INI, the format of a gst configuration file,
unless --format names another.`,
		Example: `  gg config defaults
  gg config defaults server
  gg config defaults redis --format yaml
  gg config defaults --format toml > config.toml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigDefaults(cmd, args, format)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", format, "output format: ini, json, toml, yaml")
	return cmd
}

func runConfigDefaults(cmd *cobra.Command, args []string, formatName string) error {
	format, err := normalizeConfigFormat(formatName)
	if err != nil {
		return err
	}

	data, err := defaultConfigData()
	if err != nil {
		return err
	}
	if len(args) == 1 {
		section := strings.ToLower(args[0])
		value, ok := data[section]
		if !ok {
			return errors.Newf("unknown configuration section %q; the sections are %s", args[0], strings.Join(slices.Sorted(maps.Keys(data)), ", "))
		}
		data = map[string]any{section: value}
	}

	content, err := encodeConfigData(data, format)
	if err != nil {
		return err
	}
	_, err = cmd.OutOrStdout().Write(content)
	return err
}

// defaultConfigData returns the configuration the framework starts with when
// the project sets nothing, neither in a file nor in the environment: a tree
// of sections, a nested section a map under its parent's key and every value
// the string the INI file holds.
func defaultConfigData() (map[string]any, error) {
	var content bytes.Buffer
	err := withCleanConfigEnvironment(func() error {
		return inTemporaryDirectory(func() error {
			if err := config.Init(); err != nil {
				return errors.Wrap(err, "failed to initialize default config")
			}
			if err := config.Save(&content); err != nil {
				return errors.Wrap(err, "failed to collect default config")
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return decodeINIConfig(content.Bytes())
}

func withCleanConfigEnvironment(fn func() error) error {
	keys := configEnvKeys(reflect.TypeFor[config.Config](), nil)
	saved := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			saved[key] = value
			if err := os.Unsetenv(key); err != nil {
				return errors.Wrapf(err, "failed to unset environment variable %s", key)
			}
		}
	}
	defer func() {
		for _, key := range keys {
			if value, ok := saved[key]; ok {
				_ = os.Setenv(key, value)
			}
		}
	}()
	return fn()
}

func configEnvKeys(typ reflect.Type, prefix []string) []string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}

	keys := make([]string, 0)
	for field := range typ.Fields() {
		name := configTagName(field.Tag.Get("mapstructure"))
		if name == "" {
			name = configTagName(field.Tag.Get("json"))
		}
		if name == "" {
			continue
		}

		fieldPath := append(append([]string{}, prefix...), name)
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct && fieldType.PkgPath() != "time" {
			keys = append(keys, configEnvKeys(fieldType, fieldPath)...)
			continue
		}
		keys = append(keys, strings.ToUpper(strings.Join(fieldPath, "_")))
	}
	return keys
}

func inTemporaryDirectory(fn func() error) error {
	oldWD, err := os.Getwd()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "gst-config-defaults-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := os.Chdir(tmp); err != nil {
		return err
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()
	return fn()
}

func encodeConfigData(data map[string]any, format configFileFormat) ([]byte, error) {
	switch format {
	case configFormatINI:
		return encodeINIConfig(data), nil
	case configFormatJSON:
		content, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return nil, errors.Wrap(err, "failed to encode JSON")
		}
		return append(content, '\n'), nil
	case configFormatTOML:
		content, err := toml.Marshal(data)
		if err != nil {
			return nil, errors.Wrap(err, "failed to encode TOML")
		}
		return content, nil
	case configFormatYAML:
		content, err := yaml.Marshal(data)
		if err != nil {
			return nil, errors.Wrap(err, "failed to encode YAML")
		}
		return content, nil
	default:
		return nil, errors.Newf("unsupported config format %q", format)
	}
}

func decodeINIConfig(content []byte) (map[string]any, error) {
	cfg, err := ini.Load(content)
	if err != nil {
		return nil, err
	}

	data := make(map[string]any)
	for _, section := range cfg.Sections() {
		if section.Name() == ini.DefaultSection {
			continue
		}
		values := make(map[string]any)
		for _, key := range section.Keys() {
			values[key.Name()] = key.Value()
		}
		setConfigSection(data, section.Name(), values)
	}
	return data, nil
}

// encodeINIConfig writes the tree of sections as an INI file: a nested
// section under its dotted name, the sections and the keys of each in
// alphabetical order.
func encodeINIConfig(data map[string]any) []byte {
	sections := make(map[string]map[string]any)
	flattenConfigSections("", data, sections)

	names := make([]string, 0, len(sections))
	for name := range sections {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	for i, name := range names {
		if i > 0 {
			buf.WriteByte('\n')
		}
		fmt.Fprintf(&buf, "[%s]\n", name)

		keys := make([]string, 0, len(sections[name]))
		for key := range sections[name] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&buf, "%s = %v\n", key, sections[name][key])
		}
	}
	return buf.Bytes()
}

// setConfigSection puts the keys of the INI section named section into data
// under its dotted path, beside what the sections read before it put there,
// so that a nested section may come before its parent.
func setConfigSection(data map[string]any, section string, values map[string]any) {
	current := data
	for part := range strings.SplitSeq(section, ".") {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}
		current = next
	}
	maps.Copy(current, values)
}

// flattenConfigSections collects into sections, by dotted name, the keys of
// the section value, named prefix, and of the sections nested in it.
func flattenConfigSections(prefix string, value map[string]any, sections map[string]map[string]any) {
	keys := make(map[string]any)
	for key, val := range value {
		if child, ok := val.(map[string]any); ok {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			flattenConfigSections(name, child, sections)
			continue
		}
		keys[key] = val
	}
	if prefix != "" && len(keys) > 0 {
		sections[prefix] = keys
	}
}

func normalizeConfigFormat(format string) (configFileFormat, error) {
	switch strings.ToLower(strings.TrimPrefix(format, ".")) {
	case "ini":
		return configFormatINI, nil
	case "json":
		return configFormatJSON, nil
	case "toml":
		return configFormatTOML, nil
	case "yaml", "yml":
		return configFormatYAML, nil
	default:
		return "", errors.Newf("unsupported config format %q; supported formats: ini, json, toml, yaml", format)
	}
}

func configTagName(tag string) string {
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return ""
	}
	return name
}
