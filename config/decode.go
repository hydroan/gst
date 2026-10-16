package config

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// unmarshalConfig decodes the configuration v holds into c, the framework's
// own, and refuses a key under one of the framework's sections that no field
// of the section reads: a misspelled key, such as leve under logger, would
// otherwise leave the value it meant to set at its default with nothing
// reporting it. A key outside the framework's sections passes: it is under a
// section a project registers, before Init or after it, whose own decode
// reads it. The decoder lists the keys it leaves unused in its metadata; its
// ErrorUnused option would refuse those sections too.
func unmarshalConfig(v *viper.Viper, c *Config) error {
	var md mapstructure.Metadata
	if err := v.Unmarshal(c, decodeHooks(), func(dc *mapstructure.DecoderConfig) { dc.Metadata = &md }); err != nil {
		return err
	}
	var unknown []string
	for _, key := range md.Unused {
		if section, rest, ok := strings.Cut(key, "."); ok && builtinSections()[section] {
			unknown = append(unknown, fmt.Sprintf("section %q has no key %q", section, rest))
		}
	}
	if len(unknown) == 0 {
		return c.validate()
	}
	slices.Sort(unknown)
	return errors.New(strings.Join(unknown, "; "))
}

// decodeHooks is how every configuration value is decoded, wherever it is
// read: the whole configuration, a registered section, and the check that an
// environment variable fits the key it carries. Viper's own defaults are
// repeated here because naming any hook replaces them; two of the
// framework's own fields need one viper does not have, a time and a map of
// strings; and a type that spells out its own values, one implementing
// encoding.TextUnmarshaler, reads its text itself and refuses text it does
// not know — the closed-set types of the framework's sections, DBType or
// LoggerOutput, and any such type a registered section declares.
func decodeHooks() viper.DecoderConfigOption {
	return viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		negativeToUnsignedHookFunc(),
		numberToDurationHookFunc(),
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		// RFC 3339 is the one time format the framework reads from outside,
		// the same as a URL filter bound: an offset is part of the value, so
		// the same text means the same instant on every machine.
		mapstructure.StringToTimeHookFunc(time.RFC3339),
		stringToStringMapHookFunc(),
		// Last, once the hooks above have converted a duration, a slice and
		// a time, so only text no earlier hook claimed reaches a type that
		// reads it itself.
		mapstructure.TextUnmarshallerHookFunc(),
	))
}

// negativeToUnsignedHookFunc refuses a negative number for a key whose value
// cannot be one: a port, a capacity, a size. A format with numbers of its
// own, YAML, JSON or TOML, hands -1 over as a number, and the lenient decode
// viper runs would read it into an unsigned field by wrapping it around: a
// port of -1 becomes 18446744073709551615, and nothing reports it. A string,
// from an INI file or the environment, the decoder refuses already.
func negativeToUnsignedHookFunc() mapstructure.DecodeHookFuncType {
	return func(from, to reflect.Type, data any) (any, error) {
		switch to.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		default:
			return data, nil
		}
		negative := false
		switch from.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			negative = reflect.ValueOf(data).Int() < 0
		case reflect.Float32, reflect.Float64:
			negative = reflect.ValueOf(data).Float() < 0
		}
		if negative {
			return nil, errors.Newf("want a number of zero or more, got %v", data)
		}
		return data, nil
	}
}

// numberToDurationHookFunc refuses a duration written as a bare number. A
// format with numbers of its own, YAML, JSON or TOML, hands one over as a
// number, which the decoder would otherwise read as nanoseconds: a read
// timeout of 15 is fifteen nanoseconds and a retention of 2160 meant in hours
// is gone in an instant, and nothing reports it. A number needs its unit, 15s
// or 2160h, the spelling a string is held to already (time.ParseDuration), so
// an INI file and an environment variable refuse the same mistake on their
// own. Zero alone carries no unit, as it carries none in time.ParseDuration.
func numberToDurationHookFunc() mapstructure.DecodeHookFuncType {
	return func(from, to reflect.Type, data any) (any, error) {
		// A duration that already is one, a default or a value decoded
		// earlier, is an int64 by kind and carries its unit by type.
		if to != reflect.TypeFor[time.Duration]() || from == reflect.TypeFor[time.Duration]() {
			return data, nil
		}
		switch from.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			if reflect.ValueOf(data).IsZero() {
				return time.Duration(0), nil
			}
			return nil, errors.Newf("want a duration with a unit, such as %vs, got %v", data, data)
		}
		return data, nil
	}
}

// stringToStringMapHookFunc decodes "key=value,key=value" into a map of
// strings, the one spelling a single environment variable can carry. A file
// writes the map as the format's own mapping and never reaches this hook.
func stringToStringMapHookFunc() mapstructure.DecodeHookFuncType {
	return func(from, to reflect.Type, data any) (any, error) {
		if from.Kind() != reflect.String || to.Kind() != reflect.Map {
			return data, nil
		}
		if to.Key().Kind() != reflect.String || to.Elem().Kind() != reflect.String {
			return data, nil
		}
		raw, _ := data.(string)
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return map[string]string{}, nil
		}
		pairs := map[string]string{}
		for pair := range strings.SplitSeq(raw, ",") {
			key, value, ok := strings.Cut(pair, "=")
			if !ok {
				return nil, errors.Newf("want key=value pairs separated by commas, got %q", pair)
			}
			pairs[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
		return pairs, nil
	}
}

// oneOf reads text as one of set, the spellings a configuration type admits,
// and returns the spelling as the type writes it: surrounding space and
// letter case belong to the writer, so " MySQL " reads as mysql. Empty text
// is the zero value, the "not set" each consumer of such a type reads as its
// default. Any other text is an error naming the set; the decoder adds the
// key it was under.
func oneOf[T ~string](text []byte, set ...T) (T, error) {
	value := T(strings.ToLower(strings.TrimSpace(string(text))))
	if value == "" || slices.Contains(set, value) {
		return value, nil
	}
	names := make([]string, len(set))
	for i, s := range set {
		names[i] = string(s)
	}
	return "", errors.Newf("want one of %s, got %q", strings.Join(names, ", "), text)
}
