package redis

import (
	"strings"

	"github.com/hydroan/gst/config"
)

// Key returns the storage key for key: the configured namespace, the
// separator, then the key itself. A key already carrying the namespace is
// returned unchanged, so applying it twice is harmless.
//
// Every operation in this package applies it. A caller reaching past them —
// working through the Client handle for a data structure this package does
// not wrap — has to apply it too, or its keys land outside the namespace
// everything else shares.
func Key(key string) string {
	namespace := strings.Trim(config.App.Redis.Namespace, ": ")
	if namespace == "" || hasNamespace(key, namespace) {
		return key
	}
	return namespace + ":" + key
}

// hasNamespace reports whether key already starts with namespace followed by
// the separator. It compares in place because building the prefix to hand to
// strings.HasPrefix would allocate on every key, on every Redis operation.
func hasNamespace(key, namespace string) bool {
	return len(key) > len(namespace) && key[len(namespace)] == ':' && key[:len(namespace)] == namespace
}

// namespacedKeys is Key over a slice, for the operations taking more than one
// key.
func namespacedKeys(ks []string) []string {
	if len(ks) == 0 {
		return ks
	}
	result := make([]string, len(ks))
	for i := range ks {
		result[i] = Key(ks[i])
	}
	return result
}

// pattern is Key for a scan pattern: the prefix gains a trailing wildcard
// when it has none.
func pattern(prefix string) string {
	if !strings.HasSuffix(prefix, "*") {
		prefix += "*"
	}
	return Key(prefix)
}
