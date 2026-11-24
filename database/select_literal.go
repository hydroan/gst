package database

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gst"
)

// This file is the constant side of the select builder: how a Literal renders
// and what it may hold; see gst.Literal for the contract.

// Errors reported while a constant is built; see the select errors for why
// they fail fast.
var (
	ErrInvalidLiteral      = errors.New("literal is not a plain identifier")
	ErrLiteralWithoutAlias = errors.New("a literal needs an alias, name it with As")
)

// validateLiteral checks a constant term: its value must be safe to inline,
// and it needs a name to project under.
func validateLiteral(t gst.Term) error {
	if !aliasPattern.MatchString(gst.TermLiteralOf(t)) {
		return errors.Wrapf(ErrInvalidLiteral, "%q", gst.TermLiteralOf(t))
	}
	if len(gst.TermAliasOf(t)) == 0 {
		return errors.Wrapf(ErrLiteralWithoutAlias, "%q", gst.TermLiteralOf(t))
	}
	return nil
}
