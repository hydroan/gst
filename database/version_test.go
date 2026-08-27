package database_test

import (
	"context"
	"sync"
	"testing"

	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

var (
	colVersionedNoteTitle   = types.NewColumn[string]("title")
	colVersionedNoteVersion = types.NewColumn[int64]("version")
)

// versionedNote is the optimistic-locking fixture: a model that declares
// model.Version and therefore takes part in the whole per-operation contract
// documented on that type.
type versionedNote struct {
	Title   string        `json:"title" gorm:"size:191"`
	Body    string        `json:"body" gorm:"size:191"`
	Version model.Version `json:"version" gorm:"not null;default:1"`

	model.Base
}

func (*versionedNote) TableName() string { return "versioned_notes" }

func setupVersionedNotes(t *testing.T) {
	t.Helper()
	require.NoError(t, database.DB().AutoMigrate(&versionedNote{}))
	t.Cleanup(func() {
		require.NoError(t, database.DB().Migrator().DropTable(&versionedNote{}))
	})
}

func mustCreateNote(t *testing.T, id, title string) *versionedNote {
	t.Helper()
	note := &versionedNote{Title: title}
	note.ID = id
	require.NoError(t, database.Database[*versionedNote](context.Background()).Create(note))
	return note
}

func reloadNote(t *testing.T, id string) *versionedNote {
	t.Helper()
	note := new(versionedNote)
	require.NoError(t, database.Database[*versionedNote](context.Background()).Get(note, id))
	return note
}

func TestVersionCreateInitializes(t *testing.T) {
	setupVersionedNotes(t)
	ctx := context.Background()

	// A zero version starts at 1, in the object and in the row.
	created := mustCreateNote(t, "note-init", "first")
	require.EqualValues(t, 1, created.Version)
	require.EqualValues(t, 1, reloadNote(t, "note-init").Version)

	// A carried version is kept, so imports can bring history along.
	imported := &versionedNote{Title: "imported", Version: 7}
	imported.ID = "note-imported"
	require.NoError(t, database.Database[*versionedNote](ctx).Create(imported))
	require.EqualValues(t, 7, reloadNote(t, "note-imported").Version)
}

func TestVersionUpdateGuards(t *testing.T) {
	setupVersionedNotes(t)
	ctx := context.Background()
	note := mustCreateNote(t, "note-upd", "v1")

	// A successful update bumps the row and the object.
	note.Title = "v2"
	require.NoError(t, database.Database[*versionedNote](ctx).Update(note))
	require.EqualValues(t, 2, note.Version)
	reloaded := reloadNote(t, "note-upd")
	require.Equal(t, "v2", reloaded.Title)
	require.EqualValues(t, 2, reloaded.Version)

	// A stale version matches nothing: ErrStaleObject, row untouched, and
	// the object still carries the version it was handed in with.
	stale := &versionedNote{Title: "stale-write", Version: 1}
	stale.ID = "note-upd"
	err := database.Database[*versionedNote](ctx).Update(stale)
	require.ErrorIs(t, err, database.ErrStaleObject)
	require.EqualValues(t, 1, stale.Version, "a failed update restores the carried version")
	require.Equal(t, "v2", reloadNote(t, "note-upd").Title)

	// A zero version is rejected before any database work.
	shell := &versionedNote{Title: "shell"}
	shell.ID = "note-upd"
	require.ErrorIs(t, database.Database[*versionedNote](ctx).Update(shell), database.ErrVersionRequired)

	// Updating a deleted record is stale, not "not found": the record moved
	// on after this caller read it, and the handling is the same — reload.
	current := reloadNote(t, "note-upd")
	deleteShell := new(versionedNote)
	deleteShell.ID = "note-upd"
	require.NoError(t, database.Database[*versionedNote](ctx).Delete(deleteShell))
	require.ErrorIs(t, database.Database[*versionedNote](ctx).Update(current), database.ErrStaleObject)
}

func TestVersionUpdateNarrowSelect(t *testing.T) {
	setupVersionedNotes(t)
	ctx := context.Background()
	note := mustCreateNote(t, "note-narrow", "v1")
	note.Body = "kept-out"

	// A narrowed update still checks and still bumps: the version column is
	// widened into the selection so other carried versions expire.
	note.Title = "narrowed"
	require.NoError(t, database.Database[*versionedNote](ctx).
		WithSelect(colVersionedNoteTitle).Update(note))
	reloaded := reloadNote(t, "note-narrow")
	require.Equal(t, "narrowed", reloaded.Title)
	require.Empty(t, reloaded.Body, "unselected columns stay untouched")
	require.EqualValues(t, 2, reloaded.Version)

	// The same narrowed write with the now-stale version fails.
	stale := &versionedNote{Title: "stale-narrow", Version: 1}
	stale.ID = "note-narrow"
	require.ErrorIs(t,
		database.Database[*versionedNote](ctx).WithSelect(colVersionedNoteTitle).Update(stale),
		database.ErrStaleObject)
}

func TestVersionDeleteGuards(t *testing.T) {
	setupVersionedNotes(t)
	ctx := context.Background()

	// A delete carrying a stale version fails: the decision was made over
	// data that moved on.
	note := mustCreateNote(t, "note-del", "v1")
	loaded := reloadNote(t, "note-del")
	note.Title = "v2"
	require.NoError(t, database.Database[*versionedNote](ctx).Update(note))
	require.ErrorIs(t, database.Database[*versionedNote](ctx).Delete(loaded), database.ErrStaleObject)

	// Carrying the current version deletes, and the soft-deleted row keeps
	// its version frozen: soft deletion already expires every carried
	// version, so there is nothing left for a bump to do.
	current := reloadNote(t, "note-del")
	require.NoError(t, database.Database[*versionedNote](ctx).Delete(current))
	gone := new(versionedNote)
	require.NoError(t, database.Database[*versionedNote](ctx).WithDeleted().Get(gone, "note-del"))
	require.EqualValues(t, 2, gone.Version)

	// A bare-id shell deletes unconditionally: the deliberate way around the
	// lock for cascade cleanup and programmatic deletion.
	shellTarget := mustCreateNote(t, "note-del-shell", "v1")
	shellTarget.Title = "moved-on"
	require.NoError(t, database.Database[*versionedNote](ctx).Update(shellTarget))
	shell := new(versionedNote)
	shell.ID = "note-del-shell"
	require.NoError(t, database.Database[*versionedNote](ctx).Delete(shell))
	count := new(int)
	require.NoError(t, database.Database[*versionedNote](ctx).WithQuery(&versionedNote{Title: "moved-on"}).Count(count))
	require.Zero(t, *count)

	// Hard deletion checks the same way.
	hard := mustCreateNote(t, "note-del-hard", "v1")
	staleHard := &versionedNote{Version: 0}
	staleHard.ID = "note-del-hard"
	staleHard.Version = hard.Version + 1
	require.ErrorIs(t, database.Database[*versionedNote](ctx).WithPurge().Delete(staleHard), database.ErrStaleObject)
	require.NoError(t, database.Database[*versionedNote](ctx).WithPurge().Delete(hard))
}

func TestVersionUpdateByIDBumps(t *testing.T) {
	setupVersionedNotes(t)
	ctx := context.Background()
	mustCreateNote(t, "note-byid", "v1")

	// UpdateByID waives the check but still bumps, so carried versions
	// expire: the full update below carries version 1 and must fail.
	require.NoError(t, database.Database[*versionedNote](ctx).
		UpdateByID("note-byid", colVersionedNoteTitle.Set("patched")))
	reloaded := reloadNote(t, "note-byid")
	require.Equal(t, "patched", reloaded.Title)
	require.EqualValues(t, 2, reloaded.Version)

	stale := &versionedNote{Title: "late-save", Version: 1}
	stale.ID = "note-byid"
	require.ErrorIs(t, database.Database[*versionedNote](ctx).Update(stale), database.ErrStaleObject)

	// An explicit version assignment takes the bump over.
	require.NoError(t, database.Database[*versionedNote](ctx).
		UpdateByID("note-byid", colVersionedNoteVersion.Set(41)))
	require.EqualValues(t, 41, reloadNote(t, "note-byid").Version)
}

func TestVersionUpsert(t *testing.T) {
	setupVersionedNotes(t)
	ctx := context.Background()

	// The insert path starts at version 1 like Create.
	note := &versionedNote{Title: "v1"}
	note.ID = "note-upsert"
	require.NoError(t, database.Database[*versionedNote](ctx).Upsert(note))
	require.EqualValues(t, 1, reloadNote(t, "note-upsert").Version)

	// Move the row ahead, then upsert with an object still carrying the old
	// version: the conflict update overwrites the payload — merge-overwrite
	// is the point — but bumps the ROW's version instead of writing the
	// object's, so the row can never move backwards.
	current := reloadNote(t, "note-upsert")
	current.Title = "v2"
	require.NoError(t, database.Database[*versionedNote](ctx).Update(current)) // row is at version 2 now

	staleUpsert := &versionedNote{Title: "merged", Version: 1}
	staleUpsert.ID = "note-upsert"
	require.NoError(t, database.Database[*versionedNote](ctx).Upsert(staleUpsert))
	reloaded := reloadNote(t, "note-upsert")
	require.Equal(t, "merged", reloaded.Title)
	require.EqualValues(t, 3, reloaded.Version, "conflict updates bump the row's own version")
}

func TestVersionConcurrentUpdateOneWins(t *testing.T) {
	setupVersionedNotes(t)
	mustCreateNote(t, "note-race", "v1")

	// Two writers hold the same version; exactly one write wins and the
	// other observes ErrStaleObject.
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			attempt := &versionedNote{Title: "winner", Version: 1}
			attempt.ID = "note-race"
			results[slot] = database.Database[*versionedNote](context.Background()).Update(attempt)
		}(i)
	}
	wg.Wait()

	winners, losers := 0, 0
	for _, err := range results {
		if err == nil {
			winners++
			continue
		}
		require.ErrorIs(t, err, database.ErrStaleObject)
		losers++
	}
	require.Equal(t, 1, winners)
	require.Equal(t, 1, losers)
	require.EqualValues(t, 2, reloadNote(t, "note-race").Version)
}

func TestVersionBatchUpdateRollsBack(t *testing.T) {
	setupVersionedNotes(t)
	ctx := context.Background()
	first := mustCreateNote(t, "note-batch-a", "a1")
	second := mustCreateNote(t, "note-batch-b", "b1")

	// Move the second row ahead so the batch below carries one fresh and one
	// stale version.
	moved := reloadNote(t, "note-batch-b")
	moved.Title = "b2"
	require.NoError(t, database.Database[*versionedNote](ctx).Update(moved))

	first.Title = "a2"
	second.Title = "b-stale"
	err := database.Database[*versionedNote](ctx).Update(first, second)
	require.ErrorIs(t, err, database.ErrStaleObject)

	// All-or-nothing: the first row rolled back with the batch, and both
	// objects carry the versions their rows still have.
	require.Equal(t, "a1", reloadNote(t, "note-batch-a").Title)
	require.EqualValues(t, 1, first.Version, "a rolled-back update restores the carried version")
	require.EqualValues(t, 1, second.Version)
}

// legacyAdoptedNote is the adoption fixture. It deliberately has a table of
// its own: the scenario is a table no versioned test has ever touched, and a
// shared table would also trip pgx's statement cache on the DROP COLUMN
// below (a runtime-DDL-only hazard the framework's migrate-then-deploy
// order never hits in production).
type legacyAdoptedNote struct {
	Title   string        `json:"title" gorm:"size:191"`
	Version model.Version `json:"version" gorm:"not null;default:1"`

	model.Base
}

func (*legacyAdoptedNote) TableName() string { return "legacy_adopted_notes" }

func TestVersionLegacyTableAdoption(t *testing.T) {
	// The adoption path: a table that already holds rows gains the version
	// column through a migration. The default:1 in the declared tag is what
	// backfills those rows to a live version — without it they would be
	// backfilled to zero and locked out of Update forever. This test pins
	// that the documented tag shape keeps legacy rows updatable.
	ctx := context.Background()
	require.NoError(t, database.DB().AutoMigrate(&legacyAdoptedNote{}))
	t.Cleanup(func() {
		require.NoError(t, database.DB().Migrator().DropTable(&legacyAdoptedNote{}))
	})

	// Rewind the table to its pre-adoption shape and insert a legacy row the
	// way a pre-lock deployment would have: without a version column.
	require.NoError(t, database.DB().Exec("ALTER TABLE legacy_adopted_notes DROP COLUMN version").Error)
	require.NoError(t, database.DB().Exec(
		"INSERT INTO legacy_adopted_notes (id, title, created_at, updated_at) VALUES (?, ?, NOW(), NOW())",
		"legacy-note", "written-before-adoption").Error)

	// Adoption: the migration adds the column, and the database backfills
	// the existing row with the declared default.
	require.NoError(t, database.DB().AutoMigrate(&legacyAdoptedNote{}))

	adopted := new(legacyAdoptedNote)
	require.NoError(t, database.Database[*legacyAdoptedNote](ctx).Get(adopted, "legacy-note"))
	require.EqualValues(t, 1, adopted.Version, "legacy rows must come back at version 1, not zero")

	// The legacy row is a first-class citizen of the lock from here on.
	adopted.Title = "updated-after-adoption"
	require.NoError(t, database.Database[*legacyAdoptedNote](ctx).Update(adopted))
	require.EqualValues(t, 2, adopted.Version)
}

func TestUnversionedNotFoundUnchanged(t *testing.T) {
	// The regression guard: models without model.Version keep the plain
	// contract, where a missing row is ErrRecordNotFound.
	defer cleanupTestData()
	setupTestData(t)

	ghost := &TestUser{Name: "ghost"}
	ghost.ID = "no-such-user"
	require.ErrorIs(t,
		database.Database[*TestUser](context.Background()).Update(ghost),
		database.ErrRecordNotFound)
}
