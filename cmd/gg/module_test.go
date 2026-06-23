package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggmodule"
)

func TestPrintModuleCopyPlanReportsExtraTargetModelFilesAsWarningSection(t *testing.T) {
	plan := &ggmodule.CopyPlan{
		ExtraModelFiles: []string{filepath.Join("model", "authz", "design.go")},
	}

	output := captureStdout(t, func() {
		printModuleCopyPlan(plan)
	})

	for _, want := range []string{
		"Extra Target Model Files",
		filepath.Join("model", "authz", "design.go"),
		"not present in the framework source",
		"not deleted automatically",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Extra target model files:") {
		t.Fatalf("output should not show extra model files as a normal plan group:\n%s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writePipe

	fn()

	if closeErr := writePipe.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	os.Stdout = oldStdout

	output, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := readPipe.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	return string(output)
}
