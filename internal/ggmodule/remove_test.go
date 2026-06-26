package ggmodule

import (
	"strings"
	"testing"
)

func TestRemoveModuleUnregistersFrameworkModule(t *testing.T) {
	projectDir := newModuleCommandProjectWithFramework(t)
	if _, err := AddModule(projectDir, "copytest"); err != nil {
		t.Fatalf("AddModule() error = %v", err)
	}

	result, err := RemoveModule(projectDir, "copytest")
	if err != nil {
		t.Fatalf("RemoveModule() error = %v", err)
	}
	if result.Status != ChangeRemoved {
		t.Fatalf("RemoveModule status = %s, want %s", result.Status, ChangeRemoved)
	}

	content := readProjectModuleFile(t, projectDir)
	if strings.Contains(content, "copytest.Register()") || strings.Contains(content, `"github.com/hydroan/gst/module/copytest"`) {
		t.Fatalf("module.go still contains copytest registration:\n%s", content)
	}

	_, err = RemoveModule(projectDir, "copytest")
	if err == nil || !strings.Contains(err.Error(), "is not registered as a framework module") {
		t.Fatalf("second RemoveModule() error = %v, want not registered", err)
	}
}
