package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
)

// ErrInvalidIdentifier is returned when a table or column name is not a plain (optionally schema qualified) name.
var ErrInvalidIdentifier = errors.New("invalid identifier")

// ErrEmptyFilter is returned when update or delete would touch every row.
var ErrEmptyFilter = errors.New("empty filter is not allowed")

var (
	invalidFormatError = "invalid %s format to exec %s, expecting map[string]any"
	identRe            = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	queryRe            = regexp.MustCompile(`(?i)\b(select|fetch|returning)\b`)
)

// execQueryer is what *sql.DB and *sql.Tx have in common.
type execQueryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func asMap(v any, what, op string) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf(invalidFormatError, what, op)
	}
	return m, nil
}

// execOrQuery runs sql with QueryContext when it returns rows, otherwise with ExecContext.
func execOrQuery(ctx context.Context, q execQueryer, query string, args ...any) (any, error) {
	if queryRe.MatchString(query) {
		return queryRows(ctx, q, query, args...)
	}
	return q.ExecContext(ctx, query, args...)
}

// rowScanner is the part of *sql.Rows that scanRows needs.
type rowScanner interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// queryRows returns every row as a map of column name to value.
func queryRows(ctx context.Context, q execQueryer, query string, args ...any) (any, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }() // cleanup for early returns; a close error at the end shows up in rows.Err()
	return scanRows(rows)
}

func scanRows(rows rowScanner) (any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var results []any
	// Allocated once: each row's values are copied into its own map below, and Scan overwrites every cell.
	columnData := make([]any, len(columns))
	columnPointer := make([]any, len(columns))
	for j := range columnData {
		columnPointer[j] = &columnData[j]
	}
	for rows.Next() {
		if err = rows.Scan(columnPointer...); err != nil {
			return nil, err
		}
		rowMap := make(map[string]any, len(columns))
		for j, columnName := range columns {
			rowMap[columnName] = columnData[j]
		}
		results = append(results, rowMap)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func validIdent(names ...string) error {
	for _, n := range names {
		if !identRe.MatchString(n) {
			return fmt.Errorf("%w: %q", ErrInvalidIdentifier, n)
		}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys) // stable SQL text, so the same call always gives the same statement
	return keys
}

// assignments builds "a = $1, b = $2" for the sorted keys of m, numbering placeholders from start.
func assignments(m map[string]any, start int, sep string) (string, []any) {
	keys := sortedKeys(m)
	parts := make([]string, len(keys))
	values := make([]any, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s = $%d", k, start+i)
		values[i] = m[k]
	}
	return strings.Join(parts, sep), values
}

func buildInsertQuery(collection string, data map[string]any) (string, []any, error) {
	if len(data) == 0 {
		return "", nil, errors.New("no data to insert")
	}
	keys := sortedKeys(data)
	if err := validIdent(append([]string{collection}, keys...)...); err != nil {
		return "", nil, err
	}
	placeHolders := make([]string, len(keys))
	values := make([]any, len(keys))
	for i, k := range keys {
		placeHolders[i] = fmt.Sprintf("$%d", i+1)
		values[i] = data[k]
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", collection, strings.Join(keys, ", "), strings.Join(placeHolders, ", "))
	slog.Debug("sql query", "op", "create", "query", query)
	return query, values, nil
}

func buildSelectQuery(collection string, filter map[string]any) (string, []any, error) {
	if err := validIdent(append([]string{collection}, sortedKeys(filter)...)...); err != nil {
		return "", nil, err
	}
	query := fmt.Sprintf("SELECT * FROM %s", collection)
	var values []any
	if len(filter) > 0 {
		var where string
		where, values = assignments(filter, 1, " AND ")
		query += " WHERE " + where
	}
	slog.Debug("sql query", "op", "read", "query", query)
	return query, values, nil
}

func buildUpdateQuery(collection string, filter, data map[string]any) (string, []any, error) {
	if len(data) == 0 {
		return "", nil, errors.New("no data to update")
	}
	if len(filter) == 0 {
		return "", nil, ErrEmptyFilter
	}
	if err := validIdent(append(append([]string{collection}, sortedKeys(data)...), sortedKeys(filter)...)...); err != nil {
		return "", nil, err
	}
	set, values := assignments(data, 1, ", ")
	where, whereValues := assignments(filter, len(values)+1, " AND ")
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", collection, set, where)
	slog.Debug("sql query", "op", "update", "query", query)
	return query, append(values, whereValues...), nil
}

func buildDeleteQuery(collection string, filter map[string]any) (string, []any, error) {
	if len(filter) == 0 {
		return "", nil, ErrEmptyFilter
	}
	if err := validIdent(append([]string{collection}, sortedKeys(filter)...)...); err != nil {
		return "", nil, err
	}
	where, values := assignments(filter, 1, " AND ")
	query := fmt.Sprintf("DELETE FROM %s WHERE %s", collection, where)
	slog.Debug("sql query", "op", "delete", "query", query)
	return query, values, nil
}
