package dbruntime

import (
	"reflect"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime/dberror"
	"github.com/hydroan/gst/internal/modelschema"
	"gorm.io/gorm"
)

// translateErrorCallback names the callback InstallErrorTranslation registers
// on each of gorm's operations.
const translateErrorCallback = "gst:translate_error"

// InstallErrorTranslation makes db translate the error a statement fails with
// to the sentinel of the constraint the data broke, what the database said of
// it kept in the error. The error goes to translate first, the dialect's
// table of the errors the driver's translation leaves as they are, which
// answers the sentinel with the database's text or nil. An error translate
// does not recognize goes to the driver's own translation (see
// gorm.ErrorTranslator), and a sentinel the driver answers bare, nothing of
// the database's error left in it, comes back with that error's text wrapped
// around it. An error neither recognizes stays as it is. A duplicated key
// the dialect tells by its name alone (see dberror.DuplicatedKeyError) is
// given its columns from the model the statement writes, which declares the
// key under that name (see keyColumns).
//
// The handle is opened with gorm's TranslateError off: gorm would run the
// driver's translation as it records the error, before a callback sees it,
// and what the database said would be gone by then.
//
// The callback is registered after gorm's own callbacks of each of its six
// operations, create, query, update, delete, row and raw, the commit of the
// transaction gorm opens around a write among them, which is how dbresolver
// and otelgorm hook a handle too: through callbacks, with the driver's
// dialector left in place, since gorm.io/datatypes tells the dialect apart by
// the dialector's own type. An error no operation carries, the one the commit
// of a transaction opened on the handle fails with, goes through
// TranslateError.
func InstallErrorTranslation(db *gorm.DB, translate func(err error) error) error {
	rewrite := func(stmt *gorm.DB) {
		if stmt.Error == nil {
			return
		}
		if translated := translate(stmt.Error); translated != nil {
			// A dialect whose error names the key alone leaves its columns
			// to the model the statement writes, which declares the key.
			var duplicated *dberror.DuplicatedKeyError
			if errors.As(translated, &duplicated) && len(duplicated.Columns) == 0 && duplicated.Constraint != "" && stmt.Statement != nil && stmt.Statement.Schema != nil {
				duplicated.Columns = keyColumns(stmt, duplicated.Constraint)
			}
			stmt.Error = translated
			return
		}
		translator, ok := stmt.Dialector.(gorm.ErrorTranslator)
		if !ok {
			return
		}
		// The driver hands an error it does not translate back as it is, and
		// one that keeps the database's error behind its sentinel answers an
		// error still carrying it; either is kept as answered. A sentinel
		// that carries nothing of the error is one the driver answered bare.
		translated := translator.Translate(stmt.Error)
		switch {
		case translated == nil:
		case errors.Is(translated, stmt.Error):
			stmt.Error = translated
		default:
			stmt.Error = errors.Wrap(translated, stmt.Error.Error())
		}
	}
	callbacks := db.Callback()
	for _, register := range []func(name string, fn func(*gorm.DB)) error{
		callbacks.Create().After("gorm:create").Register,
		callbacks.Query().After("gorm:query").Register,
		callbacks.Update().After("gorm:update").Register,
		callbacks.Delete().After("gorm:delete").Register,
		callbacks.Row().After("gorm:row").Register,
		callbacks.Raw().After("gorm:raw").Register,
	} {
		if err := register(translateErrorCallback, rewrite); err != nil {
			return errors.Wrap(err, "failed to register the database error translation")
		}
	}
	return nil
}

// mysqlPrimaryKey is the name MySQL knows the primary key of every table by.
const mysqlPrimaryKey = "PRIMARY"

// keyColumns returns the columns of the key the database knows as
// constraint, among the keys of the model stmt writes: the primary key under
// the name MySQL gives it, an index the model declares through Indexes (see
// modelschema.ParseIndexPlans) or one its gorm tags declare, found by its
// name; nil when no key of the model goes by the name.
func keyColumns(stmt *gorm.DB, constraint string) []string {
	parsed := stmt.Statement.Schema
	if strings.EqualFold(constraint, mysqlPrimaryKey) {
		return slices.Clone(parsed.PrimaryFieldDBNames)
	}
	plans, err := modelschema.ParseIndexPlans(stmt.Session(&gorm.Session{NewDB: true}), reflect.New(parsed.ModelType).Interface())
	if err == nil {
		for _, plan := range plans {
			if strings.EqualFold(plan.Name, constraint) {
				return slices.Clone(plan.Columns)
			}
		}
	}
	for _, index := range parsed.ParseIndexes() {
		if !strings.EqualFold(index.Name, constraint) {
			continue
		}
		columns := make([]string, 0, len(index.Fields))
		for _, field := range index.Fields {
			columns = append(columns, field.DBName)
		}
		return columns
	}
	return nil
}

// TranslateError translates err the way db translates the error of a
// statement (see InstallErrorTranslation), for an error no statement carries:
// the one the commit of a transaction fails with, a constraint checked at
// commit having let the statements through. On a handle with no translation
// installed it answers err as it is.
func TranslateError(db *gorm.DB, err error) error {
	rewrite := db.Callback().Raw().Get(translateErrorCallback)
	if err == nil || rewrite == nil {
		return err
	}
	// The callback reads the dialector and the error of the handle it is
	// given and nothing of a statement, so a handle carrying err stands in
	// for one.
	carrier := db.Session(&gorm.Session{NewDB: true})
	carrier.Error = err
	rewrite(carrier)
	return carrier.Error
}
