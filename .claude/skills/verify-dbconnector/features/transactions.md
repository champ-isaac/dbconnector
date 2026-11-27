# Transactions

Both drivers implement `abstract.TransactionManager`: `BeginTransaction(ctx)` returns an `abstract.TxStore` that can write, read its own writes, then `Commit` or `Rollback`. `tests/transfer.go` is the money-transfer example built on it.

## Sub-features

- `tx-commit` makes the writes visible after `Commit`.
- `tx-rollback` leaves no trace after `Rollback`.
- `tx-read-own-writes` lets `TxStore.Read` see uncommitted writes (PostgreSQL).
- `tx-transfer` runs `TransferBalanceInMongo` and `TransferBalanceInSql`, including rollback when the receiver is missing and parallel transfers that keep the total.

## How to get to it (user POV)

- `store.(abstract.TransactionManager).BeginTransaction(ctx)`, or `factory.NewCommandStore(cfg)` which has `BeginTransaction` directly.

## Driving it with verify.sh

Preconditions:

- `verify.sh doctor` is clean. Mongo must be a replica set (transactions need one).

- **PostgreSQL.** `verify.sh drive pg`. `caller-pg.txt` has `OK   tx sees its own write = 2`, `OK   rows after rollback = 1`, `OK   rows after commit = 2`. `psql` confirms `verify-rollback` never landed.
- **MongoDB.** `verify.sh drive mongo`. `OK   A after commit = 90`, `OK   B after commit = 60`, `OK   A after rollback = 90`. `mongosh` confirms the same numbers.
- **Transfers.** `verify.sh drive tests`. The `--- PASS:` lines in `tests.txt` include `TestTransferBalanceInMongo_InMongo`, `..._RoundTrip`, `..._RollsBackWhenReceiverIsMissing`, `..._RollsBackOnDuplicateId` and `..._ParallelTransfersConserveTotal`.

## Gotchas

- Call `tx.Disconnect()` after `Commit` or `Rollback`. It aborts a still-open transaction.
- `TransferBalanceInSql` uses a `balance` column that `docker/init.sql` does not create, so it is only unit-tested with sqlmock. There is no real-Postgres transfer journey. That is a gap, not a pass.
- On a Mongo `ErrMultipleMatch` the delta was already applied to every match inside the transaction: the caller must `Rollback`.
