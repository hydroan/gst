// Package testutil is the test support the public testutil package forwards
// to projects and the framework's own suites run on. Run prepares what a
// Server declares, the database and the redis, clickhouse, kafka, rocketmq,
// elasticsearch and minio a suite asks for, bootstraps the framework against
// them, serves for the duration of the tests and releases everything
// afterwards; the helpers beside it name the addresses the server listens at
// and assert on rows and responses.
package testutil
