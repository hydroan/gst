package mysql_test

import (
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/hydroan/gst/config"
	gstmysql "github.com/hydroan/gst/database/mysql"
	"github.com/hydroan/gst/internal/testutil/testcontainer"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormschema "gorm.io/gorm/schema"
	"gorm.io/plugin/dbresolver"
)

// TestDryRunAnswersColumnTypesWithoutAServer pins what DryRun is for: a
// handle on the MySQL dialect that never connects and still answers the
// column type a field gets, longtext for a string field without a size.
func TestDryRunAnswersColumnTypesWithoutAServer(t *testing.T) {
	db, err := gstmysql.DryRun()
	require.NoError(t, err)
	require.True(t, db.DryRun)
	require.Equal(t, "mysql", db.Dialector.Name())

	type sample struct {
		Owner string
	}
	parsed, err := gormschema.Parse(&sample{}, &sync.Map{}, db.NamingStrategy)
	require.NoError(t, err)
	require.Equal(t, "longtext", db.Migrator().FullDataTypeOf(parsed.FieldsByName["Owner"]).SQL)
}

// TestNewReachesTheServerTheWayTheTLSKeysSay runs New against a server of
// its own, one whose certificate an authority of the test signed (see
// testcontainer.SetupStandaloneMySQL), and pins what the TLS keys of the
// configuration do to a connection: none while tls_enabled is off, whatever
// the other keys hold; encrypted, the server held to the CA bundle, once it
// is on, the replicas' connections like the primary's; refused when the
// bundle does not vouch for the server, unless the verification is skipped;
// and carrying the client certificate a server asks for.
func TestNewReachesTheServerTheWayTheTLSKeysSay(t *testing.T) {
	plain, terminate, err := testcontainer.SetupStandaloneMySQL("sample", "root", "sample-pass9")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, terminate()) })

	secure := plain
	secure.TLSEnabled = true

	t.Run("plaintext while tls is off", func(t *testing.T) {
		// The settings name a CA bundle and a client certificate all along:
		// they are read only once tls_enabled is on.
		require.Empty(t, sessionCipher(t, open(t, plain)))
	})

	t.Run("encrypted and verified once tls is on", func(t *testing.T) {
		require.NotEmpty(t, sessionCipher(t, open(t, secure)))
	})

	t.Run("a replica is reached the same way", func(t *testing.T) {
		// The replica is the same server under another of the names its
		// certificate carries, so the connection to it passes only if it is
		// verified by the host it dials rather than by the primary's.
		replicaHost := secure.Host
		switch secure.Host {
		case "localhost":
			replicaHost = "127.0.0.1"
		case "127.0.0.1":
			replicaHost = "localhost"
		}
		withReplica := secure
		withReplica.Replicas = []string{net.JoinHostPort(replicaHost, strconv.FormatUint(uint64(secure.Port), 10))}

		db := open(t, withReplica)
		require.NotEmpty(t, sessionCipher(t, db.Clauses(dbresolver.Read)))
	})

	t.Run("a server the bundle does not vouch for is refused", func(t *testing.T) {
		// The client certificate is a certificate the server's does not
		// chain to, which is all a wrong bundle is.
		wrongBundle := secure
		wrongBundle.CAFile = secure.CertFile
		_, err := gstmysql.New(wrongBundle)
		require.ErrorContains(t, err, "certificate signed by unknown authority")

		wrongBundle.InsecureSkipVerify = true
		require.NotEmpty(t, sessionCipher(t, open(t, wrongBundle)), "skipping the verification takes any certificate")
	})

	t.Run("the client certificate is shown to a server asking for one", func(t *testing.T) {
		admin := open(t, secure)
		require.NoError(t, admin.Exec("CREATE USER 'sample_x509'@'%' IDENTIFIED BY 'sample-pass9' REQUIRE X509").Error)
		require.NoError(t, admin.Exec("GRANT ALL ON `sample`.* TO 'sample_x509'@'%'").Error)

		withCertificate := secure
		withCertificate.Username = "sample_x509"
		require.NotEmpty(t, sessionCipher(t, open(t, withCertificate)))

		without := withCertificate
		without.CertFile, without.KeyFile = "", ""
		_, err := gstmysql.New(without)
		require.ErrorContains(t, err, "Access denied")
	})

	t.Run("a server taking encrypted connections alone", func(t *testing.T) {
		require.NoError(t, open(t, secure).Exec("SET GLOBAL require_secure_transport = ON").Error)

		_, err := gstmysql.New(plain)
		require.ErrorContains(t, err, "Error 3159")
		require.NotEmpty(t, sessionCipher(t, open(t, secure)))
	})
}

// open opens a handle on cfg that is closed when the test ends.
func open(t *testing.T, cfg config.MySQL) *gorm.DB {
	t.Helper()

	db, err := gstmysql.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		if pool, poolErr := db.DB(); poolErr == nil {
			_ = pool.Close()
		}
	})
	return db
}

// sessionCipher returns the cipher the connection db runs a statement on is
// encrypted with, as the server reports it: empty for a plaintext one.
func sessionCipher(t *testing.T, db *gorm.DB) string {
	t.Helper()

	var name, cipher string
	require.NoError(t, db.Raw("SHOW SESSION STATUS LIKE 'Ssl_cipher'").Row().Scan(&name, &cipher))
	return cipher
}
