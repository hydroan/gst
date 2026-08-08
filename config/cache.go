package config

import "github.com/spf13/viper"

// Cache configures the in-memory cache backends.
type Cache struct {
	// MaxEntries overrides the built-in per-type entry bound of the
	// entry-addressed in-memory backends (the byte-addressed backends size
	// themselves in bytes and ignore it). Zero or negative keeps each
	// backend's built-in default.
	MaxEntries int `json:"max_entries" mapstructure:"max_entries" ini:"max_entries" yaml:"max_entries"`
}

// MaxEntriesOr returns MaxEntries when it is positive and def otherwise. The
// fallback keeps cache construction safe even before the configuration is
// loaded.
func (c *Cache) MaxEntriesOr(def int) int {
	if c.MaxEntries > 0 {
		return c.MaxEntries
	}
	return def
}

func (*Cache) setDefault(v *viper.Viper) {
	v.SetDefault("cache.max_entries", 0)
}
