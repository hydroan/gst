package database

import (
	"strings"

	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
)

// This file is the single place the framework behaves differently per database
// dialect. Everything gorm already abstracts stays out of it: identifier
// quoting goes through Statement.Quote, parameter binding through
// clause.Expr and AddVar, predicate nesting through clause.And and clause.Or,
// soft deletes through the model schema, and JSON containment through
// gorm.io/datatypes. What remains here is what gorm's Dialector does not
// model — its interface covers naming, DDL types, bind vars, quoting and log
// rendering, and nothing about functions or operators.
//
// Keeping these together makes the question "where does the framework diverge
// per database" answerable by reading one file, and makes adding a dialect a
// matter of extending the tables below rather than hunting for branches.

// dialect is the set of databases the framework adapts for. The values are the
// names the gorm drivers report from Dialector.Name().
type dialect string

const (
	dialectMySQL      dialect = "mysql"
	dialectPostgres   dialect = "postgres"
	dialectSQLite     dialect = "sqlite"
	dialectClickHouse dialect = "clickhouse"
)

// dialectOf reports the dialect of a connection handle. An unknown or unset
// driver reads as MySQL, which is the framework's primary dialect and the one
// whose spelling the fallbacks below use.
func dialectOf(ins *gorm.DB) dialect {
	if ins == nil || ins.Dialector == nil {
		return dialectMySQL
	}
	return dialect(strings.ToLower(ins.Dialector.Name()))
}

// dialect reports which database the chain talks to.
func (db *database[M]) dialect() dialect {
	if db == nil {
		return dialectMySQL
	}
	return dialectOf(db.ins)
}

// regexpOperator returns the operator that matches a value against a regular
// expression. PostgreSQL spells it as an operator, everything else as a
// keyword.
func (db *database[M]) regexpOperator() string {
	if db.dialect() == dialectPostgres {
		return "~"
	}
	return "REGEXP"
}

// timeComparableExpr renders one side of a time comparison so both sides
// agree on a comparable form. SQLite is the dialect that needs it: it stores
// time as text whose zone suffix and sub-second width vary with the value
// that was written, and a lexical comparison across those spellings misorders
// instants (an equal instant with a longer spelling reads as greater, which
// leaks boundary rows back into a cursor page). strftime normalizes both
// sides to UTC millisecond text. Every other dialect compares time natively,
// so the operand passes through and their SQL is unchanged.
//
// The operand is an already-quoted column or a bind placeholder; millisecond
// precision matches the DATETIME(3) resolution the framework uses elsewhere.
func (db *database[M]) timeComparableExpr(operand string) string {
	if db.dialect() == dialectSQLite {
		return "strftime('%Y-%m-%d %H:%M:%f', " + operand + ")"
	}
	return operand
}

// textPatternColumn renders a column so the text pattern operators (LIKE and
// the regex operator) accept it. PostgreSQL is strict about operand types:
// json and jsonb carry no text operators, so a JSON column is cast to text
// there. MySQL and SQLite already read a JSON value in its text form in a
// string context, and skipping the cast keeps their SQL unchanged.
func (db *database[M]) textPatternColumn(quotedColumn string, isJSON bool) string {
	if isJSON && db.dialect() == dialectPostgres {
		return "CAST(" + quotedColumn + " AS TEXT)"
	}
	return quotedColumn
}

// likeEscapeSuffix declares the LIKE escape character used by filters.
// The pipe is chosen over the conventional backslash because backslash inside
// a SQL string literal is itself an escape character in MySQL but a plain
// character in PostgreSQL/SQLite, so no single spelling of ESCAPE '\' parses
// the same way across those dialects. The pipe needs no such disambiguation.
// ClickHouse parses no ESCAPE clause at all — its LIKE reads backslash as the
// escape character natively — so the suffix is empty there and the pattern
// escaper below switches to backslash to match.
func (db *database[M]) likeEscapeSuffix() string {
	if db.dialect() == dialectClickHouse {
		return ""
	}
	return " ESCAPE '|'"
}

// The pattern escapers rewrite a filter value into a literal LIKE pattern
// fragment: client values are literals, not pattern language, so the
// wildcards and the escape character itself are escaped — with the pipe under
// an ESCAPE '|' clause, with backslash where the dialect hardwires it.
var (
	pipeLikeEscaper      = strings.NewReplacer("|", "||", "%", `|%`, "_", `|_`)
	backslashLikeEscaper = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
)

func (db *database[M]) escapeLikePattern(value string) string {
	if db.dialect() == dialectClickHouse {
		return backslashLikeEscaper.Replace(value)
	}
	return pipeLikeEscaper.Replace(value)
}

// timeBucketExpr renders a truncated time group key over an already quoted
// column.
//
// The result is a string label rather than each driver's own date type, which
// keeps one Go type in the result row: a report struct would otherwise depend
// on the database behind it. The labels sort lexically in chronological order,
// so ordering by a bucket needs no second expression.
//
// The truncation reads the value as the database stores it. Reporting in a
// different timezone than the stored one is not expressed here; adding it
// would be a new bucket constructor rather than a change to this one, because
// the timezone argument differs per dialect and MySQL's CONVERT_TZ needs the
// timezone tables loaded.
func (db *database[M]) timeBucketExpr(quotedColumn string, bucket types.TimeBucket) string {
	switch db.dialect() {
	case dialectPostgres:
		return "to_char(" + quotedColumn + ", '" + postgresBucketFormat(bucket) + "')"
	case dialectSQLite:
		return "strftime('" + strftimeBucketFormat(bucket) + "', " + quotedColumn + ")"
	case dialectClickHouse:
		return "formatDateTime(" + quotedColumn + ", '" + strftimeBucketFormat(bucket) + "')"
	default:
		return "DATE_FORMAT(" + quotedColumn + ", '" + strftimeBucketFormat(bucket) + "')"
	}
}

// The bucket format tables below all render the same labels: "2006-01-02
// 15:00:00" for an hour, "2006-01-02" for a day and "2006-01" for a month.

// strftimeBucketFormat serves the dialects using strftime-style specifiers:
// MySQL's DATE_FORMAT, SQLite's strftime and ClickHouse's formatDateTime all
// read the same escapes for these fields.
func strftimeBucketFormat(bucket types.TimeBucket) string {
	switch bucket {
	case types.TimeBucketHour:
		return "%Y-%m-%d %H:00:00"
	case types.TimeBucketMonth:
		return "%Y-%m"
	default:
		return "%Y-%m-%d"
	}
}

func postgresBucketFormat(bucket types.TimeBucket) string {
	switch bucket {
	case types.TimeBucketHour:
		return "YYYY-MM-DD HH24:00:00"
	case types.TimeBucketMonth:
		return "YYYY-MM"
	default:
		return "YYYY-MM-DD"
	}
}
