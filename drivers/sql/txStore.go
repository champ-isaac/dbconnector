package sql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/champ-isaac/dbconnector/abstract"
)

type TxStore struct {
	ctx context.Context
	tx  *sql.Tx
}

var _ abstract.TxStore = (*TxStore)(nil)

func (t *TxStore) Create(collection string, data any) error {
	m, err := asMap(data, "data", "create")
	if err != nil {
		return err
	}
	query, args, err := buildInsertQuery(collection, m)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, query, args...)
	return err
}

func (t *TxStore) Read(collection string, filter any) (any, error) {
	m, err := asMap(filter, "filter", "read")
	if err != nil {
		return nil, err
	}
	query, args, err := buildSelectQuery(collection, m)
	if err != nil {
		return nil, err
	}
	return queryRows(t.ctx, t.tx, query, args...)
}

func (t *TxStore) Update(collection string, filter any, data any) error {
	m, err := asMap(data, "data", "update")
	if err != nil {
		return err
	}
	mFilter, err := asMap(filter, "filter", "update")
	if err != nil {
		return err
	}
	query, args, err := buildUpdateQuery(collection, mFilter, m)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, query, args...)
	return err
}

func (t *TxStore) Delete(collection string, filter any) error {
	mFilter, err := asMap(filter, "filter", "delete")
	if err != nil {
		return err
	}
	query, args, err := buildDeleteQuery(collection, mFilter)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, query, args...)
	return err
}

func (t *TxStore) ExecWithSql(sql string, args ...any) (any, error) {
	return execOrQuery(t.ctx, t.tx, sql, args...)
}

// Disconnect rolls back the transaction when it is still open.
func (t *TxStore) Disconnect() error {
	if err := t.tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return err
	}
	return nil
}

func (t *TxStore) Ping() error {
	return errors.New("not implemented")
}

func (t *TxStore) Commit() error {
	return t.tx.Commit()
}

func (t *TxStore) Rollback() error {
	return t.tx.Rollback()
}
