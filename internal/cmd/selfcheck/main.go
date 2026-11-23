// Command selfcheck holds the framework's own source to the rules
// golangci-lint cannot express, and prints each violation it finds.
//
// Run it from the repository root through `make check`. The rules bind the
// framework alone: make check runs them over the framework, and a project is
// held to none of them.
//
// # Test placement
//
// golangci-lint's testpackage makes a test file that joins the package it
// tests say so: its name ends in _internal_test.go. That settles the name but
// not the need, and this check makes the suffix true. A file can carry the
// suffix and still use nothing unexported of its package, or even declare the
// external test package; the check reports both, so an internal test is always
// one that could not be written from outside.
//
// # Thin forwarding
//
// A function whose only work is handing its own parameters, unchanged and in
// order, to another function is a layer that adds a name and nothing else,
// and so is one whose whole body is a single call of a function of its own
// package, however it shapes the arguments. The check reports it where the
// layer can go: the function has a single use, which can call the other one
// itself, or, for a function passing its parameters on, the function it
// forwards to has no other use, so the two can be one. checkForwarding
// spells out what counts, and what the check leaves alone because it cannot
// count every use.
//
// # Source formatting
//
// The code generators build the Go code they generate as syntax trees and
// print them through one formatting path. A generator that formats source
// text instead assembled its output as a string, and one that imports
// text/template renders it from a template, so a call to a source text
// formatter is reported anywhere in the generators but that one path, and an
// import of text/template anywhere in them. checkSourceFormat names the
// formatters, the generators and the path.
//
// # Import aliases
//
// An import carries an alias in two cases alone: the package is one the
// importas section of .golangci.yml lists, spelt by that alias wherever it is
// imported, which golangci-lint holds; or another import of the same file
// binds the package's own name. An identifier named like the package is
// renamed instead. checkImportAlias reports every other alias, and a package
// spelt two ways anywhere in the tree.
//
// # Test doc comments
//
// golangci-lint's godoclint holds the doc comment of every declaration to
// opening with the declaration's name, test files excepted: godoclint has a
// switch to include them that golangci-lint does not expose (v2.15.0 still
// exposes include-unexported alone). A doc comment opening with another name
// is one an insertion between a neighbor's comment and its declaration took
// along, and test files see as many insertions as any other, so checkTestDoc
// holds them to the same rule, read the way godoclint reads it, until a
// golangci-lint exposes the switch and a line of .golangci.yml replaces it.
//
// # Log fields
//
// A log store that maps fields as they arrive types a key by the first entry
// carrying it and drops every later entry carrying the key with another type,
// so a key written two ways loses one writer's entries without a word.
// internal/logfield declares keys more than one stream writes, one constructor
// each, and checkLogFields holds the tree to them: a key of logfield's is
// written through its constructor alone, the module sources under
// internal/model and internal/service excepted, which are copied into projects
// that cannot import logfield; every other key, ip and user_agent among them,
// is written with one type wherever it appears, a key-value pair typed the way
// zap.Any types its value, and a value of interface type, typed at run time, is
// refused; zap.Object, zap.Dict, zap.Namespace and zap.Inline, which nest keys
// of their own under the entry, belong to logfield alone, whose key set is
// fixed. golangci-lint's loggercheck reads the key-value pairs for their
// shape alone, and no linter compares the type a key is given across the
// tree.
//
// # Test seams
//
// A seam that exists for tests alone stays out of the implementation files:
// one its own package's tests use is an unexported variable beside the code
// it replaces, and one the tests of other packages use, which Go lets no
// package import from a test file, is exported from an internal package's
// testhooks.go alone, the file whose header says why no path the framework
// runs calls what it declares. checkTestSeams reports an exported function or
// variable of an internal package that only test files use anywhere in the
// module while being declared in another file, and one declared in
// testhooks.go that a non-test file uses. An export that is an API by design
// although only tests reach for it yet, the counterpart of one production
// does use or the constructor of a type it uses, says so in its doc comment,
// //gst:api <reason>, and the check holds the directive to its word: one
// without a reason, or on an export non-test code uses, is reported. Judged
// are the packages a non-test file imports: in a package kept for later, or a
// cache backend the framework keeps as an alternative, every export is used
// by tests alone without being a seam. A test support package, one with a
// non-test file importing testing, exports for tests by design and is left
// alone too, as are the module sources and the test support written as
// project code.
//
// # Test directory listing
//
// go test keys a cached result on every entry of every directory a test
// lists, so a test listing the directory of another package, or the
// repository's, reruns whenever anything there moves, an editor saving a file
// or git writing .git, and its result is never reused. checkTestListing
// reports a listing in a test file, os.ReadDir, os.DirFS, filepath.WalkDir,
// filepath.Walk or filepath.Glob, unless it can follow the path back to the
// package's own directory or to a directory the test made; a test reads the
// files of another package through golist.Packages of
// internal/testutil/golist, which lists no directory.
//
// # Constant literals
//
// internal/ggconst spells the names the tooling shares, the packages, files
// and symbols gg writes and recognizes, so that each is spelt once. A string
// literal repeating one parts from the constant the day one of the two
// changes, and nothing fails until a generated file no longer matches what
// reads it. checkConstLiteral reports, in the packages importing
// internal/ggconst, the string literal that spells one of its string
// constants in full, outside a constant declaration and unless it is handed
// to a package outside the module that takes no name or path of the
// tooling's, a command line library or os/exec; a string spelt like a
// constant that means something else says so at the end of its line,
// //gst:literal <what the string means>. golangci-lint's goconst matches a
// string against the constants of its own package alone.
//
// # Validate tags
//
// The framework validates a model or a payload by the rules its fields
// declare under the binding tag, the one gin's validator reads: rules under
// the validate tag read like validation and never run. gg check refuses the
// tag in the model files a project writes and skips the subtrees gg module
// copy writes there, copied module code being checked here, so
// checkValidateTag reports a struct field carrying a validate tag in the
// model sources of the framework's modules, under internal/model.
//
// # Message constants
//
// A sentence a client is answered with is written once, as a constant its
// package declares, and its translations are registered under that
// constant. A sentence written where it is answered has no constant to
// follow it: it is written again wherever it is answered once more and
// where its translations are registered, and rewording one of them leaves
// the others behind. checkMsgConst reports a string constant handed to a
// constructor of the error a request is answered with, or to the
// translation, directly or through a function or a variable forwarding to
// them, that no constant of the framework holds; and a constant handed to
// one, or a translation is registered under, whose name does not end in Msg,
// the suffix telling the sentences of a package from its other strings.
//
// # Message reach
//
// A transport answers a request with the outermost service error of the
// error it is handed, its status and its sentence. A service error built
// where every caller drops what the function returns, or wraps it in a
// service error of its own, is never that one: nobody reads its status or
// its sentence, and the sentence stands among the ones a client is answered
// with, translated like them, without being one. checkMsgReach follows the
// error a function returns through the calls of the framework, the function
// literal a transaction runs included, and reports a service error returned
// where it reaches no function that answers a client: one nothing of the
// framework calls, one handed around as a value, or one a project may call.
// What the check does not see through counts as reaching a client, a caller
// taking the service error out with errors.As to answer with its status and
// its sentence among it: the check misses an error that is never answered
// rather than report one that is.
//
// # Translations
//
// A process answers its clients in the language its configuration names,
// and a sentence it answers with is translated from the translations the
// package it belongs to registers with internal/i18n. A sentence added
// without one is answered in English to a client reading another language,
// and nothing fails until that client meets it. A package declares its
// sentences and registers their translations in one file, translations.go,
// each translation under the constant declaring its sentence, so the two
// are read side by side. checkTranslations reports a sentence (see Message
// constants) its package registers no translation for in one of the
// languages the framework speaks; a registration written in another file,
// or one that is not a map literal under a constant language, the form the
// check reads; a translation registered under anything but a constant its
// file declares; and a translation under a constant nothing but the
// registrations uses, the sentence no client is answered with.
//
// # Layout
//
// Each check lives in the file named after it, testplacement.go for the
// check "testplacement", holding its run function and the helpers only it
// uses; helper.go holds what the checks share, and main.go runs them.
// TestEveryCheckIsDeclaredInAFileNamedAfterIt holds the layout.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// violation is a breach of one of the rules, found in one file.
type violation struct {
	// File is the path of the file, relative to the checked directory.
	File string
	// Message says what is wrong and how to fix it.
	Message string
}

// check is one of the rules selfcheck holds the framework to.
type check struct {
	// name prefixes the errors the check returns.
	name string
	// run reports the violations of the rule among pkgs, the packages under
	// root as load returns them.
	run func(root string, pkgs []*packages.Package) ([]violation, error)
}

// checks lists the rules in the order they run and report.
var checks = []check{
	{name: "testplacement", run: checkTestPlacement},
	{name: "forwarding", run: checkForwarding},
	{name: "sourceformat", run: checkSourceFormat},
	{name: "importalias", run: checkImportAlias},
	{name: "testdoc", run: checkTestDoc},
	{name: "logfields", run: checkLogFields},
	{name: "testseams", run: checkTestSeams},
	{name: "testlisting", run: checkTestListing},
	{name: "constliteral", run: checkConstLiteral},
	{name: "validatetag", run: checkValidateTag},
	{name: "msgconst", run: checkMsgConst},
	{name: "msgreach", run: checkMsgReach},
	{name: "translations", run: checkTranslations},
}

func main() {
	violations, err := run(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "selfcheck:", err)
		os.Exit(1)
	}
	for _, v := range violations {
		fmt.Println(v.Message)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}

// run loads the packages under dir once, tests included, and runs every check
// over them, returning what the checks find in the order they run. A package
// that does not type-check stops the run: no check can judge code the
// compiler rejects.
func run(dir string) ([]violation, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.Wrap(err, "resolve the checked directory")
	}
	pkgs, err := load(root, nil, "./...")
	if err != nil {
		return nil, err
	}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, errors.Newf("package %s does not type-check: %v", p.ID, p.Errors[0])
		}
	}

	var violations []violation
	for _, c := range checks {
		found, err := c.run(root, pkgs)
		if err != nil {
			return nil, errors.Wrap(err, c.name)
		}
		violations = append(violations, found...)
	}
	return violations, nil
}
