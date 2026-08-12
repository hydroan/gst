package dbmigrate

import (
	"strings"
	"testing"

	"github.com/sqldef/sqldef/v3/database"
	"github.com/sqldef/sqldef/v3/parser"
	"github.com/sqldef/sqldef/v3/schema"
	"github.com/stretchr/testify/require"
)

func TestDetectTableRenames(t *testing.T) {
	t.Run("identical definitions form a verified pair", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  `code` varchar(64) NOT NULL,\n" +
			"  PRIMARY KEY (`id`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		pairs := detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples`",
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  `code` varchar(64) NOT NULL,\n" +
				"  PRIMARY KEY (`id`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
		}, current)
		require.Equal(t, []tableRenamePair{{From: "samples", To: "records"}}, pairs)
	})

	t.Run("index names following the table rename stay a rename", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  `code` varchar(64) NOT NULL,\n" +
			"  PRIMARY KEY (`id`),\n" +
			"  KEY `idx_samples_code` (`code`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		pairs := detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples`",
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  `code` varchar(64) NOT NULL,\n" +
				"  PRIMARY KEY (`id`),\n" +
				"  INDEX `idx_records_code` (`code`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
		}, current)
		require.Equal(t, []tableRenamePair{{
			From: "samples",
			To:   "records",
			IndexRenames: []renamePair{{
				Table:   "records",
				From:    "idx_samples_code",
				To:      "idx_records_code",
				Columns: "code",
			}},
		}}, pairs)
	})

	t.Run("standalone CREATE INDEX statements on the new table participate", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  `code` varchar(64) NOT NULL,\n" +
			"  PRIMARY KEY (`id`),\n" +
			"  UNIQUE KEY `uniq_samples_code` (`code`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		pairs := detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples`",
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  `code` varchar(64) NOT NULL,\n" +
				"  PRIMARY KEY (`id`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
			"CREATE UNIQUE INDEX `uniq_records_code` ON `records` (`code`)",
		}, current)
		require.Equal(t, []tableRenamePair{{
			From: "samples",
			To:   "records",
			IndexRenames: []renamePair{{
				Table:   "records",
				From:    "uniq_samples_code",
				To:      "uniq_records_code",
				Columns: "code",
				Unique:  true,
			}},
		}}, pairs)
	})

	t.Run("a pending column addition keeps the rename and reports it as remaining", func(t *testing.T) {
		// The reported real-world case: the current table is behind on a
		// not-yet-migrated column, so the created table is a column superset
		// of the dropped one. Renaming still loses no data.
		current := "CREATE TABLE `samples` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  `code` varchar(64) NOT NULL,\n" +
			"  `deleted_at` datetime(3) NULL,\n" +
			"  PRIMARY KEY (`id`),\n" +
			"  UNIQUE KEY `uniq_samples_code` (`code`),\n" +
			"  KEY `idx_samples_deleted_at` (`deleted_at`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		pairs := detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples`",
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  `code` varchar(64) NOT NULL,\n" +
				"  `remark` varchar(255) NOT NULL DEFAULT '',\n" +
				"  `deleted_at` datetime(3) NULL,\n" +
				"  PRIMARY KEY (`id`),\n" +
				"  UNIQUE INDEX `uniq_samples_code` (`code`),\n" +
				"  INDEX `idx_records_deleted_at` (`deleted_at`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
		}, current)
		require.Len(t, pairs, 1)
		require.Equal(t, "samples", pairs[0].From)
		require.Equal(t, "records", pairs[0].To)
		require.Equal(t, []renamePair{{
			Table:   "records",
			From:    "idx_samples_deleted_at",
			To:      "idx_records_deleted_at",
			Columns: "deleted_at",
		}}, pairs[0].IndexRenames)
		require.Len(t, pairs[0].Residual, 1)
		require.Contains(t, pairs[0].Residual[0], "ADD COLUMN `remark`")
	})

	t.Run("a dropped column is a rebuild, not a rename", func(t *testing.T) {
		// Losing a column means losing its data; the pair must not be
		// certified as a safe rename.
		current := "CREATE TABLE `samples` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  `code` varchar(64) NOT NULL,\n" +
			"  `legacy` varchar(64) NOT NULL,\n" +
			"  PRIMARY KEY (`id`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		pairs := detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples`",
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  `code` varchar(64) NOT NULL,\n" +
				"  PRIMARY KEY (`id`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
		}, current)
		require.Empty(t, pairs)
	})

	t.Run("a brand-new index keeps the rename and reports it as remaining", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  `code` varchar(64) NOT NULL,\n" +
			"  PRIMARY KEY (`id`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		pairs := detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples`",
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  `code` varchar(64) NOT NULL,\n" +
				"  PRIMARY KEY (`id`),\n" +
				"  INDEX `idx_records_code` (`code`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
		}, current)
		require.Len(t, pairs, 1)
		require.Equal(t, "samples", pairs[0].From)
		require.Equal(t, "records", pairs[0].To)
		require.Empty(t, pairs[0].IndexRenames)
		require.Len(t, pairs[0].Residual, 1)
		require.Contains(t, pairs[0].Residual[0], "idx_records_code")
	})

	t.Run("changed columns are rebuilds, not renames", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  `code` varchar(64) NOT NULL,\n" +
			"  PRIMARY KEY (`id`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		pairs := detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples`",
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  `code` varchar(128) NOT NULL,\n" +
				"  PRIMARY KEY (`id`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
		}, current)
		require.Empty(t, pairs)
	})

	t.Run("changed index definitions stay a rename with remaining changes", func(t *testing.T) {
		// Index changes never touch row data, so a rename with a different
		// index set is still safe; the unpaired drop and add stay behind as
		// remaining changes instead of forming a RENAME INDEX pair.
		current := "CREATE TABLE `samples` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  `code` varchar(64) NOT NULL,\n" +
			"  `kind` varchar(64) NOT NULL,\n" +
			"  PRIMARY KEY (`id`),\n" +
			"  KEY `idx_samples_code` (`code`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		pairs := detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples`",
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  `code` varchar(64) NOT NULL,\n" +
				"  `kind` varchar(64) NOT NULL,\n" +
				"  PRIMARY KEY (`id`),\n" +
				"  INDEX `idx_records_kind` (`kind`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
		}, current)
		require.Len(t, pairs, 1)
		require.Equal(t, "samples", pairs[0].From)
		require.Equal(t, "records", pairs[0].To)
		require.Empty(t, pairs[0].IndexRenames)
		require.Len(t, pairs[0].Residual, 2)
	})

	t.Run("ambiguous multiple candidates report nothing", func(t *testing.T) {
		current := "CREATE TABLE `samples_a` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  PRIMARY KEY (`id`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;\n" +
			"CREATE TABLE `samples_b` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  PRIMARY KEY (`id`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		pairs := detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples_a`",
			"DROP TABLE `samples_b`",
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  PRIMARY KEY (`id`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
		}, current)
		require.Empty(t, pairs)
	})

	t.Run("unrelated statements report nothing", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  PRIMARY KEY (`id`)\n" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;"
		require.Empty(t, detectTableRenamesMySQL(t, []string{
			"DROP TABLE `samples`",
		}, current))
		require.Empty(t, detectTableRenamesMySQL(t, []string{
			"CREATE TABLE `records` (\n" +
				"  `id` char(36) NOT NULL,\n" +
				"  PRIMARY KEY (`id`)\n" +
				") ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_bin",
		}, current))
		require.Empty(t, detectTableRenamesMySQL(t, []string{
			"ALTER TABLE `samples` ADD COLUMN `kind` varchar(64)",
		}, current))
	})
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

func TestFormatTableRenames(t *testing.T) {
	t.Run("empty pairs render nothing", func(t *testing.T) {
		require.Empty(t, formatTableRenames(nil))
	})

	t.Run("pairs render comments plus executable statements at the end", func(t *testing.T) {
		guidance := formatTableRenames([]tableRenamePair{{
			From: "samples",
			To:   "records",
			IndexRenames: []renamePair{{
				Table:   "records",
				From:    "uniq_samples_code",
				To:      "uniq_records_code",
				Columns: "code",
				Unique:  true,
			}},
		}})
		require.Contains(t, guidance, "  -- Table `samples` -> `records`")
		require.Contains(t, guidance, "  --   index `uniq_samples_code` -> `uniq_records_code` (code, UNIQUE)")
		require.NotContains(t, guidance, "remaining", "no remaining-change note without residual statements")

		statementBlock := guidance[strings.LastIndex(guidance, "\n\n"):]
		require.Contains(t, statementBlock, "RENAME TABLE `samples` TO `records`;")
		require.Contains(t, statementBlock, "ALTER TABLE `records` RENAME INDEX `uniq_samples_code` TO `uniq_records_code`;")
		require.Less(t,
			strings.Index(statementBlock, "RENAME TABLE `samples` TO `records`;"),
			strings.Index(statementBlock, "ALTER TABLE `records` RENAME INDEX"),
			"the table must be renamed before its indexes")
		requireCopyPasteSafe(t, guidance)
	})

	t.Run("remaining changes render as comments, never as executable statements", func(t *testing.T) {
		guidance := formatTableRenames([]tableRenamePair{{
			From:     "samples",
			To:       "records",
			Residual: []string{"ALTER TABLE `records` ADD COLUMN `remark` varchar(255) NOT NULL DEFAULT ''"},
		}})
		require.Contains(t, guidance, "  --   remaining change: ALTER TABLE `records` ADD COLUMN `remark`")
		require.Contains(t, guidance, "re-run gg migrate after renaming")

		statementBlock := guidance[strings.LastIndex(guidance, "\n\n"):]
		require.NotContains(t, statementBlock, "ADD COLUMN", "residual statements stay out of the executable block")
		require.Contains(t, statementBlock, "RENAME TABLE `samples` TO `records`;")
		requireCopyPasteSafe(t, guidance)
	})
}

func TestCombineAdvisories(t *testing.T) {
	require.Empty(t, combineAdvisories("", ""))
	require.Equal(t, "a\n", combineAdvisories("a\n", ""))
	require.Equal(t, "b\n", combineAdvisories("", "b\n"))
	require.Equal(t, "a\n\nb\n", combineAdvisories("a\n", "b\n"))
}

// detectTableRenamesMySQL runs detection with the same MySQL parser wiring
// that run uses for real migration plans.
func detectTableRenamesMySQL(t *testing.T, ddls []string, currentDDLs string) []tableRenamePair {
	t.Helper()
	sqlParser := database.NewParser(parser.ParserModeMysql)
	return detectTableRenames(schema.GeneratorModeMysql, sqlParser, database.GeneratorConfig{EnableDrop: true}, "", ddls, currentDDLs)
}
