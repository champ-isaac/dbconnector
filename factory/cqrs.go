package factory

import (
	"context"
	"errors"

	"github.com/champ-isaac/dbconnector/abstract"
)

// NewQueryStore opens the query side. It takes its own Config, so reads can go to a
// replica or use a read-only role. The result has no write methods. On PostgreSQL the
// connection also starts read-only, which stops accidental writes but is not a security
// boundary (a session can ask for BEGIN READ WRITE). For enforcement give the query side a
// read-only role or a standby. MongoDB has no such switch: use a read-only user or a
// secondary in its Config.
func NewQueryStore(config Config) (abstract.QueryStore, error) {
	s, err := newStore(config, true)
	if err != nil {
		return nil, err
	}
	return queryOnly{s}, nil
}

// NewCommandStore opens the command side: writes and transactions, no reads. Use the
// primary.
func NewCommandStore(config Config) (abstract.CommandStore, error) {
	s, err := newStore(config, false)
	if err != nil {
		return nil, err
	}
	return newCommandOnly(s)
}

// queryOnly shows only the Reader methods of a store.
type queryOnly struct{ s abstract.Store }

func (q queryOnly) Read(collection string, filter any) (any, error) {
	return q.s.Read(collection, filter)
}
func (q queryOnly) Ping() error       { return q.s.Ping() }
func (q queryOnly) Disconnect() error { return q.s.Disconnect() }

// commandOnly shows only the Writer methods and transactions of a store.
type commandOnly struct {
	s  abstract.Store
	tm abstract.TransactionManager
}

func newCommandOnly(s abstract.Store) (abstract.CommandStore, error) {
	tm, ok := s.(abstract.TransactionManager)
	if !ok {
		return nil, errors.New("factory: this store has no transactions, so it cannot be a command store")
	}
	return commandOnly{s, tm}, nil
}

func (c commandOnly) Create(collection string, data any) error { return c.s.Create(collection, data) }
func (c commandOnly) Update(collection string, filter, data any) error {
	return c.s.Update(collection, filter, data)
}
func (c commandOnly) Delete(collection string, filter any) error {
	return c.s.Delete(collection, filter)
}
func (c commandOnly) BeginTransaction(ctx context.Context) (abstract.TxStore, error) {
	return c.tm.BeginTransaction(ctx)
}
func (c commandOnly) Ping() error       { return c.s.Ping() }
func (c commandOnly) Disconnect() error { return c.s.Disconnect() }
