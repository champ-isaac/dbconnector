package abstract

import "context"

// Store has both sides (see Reader and Writer) and raw SQL, which is a query and a
// command at once and so belongs to neither side.
type Store interface {
	Reader
	Writer

	ExecWithSql(sql string, args ...any) (any, error)

	Disconnect() error
}

type TransactionManager interface {
	BeginTransaction(ctx context.Context) (TxStore, error)
}

type TxStore interface {
	Store
	Commit() error
	Rollback() error
}
