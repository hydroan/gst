package config

import (
	"github.com/cockroachdb/errors"
	"github.com/go-viper/encoding/ini"
	"github.com/spf13/viper"
)

// newViper returns a viper instance carrying the framework codec registry,
// so every consumer decodes configuration files identically.
//
// Breaking change:
// https://github.com/spf13/viper/blob/master/UPGRADE.md#breaking-hcl-java-properties-ini-removed-from-core
func newViper() (*viper.Viper, error) {
	codecRegistry := viper.NewCodecRegistry()
	if err := codecRegistry.RegisterCodec("ini", ini.Codec{}); err != nil {
		return nil, errors.Wrap(err, "failed to register ini codec")
	}
	return viper.NewWithOptions(viper.WithCodecRegistry(codecRegistry)), nil
}

// Load reads one configuration file into a fresh Config carrying the same
// framework defaults the runtime applies, without touching the global App
// and without binding environment variables. Consumers that inspect
// configuration files outside a running application (tooling, tests) use
// it to learn what a file means on its own: the result depends on framework
// defaults and file content alone, so two machines loading the same file
// always reach the same conclusion regardless of their environments.
func Load(file string) (*Config, error) {
	v, err := newViper()
	if err != nil {
		return nil, err
	}

	c := new(Config)
	c.setDefault(v)

	v.SetConfigFile(file)
	if err := v.ReadInConfig(); err != nil {
		return nil, errors.Wrapf(err, "failed to read config file %s", file)
	}
	if err := v.Unmarshal(c); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal config file %s", file)
	}
	return c, nil
}
