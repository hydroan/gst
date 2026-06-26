package dbmigrate

import (
	"database/sql"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/sqldef/sqldef/v3/database"
)

type sqliteDatabase struct {
	config          database.Config
	db              *sql.DB
	generatorConfig database.GeneratorConfig
}

// newSQLiteDatabase uses the sqlite3 driver to avoid importing sqldef's SQLite adapter,
// which registers the "sqlite" driver through modernc.org/sqlite.
func newSQLiteDatabase(config database.Config) (database.Database, error) {
	db, err := sql.Open("sqlite3", config.DbName)
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(1)

	return &sqliteDatabase{
		config: config,
		db:     db,
	}, nil
}

func (d *sqliteDatabase) ExportDDLs() (string, error) {
	tableNames, err := d.tableNames()
	if err != nil {
		return "", err
	}

	ddls := make([]string, 0, len(tableNames))
	for _, tableName := range tableNames {
		ddl, exportErr := d.exportTableDDL(tableName)
		if exportErr != nil {
			return "", exportErr
		}
		ddls = append(ddls, ddl)
	}

	viewDDLs, err := d.views()
	if err != nil {
		return "", err
	}
	ddls = append(ddls, viewDDLs...)

	indexDDLs, err := d.indexes()
	if err != nil {
		return "", err
	}
	ddls = append(ddls, indexDDLs...)

	triggerDDLs, err := d.triggers()
	if err != nil {
		return "", err
	}
	ddls = append(ddls, triggerDDLs...)

	return strings.Join(ddls, "\n\n"), nil
}

func (d *sqliteDatabase) tableNames() ([]string, error) {
	rows, err := d.db.Query(`
		SELECT tbl_name
		FROM sqlite_master
		WHERE type = 'table' AND tbl_name NOT LIKE 'sqlite_%'
		ORDER BY tbl_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func (d *sqliteDatabase) exportTableDDL(table string) (string, error) {
	const query = `
		SELECT sql
		FROM sqlite_master
		WHERE tbl_name = ? AND type = 'table'
	`

	var sql string
	if err := d.db.QueryRow(query, table).Scan(&sql); err != nil {
		return "", errors.Wrapf(err, "failed to export sqlite table %s", table)
	}
	return sql + ";", nil
}

func (d *sqliteDatabase) views() ([]string, error) {
	const query = `
		SELECT sql
		FROM sqlite_master
		WHERE type = 'view'
		ORDER BY name
	`
	return d.ddls(query)
}

func (d *sqliteDatabase) indexes() ([]string, error) {
	const query = `
		SELECT sql
		FROM sqlite_master
		WHERE type = 'index' AND sql IS NOT NULL
		ORDER BY sql
	`
	return d.ddls(query)
}

func (d *sqliteDatabase) triggers() ([]string, error) {
	const query = `
		SELECT sql
		FROM sqlite_master
		WHERE type = 'trigger' AND sql IS NOT NULL
		ORDER BY name
	`
	return d.ddls(query)
}

func (d *sqliteDatabase) ddls(query string) ([]string, error) {
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ddls []string
	for rows.Next() {
		var sql string
		if err := rows.Scan(&sql); err != nil {
			return nil, err
		}
		ddls = append(ddls, sql+";")
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ddls, nil
}

func (d *sqliteDatabase) DB() *sql.DB {
	return d.db
}

func (d *sqliteDatabase) Close() error {
	return d.db.Close()
}

func (d *sqliteDatabase) GetDefaultSchema() string {
	return ""
}

func (d *sqliteDatabase) SetGeneratorConfig(config database.GeneratorConfig) {
	d.generatorConfig = config
}

func (d *sqliteDatabase) GetGeneratorConfig() database.GeneratorConfig {
	return d.generatorConfig
}

func (d *sqliteDatabase) GetTransactionQueries() database.TransactionQueries {
	return database.TransactionQueries{
		Begin:    "BEGIN",
		Commit:   "COMMIT",
		Rollback: "ROLLBACK",
	}
}

func (d *sqliteDatabase) GetConfig() database.Config {
	return d.config
}
