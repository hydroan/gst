package main

import (
	"os/exec"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/clioutput"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/ggmodule"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

var upgradeYes bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Move the project to the framework version gg is built from",
	Long: `Move the project to the framework version this gg is built from: go.mod
requires that version, every framework module copied into the project is
copied again from it, and the code the models declare is generated anew.

The version is the one of gg itself, so a newer framework is reached by
installing its gg first:

  go install ` + ggconst.ImportPathGst + `/cmd/gg@latest
  gg upgrade

A go.mod replacing the framework module, or a Go workspace supplying it,
decides what the project builds with: gg upgrade leaves the requirement and
the replacement as they are, and brings the copied modules and the generated
code up to that framework.

Copying a module again overwrites what the project changed under its model
and service directories, and deletes the files there the framework does not
have, so the command asks first; --yes answers for it.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runUpgrade(cmd.Root().Version, upgradeYes)
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeYes, "yes", false, "copy the modules again without prompting for confirmation")
}

// runUpgrade moves the project in the working directory to the framework gg
// is built from, built being the version gg records of itself. The steps are
// the ones of moving a project by hand: go get takes go.mod to the version,
// where its requirement is what the project builds with (see
// upgradeVersion); every framework module the project holds a copy of is
// copied again from the framework the project then builds with, the way gg
// module copy --force copies it; the code is generated as gg gen generates
// it; and the module is tidied. Copying a module again takes what the
// project changed in its copy, so the modules are named in a question
// first, which yes answers; a no leaves the project as it was. The run lists
// the files it changed and ends on their count. A step that fails leaves the
// steps before it done: running the command again finishes the upgrade,
// none of them redoing what is done.
func runUpgrade(built string, yes bool) error {
	started := time.Now()
	if _, err := gghelper.ModulePath("."); err != nil {
		return err
	}
	version, framework, err := upgradeVersion(built)
	if err != nil {
		return err
	}
	// The modules are the ones copied from the framework the project builds
	// with now: a directory of the project's own, named like a module only
	// the framework it moves to can copy, is not taken for a copy.
	copied, err := ggmodule.CopiedModuleNames()
	if err != nil {
		return err
	}
	if len(copied) > 0 && !yes {
		modules := "module " + copied[0]
		if len(copied) > 1 {
			modules = "modules " + strings.Join(copied, ", ")
		}
		clioutput.Warn("Copying a module again overwrites what the project changed under model/<module> and service/<module>, and deletes the files there the framework does not have")
		if !clioutput.Confirm("Copy %s again from the framework?", modules) {
			clioutput.Item("Upgrade canceled: nothing was changed")
			return nil
		}
	}

	stopped := func(err error) error {
		return errors.Newf("%v\n  Once that is fixed, run gg upgrade again to finish the upgrade", err)
	}
	log := new(changes)
	if version != "" {
		clioutput.Progress("Fetching %s %s", ggconst.ImportPathGst, version)
		err = recordModuleFiles(log, func() error {
			return errors.Wrapf(goGetFramework(version), "go get %s@%s failed", ggconst.ImportPathGst, version)
		})
		if err != nil {
			return stopped(err)
		}
	}
	if len(copied) > 0 {
		// The violations the project has before the first copy do not stop
		// the copies; the generation closing the upgrade reports them.
		baseline := collectProjectCheckBaseline()
		copyOpts := ggmodule.CopyOptions{Force: true}
		for _, name := range copied {
			plan, planErr := ggmodule.BuildCopyPlan(name, copyOpts)
			if planErr != nil {
				return stopped(planErr)
			}
			copyRun := ggmodule.CopyExecution{
				Plan:    plan,
				Options: copyOpts,
				RunGen: func() error {
					return runModuleCopyGen(baseline, plan.ServiceTargets(), log)
				},
				// Only what the copy changed is listed: most files of a module
				// copied again are found as the framework has them.
				OnFile: func(status ggmodule.CopyWriteStatus, path string) {
					if status != ggmodule.CopyWriteSkip {
						log.record(moduleCopyLabels[status], path, "")
					}
				},
			}
			if err = copyRun.Run(); err != nil {
				return stopped(err)
			}
		}
	}
	if err = genRunWithOptions(genRunOptions{Quiet: true, Log: log}); err != nil {
		return stopped(err)
	}
	if err = tidyModule(log); err != nil {
		return stopped(err)
	}

	summary := log.summary()
	if summary == "" {
		clioutput.Done("Nothing to upgrade: the project is at %s (%s)", framework, elapsedSince(started))
		return nil
	}
	clioutput.Done("Upgraded to %s: %s (%s)", framework, summary, elapsedSince(started))
	clioutput.Note("Next: build and test the project; gg migrate --dry-run shows what the models now change in the database")
	return nil
}

// upgradeVersion tells what gg upgrade, run by a gg built from the framework
// version built, does about the version the project's go.mod requires:
// version is the one to move the requirement to, "" where the requirement
// stays as it is, and framework names what the project builds with once the
// upgrade is done, for the line the upgrade ends on.
//
// The requirement stays where it does not decide what the project builds
// with, a go.mod replacing the framework module or a Go workspace supplying
// it, and where it is the version of gg already. Anywhere else it moves
// forward to the version of gg, and the upgrade is refused when gg names no
// version, built from a working copy as it is (see namesVersion), or an
// older one, which would take the project back: a gg built from v1.2.0 in a
// project requiring v1.3.0 gets
//
//	gg is built from github.com/hydroan/gst v1.2.0, older than the v1.3.0 go.mod requires: gg upgrade moves a project forward only
//	  To install the gg the project requires: go install github.com/hydroan/gst/cmd/gg@v1.3.0
func upgradeVersion(built string) (version, framework string, err error) {
	required, replaced, err := gghelper.FrameworkRequirement(".")
	if err != nil {
		return "", "", err
	}
	if required == "" {
		return "", "", errors.Newf("go.mod requires no %s: gg upgrade moves a gst project to the framework version gg is built from", ggconst.ImportPathGst)
	}
	if replaced {
		return "", "the framework go.mod replaces " + ggconst.ImportPathGst + " with", nil
	}
	inWorkspace, err := gghelper.InWorkspace()
	if err != nil {
		return "", "", err
	}
	if inWorkspace {
		return "", "the framework the Go workspace supplies", nil
	}
	if !namesVersion(built) {
		return "", "", errors.Newf("gg is built from a working copy of %[1]s and names no version to move the project to\n"+
			"  To install gg at a version: go install %[1]s/cmd/gg@latest", ggconst.ImportPathGst)
	}
	switch newer := semver.Compare(built, required); {
	case newer < 0:
		return "", "", errors.Newf("gg is built from %[1]s %[2]s, older than the %[3]s go.mod requires: gg upgrade moves a project forward only\n"+
			"  To install the gg the project requires: go install %[1]s/cmd/gg@%[3]s", ggconst.ImportPathGst, built, required)
	case newer > 0:
		version = built
	}
	return version, ggconst.ImportPathGst + " " + built, nil
}

// goGetFramework runs go get in the working directory for the framework
// module at version, printing what it says only when it fails. Tests replace
// it: it resolves the framework over the network.
var goGetFramework = func(version string) error {
	return runQuietly(exec.Command("go", "get", ggconst.ImportPathGst+"@"+version)) //nolint:gosec // G204: the version is the one gg records of itself, read from its build information.
}
