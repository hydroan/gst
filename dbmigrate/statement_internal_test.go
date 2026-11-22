package dbmigrate

import (
	"testing"

	"github.com/hydroan/gst/internal/modelschema"
	"github.com/sqldef/sqldef/v3/schema"
	"github.com/stretchr/testify/require"
)

func TestIsIndexStatement(t *testing.T) {
	for _, statement := range []string{
		"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
		"DROP INDEX uniq_samples_code",
		"ALTER TABLE `samples` ADD UNIQUE INDEX `uniq_samples_code` (`code`)",
		"CREATE INDEX `idx_samples_kind` ON `samples` (`kind`)",
		// A unique index over the live rows alone, as each dialect spells it.
		"CREATE UNIQUE INDEX `uniq_samples_code` ON `samples` (`code`,(CASE WHEN `deleted_at` IS NULL THEN 1 END))",
		`CREATE UNIQUE INDEX "uniq_samples_code" ON public."samples" ("code") WHERE deleted_at is null`,
		// The pretty-printed multi-line form the schema dumper emits for long
		// statements must classify like its single-line equivalent.
		"CREATE INDEX `idx_samples_box_id_code_kind_created_at_0f3a9b2c` ON `samples` (\n" +
			"  `box_id`,\n" +
			"  `code`,\n" +
			"  `kind`,\n" +
			"  `created_at`\n" +
			")",
	} {
		require.True(t, isIndexStatement(statement), statement)
	}

	for _, statement := range []string{
		"ALTER TABLE `samples` ADD COLUMN `kind` varchar(64)",
		"DROP TABLE `samples`",
	} {
		require.False(t, isIndexStatement(statement), statement)
	}
}

func TestParseCurrentIndexes(t *testing.T) {
	current := "CREATE TABLE `samples` (\n" +
		"  `id` char(36) NOT NULL,\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  UNIQUE KEY `uniq_samples_code` (`code`),\n" +
		"  KEY `idx_samples_box_kind` (`box_id`,`kind`) USING BTREE,\n" +
		"  KEY `idx_samples_remark` (`remark`(20))\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;"

	indexes := parseCurrentIndexes(current)
	require.Equal(t, map[string]indexDefinition{
		"uniq_samples_code":    {Columns: "code", Unique: true},
		"idx_samples_box_kind": {Columns: "box_id,kind", Unique: false},
		"idx_samples_remark":   {Columns: "remark(20)", Unique: false},
	}, indexes["samples"])

	// A unique index over the live rows alone, as each database exports it:
	// MySQL as one more key part inside the table, PostgreSQL and SQLite as a
	// statement of its own under the condition. An index under any other
	// condition is none the framework built and is left out.
	current = "CREATE TABLE `samples` (\n" +
		"  `id` char(36) NOT NULL,\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  UNIQUE KEY `uniq_samples_code` (`code`,((case when (`deleted_at` is null) then 1 end)))\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;\n" +
		"CREATE UNIQUE INDEX uniq_records_box_id_code ON public.records USING btree (box_id, code) WHERE (deleted_at IS NULL);\n" +
		"CREATE UNIQUE INDEX `uniq_items_code` ON `items` (`code`) WHERE `deleted_at` IS NULL;\n" +
		"CREATE UNIQUE INDEX uniq_items_kind ON items (kind) WHERE (kind <> 'draft');"

	indexes = parseCurrentIndexes(current)
	require.Equal(t, map[string]indexDefinition{"uniq_samples_code": {Columns: "code", Unique: true, LiveOnly: true}}, indexes["samples"])
	require.Equal(t, map[string]indexDefinition{"uniq_records_box_id_code": {Columns: "box_id,code", Unique: true, LiveOnly: true}}, indexes["records"])
	require.Equal(t, map[string]indexDefinition{"uniq_items_code": {Columns: "code", Unique: true, LiveOnly: true}}, indexes["items"])
}

// TestIndexDefinitionOf pins the example of indexDefinitionOf and of
// normalizeCondition, and what holds the two spellings of a unique index
// over the live rows alone to the one renderer of index statements: whatever
// modelschema.IndexPlan.CreateSQL writes for a dialect reads back as the
// plan's columns over the live rows.
func TestIndexDefinitionOf(t *testing.T) {
	definition, ok := indexDefinitionOf("`code`,((case when (`deleted_at` is null) then 1 end))", "", true)
	require.True(t, ok)
	require.Equal(t, indexDefinition{Columns: "code", Unique: true, LiveOnly: true}, definition)

	require.Equal(t, "deleted_at is null", normalizeCondition("(deleted_at IS NULL)"))
	require.Equal(t, "deleted_at is null", normalizeCondition(`"deleted_at" IS NULL`))

	// Any other condition, and any other expression after the columns, is no
	// index the framework built.
	_, ok = indexDefinitionOf("kind", "(kind <> 'draft')", true)
	require.False(t, ok)
	definition, ok = indexDefinitionOf("`code`,(lower(`name`))", "", true)
	require.True(t, ok)
	require.Equal(t, indexDefinition{Columns: "code,(lower(name))", Unique: true}, definition)

	plan := modelschema.IndexPlan{Name: "uniq_samples_box_id_code", Table: "samples", Columns: []string{"box_id", "code"}, Unique: true, LiveOnly: true}
	for _, mode := range []schema.GeneratorMode{schema.GeneratorModeMysql, schema.GeneratorModePostgres, schema.GeneratorModeSQLite3} {
		statement := plan.CreateSQL(dialectorFor(mode))
		m := createIndexPattern.FindStringSubmatch(statement)
		require.NotNil(t, m, statement)
		definition, ok = indexDefinitionOf(m[4], m[5], m[1] != "")
		require.True(t, ok, statement)
		require.Equal(t, indexDefinition{Columns: "box_id,code", Unique: true, LiveOnly: true}, definition, statement)
	}
}

func TestParseCurrentTables(t *testing.T) {
	current := "CREATE TABLE `samples` (\n" +
		"  `id` char(36) NOT NULL,\n" +
		"  PRIMARY KEY (`id`)\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;\n" +
		"\n" +
		"CREATE TABLE `records` (\n" +
		"  `id` char(36) NOT NULL,\n" +
		"  KEY `idx_records_kind` (`kind`)\n" +
		") ENGINE=InnoDB;"

	tables := parseCurrentTables(current)
	require.Len(t, tables, 2)
	require.Equal(t, "CREATE TABLE `samples` (\n"+
		"  `id` char(36) NOT NULL,\n"+
		"  PRIMARY KEY (`id`)\n"+
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4", tables["samples"])
	require.Equal(t, "CREATE TABLE `records` (\n"+
		"  `id` char(36) NOT NULL,\n"+
		"  KEY `idx_records_kind` (`kind`)\n"+
		") ENGINE=InnoDB", tables["records"])
}
