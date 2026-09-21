package dbmigrate

import (
	"strings"
	"testing"

	"github.com/sqldef/sqldef/v3/schema"
	"github.com/stretchr/testify/require"
)

func TestCombineAdvisories(t *testing.T) {
	require.Empty(t, combineAdvisories("", ""))
	require.Equal(t, "a\n", combineAdvisories("a\n", ""))
	require.Equal(t, "b\n", combineAdvisories("", "b\n"))
	require.Equal(t, "a\n\nb\n", combineAdvisories("a\n", "b\n"))
}

func TestFormatTableRenames(t *testing.T) {
	t.Run("empty pairs render nothing", func(t *testing.T) {
		require.Empty(t, formatTableRenames(schema.GeneratorModeMysql, nil))
	})

	t.Run("pairs render comments plus executable statements at the end", func(t *testing.T) {
		guidance := formatTableRenames(schema.GeneratorModeMysql, []tableRenamePair{{
			From: "samples",
			To:   "records",
			IndexRenames: []indexRenamePair{{
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
		guidance := formatTableRenames(schema.GeneratorModeMysql, []tableRenamePair{{
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

	t.Run("postgres pairs render that server's rename syntax", func(t *testing.T) {
		guidance := formatTableRenames(schema.GeneratorModePostgres, []tableRenamePair{{
			From: "samples",
			To:   "records",
			IndexRenames: []indexRenamePair{{
				Table:   "records",
				From:    "uniq_samples_code",
				To:      "uniq_records_code",
				Columns: "code",
				Unique:  true,
			}},
		}})
		require.Contains(t, guidance, `  -- Table "samples" -> "records"`)

		statementBlock := guidance[strings.LastIndex(guidance, "\n\n"):]
		require.Contains(t, statementBlock, `ALTER TABLE "samples" RENAME TO "records";`)
		require.Contains(t, statementBlock, `ALTER INDEX "uniq_samples_code" RENAME TO "uniq_records_code";`)
		requireCopyPasteSafe(t, guidance)
	})
}

func TestFormatIndexRenames(t *testing.T) {
	t.Run("empty pairs render nothing", func(t *testing.T) {
		require.Empty(t, formatIndexRenames(schema.GeneratorModeMysql, nil))
	})

	t.Run("pairs render comments plus executable statements at the end", func(t *testing.T) {
		guidance := formatIndexRenames(schema.GeneratorModeMysql, []indexRenamePair{
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

	t.Run("postgres pairs render that server's rename syntax", func(t *testing.T) {
		guidance := formatIndexRenames(schema.GeneratorModePostgres, []indexRenamePair{
			{Table: "groups", From: "idx_groups_group_no", To: "idx_groups_group_no2", Columns: "group_no", Unique: true},
		})
		require.Contains(t, guidance, `  -- Table "groups": "idx_groups_group_no" -> "idx_groups_group_no2" (group_no, UNIQUE)`)

		statementBlock := guidance[strings.LastIndex(guidance, "\n\n"):]
		require.Contains(t, statementBlock, `ALTER INDEX "idx_groups_group_no" RENAME TO "idx_groups_group_no2";`)
		requireCopyPasteSafe(t, guidance)
	})
}

// requireCopyPasteSafe asserts that every advisory line is either a comment,
// a blank line, or a directly executable statement, so pasting any part of
// the block into the server cannot fail.
func requireCopyPasteSafe(t *testing.T, guidance string) {
	t.Helper()

	for line := range strings.SplitSeq(guidance, "\n") {
		switch {
		case len(strings.TrimSpace(line)) == 0:
		case strings.HasPrefix(strings.TrimSpace(line), "--"):
		case strings.HasPrefix(line, "ALTER TABLE ") && strings.HasSuffix(line, ";"):
		case strings.HasPrefix(line, "ALTER INDEX ") && strings.HasSuffix(line, ";"):
		case strings.HasPrefix(line, "RENAME TABLE ") && strings.HasSuffix(line, ";"):
		default:
			t.Fatalf("line is neither comment, blank, nor executable: %q", line)
		}
	}
}
