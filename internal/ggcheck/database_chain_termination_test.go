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

func TestDatabaseChainTerminationFlagsStoredOrUnterminatedChains(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "user", "user.go"), `package user

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func stored(ctx context.Context) error {
	db := database.Database[*model.User](ctx)
	users := make([]*model.User, 0)
	return db.List(&users)
}

func storedAfterOption(ctx context.Context) error {
	db := database.Database[*model.User](ctx).WithLimit(10)
	users := make([]*model.User, 0)
	return db.List(&users)
}

func unterminated(ctx context.Context) {
	database.Database[*model.User](ctx).WithLimit(10)
}

func methodValue(ctx context.Context) {
	consume(database.Database[*model.User](ctx).List)
}

func consume(fn any) {}
`)
	// Aliased imports of the framework database package are still resolved.
	writeCheckFile(t, filepath.Join(projectDir, "cronjob", "cleanup.go"), `package cronjob

import (
	"context"

	gstdb "github.com/hydroan/gst/database"
	"tmpapp/model"
)

func keep(ctx context.Context) any {
	return gstdb.Database[*model.User](ctx)
}
`)

	violations := runCheck(ggcheck.DatabaseChainTermination)

	wantSubstrings := []string{
		filepath.Join("cronjob", "cleanup.go") + ":11:",
		filepath.Join("service", "user", "user.go") + ":11:",
		filepath.Join("service", "user", "user.go") + ":17:",
		filepath.Join("service", "user", "user.go") + ":23:",
		filepath.Join("service", "user", "user.go") + ":27:",
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
