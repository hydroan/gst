package ggmodule

import (
	"path/filepath"
	"testing"
)

func TestCopyExecutionRequiresGenRunnerBeforeWritingFiles(t *testing.T) {
	t.Chdir(t.TempDir())

	modelTarget := filepath.Join("model", "copytest", "copytest.go")
	exec := &CopyExecution{
		Plan: &CopyPlan{
			Name:       "copytest",
			ModelDir:   "model",
			ServiceDir: "service",
			Files: []moduleCopyFile{
				{
					Kind:       moduleCopyFileModel,
					TargetPath: modelTarget,
					Content:    []byte("package copytest\n"),
				},
			},
		},
	}

	err := exec.Run()
	if err == nil {
		t.Fatal("Run() succeeded, want an error when no gg gen runner is configured")
	}
	if len(exec.WrittenFiles) != 0 {
		t.Fatalf("Run() wrote %v before reporting the missing gg gen runner", exec.WrittenFiles)
	}
	if fileExists(modelTarget) {
		t.Fatalf("Run() created %s before reporting the missing gg gen runner", modelTarget)
	}
}
