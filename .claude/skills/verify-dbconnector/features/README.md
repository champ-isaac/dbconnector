# dbconnector verification map

The maintained source for verifying the user-facing behavior of dbconnector. A "user" here is a Go program that imports the module. Read this index, then the matching feature file.

## Baseline preconditions

- Docker (Colima here) is running and ports `127.0.0.1:5432` and `127.0.0.1:27017` are free or already ours.
- Run `.claude/skills/verify-dbconnector/verify.sh up`, then `verify.sh doctor`. Doctor must print no `BAD`.
- The stack is project `dbconnector-verify`: postgres 18.2 with `wal_level=logical`, mongodb 8.0.12 as a one-node replica set `rs0`, both seeded from `docker/init.sql` and `docker/init.js`. Database `data_privacy`, user `user`, password `user!@#`.
- Never drive a stack this run did not start. Two stacks cannot coexist (the tests hardcode the ports).

## Driving conventions

- Every recipe is a `verify.sh drive <slice>` call. It runs the outside caller (`caller/main.go`) and then checks the stored state with `psql` / `mongosh`. Run `verify.sh drive tests` for the repo's integration suite.
- Rows and documents made by a run carry the run id (`VERIFY_RUN`) in `last_name` (postgres) or `verify_run` (mongo), so runs never touch the seed data.
- Do not mock the databases here. Unit tests already cover that.
- A mutation proof needs a second view from outside the library. The slices do this for you.

## Proof and skip reporting

- Evidence goes to `evidence/<run id>/`. Quote the `summary.txt` lines you rely on.
- A feature that cannot run (for example a port is taken) is reported with the command tried and the unmet precondition. Do not call it verified through a different path.
- Integration tests skip silently when `PG_HOST` is unset. A skip is a failure for this skill.

## Feature entry contract

Each feature file has an H1, one paragraph, and exactly four H2s in order: `Sub-features`, `How to get to it (user POV)`, `Driving it with verify.sh`, `Gotchas`.

## Features

- [PostgreSQL store](./postgres-store.md) covers config from the environment, CRUD, raw SQL and the guard rails of the SQL driver.
- [MongoDB store](./mongo-store.md) covers CRUD, `$set` updates and `mongo.Inc`.
- [Transactions](./transactions.md) covers commit, rollback, a transaction seeing its own writes, and the transfer examples, on both databases.
- [CQRS sides](./cqrs-sides.md) covers `NewQueryStore` and `NewCommandStore`.
- [Logical replication](./replication.md) covers `replication.Connect` and creating a logical slot.
