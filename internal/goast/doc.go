// Package goast holds the go/ast helpers the framework's tooling shares: the
// questions asked of an expression, a statement or a declaration (IsQualified,
// AlwaysLeaves, FindStructTypeSpec), the edits of a file's imports and
// identifiers (InsertImportSpec, RenameIdent), the recognition of the
// framework's service base and of test functions (IsServiceBase,
// IsTestCaseFunc), and the header and line positions a generated file is
// printed with (GeneratedHeader, LineSet).
package goast
