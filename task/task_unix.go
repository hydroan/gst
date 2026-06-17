//go:build unix

package task

import (
	"os"
	"syscall"
	"time"

	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/util"
	"github.com/shirou/gopsutil/v4/process"
)

// getProcessStats gets process statistics using Unix-specific syscalls
func getProcessStats() {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err == nil {
		logger.Runtime.Infow(
			"Process Stats",
			"UserTime", time.Duration(rusage.Utime.Sec)*time.Second+time.Duration(rusage.Utime.Usec)*time.Microsecond,
			"SystemTime", time.Duration(rusage.Stime.Sec)*time.Second+time.Duration(rusage.Stime.Usec)*time.Microsecond,
			"MaxRSS", rusage.Maxrss, // Maximum resident set size
			"PageFaults", rusage.Majflt, // Major page faults
			"IOIn", rusage.Inblock, // Input operations
			"IOOut", rusage.Oublock, // Output operations
		)
	}

	// Application startup time
	var startTime time.Time
	if info, err := process.NewProcess(int32(os.Getpid())); err == nil { //nolint:gosec
		if ctime, err := info.CreateTime(); err == nil {
			startTime = time.Unix(ctime/1000, (ctime%1000)*int64(time.Millisecond))
			logger.Runtime.Infow(
				"Application Uptime",
				"StartTime", startTime,
				"Uptime", util.FormatDurationSmart(time.Since(startTime), 2),
			)
		}
	}
}
