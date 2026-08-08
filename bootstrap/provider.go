package bootstrap

import (
	"slices"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider"
	"go.uber.org/zap"
)

// optionalProviders maps every optional provider package under provider/ to
// the configuration switch that enables it. Bootstrap consults the table to
// warn when configuration enables a provider the binary does not contain.
// Keys are provider package names, which are also the names the packages
// register under.
var optionalProviders = map[string]func() bool{
	"cassandra": func() bool { return config.App.Cassandra.Enabled },
	"elastic":   func() bool { return config.App.Elasticsearch.Enabled },
	"etcd":      func() bool { return config.App.Etcd.Enabled },
	"influxdb":  func() bool { return config.App.Influxdb.Enabled },
	"kafka":     func() bool { return config.App.Kafka.Enabled },
	"ldap":      func() bool { return config.App.Ldap.Enabled },
	"memcached": func() bool { return config.App.Memcached.Enabled },
	"minio":     func() bool { return config.App.Minio.Enabled },
	"mongo":     func() bool { return config.App.Mongo.Enabled },
	"mqtt":      func() bool { return config.App.Mqtt.Enabled },
	"nats":      func() bool { return config.App.Nats.Enabled },
	"rethinkdb": func() bool { return config.App.RethinkDB.Enabled },
	"rocketmq":  func() bool { return config.App.RocketMQ.Enabled },
	"scylla":    func() bool { return config.App.Scylla.Enabled },
}

// drainProviders hands every registered provider to the initializer, wires
// its Close into shutdown cleanup, and seals the registry so a later
// Register fails fast instead of being silently skipped. It then warns
// about providers that configuration enables but the binary does not
// contain. Bootstrap calls it after the core phase, so configuration and
// logging are ready.
func drainProviders() {
	drained := provider.Registered()
	provider.Seal()

	registered := make(map[string]bool, len(drained))
	for _, p := range drained {
		registered[p.Name] = true
		ins.Register(p.Init)
		if p.Close != nil {
			registerCleanup(closeComponent(p.Name, p.Close))
		}
	}

	for _, name := range missingProviders(registered) {
		zap.S().Warnw("optional provider enabled in configuration but not compiled into this binary; its configuration is ignored",
			"provider", name, "import", "github.com/hydroan/gst/provider/"+name)
	}
}

// missingProviders returns the names of optional providers that
// configuration enables but registered does not contain, sorted by name.
func missingProviders(registered map[string]bool) []string {
	var missing []string
	for name, enabled := range optionalProviders {
		if enabled() && !registered[name] {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	return missing
}

// closeComponent adapts a component Close to a cleanup handler, logging the
// returned error centrally so shutdown always continues and individual
// components do not implement their own logging.
func closeComponent(name string, closeFn func() error) func() {
	return func() {
		if err := closeFn(); err != nil {
			zap.S().Errorw("failed to close component", "component", name, "err", err)
		}
	}
}
