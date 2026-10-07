package mysql

import (
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
	gormschema "gorm.io/gorm/schema"
)

func TestBuildDSN(t *testing.T) {
	base := config.MySQL{Host: "127.0.0.1", Port: 3306, Database: "sample", Username: "root", Password: "secret", Strict: true}
	plain := "root:secret@tcp(127.0.0.1:3306)/sample?charset=utf8mb4&parseTime=True&loc=UTC&clientFoundRows=true&interpolateParams=true"
	prefix := plain + "&sql_mode=CONCAT(@@sql_mode,%27,STRICT_TRANS_TABLES%27)"

	t.Run("without timeouts", func(t *testing.T) {
		require.Equal(t, prefix, buildDSN(base))
	})

	t.Run("strict off", func(t *testing.T) {
		cfg := base
		cfg.Strict = false
		require.Equal(t, plain, buildDSN(cfg))
	})

	t.Run("dial timeout", func(t *testing.T) {
		cfg := base
		cfg.DialTimeout = 10 * time.Second
		require.Equal(t, prefix+"&timeout=10s", buildDSN(cfg))
	})

	t.Run("all timeouts", func(t *testing.T) {
		cfg := base
		cfg.DialTimeout = 5 * time.Second
		cfg.ReadTimeout = 30 * time.Second
		cfg.WriteTimeout = time.Minute
		require.Equal(t, prefix+"&timeout=5s&readTimeout=30s&writeTimeout=1m0s", buildDSN(cfg))
	})
}

// TestDryRunAnswersColumnTypesWithoutAServer pins what DryRun is for: a
// handle on the MySQL dialect that never connects and still answers the
// column type a field gets, longtext for a string field without a size.
func TestDryRunAnswersColumnTypesWithoutAServer(t *testing.T) {
	db, err := DryRun()
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
