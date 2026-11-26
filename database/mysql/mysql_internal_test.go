package mysql

import (
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/dbruntime/dberror"
	"github.com/stretchr/testify/require"
)

// TestBuildDSNAddsOnlyWhatTheConfigurationAsksFor pins the DSN of a
// configuration: the fixed parameters every connection carries, then the
// strict mode, the timeouts and the TLS configuration, each only where the
// configuration asks for it.
func TestBuildDSNAddsOnlyWhatTheConfigurationAsksFor(t *testing.T) {
	base := config.MySQL{Host: "127.0.0.1", Port: 3306, Database: "sample", Username: "root", Password: "secret", Strict: true}
	plain := "root:secret@tcp(127.0.0.1:3306)/sample?charset=utf8mb4&parseTime=True&loc=UTC&clientFoundRows=true&interpolateParams=true"
	prefix := plain + "&sql_mode=CONCAT(@@sql_mode,%27,STRICT_TRANS_TABLES%27)"

	t.Run("without timeouts", func(t *testing.T) {
		require.Equal(t, prefix, buildDSN(base, ""))
	})

	t.Run("strict off", func(t *testing.T) {
		cfg := base
		cfg.Strict = false
		require.Equal(t, plain, buildDSN(cfg, ""))
	})

	t.Run("dial timeout", func(t *testing.T) {
		cfg := base
		cfg.DialTimeout = 10 * time.Second
		require.Equal(t, prefix+"&timeout=10s", buildDSN(cfg, ""))
	})

	t.Run("all timeouts", func(t *testing.T) {
		cfg := base
		cfg.DialTimeout = 5 * time.Second
		cfg.ReadTimeout = 30 * time.Second
		cfg.WriteTimeout = time.Minute
		require.Equal(t, prefix+"&timeout=5s&readTimeout=30s&writeTimeout=1m0s", buildDSN(cfg, ""))
	})

	t.Run("a registered tls configuration", func(t *testing.T) {
		require.Equal(t, prefix+"&tls=gst-7", buildDSN(base, "gst-7"))
	})

	t.Run("tls settings of a section that leaves tls off", func(t *testing.T) {
		// The keys beside tls_enabled are read only while it is on: the DSN
		// of a section that leaves it off is the one it always was.
		cfg := base
		cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.InsecureSkipVerify = "client-cert.pem", "client-key.pem", "ca.pem", true
		name, err := registerTLS(cfg)
		require.NoError(t, err)
		require.Empty(t, name)
		require.Equal(t, prefix, buildDSN(cfg, name))
	})
}

// TestRegisterTLSNamesAConfigurationOfItsOwnPerHandle pins what a section
// turning TLS on registers with the driver: a configuration under a name of
// its own each time, which the DSN of every endpoint of the handle selects,
// and which the driver hands each endpoint a copy of, held to the host that
// endpoint dials, so a primary and a replica, hosts apart, are each verified
// by their own name.
func TestRegisterTLSNamesAConfigurationOfItsOwnPerHandle(t *testing.T) {
	cfg := config.MySQL{Host: "primary.sample.test", Port: 3306, Database: "sample", Username: "root", TLSEnabled: true}

	first, err := registerTLS(cfg)
	require.NoError(t, err)
	second, err := registerTLS(cfg)
	require.NoError(t, err)
	require.NotEmpty(t, first)
	require.NotEqual(t, first, second, "every handle registers under a name of its own")

	primary, err := mysql.ParseDSN(buildDSN(cfg, first))
	require.NoError(t, err)
	require.NotNil(t, primary.TLS)
	require.False(t, primary.TLS.InsecureSkipVerify)
	require.Equal(t, "primary.sample.test", primary.TLS.ServerName)

	replicaCfg := cfg
	replicaCfg.Host = "replica.sample.test"
	replica, err := mysql.ParseDSN(buildDSN(replicaCfg, first))
	require.NoError(t, err)
	require.Equal(t, "replica.sample.test", replica.TLS.ServerName)

	skipping := cfg
	skipping.InsecureSkipVerify = true
	name, err := registerTLS(skipping)
	require.NoError(t, err)
	parsed, err := mysql.ParseDSN(buildDSN(skipping, name))
	require.NoError(t, err)
	require.True(t, parsed.TLS.InsecureSkipVerify)
}

// TestNewRefusesHalfOfAKeyPair pins that a section turning TLS on with a
// client certificate and no key, or a key and no certificate, is refused
// before anything is dialed.
func TestNewRefusesHalfOfAKeyPair(t *testing.T) {
	cfg := config.MySQL{Host: "127.0.0.1", Port: 3306, Database: "sample", Username: "root", TLSEnabled: true, CertFile: "client-cert.pem"}
	_, err := New(cfg)
	require.EqualError(t, err, "mysql tls: cert_file is set without key_file: a client certificate takes both")

	cfg.CertFile, cfg.KeyFile = "", "client-key.pem"
	_, err = New(cfg)
	require.EqualError(t, err, "mysql tls: key_file is set without cert_file: a client certificate takes both")
}

// TestTranslateTellsTheKeyOfADuplicateEntry pins what a duplicate entry is
// translated to: the sentinel with the key the message closes with, the
// table a newer server writes before the key left out, PRIMARY for the
// primary key, read from the end of the message, since the entry is the
// client's own value and may hold the very words the key follows; and the
// driver's text kept in front of the sentinel's.
func TestTranslateTellsTheKeyOfADuplicateEntry(t *testing.T) {
	for message, key := range map[string]string{
		"Duplicate entry 'a' for key 'records.uniq_records_code'":       "uniq_records_code",
		"Duplicate entry 'a' for key 'uniq_records_code'":               "uniq_records_code",
		"Duplicate entry '01J' for key 'records.PRIMARY'":               "PRIMARY",
		"Duplicate entry 'x' for key 'y' for key 'records.uniq_code'":   "uniq_code",
		"Duplicate entry 'a.b' for key 'records.uniq_records_tenant_c'": "uniq_records_tenant_c",
	} {
		driverErr := &mysql.MySQLError{Number: duplicateEntry, Message: message}

		translated := translate(driverErr)

		require.ErrorIs(t, translated, dberror.ErrDuplicatedKey, message)
		var duplicated *dberror.DuplicatedKeyError
		require.ErrorAs(t, translated, &duplicated, message)
		require.Equal(t, key, duplicated.Constraint, message)
		require.Empty(t, duplicated.Columns, message)
		require.Equal(t, driverErr.Error()+": "+dberror.ErrDuplicatedKey.Error(), translated.Error(), message)
	}

	unreadable := translate(&mysql.MySQLError{Number: duplicateEntry, Message: "Duplicate entry"})
	var duplicated *dberror.DuplicatedKeyError
	require.ErrorAs(t, unreadable, &duplicated)
	require.Empty(t, duplicated.Constraint)
}
