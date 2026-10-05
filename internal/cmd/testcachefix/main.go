// Command testcachefix writes back the test results go keeps under one key
// only, so a cached test never links its binary again. Run it through the
// Makefile; the commands are its steps, not a tool of their own.
//
// # Why go links cached tests again
//
// go test looks a cached result up twice: first by the link action ID of
// the test binary, which skips the link when it hits, then by the content
// ID of the linked binary. A change to a package that the linker drops from
// a test binary — a function no test of that package reaches — changes the
// link action ID and not the binary, so the first lookup misses for good
// while the second hits, and go writes a result back only after running a
// test. Every later run links the binary again, which is most of the wall
// clock of a run that runs no test. See runTestActor.Act, tryCacheWithID
// and saveOutput in cmd/go/internal/test/test.go of Go 1.27.
//
// # How the write-back works
//
// make test runs go test with -toolexec pointing at `testcachefix
// record-link`, which runs every tool unchanged and, after a test binary is
// linked, keeps the binary and the package it tests in a record directory.
// After the run, `testcachefix writeback` reads each kept binary's build ID
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
// plain go test. The command only ever adds index entries to the cache, for
// results go already holds; it removes nothing, since the cache is the
// machine's and every module on it shares it.
package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("testcachefix: ")
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
	default:
		usage()
	}
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  testcachefix record-link <record dir> <tool> [tool args...]   (go test -toolexec)
  testcachefix writeback [-timeout duration] <record dir>`)
	os.Exit(2)
}
