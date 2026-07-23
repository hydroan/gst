package dbmigrate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectIndexRenames(t *testing.T) {
	t.Run("identical definitions form a verified pair", func(t *testing.T) {
		current := "CREATE TABLE `groups` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  PRIMARY KEY (`id`),\n" +
			"  UNIQUE KEY `idx_groups_group_no` (`group_no`)\n" +
			") ENGINE=InnoDB;"
		pairs := detectIndexRenames([]string{
			"ALTER TABLE `groups` ADD UNIQUE INDEX `idx_groups_group_no2` (`group_no`)",
			"ALTER TABLE `groups` DROP INDEX `idx_groups_group_no`",
		}, current)
		require.Equal(t, []renamePair{{
			Table:   "groups",
			From:    "idx_groups_group_no",
			To:      "idx_groups_group_no2",
			Columns: "group_no",
			Unique:  true,
		}}, pairs)
	})

	t.Run("changed column sets are rebuilds, not renames", func(t *testing.T) {
		// Regression: dropping (group_id, admin_user_id, created_at) while
		// adding (group_id, created_at) must not be reported as a rename.
		current := "CREATE TABLE `admin_operation_logs` (\n" +
			"  `id` char(36) NOT NULL,\n" +
			"  PRIMARY KEY (`id`),\n" +
			"  KEY `idx_admin_operation_logs_group_id_admin_user_id_created_at` (`group_id`,`admin_user_id`,`created_at`)\n" +
			") ENGINE=InnoDB;"
		pairs := detectIndexRenames([]string{
			"CREATE INDEX `idx_admin_operation_logs_group_id_created_at` ON `admin_operation_logs` (`group_id`, `created_at`)",
			"ALTER TABLE `admin_operation_logs` DROP INDEX `idx_admin_operation_logs_group_id_admin_user_id_created_at`",
		}, current)
		require.Empty(t, pairs)
	})

	t.Run("uniqueness changes are rebuilds, not renames", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  PRIMARY KEY (`id`),\n" +
			"  KEY `idx_samples_kind` (`kind`)\n" +
			") ENGINE=InnoDB;"
		pairs := detectIndexRenames([]string{
			"ALTER TABLE `samples` ADD UNIQUE INDEX `uniq_samples_kind` (`kind`)",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
		}, current)
		require.Empty(t, pairs)
	})

	t.Run("multiple renames pair one to one across statement shapes", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  PRIMARY KEY (`id`),\n" +
			"  KEY `idx_samples_group_kind` (`group_id`,`name`),\n" +
			"  UNIQUE KEY `uniq_samples_code_kind` (`code`,`kind`)\n" +
			") ENGINE=InnoDB;"
		pairs := detectIndexRenames([]string{
			"ALTER TABLE `samples` ADD INDEX `idx_samples_group_kind2` (`group_id`, `name`)",
			"CREATE UNIQUE INDEX `uniq_samples_code_kind2` ON `samples` (`code`, `kind`)",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_group_kind`",
			"ALTER TABLE `samples` DROP INDEX `uniq_samples_code_kind`",
		}, current)
		require.Len(t, pairs, 2)
		require.Equal(t, "idx_samples_group_kind", pairs[0].From)
		require.Equal(t, "idx_samples_group_kind2", pairs[0].To)
		require.Equal(t, "uniq_samples_code_kind", pairs[1].From)
		require.Equal(t, "uniq_samples_code_kind2", pairs[1].To)
	})

	t.Run("ambiguous duplicate definitions report nothing", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  PRIMARY KEY (`id`),\n" +
			"  KEY `idx_samples_kind_a` (`kind`),\n" +
			"  KEY `idx_samples_kind_b` (`kind`)\n" +
			") ENGINE=InnoDB;"
		pairs := detectIndexRenames([]string{
			"ALTER TABLE `samples` ADD INDEX `idx_samples_kind` (`kind`)",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind_a`",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind_b`",
		}, current)
		require.Empty(t, pairs)
	})

	t.Run("tables do not share drops and adds", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  KEY `idx_samples_kind` (`kind`)\n" +
			") ENGINE=InnoDB;\n" +
			"CREATE TABLE `records` (\n" +
			"  KEY `idx_records_kind` (`kind`)\n" +
			") ENGINE=InnoDB;"
		pairs := detectIndexRenames([]string{
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
			"ALTER TABLE `records` ADD INDEX `idx_records_kind2` (`kind`)",
			"ALTER TABLE `records` DROP INDEX `idx_records_kind`",
		}, current)
		require.Equal(t, []renamePair{{
			Table:   "records",
			From:    "idx_records_kind",
			To:      "idx_records_kind2",
			Columns: "kind",
		}}, pairs)
	})

	t.Run("column prefix lengths participate in the comparison", func(t *testing.T) {
		current := "CREATE TABLE `samples` (\n" +
			"  KEY `idx_samples_remark` (`remark`(20))\n" +
			") ENGINE=InnoDB;"
		pairs := detectIndexRenames([]string{
			"ALTER TABLE `samples` ADD INDEX `idx_samples_remark2` (`remark`(20))",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_remark`",
		}, current)
		require.Len(t, pairs, 1)
		require.Equal(t, "remark(20)", pairs[0].Columns)

		// A different prefix length is a rebuild, not a rename.
		pairs = detectIndexRenames([]string{
			"ALTER TABLE `samples` ADD INDEX `idx_samples_remark2` (`remark`(30))",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_remark`",
		}, current)
		require.Empty(t, pairs)
	})

	t.Run("non-index DDL statements are ignored", func(t *testing.T) {
		require.Empty(t, detectIndexRenames([]string{
			"ALTER TABLE `samples` ADD COLUMN `kind` varchar(64)",
			"ALTER TABLE `samples` DROP COLUMN `name`",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
		}, ""))
	})
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

func TestFormatIndexRenames(t *testing.T) {
	t.Run("empty pairs render nothing", func(t *testing.T) {
		require.Empty(t, formatIndexRenames(nil))
	})

	t.Run("pairs render comments plus executable statements at the end", func(t *testing.T) {
		guidance := formatIndexRenames([]renamePair{
			{Table: "groups", From: "idx_groups_group_no", To: "idx_groups_group_no2", Columns: "group_no", Unique: true},
			{Table: "records", From: "idx_records_kind", To: "idx_records_kind2", Columns: "kind"},
		})
		require.Contains(t, guidance, "  -- Table `groups`: `idx_groups_group_no` -> `idx_groups_group_no2` (group_no, UNIQUE)")
		require.Contains(t, guidance, "  -- Table `records`: `idx_records_kind` -> `idx_records_kind2` (kind)")

		statementBlock := guidance[strings.LastIndex(guidance, "\n\n"):]
		require.Contains(t, statementBlock, "ALTER TABLE `groups` RENAME INDEX `idx_groups_group_no` TO `idx_groups_group_no2`;")
		require.Contains(t, statementBlock, "ALTER TABLE `records` RENAME INDEX `idx_records_kind` TO `idx_records_kind2`;")
		requireCopyPasteSafe(t, guidance)
	})
}

// requireCopyPasteSafe asserts that every advisory line is either a comment,
// a blank line, or a directly executable statement, so pasting any part of
// the block into MySQL cannot fail.
func requireCopyPasteSafe(t *testing.T, guidance string) {
	t.Helper()

	for line := range strings.SplitSeq(guidance, "\n") {
		switch {
		case len(strings.TrimSpace(line)) == 0:
		case strings.HasPrefix(strings.TrimSpace(line), "--"):
		case strings.HasPrefix(line, "ALTER TABLE ") && strings.HasSuffix(line, ";"):
		default:
			t.Fatalf("line is neither comment, blank, nor executable: %q", line)
		}
	}
}
