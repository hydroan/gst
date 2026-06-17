//go:build windows

package task

import (
	"os"
	"time"

	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/util"
	"github.com/shirou/gopsutil/v4/process"
)

// getProcessStats gets process statistics using Windows-compatible methods
func getProcessStats() {
	// On Windows, we use gopsutil for cross-platform process information
	if info, err := process.NewProcess(int32(os.Getpid())); err == nil {
		// Get memory info
		if memInfo, err := info.MemoryInfo(); err == nil {
			logger.Runtime.Infow(
				"Process Stats",
				"RSS", memInfo.RSS, // Resident Set Size
				"VMS", memInfo.VMS, // Virtual Memory Size
			)
		}

		// Get CPU times
		if cpuTimes, err := info.Times(); err == nil {
			logger.Runtime.Infow(
				"Process CPU Stats",
				"UserTime", time.Duration(cpuTimes.User*float64(time.Second)),
				"SystemTime", time.Duration(cpuTimes.System*float64(time.Second)),
			)
		}

		// Get application startup time
		if ctime, err := info.CreateTime(); err == nil {
			startTime := time.Unix(ctime/1000, (ctime%1000)*int64(time.Millisecond))
			logger.Runtime.Infow(
				"Application Uptime",
				"StartTime", startTime,
				"Uptime", util.FormatDurationSmart(time.Since(startTime), 2),
			)
		}
	} else {
		logger.Runtime.Warnw("Failed to get process information", "error", err)
	}
}
