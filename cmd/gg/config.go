package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

type configDefaultsOptions struct {
	format string
	output string
	force  bool
}

type configConvertOptions struct {
	from   string
	to     string
	output string
	force  bool
}

var configCmd = newConfigCmd()

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage gst configuration files",
		Long: `Manage gst configuration files.

Use this command to inspect framework default configuration and convert
configuration files between INI, JSON, TOML, and YAML formats.`,
	}

	cmd.AddCommand(
		newConfigListCmd(),
		newConfigDefaultsCmd(),
		newConfigConvertCmd(),
	)
	return cmd
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List framework configuration sections",
		Long:  "List the top-level framework configuration sections supported by gst.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, name := range configSectionNames() {
				fmt.Fprintln(cmd.OutOrStdout(), name)
			}
			return nil
		},
	}
}

func newConfigDefaultsCmd() *cobra.Command {
	opts := &configDefaultsOptions{format: string(configFormatINI)}
	cmd := &cobra.Command{
		Use:   "defaults [section]",
		Short: "Print framework default configuration",
		Long: `Print framework default configuration.

When section is provided, only that top-level configuration section is printed.
The default output format is INI, matching the default gst config file format.`,
		Example: `  gg config defaults
  gg config defaults server --format yaml
  gg config defaults server --format toml
  gg config defaults redis --format json --output redis.json
  gg config defaults --format yaml --output config.yaml --force`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigDefaults(cmd, args, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.format, "format", "f", string(configFormatINI), "output format: ini, json, toml, yaml")
	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "write output to file instead of stdout")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite output file if it exists")
	return cmd
}

func newConfigConvertCmd() *cobra.Command {
	opts := new(configConvertOptions)
	cmd := &cobra.Command{
		Use:   "convert <input> [output]",
		Short: "Convert configuration files between formats",
		Long: `Convert configuration files between INI, JSON, TOML, and YAML formats.

Input and output formats are inferred from file extensions unless --from or --to
is provided. If no output file is provided, --to is required and converted
content is written to stdout.`,
		Example: `  gg config convert config.ini config.yaml
  gg config convert config.yaml config.json
  gg config convert config.json config.toml
  gg config convert config.json config.ini --force
  gg config convert config.ini --to yaml`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigConvert(cmd, args, opts)
		},
	}

	cmd.Flags().StringVar(&opts.from, "from", "", "input format override: ini, json, toml, yaml")
	cmd.Flags().StringVar(&opts.to, "to", "", "output format override: ini, json, toml, yaml")
	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "write output to file instead of stdout")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite output file if it exists")
	return cmd
}

func runConfigDefaults(cmd *cobra.Command, args []string, opts *configDefaultsOptions) error {
	format, err := normalizeConfigFormat(opts.format)
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
			return fmt.Errorf("unknown configuration section %q; run 'gg config list' to see available sections", args[0])
		}
		data = map[string]any{section: value}
	}

	content, err := encodeConfigData(data, format)
	if err != nil {
		return err
	}
	return writeConfigOutput(cmd.OutOrStdout(), opts.output, opts.force, content)
}

func runConfigConvert(cmd *cobra.Command, args []string, opts *configConvertOptions) error {
	input := args[0]
	output := opts.output
	if len(args) == 2 {
		if output != "" {
			return errors.New("output file specified twice; use either positional output or --output")
		}
		output = args[1]
	}

	from, err := resolveInputFormat(input, opts.from)
	if err != nil {
		return err
	}
	to, err := resolveOutputFormat(output, opts.to)
	if err != nil {
		return err
	}

	raw, err := os.ReadFile(input)
	if err != nil {
		return errors.Wrapf(err, "failed to read input file %s", input)
	}
	data, err := decodeConfigData(raw, from)
	if err != nil {
		return errors.Wrapf(err, "failed to decode %s as %s", input, from)
	}
	content, err := encodeConfigData(data, to)
	if err != nil {
		return err
	}
	return writeConfigOutput(cmd.OutOrStdout(), output, opts.force, content)
}

func configSectionNames() []string {
	configType := reflect.TypeFor[config.Config]()
	names := make([]string, 0, configType.NumField())
	for field := range configType.Fields() {
		name := configTagName(field.Tag.Get("mapstructure"))
		if name == "" {
			name = configTagName(field.Tag.Get("json"))
		}
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func defaultConfigData() (map[string]any, error) {
	var content bytes.Buffer
	err := withCleanConfigEnvironment(func() error {
		return inTemporaryDirectory(func() error {
			return withStdoutDiscarded(func() error {
				if err := config.Init(); err != nil {
					return errors.Wrap(err, "failed to initialize default config")
				}
				defer config.Clean()
				if err := config.Save(&content); err != nil {
					return errors.Wrap(err, "failed to collect default config")
				}
				return nil
			})
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

func withStdoutDiscarded(fn func() error) error {
	oldStdout := os.Stdout
	null, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer null.Close()
	os.Stdout = null
	defer func() {
		os.Stdout = oldStdout
	}()
	return fn()
}

func decodeConfigData(content []byte, format configFileFormat) (map[string]any, error) {
	switch format {
	case configFormatINI:
		return decodeINIConfig(content)
	case configFormatJSON:
		var data map[string]any
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.UseNumber()
		if err := decoder.Decode(&data); err != nil {
			return nil, err
		}
		return data, nil
	case configFormatTOML:
		var data map[string]any
		if err := toml.Unmarshal(content, &data); err != nil {
			return nil, err
		}
		return data, nil
	case configFormatYAML:
		var data map[string]any
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, err
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported config format %q", format)
	}
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
		return nil, fmt.Errorf("unsupported config format %q", format)
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
			fmt.Fprintf(&buf, "%s = %s\n", key, configScalarString(sections[name][key]))
		}
	}
	return buf.Bytes()
}

func setConfigSection(data map[string]any, section string, values map[string]any) {
	parts := strings.Split(section, ".")
	current := data
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = values
}

func flattenConfigSections(prefix string, value map[string]any, sections map[string]map[string]any) {
	scalars := make(map[string]any)
	for key, val := range value {
		if child, ok := asConfigMap(val); ok {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			flattenConfigSections(name, child, sections)
			continue
		}
		scalars[key] = val
	}
	if prefix != "" && len(scalars) > 0 {
		sections[prefix] = scalars
	}
}

func asConfigMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

func configScalarString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case json.Number:
		return typed.String()
	case fmt.Stringer:
		return typed.String()
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, configScalarString(item))
		}
		return strings.Join(values, ",")
	default:
		return fmt.Sprint(typed)
	}
}

func writeConfigOutput(stdout io.Writer, output string, force bool, content []byte) error {
	if output == "" {
		_, err := stdout.Write(content)
		return err
	}
	if _, err := os.Stat(output); err == nil && !force {
		return fmt.Errorf("output file %s already exists; use --force to overwrite it", output)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "failed to inspect output file %s", output)
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return errors.Wrapf(err, "failed to create output directory for %s", output)
	}
	// The output path is explicitly provided by the CLI caller.
	return os.WriteFile(output, content, 0o600) //nolint:gosec
}

func resolveInputFormat(input, override string) (configFileFormat, error) {
	if override != "" {
		return normalizeConfigFormat(override)
	}
	return inferConfigFormat(input)
}

func resolveOutputFormat(output, override string) (configFileFormat, error) {
	if override != "" {
		return normalizeConfigFormat(override)
	}
	if output == "" {
		return "", errors.New("output file or --to format is required")
	}
	return inferConfigFormat(output)
}

func inferConfigFormat(filename string) (configFileFormat, error) {
	ext := strings.TrimPrefix(filepath.Ext(filename), ".")
	if ext == "" {
		return "", fmt.Errorf("cannot infer config format from %s; use --from or --to", filename)
	}
	return normalizeConfigFormat(ext)
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
		return "", fmt.Errorf("unsupported config format %q; supported formats: ini, json, toml, yaml", format)
	}
}

func configTagName(tag string) string {
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return ""
	}
	return name
}

// GGConfig is the local configuration used by gg commands.
type GGConfig struct {
	Prune PruneConfig `mapstructure:"prune" yaml:"prune"`
}

// PruneConfig contains service pruning options for gg.
type PruneConfig struct {
	Ignore []string `mapstructure:"ignore" yaml:"ignore"`
}

var ggConfig *GGConfig

// loadGGConfig reads .gg.yaml from the current project directory.
func loadGGConfig() (*GGConfig, error) {
	if ggConfig != nil {
		return ggConfig, nil
	}

	v := viper.New()
	v.SetConfigName(".gg")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			ggConfig = &GGConfig{
				Prune: PruneConfig{
					Ignore: []string{},
				},
			}
			return ggConfig, nil
		}
		return nil, errors.Wrap(err, "failed to read config file")
	}

	cfg := new(GGConfig)
	if err := v.Unmarshal(cfg); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	ggConfig = cfg
	return ggConfig, nil
}

// getPruneIgnorePatterns returns service files ignored by gg prune.
func getPruneIgnorePatterns() []string {
	cfg, err := loadGGConfig()
	if err != nil {
		return []string{}
	}
	return cfg.Prune.Ignore
}
