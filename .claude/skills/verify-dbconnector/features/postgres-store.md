# PostgreSQL store

A caller builds a `factory.Config` from `PG_*` environment variables, calls `factory.NewStore`, and gets an `abstract.Store` backed by `database/sql` and `lib/pq`. It can create, read, update and delete rows, run raw SQL, and it refuses the dangerous calls.

## Sub-features

- `pg-env-config` builds the Config with `factory.FromEnv(ctx, "PG_")`.
- `pg-crud` runs `Create`, `Read`, `Update`, `Delete` on `accounts`.
- `pg-exec-sql` runs `ExecWithSql` with `$1` placeholders and gets rows back as `[]any` of `map[string]any`.
- `pg-guards` refuses `Update`/`Delete` with an empty filter (`ErrEmptyFilter`) and a table name that is not a plain name (`ErrInvalidIdentifier`).

## How to get to it (user POV)

- `cfg, err := factory.FromEnv(ctx, "PG_")` then `factory.NewStore(cfg)`.
- Or fill `factory.Config{Type: factory.DbTypeSql, ...}` in code (the integration tests do this).

## Driving it with verify.sh

Preconditions:

- `verify.sh doctor` is clean. The `accounts` table exists.

- **Run the slice.** `verify.sh drive pg`. It runs `caller pg write`, checks the database, then `caller pg cleanup`.
- **Create and read.** `caller-pg.txt` has `OK   rows after create = 1`.
- **Update.** `OK   age after update = 2`.
- **Raw SQL.** `OK   ExecWithSql count = 1`.
- **Guards.** `OK   Update with an empty filter is refused`, `OK   Delete with an empty filter is refused`, `OK   a table name with SQL in it is refused`.
- **Outside proof.** `db-state.txt` has `pg marker rows ...: got '2|6|verify,verify-commit'`, and after cleanup `pg rows after cleanup: got '0' want '0'`.
- **Repo suite.** `verify.sh drive tests` also passes the five `TestStore_*_InPostgres` tests.

## Gotchas

- `FromEnv` needs `PG_HOST`, `PG_DB` and `PG_USER`. An empty user would make the driver use the operating-system user, so it is refused.
- Numeric and `pg_lsn` columns come back from `ExecWithSql` as `[]byte`: cast in SQL (`::text`, `::bigint`).
- The caller does send an empty-filter `Update` and `Delete` to the real `accounts` table and expects both to be refused. If that guard regresses, those calls rewrite and delete every row in the table. The stack is disposable and `init.sql` inserts no rows, so nothing seeded is lost, but never point the caller at a database you care about.
- `SslMode` is only `disable` or `require`. TLS is not verified here.
