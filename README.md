# Description
This package integrate features of postgres and mongo databases. Features include CRUD and transaction.
~~This package also support load balancing which can connect to database cluster.~~

## Note
Please check examples in `/tests` to see how to use dbconnector.

## Postgres behaviour
- Table and column names must be plain names (`accounts` or `public.accounts`), otherwise `ErrInvalidIdentifier` is returned. Values always go through `$n` placeholders.
- `Update` and `Delete` refuse an empty filter (`ErrEmptyFilter`) so a bug cannot touch every row. Use `ExecWithSql` on purpose for that.
- `ExecWithSql` returns rows (`[]any` of `map[string]any`) for statements with `select`, `fetch` or `returning`, otherwise the `sql.Result`.

## Tests
```
go test -race -cover ./factory ./drivers/sql   # unit tests on go-sqlmock, 100% coverage (Mongo driver is not part of the target)
docker compose up -d postgres && go test -tags integration -run InPostgres ./tests   # real Postgres
```

## Architecture
![dbconnector architecture](Privacy%20API%20Service%20Diagram.png)

## Concerns
- Handle large volume size of data write into database
  - for query, create the cache in front of the database server? what the TTL for each cache? 
  - build database replicas(master and slave), separate read and write operations
- collaborate with cache service (not implement)