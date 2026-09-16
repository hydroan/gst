package dbmigrate

import (
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/middleware"
	"github.com/hydroan/gst/internal/router"
	"github.com/hydroan/gst/module"
)

// Prepare brings up what model registration runs through, in the order a
// service's startup brings it up: the configuration, then the middleware and
// router layers a module mounts its middleware and routes on, then the
// modules, whose registration it releases. A program that reads the
// registered models — the migration program gg migrate generates — calls it
// first, and reads them once module.Wait returns.
func Prepare() error {
	if err := config.Init(); err != nil {
		return err
	}
	if err := middleware.Init(); err != nil {
		return err
	}
	if err := router.Init(); err != nil {
		return err
	}
	return module.Init()
}
