package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/database"
	gstmysql "github.com/hydroan/gst/database/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	gormschema "gorm.io/gorm/schema"
)

// TestDialectAppliesItsNamingLimits pins that the driver's own settings
// reach the handle: gorm.Open hands the configuration to the driver before
// it fills in its defaults, and postgres limits an identifier to 63
// characters there, where gorm's own default is 64; a name gorm checks by a
// longer limit than the database keeps is one it would never find again.
// The handle carries the driver's dialector as it is, which is what lets
// those settings through.
func TestDialectAppliesItsNamingLimits(t *testing.T) {
	naming, ok := database.DB().NamingStrategy.(gormschema.NamingStrategy)
	require.True(t, ok, "the handle runs on gorm's naming strategy: %T", database.DB().NamingStrategy)

	want := 64
	if config.App.Database.Type == config.DBPostgres {
		want = 63
	}
	require.Equal(t, want, naming.IdentifierMaxLength)
}

// TestMySQLConnectionsRunInStrictMode pins the sql_mode a MySQL connection
// runs under: the modes the server is configured with plus
// STRICT_TRANS_TABLES, so that a value too long for its column is refused
// and never cut short while the server's other modes stay the operator's;
// and, with the strict switch off, the server's modes alone.
func TestMySQLConnectionsRunInStrictMode(t *testing.T) {
	if config.App.Database.Type != config.DBMySQL {
		t.Skip("the sql_mode is MySQL's")
	}
	var global, session string
	require.NoError(t, database.DB().Raw("SELECT @@GLOBAL.sql_mode").Scan(&global).Error)
	require.NoError(t, database.DB().Raw("SELECT @@SESSION.sql_mode").Scan(&session).Error)
	require.ElementsMatch(t, modeSet(global+",STRICT_TRANS_TABLES"), modeSet(session), "global %q, session %q", global, session)

	t.Run("strict off", func(t *testing.T) {
		cfg := config.App.MySQL
		cfg.Strict = false
		db, err := gstmysql.New(cfg)
		require.NoError(t, err)
		t.Cleanup(func() {
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
		})
		var session string
		require.NoError(t, db.Raw("SELECT @@SESSION.sql_mode").Scan(&session).Error)
		require.ElementsMatch(t, modeSet(global), modeSet(session))
	})
}

// modeSet splits a sql_mode list into its distinct modes.
func modeSet(modes string) []string {
	var set []string
	seen := map[string]bool{}
	for mode := range strings.SplitSeq(modes, ",") {
		if mode == "" || seen[mode] {
			continue
		}
		seen[mode] = true
		set = append(set, mode)
	}
	return set
}

// TestJSONValuesFindTheRowTheyWereWrittenFrom pins that a JSON value bound
// as a condition compares equal to the row it was written from on every
// dialect. gorm.io/datatypes renders the value for the dialect the handle
// carries, on MySQL as CAST(? AS JSON), since the server compares a JSON
// column with a plain string as two strings that are never equal; it tells
// MySQL apart by the driver's own dialector type, which the handle carries
// as it is.
func TestJSONValuesFindTheRowTheyWereWrittenFrom(t *testing.T) {
	defer cleanupTestData()
	addr := datatypes.NewJSONSlice([]string{"street-1", "street-2"})
	user := &TestUser{Name: "json-user", Email: "json-user@example.com", Age: 30, Addr: addr, ID: "json-user"}
	require.NoError(t, database.Database[*TestUser](context.Background()).Create(user))

	var found TestUser
	require.NoError(t, database.DB().Where("addr = ?", addr).First(&found).Error)
	require.Equal(t, user.ID, found.ID)
}

// TestHandleCarriesTheDriversOwnDialector pins that the handle's dialector is
// the driver's own type, with nothing of the framework's standing in for it:
// gorm.io/datatypes tells MySQL apart by asserting that type before it
// renders a JSON value as CAST(? AS JSON), and the settings a driver hands
// gorm.Open, such as postgres's identifier length, travel the same way.
// Whatever the framework adds to a handle goes through gorm's callbacks and
// plugins instead.
func TestHandleCarriesTheDriversOwnDialector(t *testing.T) {
	dialector := database.DB().Dialector
	switch config.App.Database.Type {
	case config.DBMySQL:
		require.IsType(t, &mysql.Dialector{}, dialector)
	case config.DBPostgres:
		require.IsType(t, &postgres.Dialector{}, dialector)
	case config.DBSqlite:
		require.IsType(t, &sqlite.Dialector{}, dialector)
	default:
		t.Fatalf("no driver type is known for %s", config.App.Database.Type)
	}
}
