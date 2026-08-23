package tracing

import (
	"strings"
	"testing"
)

// secret stands in for the bearer credentials callers build cache keys from:
// session ids that are the login cookie's own value, MFA challenge ids and
// password-reset tokens.
const (
	secret = "9f8e7d6c5b4a39281706f5e4d3c2b1a0"
	key    = "sample:session:data:" + secret
)

// TestAttributesNeverCarryTheKey is the regression guard for credential
// exposure through traces. Every span attribute the wrapper sets is built
// here, so a key reaching a span verbatim fails this.
func TestAttributesNeverCarryTheKey(t *testing.T) {
	w := NewWrapper[string](nil, "sample")
	for _, operation := range []string{"get", "set", "delete", "exists"} {
		for _, attr := range w.attributes(operation, key) {
			value := attr.Value.String()
			if strings.Contains(value, secret) {
				t.Fatalf("attribute %q leaks the key: %q", attr.Key, value)
			}
		}
	}
}

// TestKeyNamespaceKeepsTheDomain asserts the attribute still says which cache
// domain was touched, which is what a span is read for.
func TestKeyNamespaceKeepsTheDomain(t *testing.T) {
	if got := keyNamespace(key); got != "sample:session:data" {
		t.Fatalf("want the domain, got %q", got)
	}
	if got := keyNamespace("noseparator"); got != "" {
		t.Fatalf("want no namespace for a key without a separator, got %q", got)
	}
}

// TestKeyDigestCorrelatesWithoutRevealing asserts the digest is stable for a
// key, distinct between keys, and short enough not to be a re-encoding of it.
func TestKeyDigestCorrelatesWithoutRevealing(t *testing.T) {
	first := keyDigest(key)
	if first != keyDigest(key) {
		t.Fatal("want the same digest for the same key")
	}
	if first == keyDigest(key+"x") {
		t.Fatal("want different digests for different keys")
	}
	if len(first) != 16 {
		t.Fatalf("want a truncated digest, got %d characters", len(first))
	}
}
