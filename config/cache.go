package config

import "github.com/spf13/viper"

const (
	CACHE_MAX_ENTRIES   = "CACHE_MAX_ENTRIES"   //nolint:staticcheck
	CACHE_TOPIC_SET_DEL = "CACHE_TOPIC_SET_DEL" //nolint:staticcheck
	CACHE_TOPIC_DONE    = "CACHE_TOPIC_DONE"    //nolint:staticcheck
)

type Cache struct {
	MaxEntries  int    `json:"max_entries" mapstructure:"max_entries" ini:"max_entries" yaml:"max_entries"`
	TopicSetDel string `json:"topic_set_del" mapstructure:"topic_set_del" ini:"topic_set_del" yaml:"topic_set_del"`
	TopicDone   string `json:"topic_done" mapstructure:"topic_done" ini:"topic_done" yaml:"topic_done"`
}

func (*Cache) setDefault(v *viper.Viper) {
	v.SetDefault("cache.max_entries", 0)
	v.SetDefault("cache.topic_set_del", "")
	v.SetDefault("cache.topic_done", "")
}
