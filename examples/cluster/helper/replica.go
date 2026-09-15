// Package helper holds what the example's packages share.
package helper

import (
	"os"
	"strconv"

	"github.com/hydroan/gst/config"
)

// Replica names the replica this process is: the host — the container or
// the pod — and the port, which tells the replicas of one host apart when the
// scenarios run locally. The framework's logs carry an instance id of their
// own; the rows the example writes carry this name, so the two are matched
// by host.
func Replica() string {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	return host + ":" + strconv.Itoa(config.App.Server.Port)
}
