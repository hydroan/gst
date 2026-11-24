package model_test

import (
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/model"
	"github.com/stretchr/testify/require"
)

// queuedSample is a table-backed model the queue tests register; what it
// declares beyond its table is beside their point.
type queuedSample struct {
	model.Base
}

func (*queuedSample) TableName() string { return "queued_samples" }

func TestTableDoneSignalsWaiters(t *testing.T) {
	// The queue and its wakeup are package-level state, so every subtest starts
	// from a drained signal and hands back a drained one.
	drain := func() {
		for {
			select {
			case <-model.TablesChanged():
			default:
				return
			}
		}
	}
	drain()
	t.Cleanup(drain)

	// enqueue counts one model as pending the way the database runtime sees it:
	// queued by Register, then taken off the queue and prepared.
	enqueue := func(t *testing.T, count int) {
		t.Helper()
		for range count {
			model.Register[*queuedSample]()
			<-model.TableChan
		}
	}

	t.Run("a finished table wakes a waiter", func(t *testing.T) {
		drain()
		enqueue(t, 1)
		require.Equal(t, 1, model.TablesPending())

		model.TableDone()

		require.Equal(t, 0, model.TablesPending())
		select {
		case <-model.TablesChanged():
		default:
			t.Fatal("TableDone left no wakeup for a waiter")
		}
	})

	t.Run("wakeups coalesce and never block", func(t *testing.T) {
		drain()
		// More finished tables than the signal buffer holds: the sends have to
		// fall through rather than block, because TableDone runs on the
		// goroutine that prepares the tables.
		enqueue(t, 4)
		for range 4 {
			model.TableDone()
		}

		require.Equal(t, 0, model.TablesPending())
		// One wakeup stands in for all four: it only says the count is worth
		// reading again, and the count is what a waiter trusts.
		<-model.TablesChanged()
		select {
		case <-model.TablesChanged():
			t.Fatal("the wakeups should have coalesced into one")
		default:
		}
	})
}

// RegisteredSample is a database-backed model, so registering it records and
// queues it.
type RegisteredSample struct {
	Name string

	model.Base
}

func (*RegisteredSample) TableName() string { return "registered_samples" }

// SkippedSample embeds Empty, so it maps to no table and registering it does
// nothing.
type SkippedSample struct {
	model.Empty
}

// PointerEmptySample, PointerBaseSample and PointerAutoBaseSample embed a base
// type through a pointer, the form Register rejects.
type PointerEmptySample struct {
	Name string

	*model.Empty
}

type PointerBaseSample struct {
	Name string

	*model.Base
}

func (*PointerBaseSample) TableName() string { return "pointer_base_samples" }

type PointerAutoBaseSample struct {
	Name string

	*model.AutoBase
}

func (*PointerAutoBaseSample) TableName() string { return "pointer_auto_base_samples" }

func TestRegister(t *testing.T) {
	registered := func() bool {
		return slices.ContainsFunc(model.RegisteredModels(), func(m any) bool {
			_, ok := m.(*RegisteredSample)
			return ok
		})
	}

	t.Run("a database backed model is recorded and queued", func(t *testing.T) {
		require.False(t, registered(), "the sample must not be registered before the subtest registers it")

		model.Register[*RegisteredSample]()

		// The recorded set is what a schema dump reads; the queue is what the
		// database runtime prepares tables from. Registering feeds both, which
		// is the whole point of having one entry point for it.
		require.True(t, registered())
		queued := <-model.TableChan
		require.IsType(t, &RegisteredSample{}, queued)
		// Report the model done so the pending count returns to where the
		// subtest found it.
		model.TableDone()
	})

	t.Run("an empty model maps to no table", func(t *testing.T) {
		before := len(model.RegisteredModels())

		model.Register[*SkippedSample]()

		require.Len(t, model.RegisteredModels(), before)
		require.Empty(t, model.TableChan)
	})

	t.Run("a base type embedded by pointer is rejected", func(t *testing.T) {
		before := len(model.RegisteredModels())

		require.PanicsWithValue(t, "model model_test.PointerEmptySample embeds *model.Empty; embed model.Empty by value: the framework recognizes model.Empty only when it is embedded by value", func() {
			model.Register[*PointerEmptySample]()
		})
		require.PanicsWithValue(t, "model model_test.PointerBaseSample embeds *model.Base; embed model.Base by value: the framework recognizes model.Base only when it is embedded by value", func() {
			model.Register[*PointerBaseSample]()
		})
		require.PanicsWithValue(t, "model model_test.PointerAutoBaseSample embeds *model.AutoBase; embed model.AutoBase by value: the framework recognizes model.AutoBase only when it is embedded by value", func() {
			model.Register[*PointerAutoBaseSample]()
		})
		require.Len(t, model.RegisteredModels(), before)
		require.Empty(t, model.TableChan)
	})

	t.Run("recorded models are handed out independently", func(t *testing.T) {
		require.True(t, registered(), "the first subtest registers the sample this one reads back")

		sample := func() *RegisteredSample {
			for _, m := range model.RegisteredModels() {
				if s, ok := m.(*RegisteredSample); ok {
					return s
				}
			}
			t.Fatal("the registered sample went missing")
			return nil
		}

		sample().Name = "mutated"

		require.Empty(t, sample().Name, "mutating a handed-out model must not change what is registered")
	})
}
