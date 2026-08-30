package redis

import (
	"strings"

	"github.com/hydroan/gst/config"
)

func redisKey(key string) string {
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

func redisKeys(keys []string) []string {
	if len(keys) == 0 {
		return keys
	}
	result := make([]string, len(keys))
	for i := range keys {
		result[i] = redisKey(keys[i])
	}
	return result
}

func redisPattern(prefix string) string {
	if !strings.HasSuffix(prefix, "*") {
		prefix += "*"
	}
	return redisKey(prefix)
}
