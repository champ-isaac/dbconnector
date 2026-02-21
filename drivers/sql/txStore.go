package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/champ-isaac/dbconnector/abstract"
)

type TxStore struct {
	tx *sql.Tx
}

var _ abstract.TransactionManager = (*Store)(nil)
var _ abstract.TxStore = (*TxStore)(nil)

func (t *TxStore) Create(collection string, data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf(invalidFormatError, "data", "create")
	}
	query, args := buildInsertQuery(collection, m)
	_, err := t.execSql(query, args...)
	return err
}

func (t *TxStore) Read(collection string, filter any) (any, error) {
	m, ok := filter.(map[string]any)
	if !ok {
		return nil, fmt.Errorf(invalidFormatError, "filter", "read")
	}
	query, args := buildSelectQuery(collection, m)
	return t.querySql(query, args...)
}

func (t *TxStore) Update(collection string, filter any, data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf(invalidFormatError, "data", "update")
	}
	mFilter, ok := filter.(map[string]any)
	if !ok {
		return fmt.Errorf(invalidFormatError, "filter", "update")
	}
	query, args := buildUpdateQuery(collection, mFilter, m)
	_, err := t.execSql(query, args...)
	return err
}

func (t *TxStore) Delete(collection string, filter any) error {
	mFilter, ok := filter.(map[string]any)
	if !ok {
		return fmt.Errorf(invalidFormatError, "filter", "delete")
	}
	query, args := buildDeleteQuery(collection, mFilter)
	_, err := t.execSql(query, args...)
	return err
}

func (t *TxStore) ExecWithSql(sql string, args ...any) (any, error) {
	selectRegex := regexp.MustCompile("(?i)\bselect\b")
	fetchRegex := regexp.MustCompile("(?i)\bfetch\b")
	if selectRegex.MatchString(strings.ToLower(sql)) || fetchRegex.MatchString(strings.ToLower(sql)) {
		results, err := t.querySql(sql, args...)
		return results, err
	}
	_, err := t.execSql(sql, args...)
	return nil, err
}

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

func (s *Store) BeginTransaction(ctx context.Context) (abstract.TxStore, error) {
	tx, err := s.driver.Db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &TxStore{tx: tx}, nil
}

// >>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>
func (t *TxStore) execSql(sql string, args ...any) (any, error) {
	_, err := t.tx.Exec(sql, args...)
	return nil, err
}

func (t *TxStore) querySql(sql string, args ...any) (any, error) {
	rows, err := t.tx.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = rows.Close(); err != nil {
			log.Printf("error when rows closing: %v", err)
		}
	}()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var results []any
	for rows.Next() {
		columnData := make([]any, len(columns))
		columnPointer := make([]any, len(columns))
		for j := range columnData {
			columnPointer[j] = &columnData[j]
		}
		if err = rows.Scan(columnPointer...); err != nil {
			return nil, err
		}
		rowMap := make(map[string]any)
		for j, colName := range columns {
			rowMap[colName] = columnPointer[j]
		}
		results = append(results, rowMap)
	}
	return results, nil
}
