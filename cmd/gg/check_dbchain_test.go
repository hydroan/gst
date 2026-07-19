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

func TestCheckDatabaseChainTerminationAllowsInlineTerminatedChains(t *testing.T) {
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
	writeCheckFile(t, filepath.Join(projectDir, "service", "order", "order.go"), `package order

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

	violations := CheckDatabaseChainTermination()

	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}

func TestCheckDatabaseChainTerminationFlagsStoredOrUnterminatedChains(t *testing.T) {
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

	violations := CheckDatabaseChainTermination()

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

// TestDatabaseChainMethodSetsMatchTypesInterface guards the hardcoded method
// sets against drift when methods are added to or removed from the
// types.Database and types.DatabaseOption interfaces.
func TestDatabaseChainMethodSetsMatchTypesInterface(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join("..", "..", "types", "interface.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	assertMethodSetMatches(t, "Database", interfaceMethodNames(t, file, "Database"), databaseTerminalMethods)
	assertMethodSetMatches(t, "DatabaseOption", interfaceMethodNames(t, file, "DatabaseOption"), databaseChainMethods)
}

func assertMethodSetMatches(t *testing.T, interfaceName string, declared []string, hardcoded map[string]bool) {
	t.Helper()

	for _, name := range declared {
		if !hardcoded[name] {
			t.Errorf("types.%s method %q is missing from the gg check method set", interfaceName, name)
		}
	}
	for name := range hardcoded {
		if !slices.Contains(declared, name) {
			t.Errorf("gg check method set entry %q no longer exists on types.%s", name, interfaceName)
		}
	}
}

func interfaceMethodNames(t *testing.T, file *ast.File, interfaceName string) []string {
	t.Helper()

	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || typeSpec.Name == nil || typeSpec.Name.Name != interfaceName {
			return true
		}
		interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
		if !ok || interfaceType.Methods == nil {
			return true
		}
		for _, method := range interfaceType.Methods.List {
			// Fields without names are embedded interfaces, not methods.
			for _, name := range method.Names {
				names = append(names, name.Name)
			}
		}
		return false
	})
	if len(names) == 0 {
		t.Fatalf("no methods found for interface %q in types/interface.go", interfaceName)
	}
	return names
}
