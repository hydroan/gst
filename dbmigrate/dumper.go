package dbmigrate

import (
	"context"
	"database/sql"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/maxrichie5/go-sqlfmt/sqlfmt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SchemaDumper renders models into the target schema DDL that Migrate diffs
// against the live database. It runs gorm's migrator in dry-run mode over a
// mock connection, so the statements are the ones the runtime itself would
// apply and no database is contacted.
type SchemaDumper struct {
	db   *sql.DB
	mock sqlmock.Sqlmock

	mu sync.Mutex
}

// NewSchemaDumper opens a dumper on a mock connection of its own. The caller
// owns it and closes it through Close.
func NewSchemaDumper() (*SchemaDumper, error) {
	db, mock, err := sqlmock.New()
	if err != nil {
		return nil, err
	}
	return &SchemaDumper{
		db:   db,
		mock: mock,
	}, nil
}

// Dump renders dst as the target schema for driver: one CREATE TABLE per
// model, each followed by the CREATE INDEX statements of the indexes it
// declares through its optional Indexes method, and each annotated with the model
// it came from. Models are sorted by type name, so the same input always
// renders the same schema.
func (s *SchemaDumper) Dump(driver config.DBType, dst ...any) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return "", errors.New("schema dumper is closed")
	}

	var dialector gorm.Dialector
	var tableOptions string

	switch driver {
	case config.DBMySQL:
		dialector = mysql.New(mysql.Config{Conn: s.db, SkipInitializeWithVersion: true})
		tableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin"
	case config.DBPostgres:
		dialector = postgres.New(postgres.Config{Conn: s.db, PreferSimpleProtocol: true})
	case config.DBSqlite:
		dialector = sqlite.New(sqlite.Config{Conn: s.db})
		// GORM sqlite driver might ping to check version
		s.mock.ExpectQuery("select sqlite_version()").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("3.35.0"))
	default:
		// Mirrors the Migrate refusal: analytical schemas such as ClickHouse
		// are hand-written DDL, never dumped from Go models.
		return "", errors.Newf("schema dump does not support %q: its schema is managed by hand-written DDL on the application side", driver)
	}

	// Sort by type name to ensure deterministic output order. The sort runs on
	// a copy: dst is the caller's own slice whenever the models arrive through
	// a models... expansion, and rendering a schema must not reorder it.
	models := make([]any, len(dst))
	copy(models, dst)
	sort.Slice(models, func(i, j int) bool {
		t1 := reflect.TypeOf(models[i])
		for t1.Kind() == reflect.Pointer {
			t1 = t1.Elem()
		}
		t2 := reflect.TypeOf(models[j])
		for t2.Kind() == reflect.Pointer {
			t2 = t2.Elem()
		}
		return t1.String() < t2.String()
	})

	dumpLog := &dumperLogger{}
	db, err := gorm.Open(dialector, &gorm.Config{DryRun: true, Logger: dumpLog})
	if err != nil {
		return "", err
	}

	statements := make([]schemaStatement, 0, len(models))
	indexSets := make([]modelregistry.ModelIndexPlans, 0, len(models))
	for _, v := range models {
		if err = requireExplicitTableName(v); err != nil {
			return "", err
		}

		// The table name is not supplied through Table(): gorm reads the
		// model's own TableName method through its Tabler interface, and a
		// supplied name would make gorm re-parse the schema under a special
		// table name, renaming the constraints of associated models.
		tx := db.Set("gorm:table_options", tableOptions)
		sqlStart := len(dumpLog.SQLs)
		if err = tx.Migrator().CreateTable(v); err != nil {
			return "", err
		}
		for _, sql := range dumpLog.SQLs[sqlStart:] {
			statements = append(statements, schemaStatement{
				ModelName: schemaModelName(v),
				SQL:       sql,
			})
		}

		// Append the custom indexes a model declares through its Indexes method.
		// Plans and statement rendering are shared with the bootstrap
		// executor, so the desired schema always matches the DDL that the
		// runtime actually applies.
		plans, planErr := modelregistry.ParseIndexPlans(db, v)
		if planErr != nil {
			return "", planErr
		}
		if len(plans) > 0 {
			indexSets = append(indexSets, modelregistry.ModelIndexPlans{Model: schemaModelName(v), Plans: plans})
		}
		for _, plan := range plans {
			statements = append(statements, schemaStatement{
				ModelName: schemaModelName(v),
				SQL:       plan.CreateSQL(db.Dialector),
			})
		}
	}

	// Models resolve their plans in isolation; colliding declarations across
	// the models of one table would render duplicate statements here and split
	// into two indexes on the database, so they fail the dump instead.
	if err = modelregistry.CheckCrossModelIndexPlanConflicts(indexSets); err != nil {
		return "", err
	}

	if len(statements) == 0 {
		return "", nil
	}

	var sb strings.Builder

	for _, stmt := range statements {
		sql := normalizeSchemaSQL(driver, stmt.SQL)
		if shouldAnnotateSchemaStatement(sql) {
			sb.WriteString("-- Model: " + stmt.ModelName + "\n")
		}
		sb.WriteString(sqlfmt.Format(sql) + ";\n")
	}

	return sb.String(), nil
}

// Close releases the mock connection the dumper renders through. A dumper
// already closed reports no error.
func (s *SchemaDumper) Close() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		err = s.db.Close()
		s.db = nil
		return err
	}

	return nil
}

// schemaStatement is one rendered DDL statement and the model it came from.
type schemaStatement struct {
	ModelName string
	SQL       string
}

// normalizeSchemaSQL rewrites a gorm-generated statement into the spelling
// sqldef compares against, so a model that did not change never plans a
// migration.
//
// The sqldef parser rejects the "IF NOT EXISTS" index form that GORM
// generates. The rest is dialect drift: sqldef diffs the desired DDL against
// the database schema string-wise, and GORM's "timestamptz" and "boolean"
// never match the "timestamp with time zone" and "tinyint(1)" the servers
// report back, so every run would re-plan the same ALTER COLUMN statements.
func normalizeSchemaSQL(driver config.DBType, sql string) string {
	sql = strings.ReplaceAll(sql, "CREATE INDEX IF NOT EXISTS", "CREATE INDEX")
	sql = strings.ReplaceAll(sql, "CREATE UNIQUE INDEX IF NOT EXISTS", "CREATE UNIQUE INDEX")

	if driver == config.DBPostgres {
		sql = strings.ReplaceAll(sql, "timestamptz", "timestamp with time zone")
	}
	if driver == config.DBMySQL {
		sql = strings.ReplaceAll(sql, " boolean ", " tinyint(1) ")
		sql = strings.ReplaceAll(sql, "DEFAULT true", "DEFAULT 1")
		sql = strings.ReplaceAll(sql, "DEFAULT false", "DEFAULT 0")
	}
	return sql
}

// schemaModelName is the annotation naming the model a statement came from.
func schemaModelName(model any) string {
	typ := reflect.TypeOf(model)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ.String()
}

// requireExplicitTableName rejects models that do not declare an explicit
// table name. gorm reads the same TableName method through its Tabler
// interface while rendering the DDL, so a model without one would dump a
// schema targeting an empty table name. Value models are probed through a
// zero-value pointer because the method lives on the pointer receiver.
func requireExplicitTableName(model any) error {
	tabler, ok := model.(interface{ TableName() string })
	if !ok {
		rv := reflect.ValueOf(model)
		if rv.Kind() == reflect.Struct {
			tabler, ok = reflect.TypeAssert[interface{ TableName() string }](reflect.New(rv.Type()))
		}
	}
	if !ok || len(tabler.TableName()) == 0 {
		return errors.Newf("model %T must declare an explicit table name by overriding TableName", model)
	}
	return nil
}

// shouldAnnotateSchemaStatement reports whether the statement opens a model's
// section of the schema, which is the only place the annotation belongs.
func shouldAnnotateSchemaStatement(sql string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "CREATE TABLE")
}

// dumperLogger collects the statements gorm's dry-run migrator produces
// instead of logging them.
type dumperLogger struct {
	SQLs []string
}

func (l *dumperLogger) LogMode(level logger.LogLevel) logger.Interface     { return l }
func (l *dumperLogger) Info(ctx context.Context, msg string, data ...any)  {}
func (l *dumperLogger) Warn(ctx context.Context, msg string, data ...any)  {}
func (l *dumperLogger) Error(ctx context.Context, msg string, data ...any) {}

func (l *dumperLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.SQLs = append(l.SQLs, sql)
}
