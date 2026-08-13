# module/logmgmt

Login and operation logging for gst applications.

## What it provides

- **Login log** — every IAM login lifecycle event (success, failure, logout) is
  recorded to the `login_logs` table by a login observer that
  `internal/service/logmgmt` installs into the `authn` hook at package
  initialization. Queryable through `GET /api/log/loginlog[/:id]`.
- **Operation log** — mutations recorded by the audit pipeline land in the
  `operation_logs` table. Queryable through `GET /api/log/operationlog[/:id]`.
- **Retention** — `servicelogmgmt.Cleanup` deletes rows older than the
  configured retention, batch by batch in independent transactions.

## Add path

```go
import "github.com/hydroan/gst/module/logmgmt"

logmgmt.Register()
```

Registration fails startup unless the audit pipeline is enabled
(`AUDIT_ENABLED=true` or the `audit.enabled` config key): an operation log that
silently stays empty would look like "no operations happened". The retention
job is registered automatically; its schedule comes from the
`LOGMGMT_CLEANUP_CRON` environment key (hourly by default — registration runs
at package initialization, before configuration files are loaded), and the
retention window from `logmgmt.retention` / `LOGMGMT_RETENTION` (90 days by
default).

## Copy path

```sh
gg module copy logmgmt
```

The copied service package installs the login observer the same way, through
package initialization. Two manual steps remain, printed as post notes: enable
the audit pipeline in configuration, and register the retention job in
project-owned cronjob setup.

## Tenancy

Both tables are platform-level audit data with cross-tenant reads; see
TENANT.md before exposing the query routes in a multi-tenant deployment.
