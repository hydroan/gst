package config

const (
	DEBUG_STATSVIZ_ENABLE = "DEBUG_STATSVIZ_ENABLE" //nolint:staticcheck
	DEBUG_STATSVIZ_LISTEN = "DEBUG_STATSVIZ_LISTEN" //nolint:staticcheck
	DEBUG_STATSVIZ_PORT   = "DEBUG_STATSVIZ_PORT"   //nolint:staticcheck

	DEBUG_PPROF_ENABLE = "DEBUG_PPROF_ENABLE" //nolint:staticcheck
	DEBUG_PPROF_LISTEN = "DEBUG_PPROF_LISTEN" //nolint:staticcheck
	DEBUG_PPROF_PORT   = "DEBUG_PPROF_PORT"   //nolint:staticcheck

	DEBUG_GOPS_ENABLE = "DEBUG_GOPS_ENABLE" //nolint:staticcheck
	DEBUG_GOPS_LISTEN = "DEBUG_GOPS_LISTEN" //nolint:staticcheck
	DEBUG_GOPS_PORT   = "DEBUG_GOPS_PORT"   //nolint:staticcheck
)

type Debug struct {
	StatsvizEnable bool   `json:"statsviz_enable" mapstructure:"statsviz_enable" ini:"statsviz_enable" yaml:"statsviz_enable"`
	StatsvizListen string `json:"statsviz_listen" mapstructure:"statsviz_listen" ini:"statsviz_listen" yaml:"statsviz_listen"`
	StatsvizPort   int    `json:"statsviz_port" mapstructure:"statsviz_port" ini:"statsviz_port" yaml:"statsviz_port"`

	PprofEnable bool   `json:"pprof_enable" mapstructure:"pprof_enable" ini:"pprof_enable" yaml:"pprof_enable"`
	PprofListen string `json:"pprof_listen" mapstructure:"pprof_listen" ini:"pprof_listen" yaml:"pprof_listen"`
	PprofPort   int    `json:"pprof_port" mapstructure:"pprof_port" ini:"pprof_port" yaml:"pprof_port"`

	GopsEnable bool   `json:"gops_enable" mapstructure:"gops_enable" ini:"gops_enable" yaml:"gops_enable"`
	GopsListen string `json:"gops_listen" mapstructure:"gops_listen" ini:"gops_listen" yaml:"gops_listen"`
	GopsPort   int    `json:"gops_port" mapstructure:"gops_port" ini:"gops_port" yaml:"gops_port"`
}

func (*Debug) setDefault() {
	cv.SetDefault("debug.statsviz_enable", false)
	cv.SetDefault("debug.statsviz_listen", "127.0.0.1")
	cv.SetDefault("debug.statsviz_port", 10000)

	cv.SetDefault("debug.pprof_enable", false)
	cv.SetDefault("debug.pprof_listen", "127.0.0.1")
	cv.SetDefault("debug.gops_listen", "127.0.0.1")

	cv.SetDefault("debug.gops_enable", false)
	cv.SetDefault("debug.pprof_port", 10001)
	cv.SetDefault("debug.gops_port", 10002)
}
