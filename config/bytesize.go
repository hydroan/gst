package config

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dustin/go-humanize"
)

// ByteSize is a size in bytes read from a configuration value: a number with
// its unit, "64KB" being 64 times 1000 bytes and "8MiB" 8 times 1024 squared,
// or a bare number of bytes. Empty is zero, which each key carrying a size
// reads as its own default.
type ByteSize uint64

// UnmarshalText reads text as a size, parsed with github.com/dustin/go-humanize;
// text that is not one, a negative size among it, is a configuration error
// naming the key.
func (s *ByteSize) UnmarshalText(text []byte) error {
	value := strings.TrimSpace(string(text))
	if value == "" {
		*s = 0
		return nil
	}
	size, err := humanize.ParseBytes(value)
	if err != nil {
		return errors.Newf("want a size with its unit, such as 64KB or 8MiB, got %q", text)
	}
	*s = ByteSize(size)
	return nil
}
