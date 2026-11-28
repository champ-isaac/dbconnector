---
name: verify-dbconnector
description: Verify the dbconnector Go module (github.com/champ-isaac/dbconnector) works against real PostgreSQL and MongoDB. Use after changing factory/, drivers/sql, drivers/mongo, replication/ or docker/ to prove behaviour end to end. It starts the docker-compose databases, drives the public API like an outside caller, runs the repo's integration tests, and saves evidence. This is a library, not an app: there is no UI or server to launch.
---

# Verify dbconnector

The "user" of this repo is a Go program that imports `factory` and gets an `abstract.Store`. So verification means:
real databases up, a small outside caller program using only the public API, the DB state checked from outside the library
(`psql`, `mongosh`), and the repo's own integration suite. Everything runs through one script:

```
.claude/skills/verify-dbconnector/verify.sh all          # up, doctor, drive everything, down. ~30 s with the images pulled, minutes on a first pull
```

Run it from anywhere; it `cd`s to the repo root. Exit 0 means everything passed. Pieces, when you want less:

| Command | Does |
|---|---|
| `verify.sh up` | start postgres + mongodb, wait until usable |
| `verify.sh doctor` | read-only health check; run it first if anything looks off |
| `verify.sh drive [unit\|pg\|mongo\|repl\|tests\|all]` | run one slice or all (default all) |
| `verify.sh down` | remove our containers, network, volumes |

`KEEP=1 verify.sh all` skips the teardown. `VERIFY_RUN=<id>` picks the evidence folder name.

## Real commands (from README.md and .github/workflows/ci.yml; no CLAUDE.md in the repo)

- Go 1.26, module vendored locally (`vendor/` is gitignored, `go build` uses it when present).
- Unit: `go test -race -cover ./factory ./drivers/sql ./replication ./drivers/mongo ./tests` (Mongo unit tests use an in-process fake server).
- Integration: `go test -race -count=1 -tags integration -run 'InPostgres|InMongo|^TestReplicationSlotCreateAndDrop$' -v ./tests`.
  Replication tests read `PG_HOST PG_PORT PG_DB PG_USER PG_PASSWORD` and **skip** (still green!) when `PG_HOST` is unset. `verify.sh` sets them and requires a `--- PASS:` line per test.
- Everything else the tests need is hardcoded: `127.0.0.1:5432` / `:27017`, user `user`, password `user!@#`, database `data_privacy`.

## Launch

`verify.sh up` runs `docker compose -p dbconnector-verify -f docker-compose.yaml -f .claude/skills/verify-dbconnector/compose.verify.yaml up -d postgres mongodb`.
The override (`compose.verify.yaml`) exists because of what the real compose file and the machine do:

- Fresh state: its own project name means fresh named volumes, so `docker/init.sql` and `docker/init.js` always run. The user's old `postgres` / `mongodb` containers and `dbconnector_*` volumes are never touched.
- Ports are bound to `127.0.0.1` (the compose file exposes them on all interfaces with a weak password).
- Postgres starts with `-c wal_level=logical` so the slot test can run. The plain compose file leaves `wal_level=replica`, so the slot test would fail there.
- The mongodb entrypoint copies `docker/mongo-keyfile` to `/tmp/mongo-keyfile` inside the container, sets mode `400` and owner `mongodb`, then starts mongod with `--keyFile /tmp/mongo-keyfile`. Git stores the file as mode 644, and on Linux mongod refuses a keyfile that others can read, so the bind-mounted original fails there (Docker Desktop and Colima mounts hide this).
- `mongo-ui` is not started (not needed).

Ready means usable, not just answering: postgres has the `accounts` table (init.sql ran) over TCP and `wal_level=logical`; mongodb is a writable primary (the compose healthcheck runs `rs.initiate`), the `init.js` seed exists, and the `data_privacy` user exists. The script waits up to ~3 min for each.
The images are `linux/amd64`; on an arm64 host (Colima here) they run under emulation, so first start is slow and a "platform mismatch" warning is normal.

Isolation: **cannot run two stacks.** The tests hardcode the ports. `up` refuses, and says why, if 5432 or 27017 is held by anything that is not our containers. Never kill the other owner; ask the user.

## Doctor

`verify.sh doctor` is read-only and prints ok/BAD per check: docker up, ports owned by us, schema + `wal_level=logical`, role may replicate, mongo primary, mongo seed, `go build ./...`, and no leftover `verify*` rows in postgres `accounts` from a crashed earlier run (mongo leftovers are not checked). BAD on leftovers means a previous run died between write and cleanup. A new `drive` cannot remove them, because each run deletes only rows tagged with its own run id. Run `verify.sh down` then `up` for a clean slate.

## Drive

`caller/main.go` is a standalone program (under `.claude/`, so `./...` skips it and repo coverage is untouched). It configures from the environment like a real consumer (`factory.FromEnv(ctx, "PG_")`) and uses only the public API. `verify.sh drive` runs it as `go run ./.claude/skills/verify-dbconnector/caller <pg|mongo|repl> <write|cleanup>` with `VERIFY_RUN` as the marker value.

| Slice | What it proves | Outside check (the library is not trusted to grade itself) |
|---|---|---|
| `unit` | unit tests with `-race`, `go vet -tags integration ./...` | exit codes |
| `pg` | FromEnv, NewStore, Create/Read/Update, ExecWithSql, tx rollback + commit, NewQueryStore has no Writer, QueryConnString refuses a write | `psql`: exactly 2 marker rows, `sum(age)=6`, names `verify,verify-commit` (so the rolled-back and read-only writes did not land); after cleanup 0 rows |
| `mongo` | NewStore, Create/Read/`$set` update, `mongo.Inc` refused outside a tx, tx commit/rollback, `ErrNoMatch`, NewQueryStore has no Writer | `mongosh`: A=90, B=60, note `updated`, 2 docs; after cleanup 0 |
| `repl` | `replication.Connect`, IDENTIFY_SYSTEM, temporary logical slot | `psql`: no `dbconnector_verify_%` slot left after the connection closes |
| `tests` | the repo's integration suite against the same servers | a `PASS` line for every listed test (skips fail the run) |

Per-feature detail and gotchas: [features/README.md](features/README.md).

## Evidence

Written to `.claude/skills/verify-dbconnector/evidence/<run id>/` (gitignored by `evidence/.gitignore`; run id is printed, format `v<YYYYMMDD-HHMMSS>`):
`summary.txt` (PASS/FAIL lines), `caller-{pg,mongo,repl}.txt` (every step as `OK`/`FAIL`), `db-state.txt` (got vs want from psql/mongosh), `unit.txt`, `tests.txt` + `tests-want.txt`, `up.log`, `doctor.txt` (with `all`).

Proof standards:
- Exercise the real path: `factory` to a store to a real server. No sqlmock, no fake mongo server (those are the unit tests' job).
- Check side effects from outside with `psql` / `mongosh`, not only what the library returns.
- Rollback and read-only cases must show the data did NOT change.
- Never weaken a check to make it pass. Fix the cause, or record it in the feature's Gotchas.

## Cleanup

`verify.sh down` runs `docker compose -p dbconnector-verify ... down -v --remove-orphans`: only the `dbconnector-verify` project, so nothing else is touched (never `docker rm` by name pattern, never `pkill`). `all` does this on exit, including after a failure. The `evidence/` folder is never removed by `down`. After a failed iteration run `verify.sh down` before trying again.
Test rows are also removed by each slice's own cleanup phase, so a `KEEP=1` stack stays clean.

## In CI

The `verify` job in `.github/workflows/ci.yml` runs `KEEP=1 .claude/skills/verify-dbconnector/verify.sh all` on `ubuntu-latest`, so the container logs survive a failure. Its last steps print them on failure, upload `evidence/` as the `verify-evidence` artifact, and run `verify.sh down`. The job needs this folder committed (including `verify.sh`'s executable bit). It relies on the keyfile entrypoint in `compose.verify.yaml` and has no keyfile step of its own.

## Not covered / not verified

- The keyfile entrypoint is now proven on `ubuntu-latest`: CI run 46068470642 passed the `verify` job with it. The Colima mount does not reproduce the Linux mode-644 refusal, so only the CI job exercises that path.

- Only the `postgres:18.2` and `mongo:8.0.12` images of `docker-compose.yaml`, with the `wal_level=logical` change above. `docker/postgres.conf` and `pg_hba.conf` are used as is.
- No TLS (`SslMode` is `disable`/`require` only, and `require` is not verified against a real certificate).
- MongoDB with `ReplicaMaster` set (replica-set discovery) is not exercised: the compose replica set advertises host `mongodb:27017`, which the host cannot resolve, so everything uses a direct connection.
- Streaming changes (`StartReplication`) is not implemented in this module, only `Connect`.
