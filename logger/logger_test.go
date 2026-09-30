package logger_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
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
	b.Setenv(config.LOGGER_OUTPUT, string(config.LoggerOutputFile))
	b.Setenv(config.LOGGER_FILE, filename)
	b.Setenv(config.LOGGER_DIR, b.TempDir())
	if err := config.Init(); err != nil {
		b.Fatal(err)
	}
	l := logger.New("")
	return l
}

func BenchmarkLoggerFile10(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg10)
	}
}

func BenchmarkLoggerFile100(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg100)
	}
}

func BenchmarkLoggerFile1000(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg1000)
	}
}

func BenchmarkLoggerFile10000(b *testing.B) {
	l := createLogger(b, "test.log")

	for b.Loop() {
		l.Infoz(msg10000)
	}
}

func BenchmarkLoggerWith10(b *testing.B) {
	l := createLogger(b, "test.log")

	b.ReportAllocs()
	for b.Loop() {
		l.With(keyValues10...).Info(msg10)
	}
}

func BenchmarkLoggerWith100(b *testing.B) {
	l := createLogger(b, "test.log")

	b.ReportAllocs()
	for b.Loop() {
		l.With(keyValues100...).Info(msg10)
	}
}
