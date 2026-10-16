package sql

import (
	"context"

	"github.com/champ-isaac/dbconnector/abstract"
)

type Store struct {
	driver *Driver
}

var _ abstract.Store = (*Store)(nil)

func NewStore(driver *Driver) abstract.Store {
	return &Store{driver: driver}
}

func (s *Store) Create(collection string, data any) error {
	m, err := asMap(data, "data", "create")
	if err != nil {
		return err
	}
	query, values, err := buildInsertQuery(collection, m)
	if err != nil {
		return err
	}
	_, err = s.exec(query, values...)
	return err
}

func (s *Store) Read(collection string, filter any) (any, error) {
	m, err := asMap(filter, "filter", "read")
	if err != nil {
		return nil, err
	}
	query, values, err := buildSelectQuery(collection, m)
	if err != nil {
		return nil, err
	}
	return queryRows(s.driver.Ctx, s.driver.Db, query, values...)
}

func (s *Store) Update(collection string, filter any, data any) error {
	m, err := asMap(data, "data", "update")
	if err != nil {
		return err
	}
	mFilter, err := asMap(filter, "filter", "update")
	if err != nil {
		return err
	}
	query, values, err := buildUpdateQuery(collection, mFilter, m)
	if err != nil {
		return err
	}
	_, err = s.exec(query, values...)
	return err
}

func (s *Store) Delete(collection string, filter any) error {
	mFilter, err := asMap(filter, "filter", "delete")
	if err != nil {
		return err
	}
	query, values, err := buildDeleteQuery(collection, mFilter)
	if err != nil {
		return err
	}
	_, err = s.exec(query, values...)
	return err
}

// ExecWithSql runs raw sql: statements that return rows (select, fetch, returning) give back
// []map[string]any as []any, other statements give back the sql.Result.
func (s *Store) ExecWithSql(sql string, args ...any) (any, error) {
	return execOrQuery(s.driver.Ctx, s.driver.Db, sql, args...)
}

func (s *Store) Disconnect() error {
	return s.driver.Close()
}

func (s *Store) Ping() error {
	return s.driver.Db.PingContext(s.driver.Ctx)
}

func (s *Store) exec(query string, args ...any) (any, error) {
	return s.driver.Db.ExecContext(s.driver.Ctx, query, args...)
}

var _ abstract.TransactionManager = (*Store)(nil)

func (s *Store) BeginTransaction(ctx context.Context) (abstract.TxStore, error) {
	tx, err := s.driver.Db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &TxStore{ctx: ctx, tx: tx}, nil
}
