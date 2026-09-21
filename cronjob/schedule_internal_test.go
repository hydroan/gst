package cronjob

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/require"
)

// TestSchedulesAreReadInUTC proves an expression is read in UTC — the zone
// the schedule computes in is UTC, not the process's, so every replica
// computes the same instants — and that an expression naming its own zone
// with a CRON_TZ= prefix keeps that zone. The zone is asserted on the
// schedule itself: on a host whose own zone is UTC the instants alone could
// not tell the two apart.
func TestSchedulesAreReadInUTC(t *testing.T) {
	resetCronjobState(t)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		spec string
		zone string
		next time.Time
	}{
		{spec: "0 0 2 * * *", zone: "UTC", next: time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)},
		{spec: "@daily", zone: "UTC", next: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		// Midnight UTC is 08:00 in Shanghai, so the next 02:00 there is the
		// following day's: 18:00 UTC.
		{spec: "CRON_TZ=Asia/Shanghai 0 0 2 * * *", zone: "Asia/Shanghai", next: time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			j, err := newJob(noopJob, tc.spec, "zone-job")
			require.NoError(t, err)
			spec, ok := j.schedule.(*cron.SpecSchedule)
			require.True(t, ok, "an expression parses to a spec schedule")
			require.Equal(t, tc.zone, spec.Location.String(), "the zone the schedule is read in")
			got := j.schedule.Next(from)
			require.True(t, got.Equal(tc.next), "want %s, got %s", tc.next, got.UTC())
		})
	}
}

// TestEveryRunsOnTheEpochGrid proves "@every" instants are the multiples of
// the period from the Unix epoch, not counted from the process start: the
// next instant after 10:03:20 for "@every 5m" is 10:05:00 on every replica,
// and after 10:05:00 exactly it is 10:10:00.
func TestEveryRunsOnTheEpochGrid(t *testing.T) {
	resetCronjobState(t)

	j, err := newJob(noopJob, "@every 5m", "grid-job")
	require.NoError(t, err)

	got := j.schedule.Next(time.Date(2026, 1, 1, 10, 3, 20, 0, time.UTC))
	require.Equal(t, time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC), got)
	got = j.schedule.Next(time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC))
	require.Equal(t, time.Date(2026, 1, 1, 10, 10, 0, 0, time.UTC), got)
}

// TestPreviousInstantFindsTheMostRecentOne proves the bisection behind the
// catch-up: it finds the most recent instant at or before now, an instant
// falling on now itself included, and reports none when the schedule had no
// instant within the last day.
func TestPreviousInstantFindsTheMostRecentOne(t *testing.T) {
	resetCronjobState(t)

	every, err := newJob(noopJob, "@every 5m", "grid-job")
	require.NoError(t, err)
	daily, err := newJob(noopJob, "0 0 2 * * *", "daily-job")
	require.NoError(t, err)
	yearly, err := newJob(noopJob, "0 0 0 1 1 *", "yearly-job")
	require.NoError(t, err)

	prev, ok := previousInstant(every.schedule, time.Date(2026, 1, 1, 10, 3, 20, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), prev)

	prev, ok = previousInstant(every.schedule, time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC), prev, "an instant falling on now counts as passed")

	prev, ok = previousInstant(daily.schedule, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC), prev)

	_, ok = previousInstant(yearly.schedule, time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC))
	require.False(t, ok, "an instant older than a day is history, not a missed round")
}
