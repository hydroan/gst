package config

import "time"

type CacheType string

const (
	CacheBigCache  CacheType = "bigcache"
	CacheFreeCache CacheType = "freecache"
	CacheGoMap     CacheType = "map"
	CacheGolangLRU CacheType = "golang-lru"
)

const (
	CACHE_TYPE         = "CACHE_TYPE"         //nolint:staticcheck
	CACHE_SIZE_MB      = "CACHE_SIZE_MB"      //nolint:staticcheck
	CACHE_MAX_ENTRIES  = "CACHE_MAX_ENTRIES"  //nolint:staticcheck
	CACHE_SHARDS       = "CACHE_SHARDS"       //nolint:staticcheck
	CACHE_LIFE_WINDOW  = "CACHE_LIFE_WINDOW"  //nolint:staticcheck
	CACHE_CLEAN_WINDOW = "CACHE_CLEAN_WINDOW" //nolint:staticcheck
	CACHE_EXPIRATION   = "CACHE_EXPIRATION"   //nolint:staticcheck
	CACHE_CAPACITY     = "CACHE_CAPACITY"     //nolint:staticcheck
)

type Cache struct {
	Type        CacheType     `json:"type" mapstructure:"type" ini:"type" yaml:"type"`
	SizeMB      int           `json:"size_mb" mapstructure:"size_mb" ini:"size_mb" yaml:"size_mb"`                     // 总内存限制（MB）
	MaxEntries  int           `json:"max_entries" mapstructure:"max_entries" ini:"max_entries" yaml:"max_entries"`     // 最大缓存项数量
	Shards      int           `json:"shards" mapstructure:"shards" ini:"shards" yaml:"shards"`                         // 分片数量（仅部分缓存类型支持）
	LifeWindow  time.Duration `json:"life_window" mapstructure:"life_window" ini:"life_window" yaml:"life_window"`     // 单条数据存活时间
	CleanWindow time.Duration `json:"clean_window" mapstructure:"clean_window" ini:"clean_window" yaml:"clean_window"` // 清理过期数据的周期
	Expiration  time.Duration `json:"expiration" mapstructure:"expiration" ini:"expiration" yaml:"expiration"`
	Capacity    int           `json:"capacity" mapstructure:"capacity" ini:"capacity" yaml:"capacity"`
}

func (*Cache) setDefault() {
	cv.SetDefault("cache.type", CacheBigCache)
	cv.SetDefault("cache.size_mb", 128)        // 128MB
	cv.SetDefault("cache.max_entries", 100000) // 100,000
	cv.SetDefault("cache.shards", 16)          // 16 shards
	cv.SetDefault("cache.life_window", 10*time.Minute)
	cv.SetDefault("cache.clean_window", 5*time.Minute)
	cv.SetDefault("cache.expiration", 10*time.Minute)
	cv.SetDefault("cache.capacity", 100000) // 100,000
}
