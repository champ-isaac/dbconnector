package abstract

// The two sides of a store (CQRS). A Store has both. Code that only reads takes a
// Reader or QueryStore, code that only changes data takes a Writer or CommandStore,
// so the compiler keeps the two apart and each side can use its own connection.

// Reader is the query side: it returns data and changes nothing.
type Reader interface {
	Read(collection string, filter any) (any, error)
	Ping() error
}

// Writer is the command side: it changes data.
type Writer interface {
	Create(collection string, data any) error
	Update(collection string, filter any, data any) error
	Delete(collection string, filter any) error
}

// QueryStore is a connection that can only query. It has no Writer methods, and a
// type assertion to Writer fails.
type QueryStore interface {
	Reader
	Disconnect() error
}

// CommandStore is a connection that can only issue commands: single writes and
// transactions. It has no Read. A command that must see its own writes reads through
// the TxStore of its transaction.
type CommandStore interface {
	Writer
	TransactionManager
	Ping() error
	Disconnect() error
}
