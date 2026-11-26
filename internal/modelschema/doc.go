// Package modelschema resolves what a model struct declares about its table:
// its database columns, the kind of value each stores and the URL query name
// each takes (see Column and ClassifyColumn), the columns the framework fixes
// the names of (see ValidateFrameworkColumns and DefaultCursorColumn), the
// secondary indexes it declares (see Index), its optimistic-locking column
// (see Version), the columns identifying one of its rows (see
// IdentifyingColumns), its associations (see AssociationFields) and the
// fields it declares read-only to clients (see IsReadOnly); and the
// fingerprint of everything a set of models declares about their tables (see
// Fingerprint).
//
// Beside the kind of value a column stores stands the reading of a value a
// client sends for it: an instant, a boolean and a number are each read the
// one way the framework takes them (see ParseTime, ParseBool, ParseUint and
// ValidateNumericValue), and an instant compared with a calendar day is taken
// as its day in UTC (see UTCDay).
//
// The rules a model's declaration is held to before its table is prepared
// are gathered by Validate: this package's own, and the ones the packages
// above it register for what is theirs to ask of a model (see
// RegisterValidator and RegisterFixedColumn), none of which this package
// names.
//
// It is the single place where a Go struct field is mapped to a database
// column: the mapping is delegated to gorm's own schema parser, so column
// names always match what gorm actually emits, including the column tag,
// the ignore markers, embedded struct lifting, and gorm's commonInitialisms
// handling (which a plain snake case conversion does not reproduce). The
// facts built on the columns read them through the same parser.
package modelschema
