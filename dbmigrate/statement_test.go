package dbmigrate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
