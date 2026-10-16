package config

import "github.com/spf13/viper"

const (
	CACHE_MAX_ENTRIES = "CACHE_MAX_ENTRIES"
	CACHE_TOPIC       = "CACHE_TOPIC"
)

// maxCacheEntries is the most cache.max_entries may ask for. The cache
// backends size internal tables from the setting, one allocating ten
// admission counters per entry, so a typo past it would be an out-of-memory
// crash at startup, and a large enough value would overflow the 32-bit
// capacity some backends take.
const maxCacheEntries = 10_000_000

type Cache struct {
	// MaxEntries bounds how many entries each cached type keeps; 0 or unset
	// is 100,000. A negative value, or one over 10,000,000, fails startup.
	// Default: 0
	MaxEntries int    `json:"max_entries" mapstructure:"max_entries" ini:"max_entries" yaml:"max_entries"`
	Topic      string `json:"topic" mapstructure:"topic" ini:"topic" yaml:"topic"`
}

func (*Cache) setDefault(v *viper.Viper) {
	v.SetDefault("cache.max_entries", 0)
	v.SetDefault("cache.topic", "")
}
