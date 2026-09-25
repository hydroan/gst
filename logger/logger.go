// Package logger provides global logger used by server, client and cli.
package logger

import (
	"github.com/hydroan/gst/internal/types"
	"go.uber.org/zap"
	gorml "gorm.io/gorm/logger"
)

var (
	// App is the general-purpose business logger for application code (model
	// hooks, service methods, etc.) that wants WithContext-enriched logging
	// without going through zap.S()/zap.L(). Unlike those globals, App never
	// mirrors output to stdout.
	App types.Logger

	Controller types.Logger
	Service    types.Logger
	Database   types.Logger
	Cache      types.Logger
	Dcache     types.Logger
	Redis      types.Logger

	Authz     types.Logger
	OTEL      types.Logger
	Cassandra types.Logger
	Elastic   types.Logger
	Etcd      types.Logger
	Influxdb  types.Logger
	Kafka     types.Logger
	Ldap      types.Logger
	Minio     types.Logger
	Mongo     types.Logger
	Mqtt      types.Logger
	Nats      types.Logger
	RethinkDB types.Logger
	RocketMQ  types.Logger
	Scylla    types.Logger

	// Gin is the access log of the HTTP listener and GRPC that of the gRPC
	// listener, one entry per request or call; HTTPBody carries the request
	// and response bodies the HTTP listener logs.
	Gin      *zap.Logger
	HTTPBody *zap.Logger
	GRPC     *zap.Logger
	// Recovery records the panics the handlers of both listeners recover
	// from, with their stacks. One logger serves both so the file has one
	// writer: a second one on the same path would race it at rotation.
	Recovery *zap.Logger
	Gorm     gorml.Interface
)
