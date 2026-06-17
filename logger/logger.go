// Package logger provides global logger used by server, client and cli.
package logger

import (
	"github.com/casbin/casbin/v3/log"
	"github.com/hydroan/gst/types"
	"go.uber.org/zap"
	gorml "gorm.io/gorm/logger"
)

var (
	Runtime types.Logger
	Cronjob types.Logger
	Task    types.Logger

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
	Feishu    types.Logger
	Influxdb  types.Logger
	Kafka     types.Logger
	Ldap      types.Logger
	Memcached types.Logger
	Minio     types.Logger
	Mongo     types.Logger
	Mqtt      types.Logger
	Nats      types.Logger
	RethinkDB types.Logger
	RocketMQ  types.Logger
	Scylla    types.Logger

	Protocol types.Logger
	Binary   types.Logger

	Gin    *zap.Logger
	Gorm   gorml.Interface
	Casbin log.Logger
)
