// Command buildcache keeps the Go build cache in shape for make test and
// make check: it writes back the test results go keeps under one key only,
// so a cached test never links its binary again, and it trims the cache on
// a shorter clock than go's own. Run it through the Makefile; the commands
// are its steps, not a tool of their own.
//
// # Why go links cached tests again
//
// go test looks a cached result up twice: first by the link action ID of
// the test binary, which skips the link when it hits, then by the content
// ID of the linked binary. A change to a package that the linker drops from
// a test binary — a function no test of that package reaches — changes the
// link action ID and not the binary, so the first lookup misses for good
// while the second hits, and go writes a result back only after running a
// test. Every later run links the binary again: in this repository about
// 100 seconds of CPU across the 34 largest test binaries, 9 seconds of wall
// clock on a run that runs no test. See runTestActor.Act, tryCacheWithID
// and saveOutput in cmd/go/internal/test/test.go of Go 1.27.
//
// # How the write-back works
//
// make test runs go test with -toolexec pointing at `buildcache
// record-link`, which runs every tool unchanged and, after a test binary is
// linked, keeps the binary and the package it tests in a record directory.
// After the run, `buildcache writeback` reads each kept binary's build ID
// the way go prints it, its link action ID and content ID: for a binary whose
// first-level entries are missing while its second-level entries exist, it
// files the second-level entries — the test's input list and its result for
// the current inputs — under the first-level keys as well. The keys are
// computed the way go computes them (see writeback.go), and the copy is
// refused when the computed second-level key finds no entry, so a mismatch
// with the toolchain leaves the cache as it was.
//
// -toolexec is not part of any build ID: go identifies its tools by running
// them through the wrapper, so make test shares every cache entry with a
// plain go test.
//
// # Trimming
//
// go removes cache entries unused for five days, at most once a day. A day
// of framework development writes tens of gigabytes — every edit recompiles
// its dependents with and without -race, per dialect suite and per test
// binary — so `buildcache trim` removes what went unused for two days, once
// a day, from the end of make test and make check.
package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("buildcache: ")
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "record-link":
		if len(os.Args) < 4 {
			usage()
		}
		os.Exit(recordLink(os.Args[2], os.Args[3], os.Args[4:]))
	case "writeback":
		err = writeback(os.Args[2:])
	case "trim":
		err = trim(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  buildcache record-link <record dir> <tool> [tool args...]   (go test -toolexec)
  buildcache writeback [-timeout duration] <record dir>
  buildcache trim [-age duration] [-every duration]`)
	os.Exit(2)
}
