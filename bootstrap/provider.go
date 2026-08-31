package bootstrap

import (
	"slices"

	"github.com/hydroan/gst/config"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/provider"
	"go.uber.org/zap"
)

// optionalProviders maps every optional provider package under provider/ to
// the configuration switch that enables it. Bootstrap consults the table for
// exactly one purpose: warning when configuration enables a provider the
// binary does not contain. A package that is not imported never registers,
// so this cannot be read off the registry — the table is the one piece of
// always-compiled code that knows where each provider's switch lives. The
// compiled-in side never reads it: whether a registered provider runs is
// decided by the Enabled function it registered (see drainProviders).
// Keys are provider package names, which are also the names the packages
// register under; the consistency test in this package pins the key set to
// the provider/ directory listing.
var optionalProviders = map[string]func() bool{
	"cassandra": func() bool { return config.App.Cassandra.Enabled },
	// The clickhouse section also feeds the gorm dialect. The provider is
	// only expected when ClickHouse serves as a secondary analytical store;
	// when database.type selects it as the primary database, the dialect
	// alone is a complete setup and deserves no warning.
	"clickhouse": func() bool {
		return config.App.Clickhouse.Enabled && config.App.Database.Type != config.DBClickHouse
	},
	"elastic":   func() bool { return config.App.Elasticsearch.Enabled },
	"etcd":      func() bool { return config.App.Etcd.Enabled },
	"influxdb":  func() bool { return config.App.Influxdb.Enabled },
	"kafka":     func() bool { return config.App.Kafka.Enabled },
	"ldap":      func() bool { return config.App.Ldap.Enabled },
	"minio":     func() bool { return config.App.Minio.Enabled },
	"mongo":     func() bool { return config.App.Mongo.Enabled },
	"mqtt":      func() bool { return config.App.Mqtt.Enabled },
	"nats":      func() bool { return config.App.Nats.Enabled },
	"rethinkdb": func() bool { return config.App.RethinkDB.Enabled },
	"rocketmq":  func() bool { return config.App.RocketMQ.Enabled },
	"scylla":    func() bool { return config.App.Scylla.Enabled },
}

// drainProviders hands every registered provider to the initializer, wires
// its Close into shutdown cleanup, assigns each declared logger handle its
// dedicated logger, and seals the registry so a later Register fails fast
// instead of being silently skipped. A provider whose Enabled reports false
// is left out of the lifecycle entirely — no Init, no Close — which is what
// makes "disabled means no-op" a bootstrap guarantee instead of a discipline
// every Init repeats. It then warns about providers that configuration
// enables but the binary does not contain. Bootstrap calls it after the core
// phase, so configuration and logging are ready.
func drainProviders() {
	drained := provider.Registered()
	provider.Seal()

	registered := make(map[string]bool, len(drained))
	for _, p := range drained {
		registered[p.Name] = true
		// The registry name decides the file name, and the assignment lands
		// before Init is even queued: a compiled-in provider always logs to
		// its own file, while the loggers of providers never imported keep
		// the fallback the logging package installed. Disabled providers keep
		// the binding too — it costs nothing until written to, and code
		// logging through the handle while the provider is off should still
		// land in the provider's own file.
		if p.Logger != nil {
			*p.Logger = pkgzap.New(p.Name + ".log")
		}
		if p.Enabled != nil && !p.Enabled() {
			continue
		}
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
