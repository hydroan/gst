package dbmigrate

import (
	"testing"

	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/parser"
	"github.com/stretchr/testify/require"
)

// TestReadShapeReadsTablesColumnsAndIndexes pins the example readShape gives:
// a table with its columns under the definitions the parser renders, its
// secondary index and not its primary key, and an index statement on a table
// the statements do not create left out.
func TestReadShapeReadsTablesColumnsAndIndexes(t *testing.T) {
	shape, err := readShape(database.NewParser(parser.ParserModeMysql),
		"CREATE TABLE `samples` (`id` char(36) NOT NULL, `code` varchar(64), PRIMARY KEY (`id`));\n"+
			"CREATE UNIQUE INDEX `uniq_samples_code` ON `samples` (`code`);\n"+
			"CREATE INDEX `idx_others_code` ON `others` (`code`);")
	require.NoError(t, err)

	require.Equal(t, []string{"samples"}, shape.names)
	table := shape.table("SAMPLES")
	require.NotNil(t, table, "a table is found whatever the case of its name")
	require.Equal(t, []columnShape{
		{name: "id", definition: "char(36) not null", primary: true},
		{name: "code", definition: "varchar(64)"},
	}, table.columns)
	require.Equal(t, []indexShape{{name: "uniq_samples_code", definition: "unique code", standalone: true}}, table.indexes)
	require.Nil(t, shape.table("others"))
}

// TestIndexDefinitionSpellsOneDefinitionAlike pins the examples
// indexDefinition gives, each read off the two spellings a migration
// compares: the statement the models' schema carries and the one the
// database exports, a unique index over the rows not soft-deleted among them,
// which MySQL keeps as a key part and the others as a condition.
func TestIndexDefinitionSpellsOneDefinitionAlike(t *testing.T) {
	for name, tc := range map[string]struct {
		mode     parser.ParserMode
		desired  string
		exported string
		want     string
	}{
		"mysql": {
			mode: parser.ParserModeMysql,
			desired: "CREATE TABLE `samples` (`code` varchar(64), `deleted_at` datetime(3) NULL);\n" +
				"CREATE UNIQUE INDEX `uniq_samples_code` ON `samples` (`code`, (CASE WHEN `deleted_at` IS NULL THEN 1 END));",
			exported: "CREATE TABLE `samples` (\n  `code` varchar(64) DEFAULT NULL,\n  `deleted_at` datetime(3) DEFAULT NULL,\n" +
				"  UNIQUE KEY `uniq_samples_code` (`code`,((case when (`deleted_at` is null) then 1 end)))\n) ENGINE=InnoDB;",
			want: "unique code,case when deleted_at is null then 1 end",
		},
		"postgres": {
			mode: parser.ParserModePostgres,
			desired: "CREATE TABLE \"samples\" (\"code\" varchar(64), \"deleted_at\" timestamp with time zone);\n" +
				"CREATE UNIQUE INDEX \"uniq_samples_code\" ON \"samples\" (\"code\") WHERE \"deleted_at\" IS NULL;",
			exported: "CREATE TABLE \"public\".\"samples\" (\"code\" varchar(64), \"deleted_at\" timestamp with time zone);\n" +
				"CREATE UNIQUE INDEX uniq_samples_code ON public.samples USING btree (code) WHERE (deleted_at IS NULL);",
			want: "unique code where deleted_at is null",
		},
		"sqlite": {
			mode: parser.ParserModeSQLite3,
			desired: "CREATE TABLE `samples` (`code` text, `deleted_at` datetime);\n" +
				"CREATE UNIQUE INDEX `uniq_samples_code` ON `samples` (`code`) WHERE `deleted_at` IS NULL;",
			exported: "CREATE TABLE `samples` (`code` text, `deleted_at` datetime);\n" +
				"CREATE UNIQUE INDEX `uniq_samples_code` ON `samples` (`code`) WHERE `deleted_at` IS NULL;",
			want: "unique code where deleted_at is null",
		},
	} {
		t.Run(name, func(t *testing.T) {
			sqlParser := database.NewParser(tc.mode)
			for _, ddls := range []string{tc.desired, tc.exported} {
				shape, err := readShape(sqlParser, ddls)
				require.NoError(t, err)
				index, ok := shape.table("samples").index("uniq_samples_code")
				require.True(t, ok, ddls)
				require.Equal(t, tc.want, index.definition, ddls)
			}
		})
	}
}

// TestAnnotateRenamedIndexesMarksTheStatementOfTheIndex pins the example
// annotateRenamedIndexes gives, and that the statements of the other indexes
// and a name the annotation cannot carry are left as they are.
func TestAnnotateRenamedIndexesMarksTheStatementOfTheIndex(t *testing.T) {
	ddls := "CREATE UNIQUE INDEX `uniq_records_code` ON `records` (`code`);\n" +
		"CREATE INDEX `idx_records_kind` ON `records` (`kind`);\n" +
		"CREATE INDEX `idx_records_name` ON `records` (`name`);"

	annotated := annotateRenamedIndexes(ddls, map[string]string{
		"uniq_records_code": "uniq_samples_code",
		"idx_records_name":  `odd"name`,
	})

	require.Equal(t, "CREATE UNIQUE INDEX `uniq_records_code` /* @renamed from=\"uniq_samples_code\" */ ON `records` (`code`);\n"+
		"CREATE INDEX `idx_records_kind` ON `records` (`kind`);\n"+
		"CREATE INDEX `idx_records_name` ON `records` (`name`);", annotated)
}

// TestOneToOnePairsWhatMatchesNothingElse pins the pairing every rename is
// told by: a pair is reported when each side matches the other and nothing
// else, and a side matching two, or matched by two, is reported with none.
func TestOneToOnePairsWhatMatchesNothingElse(t *testing.T) {
	// 0 matches 0 alone; 1 matches 1 and 2; 2 and 3 both match 3.
	matches := map[[2]int]bool{{0, 0}: true, {1, 1}: true, {1, 2}: true, {2, 3}: true, {3, 3}: true}

	pairs := oneToOne(4, 4, func(i, j int) bool { return matches[[2]int{i, j}] })

	require.Equal(t, [][2]int{{0, 0}}, pairs)
}
