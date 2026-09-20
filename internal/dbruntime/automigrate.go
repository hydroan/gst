package dbruntime

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// MySQLTableOptions is what a MySQL table the framework creates is created
// with. It is one definition on purpose: a table created at startup by
// auto-migration and the same table created by gg migrate must come out
// identical, and the collation is the half that changes behavior — utf8mb4_bin
// compares strings byte for byte, while a server default of utf8mb4_0900_ai_ci
// would make a unique key refuse "a" beside "A" and a lookup find either. A
// deployment whose tables came from the two paths would otherwise behave
// differently on the same query.
const MySQLTableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin"

// automigrating returns a session of handler on which AutoMigrate creates
// tables the way gg migrate does, and leaves the framework's indexes to the
// framework.
//
// gorm migrates the unique constraints its tags declare — a unique or
// uniqueIndex tag on the column — and its MySQL driver takes every other
// single-column unique index for a leftover of a tag since removed, and
// drops it. The framework's indexes are declared through Indexes(), with no
// tag on the column, so on MySQL every one of them on a single column would
// be dropped on every start against an existing table, for
// ensureCustomIndexes to create again: a full rebuild on every start, and a
// moment with no unique key under the processes already running. A start
// drops nothing the framework created: on the session a column gorm knows
// nothing of is left alone, and one a tag declares unique is migrated as
// before.
func automigrating(handler *gorm.DB) *gorm.DB {
	session := handler.Session(&gorm.Session{NewDB: true})
	session.Dialector = automigrateDialector{handler.Dialector}
	if handler.Dialector != nil && strings.EqualFold(handler.Dialector.Name(), "mysql") {
		// gorm appends these to CREATE TABLE and ignores them for a table
		// that is already there, so a start against an existing schema
		// changes nothing.
		session = session.Set("gorm:table_options", MySQLTableOptions)
	}
	return session
}

// automigrateDialector is the handler's dialector with automigrateMigrator
// for its migrator. gorm resolves the migrator through the dialector at
// every step, so the wrapping reaches every step of AutoMigrate.
type automigrateDialector struct{ gorm.Dialector }

func (d automigrateDialector) Migrator(db *gorm.DB) gorm.Migrator {
	return automigrateMigrator{d.Dialector.Migrator(db)}
}

// Translate forwards to the dialect's translator, so that an error of a
// migration statement reads the same as anywhere else; gorm looks the
// translator up on the dialector, and an embedded interface hides it. It is
// the one optional interface the session needs: only AutoMigrate runs on
// it, never a transaction, so the savepoint interface stays hidden.
func (d automigrateDialector) Translate(err error) error {
	if translator, ok := d.Dialector.(gorm.ErrorTranslator); ok {
		return translator.Translate(err)
	}
	return err
}

// automigrateMigrator is the dialect's migrator with the unique handling of
// the columns gorm does not declare unique taken out.
type automigrateMigrator struct{ gorm.Migrator }

// MigrateColumnUnique migrates the unique constraint of a column a tag
// declares unique, and leaves every other column alone.
func (m automigrateMigrator) MigrateColumnUnique(value any, field *schema.Field, columnType gorm.ColumnType) error {
	if !field.Unique && field.UniqueIndex == "" {
		return nil
	}
	return m.Migrator.MigrateColumnUnique(value, field, columnType)
}

// BuildIndexOptions forwards to the dialect's migrator: gorm asserts the
// method on the migrator it resolves, and an embedded interface hides it. A
// dialect without one gets gorm's own, which every dialect builds on.
func (m automigrateMigrator) BuildIndexOptions(opts []schema.IndexOption, stmt *gorm.Statement) []any {
	if builder, ok := m.Migrator.(migrator.BuildIndexOptionsInterface); ok {
		return builder.BuildIndexOptions(opts, stmt)
	}
	return migrator.Migrator{}.BuildIndexOptions(opts, stmt)
}
