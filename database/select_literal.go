package database

import (
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/types"
)

// This file is the constant side of the select builder: how a Literal renders
// and what it may hold; see types.Literal for the contract.

// Errors reported while a constant is built; see the select errors for why
// they fail fast.
var (
	ErrInvalidLiteral      = errors.New("literal is not a plain identifier")
	ErrLiteralWithoutAlias = errors.New("a literal needs an alias, name it with As")
)

// literalExpr renders a constant as a string literal. validate has held the
// value to a plain identifier, so the quotes cannot be closed from inside and
// the spelling is the same on every dialect.
func literalExpr(value string) string { return "'" + value + "'" }

// validateLiteral checks a constant term: its value must be safe to inline,
// and it needs a name to project under.
func validateLiteral(t types.Term) error {
	if !aliasPattern.MatchString(t.Literal) {
		return errors.Wrapf(ErrInvalidLiteral, "%q", t.Literal)
	}
	if len(t.Alias) == 0 {
		return errors.Wrapf(ErrLiteralWithoutAlias, "%q", t.Literal)
	}
	return nil
}
