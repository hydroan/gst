package ggcheck_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

func TestDatabaseChainTerminationAllowsInlineTerminatedChains(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "user", "user.go"), `package user

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func list(ctx context.Context) error {
	users := make([]*model.User, 0)
	return database.Database[*model.User](ctx).WithQuery(&model.User{}).WithLimit(10).List(&users)
}

func create(ctx context.Context, u *model.User) error {
	return database.Database[*model.User](ctx).Create(u)
}

func transact(ctx context.Context, u *model.User) error {
	return database.Transaction(ctx, func(ctx context.Context) error {
		return database.Database[*model.User](ctx).Create(u)
	})
}

func passInline(ctx context.Context) bool {
	return exists(database.Database[*model.User](ctx), "id")
}

func passInlineWithOptions(ctx context.Context) bool {
	return exists(database.Database[*model.User](ctx).WithLimit(1), "id")
}

func exists(db any, id string) bool { return db != nil }

// A chain held in a variable is terminated once, however it got there.
func held(ctx context.Context, dry bool) error {
	db := database.Database[*model.User](ctx)
	if dry {
		db = db.WithDryRun()
	}
	users := make([]*model.User, 0)
	return db.List(&users)
}

func heldAfterOption(ctx context.Context) error {
	db := database.Database[*model.User](ctx).WithLimit(10)
	users := make([]*model.User, 0)
	return db.List(&users)
}

func returned(ctx context.Context) any {
	return database.Database[*model.User](ctx)
}

func methodValue(ctx context.Context) {
	db := database.Database[*model.User](ctx)
	consume(db.List)
}

// The two terminations exclude each other, so the chain runs one.
func branches(ctx context.Context, counting bool) error {
	db := database.Database[*model.User](ctx)
	users := make([]*model.User, 0)
	if counting {
		count := 0
		return db.Count(&count)
	} else {
		return db.List(&users)
	}
}

func cases(ctx context.Context, kind int) error {
	db := database.Database[*model.User](ctx)
	users := make([]*model.User, 0)
	switch kind {
	case 1:
		return db.List(&users)
	default:
		count := 0
		return db.Count(&count)
	}
}

// A fresh chain bound to the same variable runs one operation of its own.
func rebound(ctx context.Context) error {
	db := database.Database[*model.User](ctx)
	count := 0
	if err := db.Count(&count); err != nil {
		return err
	}
	db = database.Database[*model.User](ctx)
	users := make([]*model.User, 0)
	return db.List(&users)
}

// A chain bound inside the loop runs one operation per iteration.
func perIteration(ctx context.Context, ids []string) error {
	for _, id := range ids {
		db := database.Database[*model.User](ctx)
		if err := db.Delete(&model.User{ID: id}); err != nil {
			return err
		}
	}
	return nil
}

func consume(fn any) {}
`)
	// A same-named non-gst package must not be treated as the framework database package.
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record.go"), `package record

import (
	"context"

	"tmpapp/pkg/database"
)

func keep(ctx context.Context) any {
	return database.Database[any](ctx)
}
`)
	// Test files are not checked, matching the other project checks.
	writeCheckFile(t, filepath.Join(projectDir, "service", "user", "user_test.go"), `package user

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func helperForTest(ctx context.Context) any {
	return database.Database[*model.User](ctx)
}
`)
	// Nested Go modules belong to other projects and are skipped.
	writeCheckFile(t, filepath.Join(projectDir, "legacy", "go.mod"), "module legacy\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "legacy", "legacy.go"), `package legacy

import (
	"context"

	"github.com/hydroan/gst/database"
)

func keep(ctx context.Context) any {
	return database.Database[any](ctx)
}
`)

	violations := runCheck(ggcheck.DatabaseChainTermination)

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestDatabaseChainTerminationFlagsUnterminatedAndReusedChains(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "user", "user.go"), `package user

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func unterminated(ctx context.Context) {
	database.Database[*model.User](ctx).WithLimit(10)
}

// The second operation runs on the chain the first one ran on.
func twice(ctx context.Context) error {
	db := database.Database[*model.User](ctx)
	count := 0
	if err := db.Count(&count); err != nil {
		return err
	}
	users := make([]*model.User, 0)
	return db.List(&users)
}

// A termination in an if body is followed by one after the if.
func afterBranch(ctx context.Context, counting bool) error {
	db := database.Database[*model.User](ctx)
	if counting {
		count := 0
		if err := db.Count(&count); err != nil {
			return err
		}
	}
	users := make([]*model.User, 0)
	return db.List(&users)
}

// The chain passed on is terminated by the helper, then again here.
func passedThenTerminated(ctx context.Context) error {
	db := database.Database[*model.User](ctx)
	if exists(db, "id") {
		return nil
	}
	users := make([]*model.User, 0)
	return db.List(&users)
}

// Two variables hold one chain.
func aliased(ctx context.Context) error {
	db := database.Database[*model.User](ctx)
	limited := db.WithLimit(1)
	users := make([]*model.User, 0)
	if err := limited.List(&users); err != nil {
		return err
	}
	count := 0
	return db.Count(&count)
}

// A chain bound outside the loop is terminated once per iteration.
func inLoop(ctx context.Context, ids []string) error {
	db := database.Database[*model.User](ctx)
	for _, id := range ids {
		if err := db.Delete(&model.User{ID: id}); err != nil {
			return err
		}
	}
	return nil
}

func exists(db any, id string) bool { return db != nil }
`)

	violations := runCheck(ggcheck.DatabaseChainTermination)

	wantSubstrings := []string{
		filepath.Join("service", "user", "user.go") + ":11: database.Database chain never calls a terminal operation",
		filepath.Join("service", "user", "user.go") + ":22: database.Database chain held in db is terminated again, first at line 18",
		filepath.Join("service", "user", "user.go") + ":35: database.Database chain held in db is terminated again, first at line 30",
		filepath.Join("service", "user", "user.go") + ":45: database.Database chain held in db is terminated again, first at line 41",
		filepath.Join("service", "user", "user.go") + ":57: database.Database chain held in db is terminated again, first at line 53",
		filepath.Join("service", "user", "user.go") + ":64: database.Database chain held in db is terminated inside a loop it was bound outside of",
	}
	if len(violations) != len(wantSubstrings) {
		t.Fatalf("expected %d violations, got %#v", len(wantSubstrings), violations)
	}
	for i, want := range wantSubstrings {
		if !strings.Contains(violations[i], want) {
			t.Fatalf("violation %d should contain %q, got %q", i, want, violations[i])
		}
	}
}
