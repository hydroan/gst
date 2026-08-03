package testcontainer

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/hydroan/gst/config"
	"github.com/stoewer/go-strcase"
)

// configSections maps each section type of config.Config to the name the
// section is stored under. Those mapstructure tags are the only authority for
// the name: config.Init runs viper with AutomaticEnv and a "." to "_" key
// replacer, so an environment lookup is the upper-cased config path and
// nothing else. A type name cannot stand in for the tag, config.AppInfo lives
// under section "app".
var configSections = buildConfigSections()

func buildConfigSections() map[reflect.Type]string {
	typ := reflect.TypeFor[config.Config]()
	sections := make(map[reflect.Type]string, typ.NumField())
	for field := range typ.Fields() {
		if tag := field.Tag.Get("mapstructure"); len(tag) > 0 {
			sections[field.Type] = tag
		}
	}
	return sections
}

// applyConfigToEnv exports the non-zero fields of a config section as the
// environment variables config.Init reads them back from, so a test hands over
// a config struct instead of spelling out every variable name.
//
// A variable name is the upper-cased config path: the section name followed by
// one mapstructure tag per nesting level, joined by "_". The nested section
// config.Logger.HTTPBody therefore lands on LOGGER_HTTP_BODY_ENABLED.
//
// Zero-valued fields are skipped so a partially filled section leaves the
// remaining framework defaults alone, which also means a false bool cannot be
// exported this way. Fields with no single environment representation, such as
// slices and maps, are skipped as well.
func applyConfigToEnv(cfg any) {
	val := reflect.ValueOf(cfg)
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return
	}

	applyStructToEnv(strings.ToUpper(sectionName(val.Type())), val)
}

// sectionName returns the config section a type is stored under.
func sectionName(typ reflect.Type) string {
	if name, ok := configSections[typ]; ok {
		return name
	}
	// A section registered through config.Register is named after its type.
	return strings.ToLower(strcase.SnakeCase(typ.Name()))
}

// applyStructToEnv exports every field of val, prefixing it with the
// upper-cased config path walked so far.
func applyStructToEnv(path string, val reflect.Value) {
	typ := val.Type()
	for i := range typ.NumField() {
		fieldTyp := typ.Field(i)
		fieldVal := val.Field(i)

		name := fieldTyp.Tag.Get("mapstructure")
		if len(name) == 0 {
			continue
		}
		fieldPath := path + "_" + strings.ToUpper(name)

		for fieldVal.Kind() == reflect.Pointer {
			if fieldVal.IsNil() {
				break
			}
			fieldVal = fieldVal.Elem()
		}

		switch fieldVal.Kind() {
		case reflect.Struct:
			applyStructToEnv(fieldPath, fieldVal)
		case reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64,
			reflect.String:
			if fieldVal.IsZero() {
				continue
			}
			// A time.Duration formats as the duration string viper parses back.
			os.Setenv(fieldPath, fmt.Sprintf("%v", fieldVal.Interface()))
		}
	}
}
