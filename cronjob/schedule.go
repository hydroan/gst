package cronjob

import (
	"context"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// This file holds the schedule arithmetic: reading an expression, pinning it
// to UTC so every replica computes the same instants, and finding the instant
// before a moment for the start-up catch-up. The clock the scheduler goes by
// lives here too, so a test can drive a schedule of hours in microseconds.

// parser reads the schedules: six fields with seconds first, plus the
// descriptors — the syntax the scaffold documents.
var parser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// zoneOf returns the zone a schedule names with its CRON_TZ= or TZ= prefix —
// the prefixes the parser reads — and whether it names one at all.
func zoneOf(spec string) (zone string, named bool) {
	first, _, _ := strings.Cut(spec, " ")
	if zone, named = strings.CutPrefix(first, "CRON_TZ="); named {
		return zone, true
	}
	return strings.CutPrefix(first, "TZ=")
}

// inUTC pins a parsed schedule to UTC: an expression that names no zone is
// read in UTC instead of the process's zone, and "@every" runs on the epoch
// grid instead of counting from the process start. Every replica then
// computes the same instants for a job.
func inUTC(s cron.Schedule) cron.Schedule {
	switch s := s.(type) {
	case *cron.SpecSchedule:
		// The parser leaves Location at time.Local for an expression that
		// names no zone; one with a CRON_TZ= prefix gets that zone, which
		// stays. An expression naming Local itself never gets here — newJob
		// refuses it — so time.Local can only mean no zone was named.
		if s.Location == time.Local {
			s.Location = time.UTC
		}
		return s
	case cron.ConstantDelaySchedule:
		return everySchedule{period: s.Delay}
	default:
		return s
	}
}

// everySchedule fires every period, on the multiples of the period counted
// from the Unix epoch: "@every 5m" runs at :00, :05, :10 on every replica
// alike, where counting from the process start — what the parser's own
// schedule does — puts each replica on a grid of its own.
type everySchedule struct {
	period time.Duration
}

// Next returns the first grid instant after t.
func (s everySchedule) Next(t time.Time) time.Time {
	period := s.period.Nanoseconds()
	if period <= 0 {
		return time.Time{}
	}
	return time.Unix(0, (t.UnixNano()/period+1)*period).UTC()
}

// catchUpLookback bounds how far back the start-up catch-up looks for a
// job's most recent instant: one older than this is history, not a
// missed round.
const catchUpLookback = 24 * time.Hour

// previousInstant returns the most recent instant of s at or before now,
// looking back catchUpLookback at most. The schedule only answers "the first
// instant after t", so the instant is found by bisection over t: the answer
// stays at or before now for every t before the instant and moves past now
// from the instant on.
func previousInstant(s cron.Schedule, now time.Time) (time.Time, bool) {
	lo, hi := now.Add(-catchUpLookback), now
	if first := s.Next(lo); first.IsZero() || first.After(now) {
		return time.Time{}, false
	}
	for hi.Sub(lo) > time.Millisecond {
		mid := lo.Add(hi.Sub(lo) / 2)
		if next := s.Next(mid); !next.IsZero() && !next.After(now) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return s.Next(lo), true
}

// clock is the time as the scheduler sees it: the moment now, and a wait
// until a moment. The scheduler never sleeps on its own, so a test can hand
// it a clock it drives by hand.
type clock interface {
	Now() time.Time
	// Wait blocks until t has passed or ctx ends, and reports whether t
	// passed.
	Wait(ctx context.Context, t time.Time) bool
}

// systemClock is the clock of a running process.
type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

func (systemClock) Wait(ctx context.Context, t time.Time) bool {
	if ctx.Err() != nil {
		return false
	}
	delay := time.Until(t)
	if delay <= 0 {
		return true
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
