package util_test

import (
	"bufio"
	"crypto/tls"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/require"
)

// TestTcping reports a port as reachable while something listens on it and
// unreachable once the listener closes. The listener is the test's own, so the
// outcome depends on no network outside the machine.
func TestTcping(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	require.True(t, util.Tcping("127.0.0.1", addr.Port, time.Second))

	require.NoError(t, listener.Close())
	require.False(t, util.Tcping("127.0.0.1", addr.Port, time.Second))
}

// runOrDieHelper marks the child process that runs RunOrDie for
// TestRunOrDieReportsTheCauseWithItsStack.
const runOrDieHelper = "GST_UTIL_RUN_OR_DIE_HELPER"

// TestRunOrDieReportsTheCauseWithItsStack pins the report of a failed start:
// the line on stdout names the function and the cause chain, the stack the
// cause was created with follows it, so a terminal shows where the failure
// began, and the process exits with 1. RunOrDie exits the process, so the run
// happens in a child process: the test binary runs itself again with this
// test selected and the helper marked.
func TestRunOrDieReportsTheCauseWithItsStack(t *testing.T) {
	if os.Getenv(runOrDieHelper) == "1" {
		// The start that fails, with a cause created where a listener would
		// be opened.
		util.RunOrDie(func() error { return errors.Wrap(errors.New("listen failed"), "start the listener") })
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunOrDieReportsTheCauseWithItsStack$")
	cmd.Env = append(os.Environ(), runOrDieHelper+"=1")
	out, err := cmd.CombinedOutput()

	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit, "the child must exit on the error:\n%s", out)
	require.Equal(t, 1, exit.ExitCode())
	report := string(out)
	require.Contains(t, report, "TestRunOrDieReportsTheCauseWithItsStack.func1 error: start the listener: listen failed")
	require.Contains(t, report, "attached stack trace", "the report carries the stack of the cause")
	require.Contains(t, report, "TestRunOrDieReportsTheCauseWithItsStack.func1\n", "the stack names where the cause was created")
}

// TestStringAnyFormatsEveryKindOfValue pins the text StringAny gives each
// kind of value: nothing for nil, a Stringer by its String, strings and
// bytes as they are, integers in decimal, and anything else the way %v
// prints it.
func TestStringAnyFormatsEveryKindOfValue(t *testing.T) {
	for _, tt := range []struct {
		value any
		want  string
	}{
		{nil, ""},
		{time.Second, "1s"},
		{"text", "text"},
		{[]byte("bytes"), "bytes"},
		{int8(-8), "-8"},
		{uint64(18446744073709551615), "18446744073709551615"},
		{1.5, "1.5"},
		{struct{ A int }{1}, "{1}"},
	} {
		require.Equal(t, tt.want, util.StringAny(tt.value), "%#v", tt.value)
	}
}

// TestCombineErrorRunsEveryFunction pins that CombineError runs every
// function given, in order and past a failing one, skips a nil one, and
// returns the errors of all that failed, or nil when none did.
func TestCombineErrorRunsEveryFunction(t *testing.T) {
	var ran []string
	run := func(name string, err error) func() error {
		return func() error {
			ran = append(ran, name)
			return err
		}
	}

	err := util.CombineError(run("first", errors.New("first failed")), nil, run("second", nil), run("third", errors.New("third failed")))

	require.Equal(t, []string{"first", "second", "third"}, ran)
	require.EqualError(t, err, "first failed; third failed")
	require.NoError(t, util.CombineError(run("fourth", nil)))
}

// TestNoErrorRunsTheFunctionAndReportsNoError pins that NoError runs the
// function and answers nil whatever it returned.
func TestNoErrorRunsTheFunctionAndReportsNoError(t *testing.T) {
	ran := false
	require.NoError(t, util.NoError(func() error {
		ran = true
		return errors.New("ignored")
	}))
	require.True(t, ran)
}

// TestSplitByDoublePipeSplitsAScannerAtTheDelimiter pins the tokens a
// bufio.Scanner splitting with SplitByDoublePipe yields: the text between
// the delimiters, a single pipe kept inside a token, and no empty token
// after a trailing delimiter.
func TestSplitByDoublePipeSplitsAScannerAtTheDelimiter(t *testing.T) {
	for input, want := range map[string][]string{
		"a||b||c": {"a", "b", "c"},
		"a|b||c":  {"a|b", "c"},
		"a||":     {"a"},
		"single":  {"single"},
		"":        nil,
	} {
		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(util.SplitByDoublePipe)
		var got []string
		for scanner.Scan() {
			got = append(got, scanner.Text())
		}
		require.NoError(t, scanner.Err())
		require.Equal(t, want, got, input)
	}
}

// TestParseSchemeReadsTheForwardedHeadersBeforeTheConnection pins the order
// ParseScheme reads the scheme of a request in: X-Forwarded-Proto, then
// X-Forwarded-Protocol, X-Forwarded-Ssl on, X-Url-Scheme, and only then
// the TLS state of the connection; a plain request without any is "".
func TestParseSchemeReadsTheForwardedHeadersBeforeTheConnection(t *testing.T) {
	for _, tt := range []struct {
		name   string
		header http.Header
		tls    bool
		want   string
	}{
		{name: "proto", header: http.Header{"X-Forwarded-Proto": {"http"}, "X-Forwarded-Protocol": {"https"}}, tls: true, want: "http"},
		{name: "protocol", header: http.Header{"X-Forwarded-Protocol": {"http"}, "X-Forwarded-Ssl": {"on"}}, want: "http"},
		{name: "ssl_on", header: http.Header{"X-Forwarded-Ssl": {"on"}, "X-Url-Scheme": {"http"}}, want: "https"},
		{name: "ssl_off", header: http.Header{"X-Forwarded-Ssl": {"off"}, "X-Url-Scheme": {"http"}}, tls: true, want: "http"},
		{name: "tls", tls: true, want: "https"},
		{name: "plain", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			maps.Copy(req.Header, tt.header)
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			require.Equal(t, tt.want, util.ParseScheme(req))
		})
	}
}

// TestIsConnClosedRecognizesAConnectionGoneAway pins the errors IsConnClosed
// takes for a connection the other side closed or that is closed already,
// wrapped or not, and that it takes no other error for one.
func TestIsConnClosedRecognizesAConnectionGoneAway(t *testing.T) {
	for _, err := range []error{io.EOF, net.ErrClosed, syscall.ECONNRESET, syscall.EPIPE, errors.Wrap(io.EOF, "read")} {
		require.True(t, util.IsConnClosed(err), "%v", err)
	}
	for _, err := range []error{nil, io.ErrUnexpectedEOF, errors.New("other")} {
		require.False(t, util.IsConnClosed(err), "%v", err)
	}
}

// TestDerefOrFallsBackForANilPointer pins that DerefOr reads what a pointer
// points at and falls back to the value given for nil.
func TestDerefOrFallsBackForANilPointer(t *testing.T) {
	value := 1
	require.Equal(t, 1, util.DerefOr(&value, 9))
	require.Equal(t, 9, util.DerefOr[int](nil, 9))
}

// TestStringAndBytesConvertWithoutCopying pins that StringToBytes and
// BytesToString give back the same text, sharing the memory they were given
// rather than copying it.
func TestStringAndBytesConvertWithoutCopying(t *testing.T) {
	text := "shared"
	data := util.StringToBytes(text)
	require.Equal(t, []byte("shared"), data)
	require.Same(t, unsafe.StringData(text), &data[0])

	back := util.BytesToString(data)
	require.Equal(t, text, back)
	require.Same(t, &data[0], unsafe.StringData(back))
	require.Empty(t, util.BytesToString(nil))
}
