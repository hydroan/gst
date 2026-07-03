//nolint:predeclared
package new

import (
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
)

var requiredFileContentMap = map[string]string{
	"configx/configx.go":       configxContent,
	"cronjob/cronjob.go":       cronjobContent,
	"middleware/middleware.go": middlewareContent,
	"model/model.go":           modelContent,
	"service/service.go":       serviceContent,
	"module/module.go":         moduleContent,
	"router/router.go":         routerContent,
	"dao/.gitkeep":             "",
	"provider/.gitkeep":        "",
}

var projectFileContentMap = newProjectFileContentMap()

func newProjectFileContentMap() map[string]string {
	files := make(map[string]string, len(requiredFileContentMap)+1)
	maps.Copy(files, requiredFileContentMap)
	files[".golangci.yml"] = golangciLintContent
	return files
}

// ============================================================
// Run: 初始化新项目
// ============================================================

func Run(projectName string) error {
	projectDir := filepath.Base(projectName)

	// 项目目录
	clioutput.Section("Create Project Directory")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		clioutput.Error("", "failed to create project directory")
		return err
	}
	clioutput.Success("", "%s", projectDir)

	// 切换目录
	if err := os.Chdir(projectDir); err != nil {
		return err
	}

	// 初始化 Go module
	clioutput.Section("Initialize Go Module")
	clioutput.Info("", "go mod init %s", projectName)
	cmd := exec.Command("go", "mod", "init", projectName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		clioutput.Error("", "go mod init failed")
		return err
	}
	clioutput.Success("", "Go module initialized")

	// 生成项目文件
	clioutput.Section("Generate Project Files")
	for file, content := range projectFileContentMap {
		if err := createFile(file, content); err != nil {
			clioutput.Error("", "Failed to create %s", file)
			return err
		}
		clioutput.Success("CREATE", "%s", file)
	}

	// main.go
	if err := createFile("main.go", fmt.Sprintf(mainContent,
		projectName, projectName, projectName, projectName, projectName, projectName, projectName)); err != nil {
		return err
	}
	clioutput.Success("CREATE", "%s", "main.go")

	// .gitignore
	if err := createFile(".gitignore", gitignoreContent); err != nil {
		return err
	}
	clioutput.Success("CREATE", "%s", ".gitignore")

	// config.ini.example
	if err := createTeplConfig(projectDir); err != nil {
		return err
	}
	clioutput.Success("CREATE", "%s", "config.ini.example")

	// 运行 go mod tidy
	clioutput.Section("Run Go Mod Tidy")
	cmd = exec.Command("go", "mod", "tidy")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		clioutput.Error("", "go mod tidy failed")
		return err
	}
	clioutput.Success("", "Dependencies tidied")

	// 初始化 git 仓库
	clioutput.Section("Initialize Git Repository")
	cmd = exec.Command("git", "init")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		clioutput.Error("", "git init failed")
		return err
	}
	clioutput.Success("", "Git repository initialized")

	// 最终提示
	clioutput.Section("Project Initialization Completed")
	clioutput.Done("Project %s created successfully!", clioutput.Text(clioutput.StyleBold, "%s", projectDir))
	clioutput.Section("Next Steps")
	clioutput.Command("cd %s", projectDir)
	clioutput.Command("git add .")
	clioutput.Command("git commit -m \"Initial commit\"")

	return nil
}

// ============================================================
// 辅助函数
// ============================================================

func EnsureFileExists() ([]string, error) {
	files := make([]string, 0, len(requiredFileContentMap))
	for file := range requiredFileContentMap {
		files = append(files, file)
	}
	sort.Strings(files)

	var created []string
	for _, file := range files {
		content := requiredFileContentMap[file]
		if _, err := os.Stat(file); err != nil && errors.Is(err, os.ErrNotExist) {
			if err := createFile(file, content); err != nil {
				return created, err
			}
			created = append(created, file)
		}
	}
	return created, nil
}

func createFile(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func createTeplConfig(appName string) error {
	content := fmt.Sprintf(`[app]
name = %s
description = A Go application built with gst framework

[server]
mode = dev
listen =
port = 8080

[database]
type = sqlite

[sqlite]
path = ./data.db
database = main
is_memory = true
enabled = true

[mysql]
host = 127.0.0.1
port = 3306
database =
username = root
password =
charset = utf8mb4
enabled = true

[postgres]
host = 127.0.0.1
port = 5432
database =
username = postgres
password =
sslmode = disable
timezone = Asia/Shanghai
enabled = true

[redis]
enabled = false
addr = 127.0.0.1:6379
db = 0
password =
namespace = %s
`, appName, appName)
	return os.WriteFile("config.ini.example", []byte(content), 0o600)
}
