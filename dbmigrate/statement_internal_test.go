package dbmigrate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsIndexStatement(t *testing.T) {
	for _, statement := range []string{
		"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
		"DROP INDEX uniq_samples_code",
		"ALTER TABLE `samples` ADD UNIQUE INDEX `uniq_samples_code` (`code`)",
		"CREATE INDEX `idx_samples_kind` ON `samples` (`kind`)",
		// The pretty-printed multi-line form the schema dumper emits for long
		// statements must classify like its single-line equivalent.
		"CREATE INDEX `idx_samples_group_id_code_kind_created_at_0f3a9b2c` ON `samples` (\n" +
			"  `group_id`,\n" +
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
		"  KEY `idx_samples_group_kind` (`group_id`,`kind`) USING BTREE,\n" +
		"  KEY `idx_samples_remark` (`remark`(20))\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;"

	indexes := parseCurrentIndexes(current)
	require.Equal(t, map[string]currentIndex{
		"uniq_samples_code":      {Columns: "code", Unique: true},
		"idx_samples_group_kind": {Columns: "group_id,kind", Unique: false},
		"idx_samples_remark":     {Columns: "remark(20)", Unique: false},
	}, indexes["samples"])
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
