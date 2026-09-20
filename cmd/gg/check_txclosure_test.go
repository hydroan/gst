package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckTransactionClosureContextFlagsOuterContext(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "sample.go"), `package sample

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func update(outerCtx context.Context, record *model.Record) error {
	return database.Transaction(outerCtx, func(ctx context.Context) error {
		if err := database.Database[*model.Record](ctx).Update(record); err != nil {
			return err
		}
		return database.Database[*model.Record](outerCtx).Update(record)
	})
}
`)
	// A nested Transaction call must join the enclosing transaction through the
	// closure context; passing any other context starts a separate transaction.
	writeCheckFile(t, filepath.Join(projectDir, "service", "nested", "nested.go"), `package nested

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func update(outerCtx context.Context, record *model.Record) error {
	return database.Transaction(outerCtx, func(ctx context.Context) error {
		return database.Transaction(outerCtx, func(ctx context.Context) error {
			return database.Database[*model.Record](ctx).Update(record)
		})
	})
}
`)

	violations := CheckTransactionClosureContext(newProjectIgnoreMatcher())

	wantSubstrings := []string{
		filepath.Join("service", "nested", "nested.go") + ":12:",
		filepath.Join("service", "sample", "sample.go") + ":15:",
	}
	if len(violations) != len(wantSubstrings) {
		t.Fatalf("expected %d violations, got %#v", len(wantSubstrings), violations)
	}
	for i, want := range wantSubstrings {
		if !strings.Contains(violations[i], want) {
			t.Fatalf("violation %d should contain %q, got %q", i, want, violations[i])
		}
	}
	if !strings.Contains(violations[1], "outerCtx") {
		t.Fatalf("violation should name the escaping identifier, got %q", violations[1])
	}
}

// TestCheckTransactionClosureContextFlagsEveryEntryPoint covers the calls
// besides a chain that leave the transaction the same way: a select, an
// after-commit registration — which would run its action at once instead of
// after the commit — and a context detached at the call, written plainly or
// through a derivation.
func TestCheckTransactionClosureContextFlagsEveryEntryPoint(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "entry", "entry.go"), `package entry

import (
	"context"
	"time"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func update(outerCtx context.Context, record *model.Record) error {
	return database.Transaction(outerCtx, func(ctx context.Context) error {
		rows := make([]*model.Record, 0)
		if err := database.Select[*model.Record, model.Record](outerCtx).Scan(&rows); err != nil {
			return err
		}
		if err := database.AfterCommit(outerCtx, func(context.Context) error { return nil }); err != nil {
			return err
		}
		if err := database.Database[*model.Record](context.Background()).Update(record); err != nil {
			return err
		}
		bounded, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := database.Database[*model.Record](bounded).Update(record); err != nil {
			return err
		}
		// Derived from the closure's own context: still inside the transaction.
		own, stop := context.WithTimeout(ctx, time.Second)
		defer stop()
		return database.Database[*model.Record](own).Update(record)
	})
}
`)

	violations := CheckTransactionClosureContext(newProjectIgnoreMatcher())
	joined := strings.Join(violations, "\n")
	for _, want := range []string{
		"database.Select uses context \"outerCtx\"",
		"database.AfterCommit uses context \"outerCtx\"",
		"database.Database uses context context.Background()",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected a violation naming %s, got:\n%s", want, joined)
		}
	}
	// A context bounded from the closure's own still carries the transaction,
	// so the statement under it is not flagged; one bounded from a detached
	// context is, by the name it was given.
	if strings.Contains(joined, "context \"own\"") {
		t.Fatalf("a context derived from the closure's own must pass, got:\n%s", joined)
	}
	if want := "database.Database uses context \"bounded\""; !strings.Contains(joined, want) {
		t.Fatalf("expected a violation naming %s, got:\n%s", want, joined)
	}
}

func TestCheckTransactionClosureContextAllowsClosureContext(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "sample.go"), `package sample

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func update(ctx context.Context, record *model.Record) error {
	return database.Transaction(ctx, func(ctx context.Context) error {
		if err := database.Database[*model.Record](ctx).Update(record); err != nil {
			return err
		}
		return database.Database[*model.Record](ctx).Update(record)
	})
}

func nested(ctx context.Context, record *model.Record) error {
	return database.Transaction(ctx, func(ctx context.Context) error {
		return database.Transaction(ctx, func(txCtx context.Context) error {
			return database.Database[*model.Record](txCtx).Update(record)
		})
	})
}
`)
	// Aliased imports of the framework database package are still resolved.
	writeCheckFile(t, filepath.Join(projectDir, "cronjob", "cleanup.go"), `package cronjob

import (
	"context"

	gstdb "github.com/hydroan/gst/database"
	"tmpapp/model"
)

func cleanup(ctx context.Context, record *model.Record) error {
	return gstdb.Transaction(ctx, func(ctx context.Context) error {
		return gstdb.Database[*model.Record](ctx).Delete(record)
	})
}
`)

	violations := CheckTransactionClosureContext(newProjectIgnoreMatcher())

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
