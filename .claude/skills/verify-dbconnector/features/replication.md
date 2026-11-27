# Logical replication

`replication.Connect(ctx, cfg)` opens a raw `*pgconn.PgConn` with `replication=database`, ready for `pglogrepl`. It reuses the same `factory.Config` as the store.

## Sub-features

- `repl-connect` opens the connection and runs `IDENTIFY_SYSTEM`.
- `repl-slot` creates a (temporary) logical slot, which needs `wal_level=logical`.
- `repl-connstring` builds the URL with `replication=database` (unit tested only).

## How to get to it (user POV)

- `cfg, _ := factory.FromEnv(ctx, "PG_")`, `conn, _ := replication.Connect(ctx, cfg)`, `defer conn.Close(ctx)`, then `pglogrepl.IdentifySystem(ctx, conn)`.

## Driving it with verify.sh

Preconditions:

- `verify.sh doctor` reports `postgres schema + wal_level (accounts|logical)` and `postgres role can replicate (t)`.

- **Run the slice.** `verify.sh drive repl`.
- **Connect.** `caller-repl.txt` has `OK   replication.Connect` and `OK   IDENTIFY_SYSTEM`.
- **Slot.** `OK   create temporary logical slot (needs wal_level=logical)`, `OK   slot type = logical`, `OK   slot temporary = true`. The slot is read back through a normal store from `pg_replication_slots`.
- **Outside proof.** `db-state.txt` has `temporary slot gone after the connection closed: got '0' want '0'`.
- **Repo suite.** `verify.sh drive tests` passes `TestReplicationConnectInPostgres` and `TestReplicationSlotCreateAndDrop`.

## Gotchas

- With plain `docker-compose.yaml` the server has `wal_level=replica`: connect works, slot creation fails. `compose.verify.yaml` turns on `logical`, so a run here does not prove the slot path on the unmodified compose file.
- The replication tests skip silently when `PG_HOST` is unset. `verify.sh` sets it and requires `--- PASS:`.
- Streaming (`StartReplication`) is not part of this module, so it is not verified.
- Give `Connect` a `ctx` with a deadline: there is no other connect timeout.
