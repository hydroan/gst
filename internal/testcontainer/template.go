package testcontainer

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
)

// Schema templates.
//
// Creating the tables of every registered model is what a test binary spends
// most of its startup on, and every binary of a run creates the same ones. A
// schema template is that work done once: the first binary migrates normally
// and leaves its tables behind in a database of its own, and every later
// binary copies them with one statement per table, indexes included and no
// metadata query at all.
//
// A template is named after modelregistry.SchemaFingerprint, so a model change
// names a template that does not exist yet rather than reusing a stale one.
// The fingerprint is what makes a copy safe to keep: a migration repairs a
// table whose definition moved, but never drops what a model stopped
// declaring, and that is the case the fingerprint is built not to miss.
//
// Nothing here can fail a test run. Every step reports what stopped it and
// leaves the binary to migrate, because the framework's own migration runs
// either way and a table already in place is one it has nothing left to do
// to. That is what makes a template safe to trust — it is never the authority
// on the schema, only a head start on it — and it bounds what a broken,
// half-written or vanished template can cost at the startup it meant to save.

const (
	// schemaTemplatePrefix names the databases holding a schema template. The
	// schema fingerprint follows it.
	schemaTemplatePrefix = "gst_tmpl_"

	// schemaTemplateReadyTable is created last, after every table of a
	// template is in place. A template without it was interrupted halfway, so
	// its absence is what tells a complete template from a leftover.
	schemaTemplateReadyTable = "gst_template_ready"

	// schemaTemplateStaleAfter is how long a template is kept. The shared
	// container serves more than one project and more than one branch, so a
	// template is only ever dropped on age, never because another fingerprint
	// turned up.
	schemaTemplateStaleAfter = 7 * 24 * time.Hour
)

// noSchemaTemplatePublish is the hook of a binary with nothing to publish.
func noSchemaTemplatePublish() {}

// prepareSchemaTemplate copies the schema template matching the registered
// models into the freshly provisioned database, and returns the hook that
// publishes a template when none matched. The hook runs once the framework
// has migrated, which is what makes the published tables the ones the
// migration itself produced rather than a second rendering of them.
//
// Only mysql takes part. Postgres executes schema creation an order of
// magnitude more cheaply, so it has nothing to gain here, and a dedicated
// container is not shared with anyone to gain it from. Both simply migrate,
// which is indistinguishable from copying a template except in how long it
// takes.
func prepareSchemaTemplate(host string, port uint, database string) func() {
	fingerprint := modelregistry.SchemaFingerprint()
	if len(fingerprint) == 0 {
		// Nothing registered, so there is no schema to carry over.
		return noSchemaTemplatePublish
	}
	template := schemaTemplatePrefix + fingerprint

	admin, err := sql.Open(mysqlSharedDialect.driver, mysqlSharedDialect.adminDSN(host, port))
	if err != nil {
		reportSchemaTemplateSkipped(errors.Wrap(err, "failed to open the shared container admin connection"))
		return noSchemaTemplatePublish
	}
	defer func() { _ = admin.Close() }()

	ctx := context.Background()
	var ready bool
	if err := withSharedContainerLock(sharedContainerName(mysqlImage, mysqlSharedArgs...), func() error {
		if dropErr := dropUnusableSchemaTemplates(ctx, admin); dropErr != nil {
			return dropErr
		}
		var readyErr error
		ready, readyErr = schemaTemplateReady(ctx, admin, template)
		return readyErr
	}); err != nil {
		reportSchemaTemplateSkipped(err)
		return noSchemaTemplatePublish
	}

	if !ready {
		return func() { publishSchemaTemplate(host, port, database, template) }
	}
	copySchemaTemplate(ctx, admin, template, database)
	return noSchemaTemplatePublish
}

// copySchemaTemplate creates every table of the template in database. It
// reports what stopped it instead of failing: the migration that follows
// creates whatever is missing, so a partial copy costs startup time and
// nothing else.
func copySchemaTemplate(ctx context.Context, admin *sql.DB, template, database string) {
	tables, err := schemaTemplateTables(ctx, admin, template)
	if err != nil {
		reportSchemaTemplateSkipped(err)
		return
	}

	for _, table := range tables {
		statement := fmt.Sprintf("CREATE TABLE `%s`.`%s` LIKE `%s`.`%s`", database, table, template, table)
		if _, err := admin.ExecContext(ctx, statement); err != nil {
			reportSchemaTemplateSkipped(errors.Wrapf(err, "failed to copy table %s", table))
			return
		}
	}
	reportServiceReady("mysql schema template", fmt.Sprintf("%s (%d tables copied)", template, len(tables)))
}

// publishSchemaTemplate leaves the migrated tables of database behind as the
// template, for the binaries that come after. The ready marker is created
// last, so a publish that dies halfway leaves a template the next run drops
// rather than one it copies.
//
// Holding the container lock for the whole publish is what lets the marker
// mean what it says: no other binary can be reading the template while it is
// still being filled.
func publishSchemaTemplate(host string, port uint, database, template string) {
	admin, err := sql.Open(mysqlSharedDialect.driver, mysqlSharedDialect.adminDSN(host, port))
	if err != nil {
		reportSchemaTemplateSkipped(errors.Wrap(err, "failed to open the shared container admin connection"))
		return
	}
	defer func() { _ = admin.Close() }()

	ctx := context.Background()
	if err := withSharedContainerLock(sharedContainerName(mysqlImage, mysqlSharedArgs...), func() error {
		ready, err := schemaTemplateReady(ctx, admin, template)
		if err != nil || ready {
			// Another binary of this run published it first, which is the
			// common case: every binary of a first run misses the template.
			return err
		}
		if _, createErr := admin.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+template+"`"); createErr != nil {
			return errors.Wrapf(createErr, "failed to create the schema template %s", template)
		}

		tables, listErr := schemaTemplateTables(ctx, admin, database)
		if listErr != nil {
			return listErr
		}
		for _, table := range tables {
			statement := fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s`.`%s` LIKE `%s`.`%s`", template, table, database, table)
			if _, copyErr := admin.ExecContext(ctx, statement); copyErr != nil {
				return errors.Wrapf(copyErr, "failed to publish table %s to the schema template %s", table, template)
			}
		}

		marker := fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s`.`%s` (id int PRIMARY KEY)", template, schemaTemplateReadyTable)
		if _, markErr := admin.ExecContext(ctx, marker); markErr != nil {
			return errors.Wrapf(markErr, "failed to mark the schema template %s ready", template)
		}
		reportServiceReady("mysql schema template", fmt.Sprintf("%s (%d tables published)", template, len(tables)))
		return nil
	}); err != nil {
		reportSchemaTemplateSkipped(err)
	}
}

// schemaTemplateReady reports whether the template exists and was filled all
// the way to its ready marker.
func schemaTemplateReady(ctx context.Context, admin *sql.DB, template string) (bool, error) {
	var count int
	err := admin.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?",
		template, schemaTemplateReadyTable).Scan(&count)
	if err != nil {
		return false, errors.Wrapf(err, "failed to inspect the schema template %s", template)
	}
	return count > 0, nil
}

// schemaTemplateTables returns the tables to copy out of or into a template,
// which is every table of the database except the ready marker: the marker
// tracks the template itself and belongs to no schema being copied.
func schemaTemplateTables(ctx context.Context, admin *sql.DB, database string) ([]string, error) {
	rows, err := admin.QueryContext(ctx,
		"SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_name <> ? ORDER BY table_name",
		database, schemaTemplateReadyTable)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list the tables of %s", database)
	}
	defer func() { _ = rows.Close() }()

	tables := make([]string, 0, 64)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, errors.Wrap(err, "failed to scan a table name")
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrapf(err, "failed to list the tables of %s", database)
	}
	return tables, nil
}

// dropUnusableSchemaTemplates drops the templates no run should copy from: the
// ones left half-published by a binary that died, and the ones nothing has
// rebuilt for schemaTemplateStaleAfter. Age is the only reason a complete
// template goes, because the container is shared and a fingerprint this run
// does not recognize is another project's, not a leftover.
//
// The caller holds the container lock, which is what keeps a template being
// published from looking half-finished here.
func dropUnusableSchemaTemplates(ctx context.Context, admin *sql.DB) error {
	// Age is compared by the server, against the clock that stamped
	// create_time, and the answer arrives as an integer: reading a DATETIME
	// into a Go time would need parseTime on the shared admin connection,
	// which every other user of that connection would then inherit.
	rows, err := admin.QueryContext(ctx, `
		SELECT table_schema,
		       COALESCE(MAX(create_time) < NOW() - INTERVAL ? SECOND, 0),
		       MAX(table_name = ?)
		  FROM information_schema.tables
		 WHERE table_schema LIKE ?
		 GROUP BY table_schema`,
		int(schemaTemplateStaleAfter.Seconds()), schemaTemplateReadyTable, schemaTemplatePrefix+"%")
	if err != nil {
		return errors.Wrap(err, "failed to list the schema templates")
	}
	defer func() { _ = rows.Close() }()

	unusable := make([]string, 0)
	for rows.Next() {
		var (
			name  string
			stale int
			ready int
		)
		if err := rows.Scan(&name, &stale, &ready); err != nil {
			return errors.Wrap(err, "failed to scan a schema template")
		}
		if ready == 0 || stale == 1 {
			unusable = append(unusable, name)
		}
	}
	if err := rows.Err(); err != nil {
		return errors.Wrap(err, "failed to list the schema templates")
	}

	for _, name := range unusable {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			return errors.Wrapf(err, "failed to drop the unusable schema template %s", name)
		}
	}
	return nil
}

// reportSchemaTemplateSkipped reports that this binary is creating the schema
// itself after all. Container logging is muted, see muteContainerLog, so a
// template that stopped being a shortcut would otherwise show up as nothing
// but a slower run.
func reportSchemaTemplateSkipped(reason error) {
	fmt.Fprintf(os.Stdout, "test mysql schema template skipped, migrating instead: %v\n", reason)
}
