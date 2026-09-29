package dbmigrate

import (
	"database/sql"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	// newSQLiteDatabase opens the "sqlite3" driver by name, so the package that
	// registers it has to be imported here; relying on another package's import
	// to pull it in would break the SQLite migration path at run time.
	_ "github.com/mattn/go-sqlite3"
	"github.com/sqldef/sqldef/v3/database"
)

// sqliteDatabase is the sqldef database adapter the SQLite migration path runs
// on. sqldef drives the migration through this interface; everything below the
// interface methods only serves ExportDDLs.
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

// emptySQLiteDatabase names a private in-memory database, which a dry run
// plans against in place of a database file that does not exist yet: that
// file is an empty database too, and planning against it would create it.
const emptySQLiteDatabase = ":memory:"

// sqliteFileMissing reports whether the database file a sqlite connection
// string names does not exist yet, reading the string the way the driver
// does: a file: URI names the file its path spells, percent escapes decoded;
// any other string names the file before its first question mark, the rest
// being the driver's parameters. For "./data.db?_busy_timeout=1000" it checks
// ./data.db, for "file:/tmp/my%20data.db?cache=private" /tmp/my data.db.
//
// A missing file whose directory is missing too is an error: the driver
// creates the file but not the directory it goes in, so applying the plan
// would fail, and planning fails first rather than promise the file.
func sqliteFileMissing(dsn string) (bool, error) {
	file := dsn
	if strings.HasPrefix(dsn, "file:") {
		uri, err := url.Parse(dsn)
		if err != nil {
			return false, errors.Wrapf(err, "failed to read the sqlite database path %q", dsn)
		}
		// A relative path is opaque to the URI syntax: file:data.db leaves
		// it escaped in Opaque, while file:/tmp/data.db decodes into Path.
		file = uri.Path
		if uri.Opaque != "" {
			if file, err = url.PathUnescape(uri.Opaque); err != nil {
				return false, errors.Wrapf(err, "failed to read the sqlite database path %q", dsn)
			}
		}
	} else if pos := strings.IndexByte(dsn, '?'); pos >= 1 {
		file = dsn[:pos]
	}
	_, err := os.Stat(file)
	if !errors.Is(err, fs.ErrNotExist) {
		return false, errors.Wrapf(err, "failed to inspect the sqlite database file %s", file)
	}
	dir := filepath.Dir(file)
	_, err = os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, errors.Newf("the sqlite database file %s cannot be created: its directory %s does not exist", file, dir)
	case err != nil:
		return false, errors.Wrapf(err, "failed to inspect the directory of the sqlite database file %s", file)
	}
	return true, nil
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
	// A table the runtime created carries GORM's double-quoted string
	// defaults verbatim; the desired schema rewrites them into single quotes,
	// and the reported schema must read the same, or every run would re-plan
	// the column.
	return singleQuoteSQLiteDefaults(sql) + ";", nil
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
