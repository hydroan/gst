package util

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
)

// TraceID returns a new W3C trace id for an execution outside any trace, a
// request that brought none, a cron round or a leader tenure: 16 random bytes
// in lower-case hex, never all zero, such as 4bf92f3577b34da6a3ce929d0e0e4736,
// the form the X-Trace-ID header carries and the propagator accepts.
func TraceID() string { return randomHex(16) }

// SpanID returns a new W3C span id, 8 random bytes in lower-case hex, never
// all zero, such as 00f067aa0ba902b7.
func SpanID() string { return randomHex(8) }

// randomHex returns n random bytes, n a multiple of 8, in lower-case hex,
// drawing again on the all-zero value the W3C trace context treats as
// absent. The ids are correlation keys, not secrets, so they come from the
// math/rand/v2 generator, the source the OpenTelemetry SDK's default id
// generator draws from, rather than the slower crypto/rand.
func randomHex(n int) string {
	b := make([]byte, n)
	for {
		for i := 0; i < n; i += 8 {
			binary.LittleEndian.PutUint64(b[i:], rand.Uint64()) //nolint:gosec // see the doc comment: not a secret
		}
		if slices.ContainsFunc(b, func(x byte) bool { return x != 0 }) {
			return hex.EncodeToString(b)
		}
	}
}

// PanicError turns a recovered panic value into an error carrying the stack
// of the panic site. It must be called from the deferred function that
// recovered, while the goroutine is still unwinding: the frames captured then
// still include the line that panicked, whereas a stack taken after recovery
// would only show the recovering package. A panic value that is already an
// error keeps its own, deeper stack if it has one — the error_stack field
// reports the deepest stack in the chain — and gains this one otherwise.
func PanicError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return errors.WithStack(err)
	}
	return errors.Newf("%v", recovered)
}

// BuildTLSConfig builds the TLS configuration a client dials with: the
// certificate and key pair when both files are given, the CA bundle when
// caFile is, and insecureSkipVerify as given.
func BuildTLSConfig(certFile, keyFile, caFile string, insecureSkipVerify bool) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec
	}

	if len(certFile) > 0 && len(keyFile) > 0 {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load x509 key pair")
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if len(caFile) > 0 {
		caData, err := os.ReadFile(caFile)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read ca file")
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caData) {
			return nil, errors.New("failed to append ca certs")
		}
		tlsConfig.RootCAs = caPool
	}

	return tlsConfig, nil
}

// FormatDurationSmart formats a time.Duration into a string, picking the unit
// that keeps the value readable:
//   - Less than 1µs: nanoseconds (e.g., "900ns").
//   - Less than 1ms: microseconds with the given precision (e.g., "18.40µs").
//   - Less than 1s: milliseconds with the given precision (e.g., "123.451ms").
//   - Less than 1min: seconds with the given precision (e.g., "2.013s").
//   - 1min or more: minutes with the given precision (e.g., "1.502min").
//
// Nanoseconds are the resolution of time.Duration itself, so that tier prints
// an integer and ignores precision; decimals there would only claim a
// resolution that does not exist.
//
// Negative durations are supported and formatted with a '-' sign. The precision
// parameter controls the number of digits after the decimal point (2 by
// default, minimum 0, maximum 9).
func FormatDurationSmart(d time.Duration, precisions ...int) string {
	precision := 2
	if len(precisions) > 0 {
		precision = precisions[0]
	}
	if precision < 0 {
		precision = 0
	}
	if precision > 9 {
		precision = 9
	}

	// Format string, e.g., "%.2f"
	format := "%." + strconv.Itoa(precision) + "f%s"

	ns := d.Nanoseconds()
	absNs := ns
	if absNs < 0 {
		absNs = -absNs
	}

	switch {
	case absNs < 1e3: // <1µs
		return strconv.FormatInt(ns, 10) + "ns"
	case absNs < 1e6: // <1ms
		return fmt.Sprintf(format, float64(ns)/1e3, "µs")
	case absNs < 1e9: // <1s
		return fmt.Sprintf(format, float64(ns)/1e6, "ms")
	case absNs < 60*1e9: // <1min
		return fmt.Sprintf(format, float64(ns)/1e9, "s")
	default:
		return fmt.Sprintf(format, float64(ns)/(60*1e9), "min")
	}
}
