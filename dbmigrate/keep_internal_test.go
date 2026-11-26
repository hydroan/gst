package dbmigrate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWithoutIndexLineTakesTheLineOfTheIndexOut holds the example of the
// withoutIndexLine doc comment, the last definition of a table going with
// the comma before it, beside an index in the middle of the list, one whose
// name another index begins with, and one no line declares.
func TestWithoutIndexLineTakesTheLineOfTheIndexOut(t *testing.T) {
	exported := "CREATE TABLE `samples` (\n" +
		"  `id` char(36) NOT NULL,\n" +
		"  `note` varchar(64) NOT NULL,\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  FULLTEXT KEY `ft_samples_note` (`note`)\n" +
		") ENGINE=InnoDB"
	require.Equal(t, "CREATE TABLE `samples` (\n"+
		"  `id` char(36) NOT NULL,\n"+
		"  `note` varchar(64) NOT NULL,\n"+
		"  PRIMARY KEY (`id`)\n"+
		") ENGINE=InnoDB", withoutIndexLine(exported, "ft_samples_note"))

	several := "CREATE TABLE `samples` (\n" +
		"  `id` char(36) NOT NULL,\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  UNIQUE KEY `uniq_samples_note` (`note`),\n" +
		"  KEY `idx_samples_note_title` (`note`,`title`),\n" +
		"  KEY `idx_samples_note` (`note`(10))\n" +
		") ENGINE=InnoDB"
	require.Equal(t, "CREATE TABLE `samples` (\n"+
		"  `id` char(36) NOT NULL,\n"+
		"  PRIMARY KEY (`id`),\n"+
		"  KEY `idx_samples_note_title` (`note`,`title`),\n"+
		"  KEY `idx_samples_note` (`note`(10))\n"+
		") ENGINE=InnoDB", withoutIndexLine(several, "uniq_samples_note"))
	require.Equal(t, "CREATE TABLE `samples` (\n"+
		"  `id` char(36) NOT NULL,\n"+
		"  PRIMARY KEY (`id`),\n"+
		"  UNIQUE KEY `uniq_samples_note` (`note`),\n"+
		"  KEY `idx_samples_note_title` (`note`,`title`)\n"+
		") ENGINE=InnoDB", withoutIndexLine(several, "idx_samples_note"))
	require.Equal(t, several, withoutIndexLine(several, "idx_samples_title"))
}
