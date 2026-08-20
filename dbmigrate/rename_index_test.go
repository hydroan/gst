package dbmigrate

import (
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
		require.Equal(t, []indexRenamePair{{
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
		require.Equal(t, []indexRenamePair{{
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

	t.Run("postgres statement shapes pair through the current schema", func(t *testing.T) {
		// The postgres export carries standalone CREATE INDEX statements and
		// the plan drops indexes without naming the table; the owner comes
		// from the exported schema.
		current := "CREATE TABLE public.samples (\n" +
			"    id character(36) NOT NULL,\n" +
			"    CONSTRAINT samples_pkey PRIMARY KEY (\"id\")\n" +
			");\n" +
			"CREATE UNIQUE INDEX uniq_samples_code_kind ON public.samples USING btree (code, kind);"
		pairs := detectIndexRenames([]string{
			"CREATE UNIQUE INDEX uniq_samples_code_kind2 ON public.samples (code, kind)",
			"DROP INDEX public.uniq_samples_code_kind",
		}, current)
		require.Equal(t, []indexRenamePair{{
			Table:   "samples",
			From:    "uniq_samples_code_kind",
			To:      "uniq_samples_code_kind2",
			Columns: "code,kind",
			Unique:  true,
		}}, pairs)
	})

	t.Run("non-index DDL statements are ignored", func(t *testing.T) {
		require.Empty(t, detectIndexRenames([]string{
			"ALTER TABLE `samples` ADD COLUMN `kind` varchar(64)",
			"ALTER TABLE `samples` DROP COLUMN `name`",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
		}, ""))
	})
}
