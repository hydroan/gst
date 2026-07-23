package dbmigrate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectIndexRenameHints(t *testing.T) {
	t.Run("one drop and one add form a reviewable pair", func(t *testing.T) {
		hints := detectIndexRenameHints([]string{
			"ALTER TABLE `samples` ADD UNIQUE INDEX `uniq_samples_code2` (`code`)",
			"ALTER TABLE `samples` DROP INDEX `uniq_samples_code`",
		})
		require.Len(t, hints, 1)
		require.Equal(t, "samples", hints[0].Table)
		require.Equal(t, []string{"uniq_samples_code"}, hints[0].Dropped)
		require.Equal(t, []addedIndex{{Name: "uniq_samples_code2", Columns: "`code`", Unique: true}}, hints[0].Added)
	})

	t.Run("covers plain, composite, unique, and standalone CREATE INDEX shapes", func(t *testing.T) {
		hints := detectIndexRenameHints([]string{
			"ALTER TABLE `samples` ADD UNIQUE INDEX `uniq_samples_code2` (`code`)",
			"ALTER TABLE `samples` ADD INDEX `idx_samples_kind2` (`kind`)",
			"ALTER TABLE `samples` ADD INDEX `idx_samples_group_kind2` (`group_id`, `name`)",
			"ALTER TABLE `samples` ADD UNIQUE INDEX `uniq_samples_group_code2` (`group_id`, `name`)",
			"CREATE INDEX `idx_samples_kind_time2` ON `samples` (`kind`, `created_at`)",
			"CREATE UNIQUE INDEX `uniq_samples_code_kind2` ON `samples` (`code`, `kind`)",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_group_kind`",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind_time`",
			"ALTER TABLE `samples` DROP INDEX `uniq_samples_code`",
			"ALTER TABLE `samples` DROP INDEX `uniq_samples_code_kind`",
			"ALTER TABLE `samples` DROP INDEX `uniq_samples_group_code`",
		})
		require.Len(t, hints, 1)
		require.Len(t, hints[0].Dropped, 6)
		require.Len(t, hints[0].Added, 6)
		require.Equal(t,
			addedIndex{Name: "uniq_samples_group_code2", Columns: "`group_id`, `name`", Unique: true},
			hints[0].Added[3])
		require.Equal(t,
			addedIndex{Name: "idx_samples_kind_time2", Columns: "`kind`, `created_at`", Unique: false},
			hints[0].Added[4])
	})

	t.Run("tolerates the KEY keyword", func(t *testing.T) {
		hints := detectIndexRenameHints([]string{
			"ALTER TABLE `groups` ADD UNIQUE KEY `idx_groups_group_no2` (`group_no`)",
			"ALTER TABLE `groups` DROP INDEX `idx_groups_group_no`",
		})
		require.Len(t, hints, 1)
		require.True(t, hints[0].Added[0].Unique)
	})

	t.Run("tables with only drops or only adds are not hinted", func(t *testing.T) {
		require.Empty(t, detectIndexRenameHints([]string{
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
			"ALTER TABLE `records` ADD INDEX `idx_records_kind` (`kind`)",
		}))
	})

	t.Run("tables do not share drops and adds", func(t *testing.T) {
		hints := detectIndexRenameHints([]string{
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
			"ALTER TABLE `samples` ADD INDEX `idx_samples_kind2` (`kind`)",
			"ALTER TABLE `records` DROP INDEX `idx_records_kind`",
			"ALTER TABLE `records` ADD INDEX `idx_records_kind2` (`kind`)",
		})
		require.Len(t, hints, 2)
		require.Equal(t, "samples", hints[0].Table)
		require.Equal(t, "records", hints[1].Table)
	})

	t.Run("non-index DDL statements are ignored", func(t *testing.T) {
		require.Empty(t, detectIndexRenameHints([]string{
			"ALTER TABLE `samples` ADD COLUMN `kind` varchar(64)",
			"ALTER TABLE `samples` DROP COLUMN `name`",
			"CREATE TABLE `records` (`id` char(36) NOT NULL, PRIMARY KEY (`id`))",
			"ALTER TABLE `samples` DROP INDEX `idx_samples_kind`",
		}))
	})
}

func TestFormatIndexRenameHints(t *testing.T) {
	t.Run("empty hints render nothing", func(t *testing.T) {
		require.Empty(t, formatIndexRenameHints(nil))
	})

	t.Run("a single pair renders a ready-to-run RENAME statement", func(t *testing.T) {
		guidance := formatIndexRenameHints([]indexRenameHint{{
			Table:   "groups",
			Dropped: []string{"idx_groups_group_no"},
			Added:   []addedIndex{{Name: "idx_groups_group_no2", Columns: "`group_no`", Unique: true}},
		}})
		require.Contains(t, guidance, "  -- Table `groups`: `idx_groups_group_no` -> UNIQUE `idx_groups_group_no2` (`group_no`)")
		require.Contains(t, guidance, "\nALTER TABLE `groups` RENAME INDEX `idx_groups_group_no` TO `idx_groups_group_no2`;\n")
		require.Contains(t, guidance, "only modifies metadata")
		requireCopyPasteSafe(t, guidance)
	})

	t.Run("multiple candidates render both sides as comments only", func(t *testing.T) {
		guidance := formatIndexRenameHints([]indexRenameHint{{
			Table:   "samples",
			Dropped: []string{"idx_samples_kind", "uniq_samples_code"},
			Added: []addedIndex{
				{Name: "idx_samples_kind2", Columns: "`kind`"},
				{Name: "uniq_samples_code2", Columns: "`code`", Unique: true},
			},
		}})
		require.Contains(t, guidance, "`idx_samples_kind`, `uniq_samples_code`")
		require.Contains(t, guidance, "`idx_samples_kind2` (`kind`)")
		require.Contains(t, guidance, "UNIQUE `uniq_samples_code2` (`code`)")
		require.Contains(t, guidance, "  --   template: ALTER TABLE `samples` RENAME INDEX <old> TO <new>;")
		// Ambiguous pairings must not expose an executable statement.
		requireCopyPasteSafe(t, guidance)
		for line := range strings.SplitSeq(guidance, "\n") {
			require.False(t, strings.HasPrefix(line, "ALTER"), "unexpected executable line: %q", line)
		}
	})

	t.Run("mixed hints group executable statements at the end", func(t *testing.T) {
		guidance := formatIndexRenameHints([]indexRenameHint{
			{
				Table:   "groups",
				Dropped: []string{"idx_groups_group_no"},
				Added:   []addedIndex{{Name: "idx_groups_group_no2", Columns: "`group_no`", Unique: true}},
			},
			{
				Table:   "records",
				Dropped: []string{"idx_records_kind"},
				Added:   []addedIndex{{Name: "idx_records_kind2", Columns: "`kind`"}},
			},
		})
		statementBlock := guidance[strings.LastIndex(guidance, "\n\n"):]
		require.Contains(t, statementBlock, "ALTER TABLE `groups` RENAME INDEX `idx_groups_group_no` TO `idx_groups_group_no2`;")
		require.Contains(t, statementBlock, "ALTER TABLE `records` RENAME INDEX `idx_records_kind` TO `idx_records_kind2`;")
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
