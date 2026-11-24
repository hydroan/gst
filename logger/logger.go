// Package logger is where the code of a project logs. App returns the stream
// of the application's own entries, and each provider the framework ships
// has a stream of its own, Kafka's for one, where a project logs its use of
// the provider beside the provider's own entries. The streams are opened by
// Init, which the bootstrap runs: a function called before that returns nil.
//
// Fallback and New build a logger for a component of the project's own, and
// Init and Clean open and flush the streams where no bootstrap does, in a
// test for one.
//
// The implementation lives in internal/logger; this package forwards the part
// a project uses.
package logger

import (
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
)

type (
	// Logger is the logger New builds; it implements gst.Logger.
	Logger = logger.Logger

	// Option configures the encoder and the writer of a logger New builds:
	// DisableMsg, DisableLevel and DisableCaller leave the "msg", "level" and
	// "caller" fields out of its entries, and Console mirrors a file sink to
	// os.Stdout.
	Option = logger.Option
)

// App returns the general-purpose logger for a project's own code (model
// hooks, service methods, etc.) that wants WithContext-enriched logging
// without going through zap.S()/zap.L(). Unlike those globals, it never
// mirrors output to stdout.
func App() gst.Logger { return logger.App }

// Cassandra returns the stream of the cassandra provider, as Elastic and the
// functions after it return those of the other providers the framework
// ships: where a project logs its own use of a provider. Until the provider
// is compiled into the binary and its stage starts, the stream is a fallback
// writing to the global stream under the provider's name; from then on it is
// a stream of its own, a file of its own in file mode.
func Cassandra() gst.Logger { return logger.Cassandra }

// Elastic returns the stream of the elastic provider (see Cassandra).
func Elastic() gst.Logger { return logger.Elastic }

// Etcd returns the stream of the etcd provider (see Cassandra).
func Etcd() gst.Logger { return logger.Etcd }

// Influxdb returns the stream of the influxdb provider (see Cassandra).
func Influxdb() gst.Logger { return logger.Influxdb }

// Kafka returns the stream of the kafka provider (see Cassandra).
func Kafka() gst.Logger { return logger.Kafka }

// Ldap returns the stream of the ldap provider (see Cassandra).
func Ldap() gst.Logger { return logger.Ldap }

// Minio returns the stream of the minio provider (see Cassandra).
func Minio() gst.Logger { return logger.Minio }

// Mongo returns the stream of the mongo provider (see Cassandra).
func Mongo() gst.Logger { return logger.Mongo }

// Mqtt returns the stream of the mqtt provider (see Cassandra).
func Mqtt() gst.Logger { return logger.Mqtt }

// Nats returns the stream of the nats provider (see Cassandra).
func Nats() gst.Logger { return logger.Nats }

// RethinkDB returns the stream of the rethinkdb provider (see Cassandra).
func RethinkDB() gst.Logger { return logger.RethinkDB }

// RocketMQ returns the stream of the rocketmq provider (see Cassandra).
func RocketMQ() gst.Logger { return logger.RocketMQ }

// Scylla returns the stream of the scylla provider (see Cassandra).
func Scylla() gst.Logger { return logger.Scylla }

// Init builds every stream from the configuration. The bootstrap runs it; a
// test that logs without one runs it itself. It fails when the configured
// output is neither stdout nor file.
func Init() error { return logger.Init() }

// Clean flushes the loggers Init and New built and stops their buffered
// writers, so a process about to exit, or a test about to read a log back,
// sees everything that was logged.
func Clean() { logger.Clean() }

// Fallback builds a logger for the component named that owns no file and no
// sink: it derives from the global logger Init installs, so its entries land
// in the global stream tagged with the component's name. In a process that
// never ran Init the global logger is zap's no-op and the entries are
// dropped.
func Fallback(component string) gst.Logger { return logger.Fallback(component) }

// New builds a logger writing to a stream of its own: filename is the
// stream's log file, whose name without ".log" names the stream in stdout
// mode, and opts adjust its encoder.
func New(filename string, opts ...Option) *Logger { return logger.New(filename, opts...) }
