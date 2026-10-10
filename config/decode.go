package config

import (
	"reflect"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// decodeHooks is how every configuration value is decoded, wherever it is
// read: the whole configuration, a registered section, and the check that an
// environment variable fits the key it carries. Viper's own defaults are
// repeated here because naming any hook replaces them, and two of the
// framework's own fields need one viper does not have: a time, and a map of
// strings.
func decodeHooks() viper.DecoderConfigOption {
	return viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		numberToDurationHookFunc(),
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		// RFC 3339 is the one time format the framework reads from outside,
		// the same as a URL filter bound: an offset is part of the value, so
		// the same text means the same instant on every machine.
		mapstructure.StringToTimeHookFunc(time.RFC3339),
		stringToStringMapHookFunc(),
	))
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
