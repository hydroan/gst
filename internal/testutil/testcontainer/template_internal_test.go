package testcontainer

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// templateSample is the one model this binary registers. A template is named
// after the schema of the registered models, so declaring a model no other
// binary knows is what gives this test a template of its own.
type templateSample struct {
	Name string

	modelregistry.Base
}

func (*templateSample) TableName() string { return "template_samples" }

func TestSchemaTemplate(t *testing.T) {
	isolateEnv(t,
		config.MYSQL_HOST, config.MYSQL_PORT, config.MYSQL_DATABASE,
		config.MYSQL_USERNAME, config.MYSQL_PASSWORD,
		config.DATABASE_TYPE, config.DATABASE_AUTO_MIGRATE)

	// Registering the model gives this binary a schema to fingerprint. Taking
	// it back off the queue and reporting it done leaves the pending count
	// where the test found it, since nothing here drains that queue.
	modelregistry.RegisterTable[*templateSample]()
	<-modelregistry.TableChan
	modelregistry.TableDone()

	ctx := context.Background()
	template := schemaTemplatePrefix + modelregistry.SchemaFingerprint()

	// The first setup stands in for the binary that finds no template: it
	// migrates on its own and leaves the result behind for the ones after it.
	releaseFirst, publish, err := setupSharedMySQL()
	require.NoError(t, err)
	first := os.Getenv(config.MYSQL_DATABASE)

	admin := openSharedAdmin(t, mysqlSharedDialect,
		os.Getenv(config.MYSQL_HOST), os.Getenv(config.MYSQL_PORT))
	t.Cleanup(func() {
		require.NoError(t, releaseFirst())
		_, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+template+"`")
		require.NoError(t, err)
	})

	require.False(t, sharedDatabaseListed(t, admin, mysqlSharedDialect, template),
		"a model only this binary declares cannot have a template yet")
	require.False(t, tableExists(t, admin, first, "template_samples"),
		"with no template to copy, the database starts empty")

	// Stand in for the framework migrating the registered model, which is what
	// runs between the setup and the publish hook.
	migrateTemplateSample(t, admin, first)
	publish()

	t.Run("publishing_leaves_a_complete_template_behind", func(t *testing.T) {
		require.True(t, sharedDatabaseListed(t, admin, mysqlSharedDialect, template))
		require.True(t, tableExists(t, admin, template, "template_samples"))
		// The marker is written last, so its presence is what says the
		// template was filled all the way to the end.
		require.True(t, tableExists(t, admin, template, schemaTemplateReadyTable))
	})

	t.Run("the_next_binary_starts_from_the_template", func(t *testing.T) {
		releaseSecond, _, err := setupSharedMySQL()
		require.NoError(t, err)
		second := os.Getenv(config.MYSQL_DATABASE)
		t.Cleanup(func() { require.NoError(t, releaseSecond()) })

		require.NotEqual(t, first, second)
		require.True(t, tableExists(t, admin, second, "template_samples"),
			"the table should arrive with the template, before any migration runs")
		// The marker tracks the template rather than the schema, so it is no
		// part of what a binary copies.
		require.False(t, tableExists(t, admin, second, schemaTemplateReadyTable))
	})

	t.Run("a_half_published_template_is_dropped", func(t *testing.T) {
		// What a binary that died mid-publish leaves behind: tables, but no
		// marker. The next setup must drop it rather than copy from it.
		leftover := schemaTemplatePrefix + "0123456789abcdef"
		_, err := admin.ExecContext(ctx, "CREATE DATABASE `"+leftover+"`")
		require.NoError(t, err)
		t.Cleanup(func() {
			_, dropErr := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+leftover+"`")
			require.NoError(t, dropErr)
		})
		_, err = admin.ExecContext(ctx, "CREATE TABLE `"+leftover+"`.`half_written` (id int PRIMARY KEY)")
		require.NoError(t, err)

		release, _, err := setupSharedMySQL()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, release()) })

		require.False(t, sharedDatabaseListed(t, admin, mysqlSharedDialect, leftover))
	})

	t.Run("a_copy_that_cannot_run_leaves_the_binary_to_migrate", func(t *testing.T) {
		// Copying into a database that is not there fails on the first table.
		// Nothing may come of that but a report: the framework's migration
		// creates the schema either way, which is what lets a template stay a
		// shortcut instead of becoming a dependency.
		require.NotPanics(t, func() {
			copySchemaTemplate(ctx, admin, template, "gst_test_absent_database")
		})
	})
}

// migrateTemplateSample creates the table of templateSample, standing in for
// the framework migration that runs before a template is published.
func migrateTemplateSample(t *testing.T, admin *sql.DB, database string) {
	t.Helper()
	_, err := admin.ExecContext(context.Background(),
		"CREATE TABLE `"+database+"`.`template_samples` (id varchar(36) PRIMARY KEY, name varchar(64))")
	require.NoError(t, err)
}

// tableExists reports whether the database holds a table by that name.
func tableExists(t *testing.T, admin *sql.DB, database, table string) bool {
	t.Helper()
	var count int
	require.NoError(t, admin.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?",
		database, table).Scan(&count))
	return count > 0
}
