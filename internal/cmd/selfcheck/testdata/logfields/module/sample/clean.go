package sample

import (
	"time"

	"example.com/module/internal/logfield"
	"example.com/module/util"
	"go.uber.org/zap"
)

type view struct{ Name string }

// clean writes nothing the check reports: a key of one type, a logfield
// constructor, an inlined duration, a reflected struct, a key only known at
// run time and a field handed to a key-value method.
func clean(key string, took time.Duration) {
	log.Infow("done", "name", "a", util.LogDuration(took), "view", view{Name: "x"})
	log.With("name", "b").Infow("again", logfield.Status(200), zap.String(key, "dynamic key"), zap.Reflect("view", view{}))
}
