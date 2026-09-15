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
// dao function is flagged in the directories that run under a context, a
// generic instantiation included, while the context handed down passes,
// copied framework modules are the framework's to check, and startup seeding
// in a module package stays outside the rule.
func TestCheckDetachedContextFlagsBackgroundAtDatabaseEntry(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckProjectGoMod(t, projectDir)
	writeFrameworkModuleFixture(t, projectDir, "sample")

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
	writeCheckFile(t, filepath.Join(projectDir, "service", "report", "report.go"), `package report

import (
	"context"

	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func rebuild(ctx context.Context, record *model.Record) error {
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
	writeCheckFile(t, filepath.Join(projectDir, "cronjob", "cleanup.go"), `package cronjob

import (
	"context"

	"tmpapp/dao"
)

func cleanup(context.Context) error {
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

	if len(violations) != 3 {
		t.Fatalf("expected three violations, got %#v", violations)
	}
	assertViolationContains(t, violations, filepath.Join("service", "report", "report.go"), ":16: database.Transaction receives context.Background()")
	assertViolationContains(t, violations, filepath.Join("dao", "record.go"), ":12: database.Database receives context.TODO()")
	assertViolationContains(t, violations, filepath.Join("cronjob", "cleanup.go"), ":10: dao.Records receives context.Background()")
}
