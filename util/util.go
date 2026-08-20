package util

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"
	"unsafe"

	tcping "github.com/cloverstd/tcping/ping"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types/consts"
	probing "github.com/prometheus-community/pro-bing"
	"github.com/rs/xid"
	"github.com/spf13/cast"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TraceID() string { return xid.New().String() }
func SpanID() string  { return xid.New().String() }

// Pointer will return a pointer to T with given value.
func Pointer[T comparable](t T) *T {
	if reflect.DeepEqual(t, nil) {
		return new(T)
	}
	return &t
}

// Depointer will return a T with given value.
func Depointer[T comparable](t *T) T {
	if t == nil {
		return *new(T)
	}
	return *t
}

func SafePointer[T any](v T) T {
	if reflect.DeepEqual(v, nil) {
		return *new(T)
	}
	if reflect.DeepEqual(v, reflect.Zero(reflect.TypeOf(v)).Interface()) {
		return *new(T)
		// return reflect.Zero(reflect.TypeOf(v)).Interface().(T)
	}
	return v
}

// CharSpliter is the custom split function for bufio.Scanner.
func CharSpliter(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if len(data) == 0 {
		return 0, nil, nil
	}
	if atEOF {
		return len(data), data, nil
	}
	if data[0] == '|' {
		return 1, data[:1], nil
	}
	return 0, nil, nil
}

// SplitByDoublePipe is the custom split function for bufio.Scanner.
func SplitByDoublePipe(data []byte, atEOF bool) (advance int, token []byte, err error) {
	delimiter := []byte("||")

	// Search for the delimiter in the input data
	if i := bytes.Index(data, delimiter); i >= 0 {
		return i + len(delimiter), data[:i], nil
	}

	// If the delimiter is not found, and it's at the end of the input data, return it
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}

	// If no delimiter is found, return no data and wait for more input
	return 0, nil, nil
}

// RunOrDie will panic when error encountered.
func RunOrDie(fn func() error) {
	if err := fn(); err != nil {
		name := GetFunctionName(fn)
		HandleErr(fmt.Errorf("%s error: %+w", name, err))
	}
}

// HandleErr will call os.Exit() when any error encountered.
func HandleErr(err error, notExit ...bool) {
	var flag bool
	if len(notExit) != 0 {
		flag = notExit[0]
	}
	if err != nil {
		fmt.Println(err)
		if !flag {
			os.Exit(1)
		}
	}
}

// CheckErr just check error and print it.
func CheckErr(err error) {
	HandleErr(err, true)
}

// StringAny format anything to string.
func StringAny(x any) string {
	if x == nil {
		return ""
	}
	if v, ok := x.(fmt.Stringer); ok {
		return v.String()
	}

	switch v := x.(type) {
	case string:
		return v
	case []byte:
		return *(*string)(unsafe.Pointer(&v))
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func GetFunctionName(x any) string {
	switch v := x.(type) {
	case uintptr:
		return runtime.FuncForPC(v).Name()
	default:
		return runtime.FuncForPC(reflect.ValueOf(x).Pointer()).Name()
	}
}

func ParseScheme(req *http.Request) string {
	if scheme := req.Header.Get("X-Forwarded-Proto"); len(scheme) != 0 {
		return scheme
	}
	if scheme := req.Header.Get("X-Forwarded-Protocol"); len(scheme) != 0 {
		return scheme
	}
	if ssl := req.Header.Get("X-Forwarded-Ssl"); ssl == "on" {
		return "https"
	}
	if scheme := req.Header.Get("X-Url-Scheme"); len(scheme) != 0 {
		return scheme
	}
	if req.TLS != nil {
		return "https"
	}
	return ""
}

// Tcping work like command `tcping`.
func Tcping(host string, port int, timeout time.Duration) bool {
	if timeout < 500*time.Millisecond {
		timeout = 1 * time.Second
	}
	_, _, _, res := _tcping(host, port, 1, 1, timeout)
	return res.SuccessCounter == 1
}

func _tcping(host string, port, count int, interval, timeout time.Duration) (minLatency, maxLatency, avgLatency time.Duration, result *tcping.Result) {
	pinger := tcping.NewTCPing()
	pinger.SetTarget(&tcping.Target{
		Protocol: tcping.TCP,
		Host:     host,
		Port:     port,
		Counter:  count,
		Interval: interval,
		Timeout:  timeout,
	})
	<-pinger.Start()
	if pinger.Result() == nil {
		return minLatency, maxLatency, avgLatency, result
	}
	return pinger.Result().MinDuration, pinger.Result().MaxDuration, pinger.Result().Avg(), pinger.Result()
}

// Ping work like command `ping`.
// If target ip is reachable, return true, nil,
// If target ip is unreachable, return false, nil,
// If error encountered, return false, error.
// More usage see tests in `pkg/util/util_test.go`.
func Ping(ip string, timeout time.Duration) (bool, error) {
	if len(ip) == 0 {
		return false, errors.New("ip is empty")
	}
	if timeout < 500*time.Millisecond {
		timeout = 1 * time.Second
	}
	pinger, err := probing.NewPinger(ip)
	if err != nil {
		return false, err
	}
	pinger.Count = 1
	pinger.Timeout = timeout

	err = pinger.Run()
	if err != nil {
		return false, err
	}
	return pinger.Statistics().PacketsSent == pinger.Statistics().PacketsRecv, nil
}

// NoError call fn and always return nil.
func NoError(fn func() error) error {
	if err := fn(); err != nil {
		zap.S().Warn(err)
	}
	return nil
}

// Contains check T in slice.
func Contains[T comparable](slice []T, elem T) bool {
	return slices.Contains(slice, elem)
}

// CombineError combine error from fns.
func CombineError(fns ...func() error) error {
	errs := make([]error, len(fns))
	for i := range fns {
		if fns[i] == nil {
			continue
		}
		errs[i] = fns[i]()
	}
	return multierr.Combine(errs...)
}

// FileExists check file exists.
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

// Round returns a rounded version of x with a specified precision.
//
// The precision parameter specifies the number of decimal places to round to.
// Round uses the "round half away from zero" rule to break ties.
//
// Examples:
//
//	Round(3.14159, 2) returns 3.14
//	Round(3.14159, 1) returns 3.1
//	Round(-3.14159, 1) returns -3.1
func Round[T float32 | float64](value T, precision uint) T {
	ratio := math.Pow(10, float64(precision))
	return T(math.Round(float64(value)*ratio) / ratio)
}

// IPv6ToIPv4 converts IPv6 to IPv4 if possible
func IPv6ToIPv4(ipStr string) string {
	// If its ipv4, return.
	if net.ParseIP(ipStr).To4() != nil {
		return ipStr
	}

	// handle IPv6 localhost
	if strings.HasPrefix(ipStr, "::") {
		return "127.0.0.1"
	}

	// handle IPv4-mapped IPv6 addresses
	// eg ::ffff:192.0.2.128 or ::ffff:c000:280
	if strings.Contains(ipStr, "::ffff:") {
		split := strings.Split(ipStr, "::ffff:")
		if len(split) == 2 {
			if ip := net.ParseIP(split[1]).To4(); ip != nil {
				return ip.String()
			}
		}
	}

	// handle embedded IPv4 address
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ipStr
	}

	ip4 := ip.To4()
	if ip4 != nil {
		return ip4.String()
	}

	return ipStr
}

// BuildTLSConfig creates a TLS configuration from the etcd config
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

func HashID(fields ...string) string {
	if len(fields) == 0 {
		return ""
	}
	hash := sha256.Sum256([]byte(strings.Join(fields, ":")))
	return hex.EncodeToString(hash[:16])
}

// FormatDurationSmart formats a time.Duration into a string, picking the unit
// that keeps the value readable:
//   - Less than 1µs: nanoseconds (e.g., "900ns").
//   - Less than 1ms: microseconds with the specified precision (e.g., "18.40µs").
//   - Less than 1s: milliseconds with the specified precision (e.g., "123.451ms").
//   - Less than 1min: seconds with the specified precision (e.g., "2.013s").
//   - 1min or more: minutes with the specified precision (e.g., "1.502min").
//
// Nanoseconds are the resolution of time.Duration itself, so that tier prints an
// integer and ignores precision; decimals there would only claim a resolution
// that does not exist.
//
// Negative durations are supported and formatted with a '-' sign.
// The precision parameter controls the number of digits after the decimal point (minimum 0, maximum 9).
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

// LogDuration is the only supported way to log how long an operation took. It
// emits one field pair, integer nanoseconds plus the same value rendered for
// reading:
//
//	{"msg": "executed query", "duration": 2120000, "duration_human": "2.12ms"}
//
// Both halves are needed and neither pays for the other. Log stores index the
// nanosecond field as an integer, so "slower than 500ms" is a range filter and
// averages and percentiles are aggregations; because time.Duration is integral
// there is never a mix of integer and floating point values for the store to
// reconcile, and no unit is left implicit in the field name. The rendered
// string is what a person reads while tailing a log, where raw nanoseconds are
// unreadable.
//
// Field names come from consts and are written here alone, so no call site can
// spell them differently and no query has to know which component produced the
// entry.
func LogDuration(d time.Duration) zap.Field {
	return zap.Inline(logDuration(d))
}

// logDuration renders an elapsed time as the field pair documented on
// LogDuration. It is inlined rather than nested so both fields sit at the top
// level of the entry, where the field name alone locates them.
type logDuration time.Duration

func (d logDuration) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt64(consts.LOG_DURATION, int64(d))
	enc.AddString(consts.LOG_DURATION_HUMAN, FormatDurationSmart(time.Duration(d)))
	return nil
}

func SafeGo(fn func(), names ...any) {
	go func() {
		var name string
		if len(names) == 0 {
			name = "unnamed goroutine"
		}
		var nameBuilder strings.Builder
		for _, v := range names {
			nameBuilder.WriteByte(':')
			nameBuilder.WriteString(cast.ToString(v))
		}
		name += nameBuilder.String()

		defer func() { Recovery(name) }()

		fn()
	}()
}

// Recovery global Recovery recover panic
func Recovery(name string) {
	if err := recover(); err != nil {
		fmt.Fprintf(os.Stdout, "%s recover: %+v\n%s", name, err, debug.Stack())
	}
}
