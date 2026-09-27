package configx

// The environment variables the keys of the notice section resolve from:
// the section name and the key, upper case, joined by an underscore.
const (
	NOTICE_COUNT = "NOTICE_COUNT"
	NOTICE_EVENT = "NOTICE_EVENT"
)

// Notice configures the notice stream, the section [notice] of config.ini.
type Notice struct {
	// Count is how many events one stream sends before it ends.
	Count int `json:"count" mapstructure:"count" default:"3"`
	// Event is the event name the stream sends them under.
	Event string `json:"event" mapstructure:"event" default:"notice"`
}
