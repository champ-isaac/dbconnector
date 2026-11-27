# CQRS sides

`factory.NewQueryStore(cfg)` gives a store that can only read; `factory.NewCommandStore(cfg)` gives one that can only write and run transactions. Each has its own connection pool. The PostgreSQL query side also starts read-only.

## Sub-features

- `cqrs-query-no-writer` shows a query store is not an `abstract.Writer`.
- `cqrs-pg-readonly` shows a `factory.QueryConnString` connection refuses a plain write.
- `cqrs-transfer-then-query` moves money on the command side and reads it on the query side.

## How to get to it (user POV)

- `factory.NewQueryStore(readCfg)` and `factory.NewCommandStore(writeCfg)`.

## Driving it with verify.sh

Preconditions:

- `verify.sh doctor` is clean.

- **Type split.** `verify.sh drive pg` and `drive mongo`. Both caller files have `OK   query store is a Writer = false` and a successful query-store read.
- **Read-only connection.** `caller-pg.txt` has `OK   read-only connection refuses a write`. `db-state.txt` shows the `verify-ro` row never landed (names are only `verify,verify-commit`).
- **Transfer then query.** `verify.sh drive tests`. `tests.txt` has `--- PASS: TestCQRS_TransferThenQuery_InMongo` and `--- PASS: TestQueryConnString_RefusesWrites_InPostgres`.

## Gotchas

- The read-only switch is an accident guard, not security: `BEGIN READ WRITE` overrides it. Use a read-only role for enforcement.
- The command side has no `Read`. A command that must see its own write reads through its `TxStore`.
- A query store pointed at a replica can lag a commit; the compose stack has no replica, so lag is not verified.
