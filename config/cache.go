package config

import (
	"time"

	"github.com/spf13/viper"
)

const (
	CACHE_SHARDS       = "CACHE_SHARDS"       //nolint:staticcheck
	CACHE_LIFE_WINDOW  = "CACHE_LIFE_WINDOW"  //nolint:staticcheck
	CACHE_CLEAN_WINDOW = "CACHE_CLEAN_WINDOW" //nolint:staticcheck
	CACHE_EXPIRATION   = "CACHE_EXPIRATION"   //nolint:staticcheck
	CACHE_CAPACITY     = "CACHE_CAPACITY"     //nolint:staticcheck
)

type Cache struct {
	Shards      int           `json:"shards" mapstructure:"shards" ini:"shards" yaml:"shards"`                         // number of shards (supported by some cache types only)
	LifeWindow  time.Duration `json:"life_window" mapstructure:"life_window" ini:"life_window" yaml:"life_window"`     // lifetime of a single entry
	CleanWindow time.Duration `json:"clean_window" mapstructure:"clean_window" ini:"clean_window" yaml:"clean_window"` // interval between expired entry cleanups
	Expiration  time.Duration `json:"expiration" mapstructure:"expiration" ini:"expiration" yaml:"expiration"`
	Capacity    int           `json:"capacity" mapstructure:"capacity" ini:"capacity" yaml:"capacity"`
}

func (*Cache) setDefault(v *viper.Viper) {
	v.SetDefault("cache.shards", 16) // 16 shards
	v.SetDefault("cache.life_window", 10*time.Minute)
	v.SetDefault("cache.clean_window", 5*time.Minute)
	v.SetDefault("cache.expiration", 10*time.Minute)
	v.SetDefault("cache.capacity", 100000) // 100,000
}
