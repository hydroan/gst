package dbmigrate

import (
	"testing"

	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/parser"
	"github.com/sqldef/sqldef/v3/schema"
	"github.com/stretchr/testify/require"
)

// exportedDatabase stands in for the database a migration is planned
// against: it exports the schema it is given, under the default schema it is
// given, and is never executed against.
type exportedDatabase struct {
	database.Database
	ddls          string
	defaultSchema string
	config        database.GeneratorConfig
}

func (d *exportedDatabase) ExportDDLs() (string, error) { return d.ddls, nil }

func (d *exportedDatabase) GetDefaultSchema() string { return d.defaultSchema }

func (d *exportedDatabase) SetGeneratorConfig(config database.GeneratorConfig) { d.config = config }

func (d *exportedDatabase) GetGeneratorConfig() database.GeneratorConfig { return d.config }

// TestPlanMigrationRefusesAnEmptyPlanBesideAMissingTable pins the guard on
// the names a plan is confined to. sqldef leaves out of the plan every table
// those names match none of, the tables of the models included, and then
// plans nothing at all: under a default schema MySQL table names do not
// carry, the names match no table, and the migration must fail rather than
// report a database lacking the models' table as up to date.
func TestPlanMigrationRefusesAnEmptyPlanBesideAMissingTable(t *testing.T) {
	desired := "CREATE TABLE `samples` (`id` char(36) NOT NULL, PRIMARY KEY (`id`));"
	sqlParser := database.NewParser(parser.ParserModeMysql)

	plan, err := planMigration(schema.GeneratorModeMysql, &exportedDatabase{}, sqlParser, desired, nil)
	require.NoError(t, err)
	require.Len(t, plan.Statements, 1, "under the names MySQL knows its tables by, the table is planned")

	_, err = planMigration(schema.GeneratorModeMysql, &exportedDatabase{defaultSchema: "elsewhere"}, sqlParser, desired, nil)
	require.ErrorContains(t, err, `planned nothing while the database lacks the table "samples"`)
}
