package sql

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/champ-isaac/dbconnector/abstract"
)

type Store struct {
	driver *Driver
}

var (
	invalidFormatError = "invalid %s format to exec %s, expecting map[string]any"
	queryFormat        = "exec the %s query: %s"
	clauseFormat       = "%s = $%d"
	whereFormat        = "Where %s"
	insertFormat       = `INSERT INTO %s (%s) VALUES (%s)`
	updateFormat       = `UPDATE %s SET %s `
	deleteFormat       = `DELETE FROM %s `
	selectFormat       = `SELECT * FROM %s `
)

func (s *Store) Create(collection string, data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf(invalidFormatError, "data", "create")
	}
	query, values := buildInsertQuery(collection, m)
	_, err := s.execSql(query, values...)
	return err
}

func (s *Store) Read(collection string, filter any) (any, error) {
	m, ok := filter.(map[string]any)
	if !ok {
		return nil, fmt.Errorf(invalidFormatError, "filter", "read")
	}
	query, values := buildSelectQuery(collection, m)
	result, err := s.querySql(query, values...)
	return result, err
}

func (s *Store) Update(collection string, filter any, data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf(invalidFormatError, "data", "update")
	}
	mFilter, ok := filter.(map[string]any)
	if !ok {
		return fmt.Errorf(invalidFormatError, "filter", "update")
	}
	query, values := buildUpdateQuery(collection, mFilter, m)
	_, err := s.execSql(query, values...)
	return err
}

func (s *Store) Delete(collection string, filter any) error {
	mFilter, ok := filter.(map[string]any)
	if !ok {
		return fmt.Errorf(invalidFormatError, "filter", "delete")
	}
	query, values := buildDeleteQuery(collection, mFilter)
	_, err := s.execSql(query, values...)
	return err
}

func (s *Store) ExecWithSql(sql string, args ...any) (any, error) {
	selectRegex := regexp.MustCompile("(?i)\bselect\b")
	fetchRegex := regexp.MustCompile("(?i)\bfetch\b")

	if selectRegex.MatchString(strings.ToLower(sql)) || fetchRegex.MatchString(strings.ToLower(sql)) {
		return s.querySql(sql, args...)
	}
	return s.execSql(sql, args...)
}

func (s *Store) Disconnect() error {
	return s.driver.Db.Close()
}

func (s *Store) Ping() error {
	return s.driver.Db.PingContext(s.driver.Ctx)
}

func NewStore(driver *Driver) abstract.Store {
	return &Store{driver: driver}
}

// >>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>
func (s *Store) querySql(sql string, args ...any) (any, error) {
	rows, err := s.driver.Db.QueryContext(s.driver.Ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
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
		for j, columnName := range columns {
			rowMap[columnName] = columnData[j]
		}
		results = append(results, rowMap)
	}
	return results, nil
}

func (s *Store) execSql(sql string, args ...any) (any, error) {
	return s.driver.Exec(sql, args...)
}

func buildInsertQuery(collection string, data map[string]any) (string, []any) {
	keys := ""
	placeHolders := ""
	var values []any
	i := 1
	for k, v := range data {
		if keys != "" {
			keys += ", "
			placeHolders += ", "
		}
		keys += k
		//using sql placeholder format; for mysql use "?"
		placeHolders += fmt.Sprintf("$%d", i)
		values = append(values, v)
		i++
	}
	query := fmt.Sprintf(insertFormat, collection, keys, placeHolders)
	log.Printf(queryFormat, "create", query)
	return query, values
}

func buildSelectQuery(collection string, filter map[string]any) (string, []any) {
	var whereClause []string
	var values []any
	i := 1
	for k, v := range filter {
		whereClause = append(whereClause, fmt.Sprintf(clauseFormat, k, i))
		values = append(values, v)
		i++
	}
	query := fmt.Sprintf(selectFormat, collection)
	if len(whereClause) > 0 {
		query += fmt.Sprintf(whereFormat, strings.Join(whereClause, " AND "))
	}
	log.Printf(queryFormat, "read", query)
	return query, values
}

func buildUpdateQuery(collection string, filter, data map[string]any) (string, []any) {
	var setClause []string
	var values []any
	i := 1
	for k, v := range data {
		setClause = append(setClause, fmt.Sprintf(clauseFormat, k, i))
		values = append(values, v)
		i++
	}
	var whereClause []string
	for k, v := range filter {
		whereClause = append(whereClause, fmt.Sprintf(clauseFormat, k, i))
		values = append(values, v)
		i++
	}
	query := fmt.Sprintf(updateFormat, collection, strings.Join(setClause, ", "))
	if len(whereClause) > 0 {
		query += fmt.Sprintf(whereFormat, strings.Join(whereClause, " AND "))
	}
	log.Printf(queryFormat, "update", query)
	return query, values
}

func buildDeleteQuery(collection string, filter map[string]any) (string, []any) {
	var whereClause []string
	var values []any
	i := 1
	for k, v := range filter {
		whereClause = append(whereClause, fmt.Sprintf(clauseFormat, k, i))
		values = append(values, v)
		i++
	}
	query := fmt.Sprintf(deleteFormat, collection)
	if len(whereClause) > 0 {
		query += fmt.Sprintf(whereFormat, strings.Join(whereClause, " AND "))
	}
	log.Printf(queryFormat, "delete", query)
	return query, values
}
