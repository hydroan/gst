package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggnew"
)

// TestUpgradeVersion pins what gg upgrade does about the version go.mod
// requires, the example upgradeVersion gives among the cases: the
// requirement moves forward to the version of gg, tagged or of a commit,
// and stays where it is that version already, where go.mod replaces the
// framework module and in a Go workspace; a gg older than the project, a gg
// naming no version in a project whose requirement decides, and a go.mod
// requiring no framework are refused.
func TestUpgradeVersion(t *testing.T) {
	const (
		requires = "module example.com/app\n\ngo 1.26\n\nrequire github.com/hydroan/gst v1.2.0\n"
		replaces = requires + "\nreplace github.com/hydroan/gst => ../gst\n"
		noCopy   = "gg is built from a working copy of github.com/hydroan/gst and names no version to move the project to\n" +
			"  To install gg at a version: go install github.com/hydroan/gst/cmd/gg@latest"
	)
	for _, tt := range []struct {
		name          string
		built         string
		goMod         string
		goWork        string
		wantVersion   string
		wantFramework string
		wantErr       string
	}{
		{name: "a newer gg", built: "v1.3.0", goMod: requires, wantVersion: "v1.3.0", wantFramework: "github.com/hydroan/gst v1.3.0"},
		{name: "a gg of a later commit", built: "v1.2.1-0.20261123152200-fbd72d5f0000", goMod: requires, wantVersion: "v1.2.1-0.20261123152200-fbd72d5f0000", wantFramework: "github.com/hydroan/gst v1.2.1-0.20261123152200-fbd72d5f0000"},
		{name: "the same version", built: "v1.2.0", goMod: requires, wantFramework: "github.com/hydroan/gst v1.2.0"},
		{name: "an older gg", built: "v1.2.0", goMod: strings.Replace(requires, "v1.2.0", "v1.3.0", 1), wantErr: "gg is built from github.com/hydroan/gst v1.2.0, older than the v1.3.0 go.mod requires: gg upgrade moves a project forward only\n" +
			"  To install the gg the project requires: go install github.com/hydroan/gst/cmd/gg@v1.3.0"},
		{name: "a gg recording no version", built: "(devel)", goMod: requires, wantErr: noCopy},
		{name: "a gg of a working copy with changes", built: "v1.3.0+dirty", goMod: requires, wantErr: noCopy},
		{name: "a go.mod replacing the framework", built: "v1.3.0", goMod: replaces, wantFramework: "the framework go.mod replaces github.com/hydroan/gst with"},
		{name: "a go.mod replacing the framework, a gg recording no version", built: "(devel)", goMod: replaces, wantFramework: "the framework go.mod replaces github.com/hydroan/gst with"},
		{name: "a Go workspace", built: "v1.3.0", goMod: requires, goWork: "go 1.26\n\nuse .\n", wantFramework: "the framework the Go workspace supplies"},
		{name: "a go.mod requiring no framework", built: "v1.3.0", goMod: "module example.com/app\n\ngo 1.26\n", wantErr: "go.mod requires no github.com/hydroan/gst: gg upgrade moves a gst project to the framework version gg is built from"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for name, content := range map[string]string{"go.mod": tt.goMod, "go.work": tt.goWork} {
				if content == "" {
					continue
				}
				if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			version, framework, err := upgradeVersion(tt.built)
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if version != tt.wantVersion || framework != tt.wantFramework || gotErr != tt.wantErr {
				t.Fatalf("upgradeVersion(%s) = %q, %q, %q, want %q, %q, %q", tt.built, version, framework, gotErr, tt.wantVersion, tt.wantFramework, tt.wantErr)
			}
		})
	}
}

// TestUpgradeCopiesTheModulesAgainAndGenerates pins the upgrade of a project
// holding a copied module, in a project whose go.mod replaces the framework,
// which leaves go.mod to itself: the copy the project changed is taken back
// to what the framework has — a file it edited written over, one it added
// deleted — and the code is generated, a column file it lost created again;
// only the files that changed are listed, and counted in the line the
// upgrade ends on. Before that it warns of what a module copied again takes
// and asks, naming the module, and a no changes nothing; a second upgrade
// finds nothing to do.
func TestUpgradeCopiesTheModulesAgainAndGenerates(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	files, err := ggnew.ProjectFiles("tmpapp")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		writeProjectFile(t, filepath.Join(projectDir, file.Path), file.Content)
	}
	if err := goModTidy(); err != nil {
		t.Fatal(err)
	}
	var copyErr error
	if output := captureOutput(t, func() { copyErr = runModuleCopy("logmgmt", moduleCopyOptions{Yes: true}) }); copyErr != nil {
		t.Fatalf("runModuleCopy() error = %v:\n%s", copyErr, output)
	}
	// The project makes the assembly call the module declares, which gg gen
	// holds every project with a copy of the module to.
	writeProjectFile(t, filepath.Join("module", "module.go"), `package module

import (
	"github.com/hydroan/gst/authn"

	servicelogmgmt "tmpapp/service/logmgmt"
)

func init() {
	authn.AddLoginObserver(servicelogmgmt.RecordLoginEvent)
}
`)
	goGet := goGetFramework
	goGetFramework = func(version string) error {
		t.Errorf("go get ran for %s in a project whose go.mod replaces the framework", version)
		return nil
	}
	t.Cleanup(func() { goGetFramework = goGet })

	edited := filepath.Join("model", "logmgmt", "loginlog.go")
	added := filepath.Join("model", "logmgmt", "extra.go")
	generated := filepath.Join("model", "logmgmt", "operationlog.gen.go")
	copiedSource := readProjectFile(t, edited)
	writeProjectFile(t, edited, copiedSource+"\n// Edited by the project.\n")
	writeProjectFile(t, added, "package logmgmt\n")
	if err := os.Remove(generated); err != nil {
		t.Fatal(err)
	}

	var upgradeErr error
	var asked string
	withStdin(t, "n\n", func() {
		asked = captureOutput(t, func() { upgradeErr = runUpgrade("(devel)", false) })
	})
	if upgradeErr != nil {
		t.Fatalf("runUpgrade() answered no, error = %v:\n%s", upgradeErr, asked)
	}
	for _, want := range []string{
		"  ⚠ Copying a module again overwrites what the project changed under model/<module> and service/<module>, and deletes the files there the framework does not have\n",
		"\n? Copy module logmgmt again from the framework? (y/N): ",
		"  → Upgrade canceled: nothing was changed\n",
	} {
		if !strings.Contains(asked, want) {
			t.Fatalf("the upgrade answered no lacks %q:\n%s", want, asked)
		}
	}
	if !strings.HasSuffix(readProjectFile(t, edited), "// Edited by the project.\n") {
		t.Fatal("the upgrade answered no wrote over the file the project edited")
	}

	output := captureOutput(t, func() { upgradeErr = runUpgrade("(devel)", true) })
	if upgradeErr != nil {
		t.Fatalf("runUpgrade() error = %v:\n%s", upgradeErr, output)
	}
	for _, want := range []string{
		"  UPDATE  " + edited + "\n",
		"  REMOVE  " + added + "\n",
		"  CREATE  " + generated + "\n",
		"\n✔ Upgraded to the framework go.mod replaces github.com/hydroan/gst with: 1 created, 1 updated, 1 removed (",
		"  Next: build and test the project; gg migrate --dry-run shows what the models now change in the database\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("the upgrade output lacks %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"SKIP", "? Copy module", "Fetching"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("the upgrade output holds %q: it lists only what changed, asks nothing under --yes and fetches nothing for a replaced framework:\n%s", unwanted, output)
		}
	}
	if got := readProjectFile(t, edited); got != copiedSource {
		t.Fatalf("%s after the upgrade is not what the framework has:\n%s", edited, got)
	}
	requireProjectCompiles(t)

	output = captureOutput(t, func() { upgradeErr = runUpgrade("(devel)", true) })
	if upgradeErr != nil || !strings.Contains(output, "\n✔ Nothing to upgrade: the project is at the framework go.mod replaces github.com/hydroan/gst with (") {
		t.Fatalf("a second upgrade: error = %v, output:\n%s\nwant nothing to upgrade", upgradeErr, output)
	}
}

// TestUpgradeMovesGoModToTheVersionOfGG pins the first step of an upgrade
// in a project whose go.mod decides the framework it builds with: go get
// runs for the version of gg before anything is generated, the go.mod it
// rewrote is listed and counted, and the upgrade ends on the version; a go
// get that fails ends the upgrade on its error and on what to do next, the
// project left as it was. go get itself is replaced, the network being out
// of reach of a test: the one standing in for it writes the version into
// go.mod with a replacement pointing the framework to this repository, the
// version existing nowhere else. Until then the go.mod of the project
// requires a version and replaces nothing, and the go command, asked where
// the framework is before the upgrade changes anything, is pointed to an
// alternate go.mod that knows.
func TestUpgradeMovesGoModToTheVersionOfGG(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	files, err := ggnew.ProjectFiles("tmpapp")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		writeProjectFile(t, filepath.Join(projectDir, file.Path), file.Content)
	}
	if err := goModTidy(); err != nil {
		t.Fatal(err)
	}

	const unversioned = "require github.com/hydroan/gst v0.0.0-00010101000000-000000000000"
	replaced := readProjectFile(t, "go.mod")
	alternate := t.TempDir()
	writeProjectFile(t, filepath.Join(alternate, "alt.mod"), replaced)
	writeProjectFile(t, filepath.Join(alternate, "alt.sum"), readProjectFile(t, "go.sum"))
	t.Setenv("GOFLAGS", "-modfile="+filepath.Join(alternate, "alt.mod"))
	var pinned []string
	for line := range strings.SplitSeq(replaced, "\n") {
		if !strings.HasPrefix(line, "replace github.com/hydroan/gst ") {
			pinned = append(pinned, strings.Replace(line, unversioned, "require github.com/hydroan/gst v1.2.0", 1))
		}
	}
	writeProjectFile(t, "go.mod", strings.Join(pinned, "\n"))
	goModBefore := readProjectFile(t, "go.mod")
	if !strings.Contains(goModBefore, "require github.com/hydroan/gst v1.2.0\n") || strings.Contains(goModBefore, "replace github.com/hydroan/gst") {
		t.Fatalf("the fixture go.mod does not require v1.2.0 alone:\n%s", goModBefore)
	}

	// A generated file the project lost, for the upgrade to write again.
	generated := filepath.Join("model", "model.gen.go")
	if err := os.Remove(generated); err != nil {
		t.Fatal(err)
	}
	var fetched []string
	goGet := goGetFramework
	goGetFramework = func(version string) error {
		fetched = append(fetched, version)
		if version == "v1.4.0" {
			return errors.New("no route to the module")
		}
		if _, statErr := os.Stat(generated); statErr == nil {
			t.Errorf("go get ran after %s was generated", generated)
		}
		writeProjectFile(t, "go.mod", strings.Replace(replaced, unversioned, "require github.com/hydroan/gst "+version, 1))
		return nil
	}
	t.Cleanup(func() { goGetFramework = goGet })

	var upgradeErr error
	output := captureOutput(t, func() { upgradeErr = runUpgrade("v1.4.0", false) })
	const wantErr = "go get github.com/hydroan/gst@v1.4.0 failed: no route to the module\n  Once that is fixed, run gg upgrade again to finish the upgrade"
	if upgradeErr == nil || upgradeErr.Error() != wantErr {
		t.Fatalf("runUpgrade() with a failing go get: error = %v, want %q", upgradeErr, wantErr)
	}
	if _, statErr := os.Stat(generated); statErr == nil || strings.Contains(output, "Upgraded") || strings.Contains(output, "UPDATE") || readProjectFile(t, "go.mod") != goModBefore {
		t.Fatalf("a failing go get must leave the project as it was and report nothing done:\n%s", output)
	}

	output = captureOutput(t, func() { upgradeErr = runUpgrade("v1.3.0", false) })
	if upgradeErr != nil {
		t.Fatalf("runUpgrade() error = %v:\n%s", upgradeErr, output)
	}
	if len(fetched) != 2 || fetched[1] != "v1.3.0" {
		t.Fatalf("go get ran for %v, want v1.4.0, which failed, then v1.3.0", fetched)
	}
	for _, want := range []string{
		"  → Fetching github.com/hydroan/gst v1.3.0\n",
		"  UPDATE  go.mod\n",
		"  CREATE  " + generated + "\n",
		"\n✔ Upgraded to github.com/hydroan/gst v1.3.0: ",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("the upgrade output lacks %q:\n%s", want, output)
		}
	}
	if !strings.Contains(readProjectFile(t, "go.mod"), "require github.com/hydroan/gst v1.3.0\n") {
		t.Fatalf("go.mod after the upgrade does not require v1.3.0:\n%s", readProjectFile(t, "go.mod"))
	}
}
