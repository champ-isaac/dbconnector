package abstract

import "context"

type Store interface {
	Create(collection string, data any) error
	Read(collection string, filter any) (any, error)
	Update(collection string, filter any, data any) error
	Delete(collection string, filter any) error

	ExecWithSql(sql string, args ...any) (any, error)

	Disconnect() error
	Ping() error
}

type TransactionManager interface {
	BeginTransaction(ctx context.Context) (TxStore, error)
}

type TxStore interface {
	Store
	Commit() error
	Rollback() error
}
