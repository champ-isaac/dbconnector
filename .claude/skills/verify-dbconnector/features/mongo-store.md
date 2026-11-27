# MongoDB store

A caller fills `factory.Config{Type: factory.DbTypeMongo, ...}` (there is no env loader for Mongo), calls `factory.NewStore`, and gets an `abstract.Store` on the mongo-driver. Documents are plain maps; reads come back as `[]bson.M`.

## Sub-features

- `mongo-crud` runs `Create`, `Read`, `Update` (`$set`) and `Delete`.
- `mongo-inc` changes numbers by a delta with `mongo.Inc` (inside a transaction only).
- `mongo-inc-guards` refuses `Inc` outside a transaction (`ErrIncNeedsTransaction`) and reports `ErrNoMatch` for a missing document.

## How to get to it (user POV)

- `factory.NewStore(factory.Config{Type: factory.DbTypeMongo, Schema: "mongodb", Host: "127.0.0.1", Port: 27017, Db: "data_privacy", Username: "user", Password: "user!@#", ...})`.

## Driving it with verify.sh

Preconditions:

- `verify.sh doctor` is clean: mongo is a writable primary and the `init.js` seed exists.

- **Run the slice.** `verify.sh drive mongo`.
- **Create, read, update.** `caller-mongo.txt` has `OK   A balance = 100` and `OK   update $set`.
- **Inc guards.** `OK   Inc outside a transaction is refused` and `OK   Inc on a missing doc is ErrNoMatch`.
- **Outside proof.** `db-state.txt` has `mongo marker docs (A|B|note|count): got '90|60|updated|2'`, and after cleanup `got '0' want '0'`.
- **Repo suite.** `verify.sh drive tests` also passes the five `TestStore_*_InMongo` tests.

## Gotchas

- `ExecWithSql` is not implemented for Mongo and panics on purpose. Do not call it.
- `Update` does not check how many documents matched.
- The caller drives only `ErrIncNeedsTransaction` and `ErrNoMatch`. `ErrEmptyInc` and `ErrNonFiniteInc` are covered by unit tests only, and `ErrMultipleMatch` only by `TestTransferBalanceInMongo_InMongo_RollsBackOnDuplicateId` in the repo suite.
- The caller uses a direct connection (`ReplicaMaster` empty). The compose replica set advertises the host name `mongodb:27017`, which the host cannot resolve, so replica-set discovery is not verified.
- The query side has no server-side read-only enforcement on Mongo, so only the `QueryStore` type keeps it from writing.
