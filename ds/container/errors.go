package container

import (
	"github.com/cockroachdb/errors"
)

// The errors a constructor or a method returns for an argument it cannot
// work without.
var (
	ErrComparisonNil = errors.New("comparison function cannot be nil")
	ErrFuncNil       = errors.New("function cannot be nil")
)
