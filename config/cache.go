package config

import "github.com/spf13/viper"

const (
	CACHE_MAX_ENTRIES = "CACHE_MAX_ENTRIES" //nolint:staticcheck
	CACHE_TOPIC       = "CACHE_TOPIC"       //nolint:staticcheck
)

type Cache struct {
	MaxEntries int    `json:"max_entries" mapstructure:"max_entries" ini:"max_entries" yaml:"max_entries"`
	Topic      string `json:"topic" mapstructure:"topic" ini:"topic" yaml:"topic"`
}

func (*Cache) setDefault(v *viper.Viper) {
	v.SetDefault("cache.max_entries", 0)
	v.SetDefault("cache.topic", "")
}
