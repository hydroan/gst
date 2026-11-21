package testcontainer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/cockroachdb/errors"
	// Registers the "mysql" driver the shared setup provisions databases with.
	_ "github.com/go-sql-driver/mysql"
	"github.com/hydroan/gst/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

const (
	mysqlImage = "mysql:8.4"
	mysqlPort  = "3306/tcp"

	// mysqlDatabase, mysqlUsername, mysqlRootUsername and mysqlPassword are
	// the credentials of the two setups: the dedicated setup runs as a plain
	// user on the database the container creates; the shared setup runs as
	// root, which is what lets it create and drop a database per test binary.
	// Both use the same password.
	mysqlDatabase     = "test"
	mysqlUsername     = "test"
	mysqlRootUsername = "root"
	mysqlPassword     = "test"

	// mysqlSharedMaxConnections raises the server default of 151, which many
	// binaries sharing one instance would exhaust.
	mysqlSharedMaxConnections = 500
)

// mysqlSharedArgs is the command line the shared mysql container is created
// with. It is the single source for both the arguments themselves and the
// fingerprint sharedContainerName folds into the container name.
//
// Past the connection limit it turns off the durability machinery a throwaway
// instance has no use for. Creating the tables of every registered model is
// what a test binary spends most of its startup on, and every one of those DDL
// statements otherwise flushes the binary log and the redo log, twice per
// statement; the flushes, not the schema work, dominate. A test container that
// crashes is recreated rather than recovered, so the durability being traded
// away buys nothing to begin with.
//
// performance_schema stays on: it costs nothing measurable here and the sys
// views built on it are what a human debugging a shared instance reaches for.
var mysqlSharedArgs = []string{
	fmt.Sprintf("--max-connections=%d", mysqlSharedMaxConnections),
	"--skip-log-bin",
	"--innodb-flush-log-at-trx-commit=0",
	"--innodb-doublewrite=0",
}

// mysqlSharedDialect provisions per-binary databases inside the shared mysql
// container. DROP DATABASE cuts off live connections on its own here, no
// force clause needed.
var mysqlSharedDialect = sharedSQLDialect{
	driver: "mysql",
	adminDSN: func(host string, port uint) string {
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/?timeout=10s&readTimeout=30s",
			mysqlRootUsername, mysqlPassword, host, port)
	},
	listDatabases:  "SELECT schema_name FROM information_schema.schemata",
	createDatabase: func(name string) string { return "CREATE DATABASE `" + name + "`" },
	dropDatabase:   func(name string) string { return "DROP DATABASE `" + name + "`" },
}

// setupMySQL prepares a mysql database and points the framework at it. The
// returned functions release it and publish its schema template, see
// prepareSchemaTemplate.
func setupMySQL() (func() error, func(), error) {
	if dedicatedContainersRequested() {
		// A container of its own is shared with nobody, so there is no
		// template to copy from or to leave behind.
		release, err := setupDedicatedMySQL()
		return release, noSchemaTemplatePublish, err
	}
	return setupSharedMySQL()
}

// setupDedicatedMySQL starts a mysql container of its own and points the
// framework at it. The returned function terminates that container.
func setupDedicatedMySQL() (func() error, error) {
	muteContainerLog()
	ctx := context.Background()

	c, err := mysql.Run(
		ctx, mysqlImage,
		mysql.WithDatabase(mysqlDatabase),
		mysql.WithUsername(mysqlUsername),
		mysql.WithPassword(mysqlPassword),
		patientStart(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start mysql container")
	}
	terminate := func() error { return c.Terminate(ctx) }

	host, port, err := endpoint(ctx, c, mysqlPort)
	if err != nil {
		return nil, errors.CombineErrors(err, terminate())
	}

	ApplyConfigToEnv(config.MySQL{
		Host:     host,
		Port:     port,
		Database: mysqlDatabase,
		Username: mysqlUsername,
		Password: mysqlPassword,
	})
	useDatabase(config.DBMySQL)
	reportServiceReady(string(config.DBMySQL), fmt.Sprintf("%s:%d/%s", host, port, mysqlDatabase))

	return terminate, nil
}

// The files the standalone mysql server reads its certificate from, inside
// the container, and the names the files a test dials it with go by, in a
// directory of the test's own.
const (
	mysqlTLSAuthorityPath  = "/etc/mysql/tls/ca.pem"
	mysqlTLSServerCertPath = "/etc/mysql/tls/server-cert.pem"
	mysqlTLSServerKeyPath  = "/etc/mysql/tls/server-key.pem"

	mysqlTLSAuthorityFile  = "ca.pem"
	mysqlTLSClientCertFile = "client-cert.pem"
	mysqlTLSClientKeyFile  = "client-key.pem"
)

// SetupStandaloneMySQL starts a mysql container of its own with the given
// database name and credentials, and returns its connection settings without
// touching the process environment or the framework's default database.
// Tests use it for topologies the single default database cannot express —
// a non-replicating stand-in replica for routing assertions, a second
// instance for cross-instance behavior, a server reached over TLS.
//
// The server runs with a certificate an authority made for it signed for the
// hosts it is reached at, and takes plaintext connections as before. The
// settings returned name the authority's certificate (CAFile) and a client
// certificate it signed (CertFile and KeyFile) and leave TLS off: a test of
// TLS turns TLSEnabled on in them, and every other test dials as it always
// did. The returned function terminates the container and removes those
// files.
func SetupStandaloneMySQL(database, username, password string) (config.MySQL, func() error, error) {
	muteContainerLog()
	ctx := context.Background()

	// The certificate names the host the container's ports are reached at,
	// which the daemon knows before any container exists.
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return config.MySQL{}, nil, errors.Wrap(err, "failed to reach the docker daemon")
	}
	daemonHost, err := provider.DaemonHost(ctx)
	if err = errors.CombineErrors(err, provider.Close()); err != nil {
		return config.MySQL{}, nil, errors.Wrap(err, "failed to resolve the docker daemon host")
	}
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if !slices.Contains(hosts, daemonHost) {
		hosts = append(hosts, daemonHost)
	}
	issued, err := issueCertificates(hosts...)
	if err != nil {
		return config.MySQL{}, nil, err
	}

	tlsDir, err := os.MkdirTemp("", "gst-mysql-tls-")
	if err != nil {
		return config.MySQL{}, nil, errors.Wrap(err, "failed to create the directory of the mysql tls files")
	}
	removeTLSFiles := func() error { return os.RemoveAll(tlsDir) }
	for name, content := range map[string][]byte{
		mysqlTLSAuthorityFile:  issued.authority,
		mysqlTLSClientCertFile: issued.clientCert,
		mysqlTLSClientKeyFile:  issued.clientKey,
	} {
		if err = os.WriteFile(filepath.Join(tlsDir, name), content, 0o600); err != nil {
			return config.MySQL{}, nil, errors.CombineErrors(errors.Wrap(err, "failed to write the mysql tls files"), removeTLSFiles())
		}
	}

	options := []testcontainers.ContainerCustomizer{
		mysql.WithDatabase(database),
		mysql.WithPassword(password),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{Reader: bytes.NewReader(issued.authority), ContainerFilePath: mysqlTLSAuthorityPath, FileMode: 0o644},
			testcontainers.ContainerFile{Reader: bytes.NewReader(issued.serverCert), ContainerFilePath: mysqlTLSServerCertPath, FileMode: 0o644},
			testcontainers.ContainerFile{Reader: bytes.NewReader(issued.serverKey), ContainerFilePath: mysqlTLSServerKeyPath, FileMode: 0o644},
		),
		testcontainers.WithCmdArgs("--ssl-ca="+mysqlTLSAuthorityPath, "--ssl-cert="+mysqlTLSServerCertPath, "--ssl-key="+mysqlTLSServerKeyPath),
		patientStart(),
	}
	// The mysql module refuses to provision root as a plain user; root works
	// through the root password alone.
	if username != mysqlRootUsername {
		options = append(options, mysql.WithUsername(username))
	}
	c, err := mysql.Run(ctx, mysqlImage, options...)
	if err != nil {
		return config.MySQL{}, nil, errors.CombineErrors(errors.Wrap(err, "failed to start standalone mysql container"), removeTLSFiles())
	}
	terminate := func() error { return errors.CombineErrors(c.Terminate(ctx), removeTLSFiles()) }

	host, port, err := endpoint(ctx, c, mysqlPort)
	if err != nil {
		return config.MySQL{}, nil, errors.CombineErrors(err, terminate())
	}

	cfg := config.MySQL{
		Host:     host,
		Port:     port,
		Database: database,
		Username: username,
		Password: password,
		CAFile:   filepath.Join(tlsDir, mysqlTLSAuthorityFile),
		CertFile: filepath.Join(tlsDir, mysqlTLSClientCertFile),
		KeyFile:  filepath.Join(tlsDir, mysqlTLSClientKeyFile),
		Enabled:  true,
	}
	reportServiceReady("mysql-standalone", fmt.Sprintf("%s:%d/%s", host, port, database))
	return cfg, terminate, nil
}

// setupSharedMySQL attaches to the shared mysql container, creating it when
// it is not running yet, and provisions a database of its own for this test
// binary, filled from the shared schema template where one matches. The first
// returned function drops that database and the second publishes the template,
// see prepareSchemaTemplate; the container stays either way.
func setupSharedMySQL() (func() error, func(), error) {
	muteContainerLog()
	ctx := context.Background()
	containerName := sharedContainerName(mysqlImage, mysqlSharedArgs...)

	var (
		host     string
		port     uint
		database string
		release  func() error
	)
	err := withSharedContainerLock(containerName, func() error {
		c, err := mysql.Run(
			ctx, mysqlImage,
			mysql.WithUsername(mysqlRootUsername),
			mysql.WithPassword(mysqlPassword),
			testcontainers.WithCmdArgs(mysqlSharedArgs...),
			patientStart(),
			reuseAcrossRuns(containerName),
		)
		if err != nil {
			return errors.Wrap(err, "failed to start the shared mysql container")
		}

		if host, port, err = endpoint(ctx, c, mysqlPort); err != nil {
			return err
		}

		database, release, err = provisionSharedDatabase(ctx, mysqlSharedDialect, host, port)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	ApplyConfigToEnv(config.MySQL{
		Host:     host,
		Port:     port,
		Database: database,
		Username: mysqlRootUsername,
		Password: mysqlPassword,
	})
	useDatabase(config.DBMySQL)
	reportServiceReady(string(config.DBMySQL), fmt.Sprintf("%s:%d/%s", host, port, database))

	return release, prepareSchemaTemplate(host, port, database), nil
}
