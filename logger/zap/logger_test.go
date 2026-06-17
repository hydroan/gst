package zap_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/types"
)

var (
	msg10    = "0000000000"
	msg100   = strings.Repeat(msg10, 10)
	msg1000  = strings.Repeat(msg10, 100)
	msg10000 = strings.Repeat(msg10, 1000)

	keyValues10  = []string{}
	keyValues100 = []string{}
)

func init() {
	// init keyValues10
	for i := range 10 {
		keyValues10 = append(keyValues10, "key"+strconv.Itoa(i), "value"+strconv.Itoa(i))
	}
	// init keyValues100
	for i := range 100 {
		keyValues100 = append(keyValues100, "key"+strconv.Itoa(i), "value"+strconv.Itoa(i))
	}
}

func createLogger(b *testing.B, filename string) types.Logger {
	b.Helper()
	b.Setenv(config.LOGGER_FILE, filename)
	b.Setenv(config.LOGGER_DIR, "/tmp/gst")
	if err := config.Init(); err != nil {
		b.Fatal(err)
	}
	l := zap.New("")
	return l
}

func TestLogger(b *testing.T) {
	b.Setenv(config.LOGGER_FILE, "")
	b.Setenv(config.LOGGER_DIR, "/tmp/gst")
	if err := config.Init(); err != nil {
		b.Fatal(err)
	}
	l := zap.New("")
	l.With("key1", "value1", "key2", "value2").Info("hello world")
}

func BenchmarkLogger_File10(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg10)
	}
}

func BenchmarkLogger_File100(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg100)
	}
}

func BenchmarkLogger_File1000(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg1000)
	}
}

func BenchmarkLogger_File10000(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg10000)
	}
}

func BenchmarkLogger_Discard10(b *testing.B) {
	l := createLogger(b, "/dev/null")

	for b.Loop() {
		l.Infoz(msg10)
	}
}

func BenchmarkLogger_Discard100(b *testing.B) {
	l := createLogger(b, "/dev/null")

	for b.Loop() {
		l.Infoz(msg100)
	}
}

func BenchmarkLogger_Discard1000(b *testing.B) {
	l := createLogger(b, "/dev/null")
	for b.Loop() {
		l.Infoz(msg1000)
	}
}

func BenchmarkLogger_Discard10000(b *testing.B) {
	l := createLogger(b, "/dev/null")

	for b.Loop() {
		l.Infoz(msg10000)
	}
}

func BenchmarkLogger_With10(b *testing.B) {
	l := createLogger(b, "test.log")

	b.ReportAllocs()
	for b.Loop() {
		l.With(keyValues10...).Info(msg10)
	}
}

func BenchmarkLogger_With100(b *testing.B) {
	l := createLogger(b, "test.log")

	b.ReportAllocs()
	for b.Loop() {
		l.With(keyValues100...).Info(msg10)
	}
}
