// Package modelschema resolves what a model struct declares about its table:
// its database columns, the kind of value each stores and the URL query name
// each takes (see Column and ClassifyColumn), the columns the framework fixes
// the names of (see ValidateFrameworkColumns and DefaultCursorColumn), the
// secondary indexes it declares (see Index), its optimistic-locking column
// (see Version), the columns identifying one of its rows (see
// IdentifyingColumns) and its associations (see AssociationFields); and the
// fingerprint of everything a set of models declares about their tables (see
// Fingerprint).
//
// It is the single place where a Go struct field is mapped to a database
// column: the mapping is delegated to gorm's own schema parser, so column
// names always match what gorm actually emits, including the column tag,
// the ignore markers, embedded struct lifting, and gorm's commonInitialisms
// handling (which a plain snake case conversion does not reproduce). The
// facts built on the columns read them through the same parser.
package modelschema
