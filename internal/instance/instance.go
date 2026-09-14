// Package instance answers one question — which process is this — for every
// part of the framework that tells replicas apart: the holder a lease is
// written under, the field every log entry carries, the tracing resource,
// the origin a replicated cache event names. One answer, read everywhere,
// instead of each place deriving its own by its own rule.
//
// The identity is the hostname — the pod name under Kubernetes, the
// container id under Docker — followed by a short random suffix drawn once
// at process start. The hostname alone would not do: two processes on one
// host share it, and a restarted container keeps it, yet the restarted
// process must not pass for the one it replaced — a lease the old process
// held is not the new one's to keep.
package instance

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"sync"
)

// suffixBytes sizes the random suffix: four bytes render as eight hex
// characters, enough that two starts never draw the same suffix in practice
// while the identity stays short enough to read in a log line.
const suffixBytes = 4

// unknownHostname stands in for the hostname when the system cannot report
// one, so the identity keeps its shape; the suffix alone still makes it
// unique.
const unknownHostname = "unknown"

// identity is what the process knows about itself.
type identity struct {
	hostname string
	id       string
}

// current computes the identity on first use and hands out the same one for
// the life of the process.
var current = sync.OnceValue(func() identity {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	return identityOf(strings.TrimSpace(hostname))
})

// identityOf builds the identity of a process on hostname, drawing a fresh
// suffix: called once per process, but a test may call it more.
func identityOf(hostname string) identity {
	suffix := make([]byte, suffixBytes)
	if _, err := rand.Read(suffix); err != nil {
		// Never happens: crypto/rand.Read fills the slice or does not return.
		panic("instance: cannot draw the random suffix: " + err.Error())
	}

	name := hostname
	if name == "" {
		name = unknownHostname
	}
	return identity{hostname: hostname, id: name + "-" + hex.EncodeToString(suffix)}
}

// ID returns this process's identity, "<hostname>-<8 hex characters>": the
// same value for the life of the process, a different one for every start.
func ID() string {
	return current().id
}

// Hostname returns the hostname the identity is built on, or "" when the
// system cannot report one.
func Hostname() string {
	return current().hostname
}
