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

	violations := CheckTransactionClosureContext()

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

	violations := CheckTransactionClosureContext()

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
