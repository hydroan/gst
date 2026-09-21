package cronjob

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/dbruntime"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/lease"
	"github.com/hydroan/gst/internal/testutil/oteltest"
	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// TestLoopRunsAtEachInstant proves a started job runs once at every instant
// of its schedule and not before: the loop waits for the instant, runs, and
// waits for the next.
func TestLoopRunsAtEachInstant(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	runs := make(chan time.Time, 8)
	Register(func(context.Context) error {
		runs <- clk.Now()
		return nil
	}, "@every 1m", "grid-loop-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(30 * time.Second)
	require.Equal(t, time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC), awaitRun(t, runs))
	// The loop computes the next instant from the clock once the run ended;
	// the clock only moves on once it is waiting for that instant.
	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	require.Equal(t, time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC), awaitRun(t, runs))
	require.NoError(t, stop(context.Background()))
	require.Empty(t, runs, "no instant ran twice and none ran early")
}

// TestClaimThatFailedIsTriedAgainWithinTheInstant proves what a database
// that stumbles costs the job: nothing. The claim of an instant is the
// job's only chance at it — an instant no replica claimed is never caught up
// once a later round records having overrun it — so a claim the database
// could not answer is tried again until the next instant is due.
func TestClaimThatFailedIsTriedAgainWithinTheInstant(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	runs := newRunLog()
	Register(func(context.Context) error {
		runs.record(clk.Now())
		return nil
	}, "@every 1m", "claim-retry-job")
	require.NoError(t, start(context.Background()))
	clock.untilWaiting(t)

	// The lease table out of reach: the claim of 10:01 fails the way it does
	// while the primary cannot answer.
	require.NoError(t, dbruntime.DB.Exec("ALTER TABLE gst_leases RENAME TO gst_leases_hidden").Error)
	clock.Advance(30 * time.Second)
	// The loop is waiting again — on its retry, not on the next instant.
	clock.untilWaiting(t)
	require.Empty(t, runs.counts(), "the round cannot have run while the claim failed")

	require.NoError(t, dbruntime.DB.Exec("ALTER TABLE gst_leases_hidden RENAME TO gst_leases").Error)
	clock.Advance(lease.RetryInterval())
	runs.await(t)
	require.NoError(t, stop(context.Background()))

	require.Len(t, runs.counts(), 1, "the retry runs the round once, not once per try")
	// Which instant ran is what the lease row records: the round itself ran
	// later than 10:01, at the try that found the database answering again.
	var slotMs int64
	require.NoError(t, dbruntime.DB.Raw("SELECT slot_ms FROM gst_leases WHERE name = ?", "cron:claim-retry-job").Scan(&slotMs).Error)
	require.Equal(t, time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC), time.UnixMilli(slotMs).UTC(),
		"the instant the claim failed for is the one that ran")
}

// TestRoundStillRunningAtItsNextInstantSaysSo proves the warning a round
// that overran writes while it is still running. The entries naming the
// instants a round skipped come only once it returns, and a round that
// stopped the job for the whole deployment — the lease it holds is the one
// every replica claims the next instant with — must not stay silent until
// then, if it ever returns at all.
//
// The instants here pass in real time: the warning waits on the clock the
// process runs on, not on the one a test drives by hand.
func TestRoundStillRunningAtItsNextInstantSaysSo(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)

	release := make(chan struct{})
	started := make(chan struct{}, 1)
	Register(func(context.Context) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}, "@every 1s", "overrunning-job")
	require.NoError(t, start(context.Background()))

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the round did not start")
	}
	// Two instants of the schedule pass while the round is held.
	time.Sleep(2500 * time.Millisecond)
	close(release)
	require.NoError(t, stop(context.Background()))

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob round is still running at its next instant")
	require.Equal(t, "overrunning-job", entry["name"])
	require.EqualValues(t, 1, entry["overrun"], "the first entry is written at the first instant the round overran")
}

// TestInstantRunsOnceAcrossInstances proves the cluster contract: two
// scheduler instances sharing one primary database — two replicas — run
// each instant of a job exactly once between them.
func TestInstantRunsOnceAcrossInstances(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	runs := newRunLog()
	Register(func(context.Context) error {
		runs.record(clk.Now())
		return nil
	}, "@every 1m", "shared-job")
	startInstances(t, 2)

	for range 3 {
		clock.untilWaiters(t, 2)
		clock.Advance(time.Minute)
		runs.await(t)
	}
	clock.untilWaiters(t, 2)

	require.Equal(t, map[time.Time]int{
		time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC): 1,
		time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC): 1,
		time.Date(2026, 1, 1, 10, 3, 0, 0, time.UTC): 1,
	}, runs.counts(), "each instant runs exactly once across the instances")
}

// TestPerInstanceJobRunsOnEveryInstance proves RegisterPerInstance opts a
// job out of the cluster contract: every instance runs every instant.
func TestPerInstanceJobRunsOnEveryInstance(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	runs := newRunLog()
	RegisterPerInstance(func(context.Context) error {
		runs.record(clk.Now())
		return nil
	}, "@every 1m", "local-job")
	startInstances(t, 2)

	clock.untilWaiters(t, 2)
	clock.Advance(time.Minute)
	runs.await(t)
	runs.await(t)
	clock.untilWaiters(t, 2)

	require.Equal(t, map[time.Time]int{
		time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC): 2,
	}, runs.counts(), "a per-instance job runs on every instance")
}

// TestHeldInstantKeepsTheNextFromEveryone proves a round still holding its
// lease keeps the following instants from the other instances too: the
// instant is skipped across the deployment, not run elsewhere — nor caught up
// by an instance starting once the round is over, the round having recorded
// it as it ended — and the instants after the round are shared again.
func TestHeldInstantKeepsTheNextFromEveryone(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	runs := newRunLog()
	var first atomic.Bool
	release := make(chan struct{})
	Register(func(context.Context) error {
		runs.record(clk.Now())
		if first.CompareAndSwap(false, true) {
			<-release
		}
		return nil
	}, "@every 1m", "shared-job")
	startInstances(t, 2)

	// 10:01: one instance wins and holds the lease for the whole round.
	clock.untilWaiters(t, 2)
	clock.Advance(time.Minute)
	runs.await(t)
	// 10:02: the other instance is refused — the lease is held — and the
	// winner is still busy, so no one runs it.
	clock.untilWaiters(t, 1)
	clock.Advance(time.Minute)
	clock.untilWaiters(t, 1)
	close(release)
	// The round is over and the lease released. An instance starting now
	// finds 10:02, the most recent instant, overrun rather than missed: it
	// catches nothing up.
	clock.untilWaiters(t, 2)
	startInstances(t, 1)
	// 10:03: one instance runs.
	clock.untilWaiters(t, 3)
	clock.Advance(time.Minute)
	runs.await(t)
	clock.untilWaiters(t, 3)

	require.Equal(t, map[time.Time]int{
		time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC): 1,
		time.Date(2026, 1, 1, 10, 3, 0, 0, time.UTC): 1,
	}, runs.counts(), "the instant under a held lease runs nowhere")
}

// TestCatchUpRunsTheMostRecentInstantNoReplicaRan proves the start-up
// catch-up: a job that has run before, whose most recent instant passed
// within the last day without any replica claiming it, runs that instant
// once at start — marked as a catch-up — and then goes on with its next.
func TestCatchUpRunsTheMostRecentInstantNoReplicaRan(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	// The job ran before: an earlier instant is on record.
	lastRun(t, "cron:catch-up-job", time.Date(2026, 1, 1, 9, 58, 0, 0, time.UTC))

	runs := make(chan time.Time, 8)
	Register(func(context.Context) error {
		runs <- clk.Now()
		return nil
	}, "@every 1m", "catch-up-job")
	require.NoError(t, start(context.Background()))

	require.Equal(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC), awaitRun(t, runs), "the catch-up runs at start")
	clock.untilWaiting(t)
	clock.Advance(30 * time.Second)
	require.Equal(t, time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC), awaitRun(t, runs), "the schedule goes on with the next instant")
	require.NoError(t, stop(context.Background()))

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Equal(t, "2026-01-01T10:00:00Z", entry["at"], "the catch-up runs for the instant it caught up")
	require.Equal(t, true, entry["catch_up"])
	require.EqualValues(t, 2, entry["term"], "the catch-up starts the next term of the job's lease")
}

// TestInstantsPassingDuringACatchUpAreSkipped proves the instants that pass
// while the catch-up round is in flight are skipped like any that pass
// during a round: the loop resumes with the first instant after the
// catch-up ended, not with the one computed before it began.
func TestInstantsPassingDuringACatchUpAreSkipped(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	lastRun(t, "cron:slow-catch-up-job", time.Date(2026, 1, 1, 9, 58, 0, 0, time.UTC))

	runs := newRunLog()
	var first atomic.Bool
	release := make(chan struct{})
	Register(func(context.Context) error {
		runs.record(clk.Now())
		if first.CompareAndSwap(false, true) {
			<-release
		}
		return nil
	}, "@every 1m", "slow-catch-up-job")
	require.NoError(t, start(context.Background()))

	// The catch-up for 10:00 runs at once and lasts past 10:01, the instant
	// the scheduler computed before it began.
	runs.await(t)
	clock.Advance(time.Minute)
	close(release)
	clock.untilWaiting(t)
	clock.Advance(30 * time.Second)
	runs.await(t)
	require.NoError(t, stop(context.Background()))

	require.Equal(t, map[time.Time]int{
		time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC): 1,
		time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC):  1,
	}, runs.counts(), "the instant overrun by the catch-up must be skipped, not run late")

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob skipped instants")
	require.EqualValues(t, 1, entry["skipped"])
	require.Equal(t, "2026-01-01T10:00:00Z", entry["after"], "the skip is counted from the instant the catch-up ran for")
}

// TestCatchUpSkipsAJobThatNeverRan proves a job appearing for the first
// time starts with its next instant: the instants before its first
// deployment were never its to run.
func TestCatchUpSkipsAJobThatNeverRan(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	runs := newRunLog()
	Register(func(context.Context) error {
		runs.record(clk.Now())
		return nil
	}, "@every 1m", "new-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	require.Empty(t, runs.counts(), "a job never run catches nothing up")
}

// TestCatchUpSkipsAnInstantAlreadyRun proves the catch-up never repeats an
// instant: when the most recent instant is the one on record, nothing runs
// at start.
func TestCatchUpSkipsAnInstantAlreadyRun(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))

	lastRun(t, "cron:settled-job", time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	runs := newRunLog()
	Register(func(context.Context) error {
		runs.record(clk.Now())
		return nil
	}, "@every 1m", "settled-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	require.Empty(t, runs.counts(), "an instant already run is never caught up")
}

// TestRoundCutShortByAShutdownRunsAgainElsewhere proves a round counts once
// it has run to its end: the replica running it shuts down, the round
// returns its context's ending and gives its lease back, and another replica
// finds the round and runs it a second time, for the same instant — marked
// as a second run — after which the instant is settled.
func TestRoundCutShortByAShutdownRunsAgainElsewhere(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	withFastSweep(t)

	entered := make(chan struct{}, 2)
	var rounds atomic.Int32
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		if rounds.Add(1) == 1 {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}, "@every 1m", "cut-short-job")

	shuttingDown := startInstances(t, 1)[0]
	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")
	takingOver := startInstances(t, 1)[0]
	require.NoError(t, shuttingDown.stop(context.Background()))
	awaitSignal(t, entered, "the round run a second time")
	require.NoError(t, takingOver.stop(context.Background()))
	pkgzap.Clean()

	interrupted := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob interrupted")
	require.Equal(t, "2026-01-01T10:01:00Z", interrupted["at"])
	require.Equal(t, "shutting down", interrupted["reason"])
	finished := readLogEntries(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Len(t, finished, 1)
	require.Equal(t, "2026-01-01T10:01:00Z", finished[0]["at"], "the second round runs for the instant cut short")
	require.Equal(t, true, finished[0]["rerun"])
	require.EqualValues(t, 2, finished[0]["term"])
	require.Empty(t, unfinishedInstants(t, "cron:cut-short-job"), "an instant run to its end is settled")
}

// TestRoundCutShortByACrashRunsAgainOnce proves a round whose process died —
// its lease never given back — runs a second time once the lease expires,
// and only once: that round cut short too is given up, and no replica finds
// the instant again.
func TestRoundCutShortByACrashRunsAgainOnce(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))
	withFastLease(t)
	withFastSweep(t)

	cutShortRun(t, "cron:crashed-job", time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), true)

	entered := make(chan struct{}, 2)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}, "@every 1m", "crashed-job")
	instance := startInstances(t, 1)[0]
	awaitSignal(t, entered, "the round run a second time")
	require.NoError(t, instance.stop(context.Background()))
	pkgzap.Clean()

	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob interrupted")
	require.Equal(t, "2026-01-01T10:00:00Z", entry["at"], "the second round runs for the instant the crash cut short")
	require.Equal(t, true, entry["rerun"])
	require.Zero(t, readLeaseRow(t, "cron:crashed-job").ExpiresAtMs, "the second round gave its lease back as it stopped")
	require.Empty(t, unfinishedInstants(t, "cron:crashed-job"), "an instant cut short a second time never runs a third")
}

// TestInstantKeptByACrashedLeaseStaysSkipped proves an instant that passes
// while the lease of a crashed round has yet to expire is skipped for good: no
// replica can claim it then, the second run of the crashed round records it
// as that run ends, and an instance starting afterwards catches nothing up.
func TestInstantKeptByACrashedLeaseStaysSkipped(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC))
	// A lease long enough for 10:01 to come while the crashed round's lease
	// still holds, however slow the machine running the test.
	t.Cleanup(lease.SetTimings(2*time.Second, 200*time.Millisecond, time.Second, 500*time.Millisecond))
	withFastSweep(t)

	cutShortRun(t, "cron:crash-kept-job", time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), true)

	ran := make(chan struct{}, 4)
	Register(func(context.Context) error {
		ran <- struct{}{}
		return nil
	}, "@every 1m", "crash-kept-job")
	running := startInstances(t, 1)[0]

	// 10:01 comes while the crashed round's lease holds: its claim is refused.
	clock.untilWaiting(t)
	clock.Advance(30 * time.Second)
	// Once the lease expires, the crashed round runs a second time and ends.
	awaitSignal(t, ran, "the crashed round run a second time")
	clock.untilWaiting(t)
	starting := startInstances(t, 1)[0]
	// 10:02: one instance runs.
	clock.untilWaiters(t, 2)
	clock.Advance(time.Minute)
	awaitSignal(t, ran, "the round for 10:02")
	clock.untilWaiters(t, 2)
	require.NoError(t, running.stop(context.Background()))
	require.NoError(t, starting.stop(context.Background()))
	pkgzap.Clean()

	finished := readLogEntries(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Len(t, finished, 2, "10:01, kept from everyone by the crashed round's lease, runs nowhere")
	require.Equal(t, "2026-01-01T10:00:00Z", finished[0]["at"])
	require.Equal(t, true, finished[0]["rerun"])
	require.Equal(t, "2026-01-01T10:02:00Z", finished[1]["at"])
}

// TestRoundWhoseEndIsNotRecordedRunsAgain proves a round that ran to its end
// but whose end the database failed to record is left the way a crash leaves
// one: the failure is logged, the lease is not given back, and once it expires
// a replica runs the instant a second time — marked as a second run — after
// which the instant is settled.
func TestRoundWhoseEndIsNotRecordedRunsAgain(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	withFirstFinishFailing(t, "cron:unrecorded-job")
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	withFastLease(t)
	withFastSweep(t)

	entered := make(chan struct{}, 2)
	Register(func(context.Context) error {
		entered <- struct{}{}
		return nil
	}, "@every 1m", "unrecorded-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")
	awaitSignal(t, entered, "the round run a second time")
	require.NoError(t, stop(context.Background()))
	pkgzap.Clean()

	unrecorded := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob could not record its round as finished")
	require.Equal(t, "WARN", unrecorded["level"])
	require.Equal(t, "2026-01-01T10:01:00Z", unrecorded["at"])
	finished := readLogEntries(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Len(t, finished, 2)
	require.Equal(t, "2026-01-01T10:01:00Z", finished[1]["at"], "the second round runs for the instant whose end went unrecorded")
	require.Equal(t, true, finished[1]["rerun"])
	require.Zero(t, readLeaseRow(t, "cron:unrecorded-job").UnfinishedSlotMs, "the second round's end is recorded")
}

// TestRoundThatFailedDoesNotRunAgain proves a round that failed ran to its
// end — the job returned an error of its own, or panicked — so the instant is
// settled, and no replica runs it again.
func TestRoundThatFailedDoesNotRunAgain(t *testing.T) {
	cases := []struct {
		name string
		job  func() error
	}{
		{
			name: "returned error",
			job:  func() error { return errors.New("sample failure") },
		},
		{
			name: "panic",
			job:  func() error { panic("sample panic") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withCronjobLoggerConfig(t)
			resetCronjobState(t)
			clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

			entered := make(chan struct{}, 1)
			Register(func(context.Context) error {
				entered <- struct{}{}
				return tc.job()
			}, "@every 1m", "failed-job")
			require.NoError(t, start(context.Background()))

			clock.untilWaiting(t)
			clock.Advance(time.Minute)
			awaitSignal(t, entered, "the round")
			// The loop waits for the next instant once the round has recorded
			// its end.
			clock.untilWaiting(t)
			row := readLeaseRow(t, "cron:failed-job")
			require.Zero(t, row.UnfinishedSlotMs, "a round that failed ran to its end: its instant is not run again")
			require.Zero(t, row.ExpiresAtMs, "and its lease is given back")
		})
	}
}

// TestNextInstantGivesUpARoundCutShort proves a round cut short is given up
// once a later instant is claimed before any replica ran it again — the next
// instant on the schedule, or the most recent one a starting scheduler
// catches up: that round runs, a warning names the instant given up, and
// nothing is left to run.
func TestNextInstantGivesUpARoundCutShort(t *testing.T) {
	cases := []struct {
		name string
		// start is where the clock stands as the scheduler starts; the
		// clock moves on by advance once the loop waits, when advance is not
		// zero.
		start   time.Time
		advance time.Duration
		// claimed is the later instant whose claim gives the round up.
		claimed string
		catchUp bool
	}{
		{
			name:    "next instant",
			start:   time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC),
			advance: 30 * time.Second,
			claimed: "2026-01-01T10:01:00Z",
		},
		{
			name:    "catch-up at start",
			start:   time.Date(2026, 1, 1, 10, 2, 30, 0, time.UTC),
			claimed: "2026-01-01T10:02:00Z",
			catchUp: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := withCronjobLoggerConfig(t)
			resetCronjobState(t)
			withBoundCronjobLogger(t)
			clock := withFakeClock(t, tc.start)

			// A shutdown cut the round for 10:00 short; the scheduler's first
			// look for such rounds is 15 seconds away, and a later instant is
			// claimed first.
			cutShortRun(t, "cron:given-up-job", time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), false)

			runs := make(chan time.Time, 4)
			Register(func(context.Context) error {
				runs <- clk.Now()
				return nil
			}, "@every 1m", "given-up-job")
			require.NoError(t, start(context.Background()))

			if tc.advance != 0 {
				clock.untilWaiting(t)
				clock.Advance(tc.advance)
			}
			awaitRun(t, runs)
			require.NoError(t, stop(context.Background()))
			pkgzap.Clean()

			entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob gave up a round cut short")
			require.Equal(t, "WARN", entry["level"])
			require.Equal(t, "given-up-job", entry["name"])
			require.Equal(t, "2026-01-01T10:00:00Z", entry["at"])
			require.Equal(t, tc.claimed, entry["next"])
			require.Equal(t, false, entry["rerun"])
			finished := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
			require.Equal(t, tc.claimed, finished["at"], "the later instant runs")
			if tc.catchUp {
				require.Equal(t, true, finished["catch_up"])
			}
			require.Empty(t, unfinishedInstants(t, "cron:given-up-job"))
		})
	}
}

// TestInstantsPassingDuringARunAreSkipped proves a slow round never has the
// instants it overran piled on top of it: the loop resumes with the first
// instant after the run ended, and logs how many instants the run cost. A
// round overrunning three instants is followed by one run, not four, and by
// a warning naming the three.
func TestInstantsPassingDuringARunAreSkipped(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	var runs atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	Register(func(context.Context) error {
		entered <- struct{}{}
		if runs.Add(1) == 1 {
			<-release
		}
		return nil
	}, "@every 1m", "overrun-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the first round")
	// Three instants pass while the first round is still in flight.
	clock.Advance(3 * time.Minute)
	close(release)
	// The next instant after the run ended is 10:05, not the 10:02 it
	// overran: once the loop waits for it, one more minute brings it.
	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round after the overrun")
	require.NoError(t, stop(context.Background()))
	require.EqualValues(t, 2, runs.Load(), "the instants overrun by the first round must be skipped")

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob skipped instants")
	require.Equal(t, "overrun-job", entry["name"])
	require.EqualValues(t, 3, entry["skipped"], "the warning must count the instants the run cost")
	require.Equal(t, "2026-01-01T10:05:00Z", entry["next"])
}

// TestRoundThatLosesItsLeaseIsCutShortAndLogged proves a round outliving
// its lease ends with it: once another replica has taken the name — the
// lease ended behind the round's back — the round's context ends with
// ErrLost as the cause, the round returning that ending — the context's
// error or its cause — is logged as an interruption naming the loss, once,
// the instant stays on record to run a second time, and the scheduler goes on
// to the next instant.
func TestRoundThatLosesItsLeaseIsCutShortAndLogged(t *testing.T) {
	cases := []struct {
		name    string
		returns func(ctx context.Context) error
	}{
		{name: "context's error", returns: func(ctx context.Context) error { return ctx.Err() }},
		{name: "context's cause", returns: context.Cause},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := withCronjobLoggerConfig(t)
			resetCronjobState(t)
			withBoundCronjobLogger(t)
			clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
			withFastLease(t)

			entered := make(chan struct{}, 1)
			ended := make(chan error, 1)
			Register(func(ctx context.Context) error {
				entered <- struct{}{}
				<-ctx.Done()
				ended <- context.Cause(ctx)
				return tc.returns(ctx)
			}, "@every 1m", "lost-job")
			require.NoError(t, start(context.Background()))

			clock.untilWaiting(t)
			clock.Advance(time.Minute)
			awaitSignal(t, entered, "the round")

			takeOver(t, "cron:lost-job")
			select {
			case cause := <-ended:
				require.ErrorIs(t, cause, lease.ErrLost)
			case <-time.After(5 * time.Second):
				t.Fatal("the round must end once its lease is lost")
			}
			clock.untilWaiting(t)
			pkgzap.Clean()

			interrupted := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob interrupted")
			require.Equal(t, "lost-job", interrupted["name"])
			require.Equal(t, "lease lost", interrupted["reason"], "the round returning its context's ending is an interruption, not a failure")
			require.EqualValues(t, 1, interrupted["term"], "the entry names the term the round ran in")
			require.Empty(t, readLogEntries(t, filepath.Join(dir, "cronjob.log"), "cronjob lost its lease during the round"),
				"the interruption entry is the record of the loss; a second entry would say the same thing")
			// The replica that took the name over gives it back.
			endLease(t, "cron:lost-job")
			require.Equal(t, []time.Time{time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC)}, unfinishedInstants(t, "cron:lost-job"),
				"a round the loss cut short stays on record to run a second time")
		})
	}
}

// TestRoundThatReturnsNothingAfterLosingItsLeaseIsLogged proves the loss
// is recorded even when the round's own entry does not carry it: a job that
// returns nothing once its context ended finishes as far as its own entry
// goes, so the scheduler records the lost lease beside it.
func TestRoundThatReturnsNothingAfterLosingItsLeaseIsLogged(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	withFastLease(t)

	entered := make(chan struct{}, 1)
	ended := make(chan struct{}, 1)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		ended <- struct{}{}
		return nil
	}, "@every 1m", "quiet-lost-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")

	takeOver(t, "cron:quiet-lost-job")
	awaitSignal(t, ended, "the round's end")
	clock.untilWaiting(t)
	pkgzap.Clean()

	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob lost its lease during the round")
	require.Equal(t, "quiet-lost-job", entry["name"])
	require.EqualValues(t, 1, entry["term"], "the entry names the term the round ran in")
	finished := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Equal(t, "quiet-lost-job", finished["name"], "the round's own entry says it finished: it returned nothing")
	// The replica that took the name over gives it back.
	endLease(t, "cron:quiet-lost-job")
	require.Empty(t, unfinishedInstants(t, "cron:quiet-lost-job"), "a round that returned nothing ran to its end, its lease lost or not")
}

// TestRoundThatLosesItsLeaseWhileWindingDownIsLoggedAsALoss proves a loss
// found once the process began shutting down is recorded as a loss: the
// round's context ended with the shutdown, the renewals went on while the
// round wound down and found the name taken, and the round returning its
// context's ending is logged as an interruption naming the lost lease. The
// name is no longer the round's to give back, and its instant stays on
// record to run a second time.
func TestRoundThatLosesItsLeaseWhileWindingDownIsLoggedAsALoss(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	withFastLease(t)
	withRecordedFailures(t)

	entered, windingDown := make(chan struct{}, 1), make(chan struct{}, 1)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		windingDown <- struct{}{}
		// The round winds down until the renewals find the name taken.
		h, _ := lease.FromContext(ctx)
		for !h.Lost() {
			time.Sleep(5 * time.Millisecond)
		}
		return ctx.Err()
	}, "@every 1m", "winding-down-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")
	stopped := make(chan error, 1)
	go func() { stopped <- stop(context.Background()) }()
	awaitSignal(t, windingDown, "the round's wind-down")
	takeOver(t, "cron:winding-down-job")
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the scheduler must stop once the round returned")
	}
	pkgzap.Clean()

	interrupted := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob interrupted")
	require.Equal(t, "lease lost", interrupted["reason"], "the loss is what ended the round, not the shutdown before it")
	endLease(t, "cron:winding-down-job")
	require.Equal(t, []time.Time{time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC)}, unfinishedInstants(t, "cron:winding-down-job"))
}

// TestRoundInterruptedAtShutdownIsAWarning proves a round that stops because
// its context ended is not recorded as a failure, in the log or in the
// trace: the job returned the context's own cancellation, which is what it
// is asked to do when the process shuts down, and every rolling deployment
// ends a long round this way. The round's span carries the interruption as
// an event and no error status, so a search for failed rounds skips it.
func TestRoundInterruptedAtShutdownIsAWarning(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	entered := make(chan struct{}, 1)
	Register(func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}, "@every 1m", "interrupted-job")
	require.NoError(t, start(context.Background()))

	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")
	require.NoError(t, stop(context.Background()))

	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob interrupted")
	require.Equal(t, "interrupted-job", entry["name"])
	require.Equal(t, "shutting down", entry["reason"])
	require.Equal(t, "WARN", entry["level"])

	span := oteltest.EndedNamed(t, recorder, "cronjob.InterruptedJob")
	require.Equal(t, codes.Unset, span.Status().Code, "an interrupted round is neither a success nor a failure")
	require.Len(t, span.Events(), 1)
	require.Equal(t, "interrupted", span.Events()[0].Name)
	require.Equal(t, "shutting down", spanEventAttribute(span.Events()[0], "reason"))
}

// spanEventAttribute reads one string attribute of a span event.
func spanEventAttribute(event sdktrace.Event, key string) string {
	for _, kv := range event.Attributes {
		if string(kv.Key) == key {
			return kv.Value.AsString()
		}
	}
	return ""
}

// TestNeverMatchingScheduleEndsItsLoop proves a schedule with no instant
// left — a day that never comes — ends its loop with a warning instead of
// spinning on a zero instant.
func TestNeverMatchingScheduleEndsItsLoop(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	Register(noopJob, "0 0 0 30 2 *", "never-job")
	require.NoError(t, start(context.Background()))

	<-current.done
	pkgzap.Clean()
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "cronjob has no further instant")
	require.Equal(t, "never-job", entry["name"])
}

// TestRunLogsFailureWithErrorStack proves a failed round leaves an entry the
// error_stack field can locate. For a job the cronjob log is the only record
// of its failure, so the entry has to point at the failing line, not just
// name the job: the returned error goes out as a typed error field, and a
// panic becomes an error whose stack still points at the line that panicked.
func TestRunLogsFailureWithErrorStack(t *testing.T) {
	cases := []struct {
		name string
		job  func() error
		msg  string
		err  string
	}{
		{
			name: "returned error",
			job:  func() error { return errors.New("sample failure") },
			msg:  "finished cronjob with error",
			err:  "sample failure",
		},
		{
			name: "panic",
			job:  func() error { panic("sample panic") },
			msg:  "cronjob panicked",
			err:  "sample panic",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := withCronjobLoggerConfig(t)
			resetCronjobState(t)
			withBoundCronjobLogger(t)
			clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

			entered := make(chan struct{}, 1)
			Register(func(context.Context) error {
				entered <- struct{}{}
				return tc.job()
			}, "* * * * * *", "failing-job")
			require.NoError(t, start(context.Background()))

			clock.Advance(time.Second)
			awaitSignal(t, entered, "the round")
			// Stop waits for the in-flight round, whose outcome entry is
			// written before the round returns.
			require.NoError(t, stop(context.Background()))
			pkgzap.Clean()

			entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), tc.msg)
			require.Equal(t, tc.err, entry["error"])
			require.NotEmpty(t, entry[consts.TRACE_ID], "the outcome entry must carry the round's trace id")
			require.Contains(t, entry["error_stack"], "job_internal_test.go",
				"error_stack must point at the line inside the job that failed")
		})
	}
}

// TestRunStampsRoundIdentity proves the context a job runs on carries the
// round's identity and its lease, and that the outcome entry carries the
// same trace id, the instant the round ran for and the lease's term, so the
// round is found again from either side.
func TestRunStampsRoundIdentity(t *testing.T) {
	dir := withCronjobLoggerConfig(t)
	resetCronjobState(t)
	withBoundCronjobLogger(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	type observation struct {
		identity execctx.Identity
		term     uint64
		leased   bool
	}
	seen := make(chan observation, 1)
	Register(func(ctx context.Context) error {
		term, leased := lease.TermFromContext(ctx)
		seen <- observation{identity: execctx.FromContext(ctx), term: term, leased: leased}
		return nil
	}, "* * * * * *", "identity-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(time.Second)
	var got observation
	select {
	case got = <-seen:
	case <-time.After(5 * time.Second):
		t.Fatal("the round did not run")
	}
	require.NoError(t, stop(context.Background()))
	pkgzap.Clean()

	require.Equal(t, "identity-job", got.identity.Cronjob)
	require.NotEmpty(t, got.identity.TraceID)
	require.True(t, got.leased, "the round runs under the instant's lease")
	require.EqualValues(t, 1, got.term)
	entry := readLogEntry(t, filepath.Join(dir, "cronjob.log"), "finished cronjob")
	require.Equal(t, got.identity.TraceID, entry[consts.TRACE_ID])
	require.Equal(t, "2026-01-01T10:00:01Z", entry["at"], "the outcome entry names the instant the round ran for")
	require.EqualValues(t, 1, entry["term"], "the outcome entry names the lease's term")
}

// TestRunOpensRoundSpanWhenTracingIsOn proves a round gets a root span of its
// own when tracing is on: the job runs under it, and the round's trace id is
// the span's, so the trace in the tracing backend and the id in the logs and
// statement comments are one trail.
func TestRunOpensRoundSpanWhenTracingIsOn(t *testing.T) {
	withCronjobLoggerConfig(t)
	oteltest.Enable(t)
	recorder := oteltest.Record(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))

	seen := make(chan roundObservation, 1)
	Register(func(ctx context.Context) error {
		seen <- roundObservation{
			identity: execctx.FromContext(ctx),
			span:     oteltrace.SpanFromContext(ctx).SpanContext(),
		}
		return nil
	}, "* * * * * *", "traced-job")
	require.NoError(t, start(context.Background()))

	clock.Advance(time.Second)
	var got roundObservation
	select {
	case got = <-seen:
	case <-time.After(5 * time.Second):
		t.Fatal("the round did not run")
	}
	require.NoError(t, stop(context.Background()))
	pkgzap.Clean()

	require.True(t, got.span.HasTraceID(), "the job must run under the round's span")
	require.Equal(t, got.span.TraceID().String(), got.identity.TraceID)
	span := oteltest.EndedNamed(t, recorder, "cronjob.TracedJob")
	require.Equal(t, got.identity.TraceID, span.SpanContext().TraceID().String())
}

// TestRoundThatIgnoresTheLossFailsTheProcess proves the last line behind the
// lease is the scheduler's too: a round still running once its lease is lost
// and the grace has passed would run beside the next instant's round on
// another replica, so the process is failed. The failure is recorded instead
// of tripping the process-wide one, which is one-way and would keep the test
// from running twice.
func TestRoundThatIgnoresTheLossFailsTheProcess(t *testing.T) {
	withCronjobLoggerConfig(t)
	resetCronjobState(t)
	clock := withFakeClock(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	withFastLease(t)
	failures := withRecordedFailures(t)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	Register(func(context.Context) error {
		entered <- struct{}{}
		<-release
		return nil
	}, "@every 1m", "stubborn-job")
	require.NoError(t, start(context.Background()))
	// Released before the loops are drained, so the round ends within this
	// test instead of running on into the next one's loggers.
	t.Cleanup(func() { close(release) })

	clock.untilWaiting(t)
	clock.Advance(time.Minute)
	awaitSignal(t, entered, "the round")

	takeOver(t, "cron:stubborn-job")
	select {
	case err := <-failures:
		require.ErrorContains(t, err, `lease "cron:stubborn-job" was lost`)
	case <-time.After(5 * time.Second):
		t.Fatal("a round ignoring the loss must fail the process")
	}
}
