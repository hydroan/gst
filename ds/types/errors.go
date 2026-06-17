package types

import (
	"github.com/cockroachdb/errors"
)

var (
	ErrComparisonNil = errors.New("comparison function cannot be nil")
	ErrEqualNil      = errors.New("equality function cannot be nil")
	ErrFuncNil       = errors.New("function cannot be nil")
)
