package testcontainer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hydroan/gst/config"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestDedicatedContainersRequested(t *testing.T) {
	isolateEnv(t, envDedicatedContainers)
	require.False(t, dedicatedContainersRequested())

	for value, want := range map[string]bool{"1": true, "true": true, "0": false, "yes": false} {
		t.Setenv(envDedicatedContainers, value)
		require.Equal(t, want, dedicatedContainersRequested(), "value %q", value)
	}
}

func TestSharedContainerName(t *testing.T) {
	t.Run("image_reference_shapes_the_name", func(t *testing.T) {
		require.Equal(t, "gst-test-mysql-8-4", sharedContainerName("mysql:8.4"))
		require.Equal(t, "gst-test-redis-7-alpine", sharedContainerName("redis:7-alpine"))
		require.Equal(t, "gst-test-sample-server-24-8-alpine", sharedContainerName("sample/server:24.8-alpine"))
	})

	t.Run("arguments_are_fingerprinted_into_the_name", func(t *testing.T) {
		plain := sharedContainerName("sample:1")
		tuned := sharedContainerName("sample:1", "--flag=1")

		// The fingerprint extends the image-derived name rather than replacing
		// it, so the container a name belongs to stays readable.
		require.Equal(t, "gst-test-sample-1", plain)
		require.True(t, strings.HasPrefix(tuned, plain+"-"), "name %q", tuned)

		// Same arguments, same name: a rerun of an unchanged setup attaches to
		// the container the previous run created.
		require.Equal(t, tuned, sharedContainerName("sample:1", "--flag=1"))

		// Any change to the command line names a container that does not exist
		// yet, which is what makes a tuning change take effect on its own.
		require.NotEqual(t, tuned, sharedContainerName("sample:1", "--flag=2"))
		require.NotEqual(t, tuned, sharedContainerName("sample:1", "--flag=1", "--extra"))
		require.NotEqual(t, plain, tuned)

		// Arguments are fingerprinted as a list, not as concatenated text, so
		// a differently split command line is a different container.
		require.NotEqual(t,
			sharedContainerName("sample:1", "-c", "a=1"),
			sharedContainerName("sample:1", "-ca=1"))
	})

	t.Run("every_shared_setup_fingerprints_its_command_line", func(t *testing.T) {
		// Each setup creates its container with arguments, so each name must
		// carry a fingerprint rather than the bare image-derived name; that is
		// what keeps the name and the running command line in step.
		for _, tt := range []struct {
			image string
			args  []string
		}{
			{image: mysqlImage, args: mysqlSharedArgs},
			{image: postgresImage, args: postgresSharedArgs},
			{image: redisImage, args: redisSharedArgs},
		} {
			require.NotEmpty(t, tt.args, "image %q", tt.image)
			require.NotEqual(t, sharedContainerName(tt.image), sharedContainerName(tt.image, tt.args...),
				"image %q", tt.image)
		}
	})
}

func TestWithSharedContainerLock(t *testing.T) {
	var inside, peak atomic.Int32
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			_ = withSharedContainerLock("gst-test-lock-sample", func() error {
				now := inside.Add(1)
				peak.Store(max(peak.Load(), now))
				time.Sleep(10 * time.Millisecond)
				inside.Add(-1)
				return nil
			})
		})
	}
	wg.Wait()

	require.Equal(t, int32(1), peak.Load(), "the lock must serialize its holders")
}

func TestSharedDatabaseAbandoned(t *testing.T) {
	// A fresh name of this very process is owned, not abandoned.
	require.False(t, sharedDatabaseAbandoned(newSharedDatabaseName()))

	// A dead pid marks a leftover no matter how fresh the name is.
	require.True(t, sharedDatabaseAbandoned(
		fmt.Sprintf("gst_test_999999999_%d_ffff", time.Now().Unix()),
	))

	// A live pid on a name past the stale age is a recycled pid.
	require.True(t, sharedDatabaseAbandoned(
		fmt.Sprintf("gst_test_%d_%d_ffff", os.Getpid(), time.Now().Add(-25*time.Hour).Unix()),
	))

	// A name this setup did not generate is never ours to drop.
	for _, name := range []string{
		"test", "information_schema", "gst_test_", "gst_test_1_abc_ffff", "gst_test_1_123", "other_1_123_ffff",
	} {
		require.False(t, sharedDatabaseAbandoned(name), "name %q", name)
	}
}

func TestRedisLeaseHolderAlive(t *testing.T) {
	// The lease of this very process names a running process.
	require.True(t, redisLeaseHolderAlive(formatRedisLease()))

	// A pid beyond any platform's pid range names no process.
	require.False(t, redisLeaseHolderAlive("999999999:0"))

	// An unparseable lease counts as alive, taking over is the risky move.
	for _, lease := range []string{"garbage", "abc:123", "-1:123", ""} {
		require.True(t, redisLeaseHolderAlive(lease), "lease %q", lease)
	}
}

func TestSharedMySQLProvisioning(t *testing.T) {
	isolateEnv(t,
		config.MYSQL_HOST, config.MYSQL_PORT, config.MYSQL_DATABASE,
		config.MYSQL_USERNAME, config.MYSQL_PASSWORD,
		config.DATABASE_TYPE, config.DATABASE_AUTO_MIGRATE)

	// Two setups stand in for two test binaries: each gets a database of its
	// own inside the one shared container.
	releaseFirst, err := setupSharedMySQL()
	require.NoError(t, err)
	first := os.Getenv(config.MYSQL_DATABASE)

	releaseSecond, err := setupSharedMySQL()
	require.NoError(t, err)
	second := os.Getenv(config.MYSQL_DATABASE)

	require.NotEqual(t, first, second)
	require.Equal(t, string(config.DBMySQL), os.Getenv(config.DATABASE_TYPE))

	admin := openSharedAdmin(t, mysqlSharedDialect,
		os.Getenv(config.MYSQL_HOST), os.Getenv(config.MYSQL_PORT))
	require.True(t, sharedDatabaseListed(t, admin, mysqlSharedDialect, first))
	require.True(t, sharedDatabaseListed(t, admin, mysqlSharedDialect, second))

	// Releasing drops the database of the released binary and nothing else.
	require.NoError(t, releaseFirst())
	require.False(t, sharedDatabaseListed(t, admin, mysqlSharedDialect, first))
	require.True(t, sharedDatabaseListed(t, admin, mysqlSharedDialect, second))

	require.NoError(t, releaseSecond())
	require.False(t, sharedDatabaseListed(t, admin, mysqlSharedDialect, second))
}

func TestSharedPostgresProvisioning(t *testing.T) {
	isolateEnv(t,
		config.POSTGRES_HOST, config.POSTGRES_PORT, config.POSTGRES_DATABASE,
		config.POSTGRES_USERNAME, config.POSTGRES_PASSWORD, config.POSTGRES_SSLMODE,
		config.DATABASE_TYPE, config.DATABASE_AUTO_MIGRATE)

	// Two setups stand in for two test binaries: each gets a database of its
	// own inside the one shared container.
	releaseFirst, err := setupSharedPostgres()
	require.NoError(t, err)
	first := os.Getenv(config.POSTGRES_DATABASE)

	releaseSecond, err := setupSharedPostgres()
	require.NoError(t, err)
	second := os.Getenv(config.POSTGRES_DATABASE)

	require.NotEqual(t, first, second)
	require.Equal(t, string(config.DBPostgres), os.Getenv(config.DATABASE_TYPE))

	admin := openSharedAdmin(t, postgresSharedDialect,
		os.Getenv(config.POSTGRES_HOST), os.Getenv(config.POSTGRES_PORT))
	require.True(t, sharedDatabaseListed(t, admin, postgresSharedDialect, first))
	require.True(t, sharedDatabaseListed(t, admin, postgresSharedDialect, second))

	// Releasing drops the database of the released binary and nothing else.
	require.NoError(t, releaseFirst())
	require.False(t, sharedDatabaseListed(t, admin, postgresSharedDialect, first))
	require.True(t, sharedDatabaseListed(t, admin, postgresSharedDialect, second))

	require.NoError(t, releaseSecond())
	require.False(t, sharedDatabaseListed(t, admin, postgresSharedDialect, second))
}

func TestSharedClickhouseProvisioning(t *testing.T) {
	// Two setups stand in for two test binaries: each gets a database of its
	// own inside the one shared container.
	firstCfg, releaseFirst, err := SetupClickhouse()
	require.NoError(t, err)
	secondCfg, releaseSecond, err := SetupClickhouse()
	require.NoError(t, err)

	require.NotEqual(t, firstCfg.Database, secondCfg.Database)

	admin := openSharedAdmin(t, clickhouseSharedDialect,
		firstCfg.Host, strconv.Itoa(int(firstCfg.Port)))
	require.True(t, sharedDatabaseListed(t, admin, clickhouseSharedDialect, firstCfg.Database))
	require.True(t, sharedDatabaseListed(t, admin, clickhouseSharedDialect, secondCfg.Database))

	// Releasing drops the database of the released binary and nothing else.
	require.NoError(t, releaseFirst())
	require.False(t, sharedDatabaseListed(t, admin, clickhouseSharedDialect, firstCfg.Database))
	require.True(t, sharedDatabaseListed(t, admin, clickhouseSharedDialect, secondCfg.Database))

	require.NoError(t, releaseSecond())
	require.False(t, sharedDatabaseListed(t, admin, clickhouseSharedDialect, secondCfg.Database))
}

func TestSharedRedisClaims(t *testing.T) {
	isolateEnv(t, config.REDIS_ADDR, config.REDIS_DB, config.REDIS_ENABLED)
	ctx := context.Background()

	// Two setups stand in for two test binaries: each claims an index of its
	// own inside the one shared container.
	releaseFirst, err := setupSharedRedis()
	require.NoError(t, err)
	addr := os.Getenv(config.REDIS_ADDR)
	first, err := strconv.Atoi(os.Getenv(config.REDIS_DB))
	require.NoError(t, err)

	releaseSecond, err := setupSharedRedis()
	require.NoError(t, err)
	second, err := strconv.Atoi(os.Getenv(config.REDIS_DB))
	require.NoError(t, err)

	require.NotEqual(t, first, second)
	require.Equal(t, "true", os.Getenv(config.REDIS_ENABLED))

	// A key written into one index is invisible in the other.
	firstClient := goredis.NewClient(&goredis.Options{Addr: addr, DB: first})
	defer firstClient.Close()
	secondClient := goredis.NewClient(&goredis.Options{Addr: addr, DB: second})
	defer secondClient.Close()
	require.NoError(t, firstClient.Set(ctx, "sample", "value", 0).Err())
	require.ErrorIs(t, secondClient.Get(ctx, "sample").Err(), goredis.Nil)

	// Releasing frees the index; whichever index the next claim lands on
	// starts empty, even a reused one.
	require.NoError(t, releaseFirst())
	releaseThird, err := setupSharedRedis()
	require.NoError(t, err)
	third, err := strconv.Atoi(os.Getenv(config.REDIS_DB))
	require.NoError(t, err)
	thirdClient := goredis.NewClient(&goredis.Options{Addr: addr, DB: third})
	defer thirdClient.Close()

	require.NotEqual(t, second, third, "a live lease must not be taken over")
	require.ErrorIs(t, thirdClient.Get(ctx, "sample").Err(), goredis.Nil)

	require.NoError(t, releaseThird())
	require.NoError(t, releaseSecond())
}

// openSharedAdmin connects to a shared container through the admin connection
// its dialect describes.
func openSharedAdmin(t *testing.T, d sharedSQLDialect, host, port string) *sql.DB {
	t.Helper()
	portNum, err := strconv.ParseUint(port, 10, 32)
	require.NoError(t, err)
	admin, err := sql.Open(d.driver, d.adminDSN(host, uint(portNum)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	return admin
}

// sharedDatabaseListed reports whether the shared container lists a database
// by that name.
func sharedDatabaseListed(t *testing.T, admin *sql.DB, d sharedSQLDialect, name string) bool {
	t.Helper()
	rows, err := admin.Query(d.listDatabases)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var listed string
		require.NoError(t, rows.Scan(&listed))
		if listed == name {
			return true
		}
	}
	require.NoError(t, rows.Err())
	return false
}
