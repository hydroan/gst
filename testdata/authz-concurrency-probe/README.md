# authz concurrency probe

Measures whether replacing one role's permission set stays atomic when two
transactions do it at once, per database engine.

```bash
go run ./testdata/authz-concurrency-probe
```

Needs the containers in `testdata/mysql` and `testdata/postgresql`. It creates
`probe_casbin_rule` and `probe_roles` in the `test` database and drops both on
the way out. Exits non-zero if any scenario unions.

## Why it exists

`replacePermissions` replaces a role's permissions by deleting its policy rows
and inserting the new ones. Two of those interleaving can leave the union of
both sets in storage while the deciding process holds only the later one, and
nothing detects the difference until something reloads.

Whether they can interleave is not a property of the RBAC package. It depends on
what the surrounding transaction is already holding, which differs per write
path and per engine — so it is measured rather than reasoned about. This probe
is what established that only one write path was ever exposed.

## What it reports

The role step is the difference between the framework's write paths:

| role step | write path |
|---|---|
| `written` | `Role.CreateAfter` / `Role.UpdateAfter` — the role row is written first, so its exclusive lock is held for the rest of the transaction |
| `read` | `Menu.UpdateAfter` before `rolesToRefresh` took a lock — the roles are read and nothing is held |
| `locked` | `Menu.UpdateAfter` as it stands — the roles are read `FOR UPDATE` |

Each runs against a role that already holds permissions and one that holds
none, because rows that do not exist cannot be locked and that decides the
outcome.

Result at the time of writing:

| role step | role has permissions | PostgreSQL | MySQL |
|---|---|---|---|
| `written` | either | serialized | serialized |
| `read` | yes | serialized | serialized |
| `read` | **no** | **UNION** | serialized |
| `locked` | either | serialized | serialized |

The single failing cell is the defect, and it is invisible on MySQL: under
REPEATABLE READ a delete that matches nothing still locks the range it scanned,
so the second writer waits. PostgreSQL under READ COMMITTED has nothing to lock
in an empty range, so neither delete removes the other's insert.

`locked` is the state the code is in now, and it is serialized everywhere —
which is what the lock in `rolesToRefresh` is for.

SQLite is not probed: it admits one writer at a time, so this interleaving
cannot occur there.
