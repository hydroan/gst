package modelregistry_test

import (
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

func TestTableDoneSignalsWaiters(t *testing.T) {
	// The queue and its wakeup are package-level state, so every subtest starts
	// from a drained signal and hands back a drained one.
	drain := func() {
		for {
			select {
			case <-modelregistry.TablesChanged():
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
			modelregistry.Register[*IndexedSample]()
			<-modelregistry.TableChan
		}
	}

	t.Run("a_finished_table_wakes_a_waiter", func(t *testing.T) {
		drain()
		enqueue(t, 1)
		require.Equal(t, 1, modelregistry.TablesPending())

		modelregistry.TableDone()

		require.Equal(t, 0, modelregistry.TablesPending())
		select {
		case <-modelregistry.TablesChanged():
		default:
			t.Fatal("TableDone left no wakeup for a waiter")
		}
	})

	t.Run("wakeups_coalesce_and_never_block", func(t *testing.T) {
		drain()
		// More finished tables than the signal buffer holds: the sends have to
		// fall through rather than block, because TableDone runs on the
		// goroutine that prepares the tables.
		enqueue(t, 4)
		for range 4 {
			modelregistry.TableDone()
		}

		require.Equal(t, 0, modelregistry.TablesPending())
		// One wakeup stands in for all four: it only says the count is worth
		// reading again, and the count is what a waiter trusts.
		<-modelregistry.TablesChanged()
		select {
		case <-modelregistry.TablesChanged():
			t.Fatal("the wakeups should have coalesced into one")
		default:
		}
	})
}

// RegisteredSample is a database-backed model, so registering it records and
// queues it.
type RegisteredSample struct {
	Name string

	modelregistry.Base
}

func (*RegisteredSample) TableName() string { return "registered_samples" }

// SkippedSample embeds Empty, so it maps to no table and registering it does
// nothing.
type SkippedSample struct {
	modelregistry.Empty
}

// PointerEmptySample, PointerBaseSample and PointerAutoBaseSample embed a base
// type through a pointer, the form Register rejects.
type PointerEmptySample struct {
	Name string

	*modelregistry.Empty
}

type PointerBaseSample struct {
	Name string

	*modelregistry.Base
}

func (*PointerBaseSample) TableName() string { return "pointer_base_samples" }

type PointerAutoBaseSample struct {
	Name string

	*modelregistry.AutoBase
}

func (*PointerAutoBaseSample) TableName() string { return "pointer_auto_base_samples" }

func TestRegister(t *testing.T) {
	registered := func() bool {
		return slices.ContainsFunc(modelregistry.RegisteredModels(), func(m any) bool {
			_, ok := m.(*RegisteredSample)
			return ok
		})
	}

	t.Run("a_database_backed_model_is_recorded_and_queued", func(t *testing.T) {
		require.False(t, registered(), "the sample must not be registered before the subtest registers it")

		modelregistry.Register[*RegisteredSample]()

		// The recorded set is what a schema dump reads; the queue is what the
		// database runtime prepares tables from. Registering feeds both, which
		// is the whole point of having one entry point for it.
		require.True(t, registered())
		queued := <-modelregistry.TableChan
		require.IsType(t, &RegisteredSample{}, queued)
		// Report the model done so the pending count returns to where the
		// subtest found it.
		modelregistry.TableDone()
	})

	t.Run("an_empty_model_maps_to_no_table", func(t *testing.T) {
		before := len(modelregistry.RegisteredModels())

		modelregistry.Register[*SkippedSample]()

		require.Len(t, modelregistry.RegisteredModels(), before)
		require.Empty(t, modelregistry.TableChan)
	})

	t.Run("a_base_type_embedded_by_pointer_is_rejected", func(t *testing.T) {
		before := len(modelregistry.RegisteredModels())

		require.PanicsWithValue(t, "model modelregistry_test.PointerEmptySample embeds *model.Empty; embed model.Empty by value: the framework recognizes model.Empty only when it is embedded by value", func() {
			modelregistry.Register[*PointerEmptySample]()
		})
		require.PanicsWithValue(t, "model modelregistry_test.PointerBaseSample embeds *model.Base; embed model.Base by value: the framework recognizes model.Base only when it is embedded by value", func() {
			modelregistry.Register[*PointerBaseSample]()
		})
		require.PanicsWithValue(t, "model modelregistry_test.PointerAutoBaseSample embeds *model.AutoBase; embed model.AutoBase by value: the framework recognizes model.AutoBase only when it is embedded by value", func() {
			modelregistry.Register[*PointerAutoBaseSample]()
		})
		require.Len(t, modelregistry.RegisteredModels(), before)
		require.Empty(t, modelregistry.TableChan)
	})

	t.Run("recorded_models_are_handed_out_independently", func(t *testing.T) {
		require.True(t, registered(), "the first subtest registers the sample this one reads back")

		sample := func() *RegisteredSample {
			for _, m := range modelregistry.RegisteredModels() {
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
