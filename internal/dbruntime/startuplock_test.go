package dbruntime

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestStartupLockNameIsOnePerPurposeAndDatabase pins the lock's scope:
// deployments on two databases of one MySQL server hold two locks, the
// table preparation and the seeding of one deployment hold two, and every
// name fits MySQL's 64-character limit whatever the database is called.
func TestStartupLockNameIsOnePerPurposeAndDatabase(t *testing.T) {
	require.NotEqual(t, startupLockName("migrate", "app"), startupLockName("migrate", "app_staging"))
	require.NotEqual(t, startupLockName("migrate", "app"), startupLockName("seed", "app"))
	require.True(t, strings.HasPrefix(startupLockName("migrate", "app"), "gst:migrate:"))
	require.LessOrEqual(t, len(startupLockName("migrate", strings.Repeat("d", 64))), 64)
	require.NotEqual(t, startupLockKey(startupLockName("migrate", "app")), startupLockKey(startupLockName("migrate", "app_staging")))
}

// TestSerializedRunsOneProcessAtATime proves the startup lock serializes a
// step across the processes of a deployment: several handles — each a pool
// of its own, the way separate processes look to the server — run the step
// at once, and never two of them inside it together, on the dialects that
// offer the lock.
func TestSerializedRunsOneProcessAtATime(t *testing.T) {
	withFastStartupLock(t)

	for _, dialect := range []struct {
		name config.DBType
		open func(t *testing.T) *gorm.DB
	}{
		{name: config.DBMySQL, open: newMySQLDB},
		{name: config.DBPostgres, open: newPostgresDB},
	} {
		t.Run(string(dialect.name), func(t *testing.T) {
			const processes = 4
			handles := make([]*gorm.DB, 0, processes)
			for range processes {
				handles = append(handles, dialect.open(t))
			}

			var inside, overlaps atomic.Int32
			errs := make([]error, len(handles))
			var wg sync.WaitGroup
			for i, handle := range handles {
				wg.Go(func() {
					errs[i] = serialized(context.Background(), handle, "sample", func() error {
						if inside.Add(1) > 1 {
							overlaps.Add(1)
						}
						time.Sleep(50 * time.Millisecond)
						inside.Add(-1)
						return nil
					})
				})
			}
			wg.Wait()

			for i, err := range errs {
				require.NoErrorf(t, err, "process %d must take its turn", i)
			}
			require.Zero(t, overlaps.Load(), "no two processes may be inside the step at once")
		})
	}
}

// TestStartupLockWaitsForTheHolderAndSaysSo proves a process waits for the
// holder of a startup lock however long it takes, says so while it waits,
// and stops waiting when told to: with the lock held by another handle the
// step does not start, a warning is logged once the report interval passes,
// a waiter whose context ends returns with that ending instead of the
// lock, and the step runs as soon as the holder lets go.
func TestStartupLockWaitsForTheHolderAndSaysSo(t *testing.T) {
	for _, dialect := range []struct {
		name config.DBType
		open func(t *testing.T) *gorm.DB
	}{
		{name: config.DBMySQL, open: newMySQLDB},
		{name: config.DBPostgres, open: newPostgresDB},
	} {
		t.Run(string(dialect.name), func(t *testing.T) {
			logs := withObservedGlobalLogger(t)
			withFastStartupLock(t)

			holder, waiter, quitter := dialect.open(t), dialect.open(t), dialect.open(t)
			unlock, err := lockStartup(context.Background(), holder, "sample")
			require.NoError(t, err)
			unlocked := false
			t.Cleanup(func() {
				if !unlocked {
					unlock()
				}
			})

			entered := make(chan struct{})
			returned := make(chan error, 1)
			go func() {
				returned <- serialized(context.Background(), waiter, "sample", func() error {
					close(entered)
					return nil
				})
			}()

			select {
			case <-entered:
				t.Fatal("the step must not start while another process holds the lock")
			case <-time.After(200 * time.Millisecond):
			}
			require.NotEmpty(t, logs.FilterMessage("still waiting for the startup lock held by another process").All(),
				"a process waiting past the report interval must say so")

			quitCtx, quit := context.WithCancelCause(context.Background())
			quitReturned := make(chan error, 1)
			go func() {
				quitReturned <- serialized(quitCtx, quitter, "sample", func() error {
					t.Error("a waiter told to stop must not run the step")
					return nil
				})
			}()
			// Told to stop once it is waiting, not before it starts to.
			<-time.After(2 * startupLockWaitReport)
			quit(errors.New("sample stop"))
			select {
			case err := <-quitReturned:
				require.ErrorContains(t, err, "sample stop", "the waiter reports why its wait ended")
			case <-time.After(5 * time.Second):
				t.Fatal("a waiter told to stop must return")
			}

			unlock()
			unlocked = true
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("the step must start once the holder lets go")
			}
			require.NoError(t, <-returned)
		})
	}
}
