# Transactions

Both drivers implement `abstract.TransactionManager`: `BeginTransaction(ctx)` returns an `abstract.TxStore` that can write, read its own writes, then `Commit` or `Rollback`. `tests/transfer.go` is the money-transfer example built on it.

## Sub-features

- `tx-commit` makes the writes visible after `Commit`.
- `tx-rollback` leaves no trace after `Rollback`.
- `tx-read-own-writes` lets `TxStore.Read` see uncommitted writes (PostgreSQL).
- `tx-transfer` runs `TransferBalanceInMongo` (rollback when the receiver is missing, parallel transfers that keep the total) and `TransferBalanceInSql` on real PostgreSQL (a successful transfer, rollback on an invalid UUID, a rejected overdraft).

## How to get to it (user POV)

- `store.(abstract.TransactionManager).BeginTransaction(ctx)`, or `factory.NewCommandStore(cfg)` which has `BeginTransaction` directly.

## Driving it with verify.sh

Preconditions:

- `verify.sh doctor` is clean. Mongo must be a replica set (transactions need one).

- **PostgreSQL.** `verify.sh drive pg`. `caller-pg.txt` has `OK   tx sees its own write = 2`, `OK   rows after rollback = 1`, `OK   rows after commit = 2`. `psql` confirms `verify-rollback` never landed.
- **MongoDB.** `verify.sh drive mongo`. `OK   A after commit = 90`, `OK   B after commit = 60`, `OK   A after rollback = 90`. `mongosh` confirms the same numbers.
- **Transfers.** `verify.sh drive tests`. The `--- PASS:` lines in `tests.txt` include `TestTransferBalanceInMongo_InMongo`, `..._RoundTrip`, `..._RollsBackWhenReceiverIsMissing`, `..._RollsBackOnDuplicateId` and `..._ParallelTransfersConserveTotal`. For PostgreSQL they include `TestTransferBalanceInSql_InPostgres` (200.50 and 100.25 become 150.25 and 150.50 after moving 50.25), `..._RollsBackOnInvalidUuid` (the receiver id is `not-a-uuid`, so the second update fails and both balances stay 200.50 and 100.25), `..._RejectsUnknownReceiver` (a valid UUID that matches no account: `ErrAccountNotFound`, the sender keeps 200.50), `..._RejectsUnknownSender` (the same for the sender: `ErrAccountNotFound`, the receiver keeps 100.25) and `..._RejectsOverdraft` (20.00 cannot send 50.25: `violates check constraint`, nothing moves).

## Gotchas

- Call `tx.Disconnect()` after `Commit` or `Rollback`. It aborts a still-open transaction.
- `TransferBalanceInSql` updates `accounts.balance`, which `docker/init.sql` does not create. The `TestTransferBalanceInSql_InPostgres*` tests add the column (with `CHECK (balance >= 0)`) themselves and drop it when they end, so `init.sql` is unchanged and the tests also run in the CI `integration-postgres` job. A crashed run can leave the column behind; the next run reuses it (`ADD COLUMN IF NOT EXISTS`).
- `TransferBalanceInSql` checks how many rows each update matched. A sender or receiver id that matches no row (for example a valid UUID with no account) rolls the transaction back and returns `ErrAccountNotFound`, as `TransferBalanceInMongo` does with `ErrNoMatch`. `TestTransferBalanceInSql_InPostgres_RejectsUnknownReceiver` (the sender keeps 200.50) and `..._RejectsUnknownSender` (the receiver keeps 100.25) cover both cases on real PostgreSQL. The check applies only when the store returns a `sql.Result`, so the mock tests, whose result is `nil`, skip it.
- On a Mongo `ErrMultipleMatch` the delta was already applied to every match inside the transaction: the caller must `Rollback`.
