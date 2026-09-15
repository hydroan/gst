package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestDatabaseEntryPointsMatchThePackage pins the dot-import list to the
// database package itself: every exported function whose first parameter is
// a context is an entry point the rule has to know, so a function added to
// the package cannot slip past a file that dot-imports it.
func TestDatabaseEntryPointsMatchThePackage(t *testing.T) {
	sources, err := filepath.Glob(filepath.Join(frameworkRepoRoot(t), "database", "*.go"))
	if err != nil {
		t.Fatal(err)
	}

	var want []string
	fset := token.NewFileSet()
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, source, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() || len(fn.Type.Params.List) == 0 {
				continue
			}
			if selector, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); ok && selector.Sel.Name == "Context" {
				want = append(want, fn.Name.Name)
			}
		}
	}
	slices.Sort(want)

	got := slices.Clone(databaseEntryPoints)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("databaseEntryPoints must list the package's context-taking functions:\n got %v\nwant %v", got, want)
	}
}

// TestCheckDetachedContextFlagsBackgroundAtDatabaseEntry pins the rule's
// reach: a detached context handed to a framework database function or a
// dao function is flagged in the directories that run under a context —
// written at the call, held in a variable first, derived through the context
// package, under dot imports, and through a dao package whose name differs
// from its directory's — a generic instantiation included, while the context
// handed down passes, a variable that also receives a real context passes,
// a closure's variable does not shadow the enclosing function's parameter,
// copied framework modules are the framework's to check, ignored directories
// are not read, and startup seeding in a module package stays outside the
// rule.
func TestCheckDetachedContextFlagsBackgroundAtDatabaseEntry(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	writeFrameworkModuleFixture(t, projectDir, "sample")
	writeCheckFile(t, filepath.Join(projectDir, ".gitignore"), "service/scratch/\n")

	// A variable holding a detached context is the context at the call.
	writeCheckFile(t, filepath.Join(projectDir, "leader", "counter.go"), `package leader

import (
	"context"

	"tmpapp/dao"
)

func count(context.Context) error {
	ctx := context.Background()
	_, err := dao.Records(ctx)
	return err
}
`)
	// A context derived from a detached one is detached; one derived from
	// the context handed down is not.
	writeCheckFile(t, filepath.Join(projectDir, "lock", "rebuild.go"), `package lock

import (
	"context"
	"time"

	"github.com/hydroan/gst/database"
)

func rebuild(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := database.Transaction(bounded, func(context.Context) error { return nil }); err != nil {
		return err
	}
	return database.Transaction(context.WithoutCancel(context.Background()), func(context.Context) error { return nil })
}
`)
	// Dot imports of both packages leave the calls unqualified.
	writeCheckFile(t, filepath.Join(projectDir, "service", "sweep", "sweep.go"), `package sweep

import (
	. "context"

	. "github.com/hydroan/gst/database"
)

func sweep() error {
	return Transaction(Background(), func(Context) error { return nil })
}
`)
	// A dao package named unlike its directory is used under its own name.
	writeCheckFile(t, filepath.Join(projectDir, "dao", "menu", "menu.go"), `package menudao

import "context"

func Seed(context.Context) error { return nil }
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "menu", "menu.go"), `package menu

import (
	"context"

	"tmpapp/dao/menu"
)

func seed() error {
	return menudao.Seed(context.TODO())
}
`)
	// A parameter overwritten with a detached context, and a variable
	// declared empty and then given one, both hold a detached context at
	// the call.
	writeCheckFile(t, filepath.Join(projectDir, "service", "override", "override.go"), `package override

import (
	"context"

	"tmpapp/dao"
)

func override(ctx context.Context) error {
	ctx = context.Background()
	_, err := dao.Records(ctx)
	return err
}

func declared() error {
	var ctx context.Context
	ctx = context.Background()
	_, err := dao.Records(ctx)
	return err
}
`)
	// A dao package under a dot import cannot be checked, and says so.
	writeCheckFile(t, filepath.Join(projectDir, "service", "dotted", "dotted.go"), `package dotted

import (
	"context"

	. "tmpapp/dao"
)

func dotted() error {
	_, err := Records(context.Background())
	return err
}
`)
	// A variable whose last value is the context handed down is left alone:
	// the check reads the syntax in source order, not the flow.
	writeCheckFile(t, filepath.Join(projectDir, "service", "either", "either.go"), `package either

import (
	"context"

	"tmpapp/dao"
)

func either(handed context.Context, detached bool) error {
	ctx := context.Background()
	if !detached {
		ctx = handed
	}
	_, err := dao.Records(ctx)
	return err
}
`)
	// A closure's ctx is the closure's: the parameter of the enclosing
	// function is not shadowed by it, and the detached one is only flagged
	// where it is used.
	writeCheckFile(t, filepath.Join(projectDir, "service", "shadow", "shadow.go"), `package shadow

import (
	"context"

	"github.com/hydroan/gst/database"
)

func shadow(ctx context.Context) error {
	if err := database.Transaction(ctx, func(context.Context) error { return nil }); err != nil {
		return err
	}
	run := func() error {
		ctx := context.Background()
		return database.Transaction(ctx, func(context.Context) error { return nil })
	}
	return run()
}
`)
	// An ignored directory is not read.
	writeCheckFile(t, filepath.Join(projectDir, "service", "scratch", "scratch.go"), `package scratch

import (
	"context"

	"tmpapp/dao"
)

func scratch() error {
	_, err := dao.Records(context.Background())
	return err
}
`)

	// A copied module subtree keeps whatever the framework repository ships.
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "copied.go"), `package sample

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func copied(record *model.Record) error {
	return database.Database[*model.Record](context.Background()).Create(record)
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "record", "record.go"), `package record

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func refresh(ctx context.Context, record *model.Record) error {
	if err := database.Transaction(ctx, func(ctx context.Context) error {
		return database.Database[*model.Record](ctx).Update(record)
	}); err != nil {
		return err
	}
	return database.Transaction(context.Background(), func(ctx context.Context) error {
		return database.Database[*model.Record](ctx).Update(record)
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "dao", "record.go"), `package dao

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func Records() ([]*model.Record, error) {
	records := make([]*model.Record, 0)
	err := database.Database[*model.Record](context.TODO()).List(&records)
	return records, err
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "cronjob", "sweep.go"), `package cronjob

import (
	"context"

	"tmpapp/dao"
)

func sweep(context.Context) error {
	_, err := dao.Records(context.Background())
	return err
}
`)
	// Seeding at startup has no context to inherit; it lives outside the
	// directories the rule covers and passes its context down from there.
	writeCheckFile(t, filepath.Join(projectDir, "module", "seed.go"), `package module

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func seed() error {
	return database.Transaction(context.Background(), func(ctx context.Context) error {
		return database.Database[*model.Record](ctx).Create(&model.Record{})
	})
}
`)

	violations := CheckDetachedContext(newProjectIgnoreMatcher())

	if len(violations) != 11 {
		t.Fatalf("expected eleven violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("service", "shadow", "shadow.go"), ":15: database.Transaction receives a context held in ctx")
	assertViolationContains(t, violations, filepath.Join("service", "dotted", "dotted.go"), `:6: dot-imports "tmpapp/dao"`)
	overrides := 0
	for _, violation := range violations {
		if strings.Contains(violation, filepath.Join("service", "override", "override.go")) && strings.Contains(violation, "dao.Records receives a context held in ctx") {
			overrides++
		}
	}
	if overrides != 2 {
		t.Fatalf("expected the overwritten parameter and the declared variable to be flagged, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("service", "record", "record.go"), ":16: database.Transaction receives context.Background()")
	assertViolationContains(t, violations, filepath.Join("dao", "record.go"), ":12: database.Database receives context.TODO()")
	assertViolationContains(t, violations, filepath.Join("cronjob", "sweep.go"), ":10: dao.Records receives context.Background()")
	assertViolationContains(t, violations, filepath.Join("leader", "counter.go"), ":11: dao.Records receives a context held in ctx")
	assertViolationContains(t, violations, filepath.Join("lock", "rebuild.go"), ":16: database.Transaction receives context.Background() through context.WithoutCancel")
	assertViolationContains(t, violations, filepath.Join("service", "sweep", "sweep.go"), ":10: database.Transaction receives context.Background()")
	assertViolationContains(t, violations, filepath.Join("service", "menu", "menu.go"), ":10: menudao.Seed receives context.TODO()")
}
