// Package answer builds service errors that reach a client as they stand,
// and service errors every caller drops or wraps in another.
package answer

import (
	"fmt"

	"example.com/module/api"
	"example.com/module/database"
	"example.com/module/internal/gst"
	"github.com/cockroachdb/errors"
)

// failedMsg is what Serve answers with for whatever it calls.
const failedMsg = "the sample failed"

// errGone is an error Serve tells from the others.
var errGone = errors.New("gone")

// Serve stands for what the framework calls: nothing of the module calls
// it, so what it returns is what a client is answered with.
func Serve(id string) error {
	if err := wrapped(id); err != nil {
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	err := assigned(id)
	if err != nil {
		if err == errGone || errors.Is(err, errGone) {
			return gst.NewError(404, failedMsg)
		}
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := relayed(id); err != nil {
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := Shared(id); err != nil {
		return api.NewErrorWithCause(500, failedMsg, err)
	}
	if err := saved(id); err != nil {
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := api.Check(id); err != nil {
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := handed(id); err != nil {
		return err
	}
	if err := either(id); err != nil {
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := prefixed(id); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	if err := sorted(id); err != nil {
		if id == "known" {
			return err
		}
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := rebuilt(id); err != nil {
		var inner *gst.Error
		if errors.As(err, &inner) {
			return gst.NewErrorWithCause(inner.Status(), inner.Msg(), err)
		}
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := logged(id); err != nil {
		note(err)
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := matched(id); err != nil {
		if api.Is(err, 404) {
			return gst.NewError(404, failedMsg)
		}
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := helped(id); err != nil {
		return refuse(failedMsg, err)
	}
	if err := lingering(id); err != nil {
		failures++
	}
	if err := stored(id); err != nil {
		last = err
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	last = beyond(id)
	if last != nil {
		return gst.NewErrorWithCause(500, failedMsg, last)
	}
	collected := []error{kept(id)}
	run(called)
	if err := called(id); err != nil {
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := closed(id, collected); err != nil {
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	if err := crossed(id); err != nil {
		return err
	}
	if err := each(id); err != nil {
		return err
	}
	if err := jumped(id); err != nil {
		return err
	}
	if err := shown(id); err != nil {
		return err
	}
	if err := tallied(id); err != nil {
		return err
	}
	return wrote(id)
}

// Other stands for a second thing the framework calls, which hands the
// error of either on as it stands.
func Other(id string) error {
	return either(id)
}

// Third stands for a third, which hands the error of named on.
func Third(id string) error {
	return named(id)
}

// service stands for a service, whose methods the framework calls.
type service struct{}

// Create builds a service error nothing of the module calls for.
func (service) Create(id string) error {
	return gst.NewError(409, "the sample exists")
}

// last is a variable of the package: Serve stores an error in it before
// wrapping it, and assigns it the error of beyond.
var last error

// failures counts the errors met.
var failures int

// initial is built as the package initializes.
var initial = early("")

// The functions below build the service errors the check reports: each is
// dropped or wrapped in another, whichever way it goes.

// wrapped is called by the if statement that wraps its error.
func wrapped(id string) error {
	if id == "" {
		return gst.NewError(400, "the sample is wrapped")
	}
	return nil
}

// assigned is called by the statement before the one that wraps its error,
// which compares the error, tests it with errors.Is and answers one input
// without it.
func assigned(id string) error {
	if id == "" {
		return gst.NewError(400, "the sample is assigned")
	}
	return nil
}

// relayed hands the error of inner on as it stands, and its one caller wraps
// what it returns.
func relayed(id string) error {
	return inner(id)
}

func inner(id string) error {
	if id == "" {
		return gst.NewError(400, "the sample is inner")
	}
	return nil
}

// Shared is exported by a package no project imports and builds its error
// with the constructor a public package forwards; its one caller wraps what
// it returns with the other one forwarded.
func Shared(id string) error {
	if id == "" {
		return api.NewError(400, "the sample is shared")
	}
	return nil
}

// saved returns the error of a transaction, whose function literal builds
// one service error and hands the error of deep on; its one caller wraps
// what it returns.
func saved(id string) error {
	return database.Transaction(id, func() error {
		if id == "" {
			return gst.NewError(400, "the sample is saved")
		}
		return deep(id)
	})
}

func deep(id string) error {
	return gst.NewError(400, "the sample "+id+" is deep")
}

// wrote wraps the error of its transaction itself: the service error the
// function literal builds stops there, while the one wrote builds goes on to
// Serve, which returns it as it stands.
func wrote(id string) error {
	if err := database.Transaction(id, func() error {
		return gst.NewError(400, "the sample is written")
	}); err != nil {
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	return nil
}

// each calls skipped and looped for every part of id. The error of skipped
// is dropped by a continue, with the variable declared for it; the error of
// looped outlives the loop a break leaves, in a variable declared before
// it, and is returned as it stands.
func each(id string) error {
	var err error
	for _, part := range []string{id, id} {
		if dropped := skipped(part); dropped != nil {
			continue
		}
		err = looped(part)
		if err != nil {
			break
		}
	}
	return err
}

func skipped(id string) error {
	return gst.NewError(400, "the sample is skipped")
}

// tallied wraps the error of counted, held by its named result, which the
// function literal it defers only compares.
func tallied(id string) (err error) {
	defer func() {
		if err != nil {
			failures++
		}
	}()
	err = counted(id)
	if err != nil {
		return gst.NewErrorWithCause(500, failedMsg, err)
	}
	return nil
}

func counted(id string) error {
	return gst.NewError(400, "the sample is counted")
}

// The functions below build service errors the check reports nothing for:
// each may reach a client as it stands, or goes where the check does not
// follow it.

// handed is called by a caller returning its error as it stands.
func handed(id string) error {
	return gst.NewError(400, "the sample is handed")
}

// either is wrapped by Serve and handed on by Other.
func either(id string) error {
	return gst.NewError(400, "the sample is either")
}

// prefixed is wrapped in an error that is no service error, which leaves
// its own the outermost.
func prefixed(id string) error {
	return gst.NewError(400, "the sample is prefixed")
}

// sorted is returned as it stands for one input and wrapped for the rest.
func sorted(id string) error {
	return gst.NewError(400, "the sample is sorted")
}

// rebuilt is called by a caller that takes the service error out of what it
// returns with errors.As and answers with its status and its sentence.
func rebuilt(id string) error {
	return gst.NewError(400, "the sample is rebuilt")
}

// logged is wrapped by a caller that hands its error to another call first,
// one the check knows nothing of.
func logged(id string) error {
	return gst.NewError(400, "the sample is logged")
}

// matched is wrapped by a caller that hands its error first to a function
// of another package than errors, named as the test of an error against
// another is, which reads its status.
func matched(id string) error {
	return gst.NewError(404, "the sample is matched")
}

// helped is wrapped by refuse, a function handing a sentence on to a
// constructor: what it does with the cause is its own business, and it
// answers with the service error it finds there.
func helped(id string) error {
	return gst.NewError(400, "the sample is helped")
}

func refuse(msg string, cause error) error {
	var inner *gst.Error
	if errors.As(cause, &inner) {
		return inner
	}
	return gst.NewErrorWithCause(500, msg, cause)
}

// named returns the error of bare by its named result, with a return
// statement naming nothing.
func named(id string) (err error) {
	err = bare(id)
	if err != nil {
		return
	}
	return nil
}

func bare(id string) error {
	return gst.NewError(400, "the sample is bare")
}

// lingering is called by an if statement that does not leave, so the error
// is not seen to stop there.
func lingering(id string) error {
	return gst.NewError(400, "the sample is lingering")
}

// stored is wrapped by a statement that keeps the error elsewhere first.
func stored(id string) error {
	return gst.NewError(400, "the sample is stored")
}

// beyond is wrapped by a caller holding its error in a variable of the
// package, which anything may read.
func beyond(id string) error {
	return gst.NewError(400, "the sample is beyond")
}

// kept is called where its error goes into a list.
func kept(id string) error {
	return gst.NewError(400, "the sample is kept")
}

// crossed tests another error than the one it was just handed, which it
// returns as it stands.
func crossed(id string) error {
	first := handed(id)
	err := mismatched(id)
	if first != nil {
		return gst.NewErrorWithCause(500, failedMsg, first)
	}
	return err
}

func mismatched(id string) error {
	return gst.NewError(400, "the sample is mismatched")
}

// looped is called by each, whose loop a break leaves with its error.
func looped(id string) error {
	return gst.NewError(400, "the sample is looped")
}

// jumped leaves the test of the error of landed by a goto, and returns the
// error as it stands where the goto lands.
func jumped(id string) error {
	err := landed(id)
	if err != nil {
		goto out
	}
	return nil
out:
	return err
}

func landed(id string) error {
	return gst.NewError(400, "the sample is landed")
}

// shown names the variable holding the error of captured in a function
// literal, which returns it as it stands.
func shown(id string) error {
	var err error
	fail := func() error { return err }
	err = captured(id)
	if err != nil {
		return fail()
	}
	return nil
}

func captured(id string) error {
	return gst.NewError(400, "the sample is captured")
}

// called is handed to run as a value, and called by a caller wrapping its
// error as well.
func called(id string) error {
	return gst.NewError(400, "the sample is called")
}

// early is called as the package initializes.
func early(id string) error {
	return gst.NewError(400, "the sample is early")
}

// closed is wrapped by its one caller; the service error it builds is
// returned by a function literal that is no transaction's, which calls
// nested, and the one it keeps in a variable is not followed either.
func closed(id string, collected []error) error {
	check := func() error {
		if len(collected) > 9 {
			return gst.NewError(400, "the sample is closed")
		}
		return nested(id)
	}
	if id == "held" {
		held := gst.NewError(400, "the sample is held")
		return held
	}
	return check()
}

func nested(id string) error {
	return gst.NewError(400, "the sample is nested")
}

// note stands for a call whose result is dropped, a log line.
func note(err error) {}

// run calls f.
func run(f func(string) error) { _ = f("") }
