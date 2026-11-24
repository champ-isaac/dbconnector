package factory

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/drivers/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spyStore is a Store with transactions. It records every call and fails with err.
type spyStore struct {
	calls []string
	args  []any
	err   error
	rows  any
}

func (s *spyStore) rec(name string, args ...any) error {
	s.calls = append(s.calls, name)
	s.args = append(s.args, args...)
	return s.err
}
func (s *spyStore) Create(c string, d any) error    { return s.rec("Create", c, d) }
func (s *spyStore) Update(c string, f, d any) error { return s.rec("Update", c, f, d) }
func (s *spyStore) Delete(c string, f any) error    { return s.rec("Delete", c, f) }
func (s *spyStore) Ping() error                     { return s.rec("Ping") }
func (s *spyStore) Disconnect() error               { return s.rec("Disconnect") }
func (s *spyStore) ExecWithSql(string, ...any) (any, error) {
	return nil, s.rec("ExecWithSql")
}
func (s *spyStore) Read(c string, f any) (any, error) { return s.rows, s.rec("Read", c, f) }
func (s *spyStore) BeginTransaction(context.Context) (abstract.TxStore, error) {
	return nil, s.rec("BeginTransaction")
}

// noTxStore is a Store that is not a TransactionManager.
type noTxStore struct{ spyStore }

func (noTxStore) BeginTransaction() {} // a different signature hides the spy's method

var (
	_ abstract.Store              = (*spyStore)(nil)
	_ abstract.TransactionManager = (*spyStore)(nil)
	_ abstract.Store              = (*noTxStore)(nil)
)

func mongoConfig() Config {
	cfg := baseConfig(DbTypeMongo)
	cfg.Schema = "mongodb"
	return cfg
}

// Neither side offers the other's methods: a type assertion cannot get them back.
func TestQueryStore_HasNoWriteMethods(t *testing.T) {
	q, err := NewQueryStore(mongoConfig())
	require.NoError(t, err)
	defer func() { assert.NoError(t, q.Disconnect()) }()

	var v any = q
	_, isReader := v.(abstract.Reader)
	_, isWriter := v.(abstract.Writer)
	_, isTx := v.(abstract.TransactionManager)
	_, isStore := v.(abstract.Store)
	assert.True(t, isReader)
	assert.False(t, isWriter, "the query side cannot write")
	assert.False(t, isTx, "the query side has no transactions")
	assert.False(t, isStore)
}

func TestCommandStore_HasNoQueryMethods(t *testing.T) {
	c, err := NewCommandStore(mongoConfig())
	require.NoError(t, err)
	defer func() { assert.NoError(t, c.Disconnect()) }()

	var v any = c
	_, isWriter := v.(abstract.Writer)
	_, isTx := v.(abstract.TransactionManager)
	_, isReader := v.(abstract.Reader)
	_, isStore := v.(abstract.Store)
	assert.True(t, isWriter)
	assert.True(t, isTx)
	assert.False(t, isReader, "the command side cannot read outside a transaction")
	assert.False(t, isStore)
}

func TestNewQueryAndCommandStore_ReplicaSetAndBadScheme(t *testing.T) {
	cfg := mongoConfig()
	cfg.ReplicaMaster = "rs0"
	q, err := NewQueryStore(cfg)
	require.NoError(t, err)
	assert.NoError(t, q.Disconnect())
	c, err := NewCommandStore(cfg)
	require.NoError(t, err)
	assert.NoError(t, c.Disconnect())

	cfg.Schema = "not-a-mongo-scheme"
	_, err = NewQueryStore(cfg)
	assert.Error(t, err)
	_, err = NewCommandStore(cfg)
	assert.Error(t, err)
}

func TestNewQueryAndCommandStore_UnknownType(t *testing.T) {
	_, err := NewQueryStore(baseConfig(DbType(99)))
	assert.ErrorContains(t, err, "unknown db type")
	_, err = NewCommandStore(baseConfig(DbType(99)))
	assert.ErrorContains(t, err, "unknown db type")
}

// stubSqlDriver records the connection string and fails, so no PostgreSQL is needed.
func stubSqlDriver(t *testing.T, got *string, err error) {
	t.Helper()
	old := newSqlDriver
	t.Cleanup(func() { newSqlDriver = old })
	newSqlDriver = func(_ context.Context, dsn string, _, _ int, _, _ time.Duration) (*sql.Driver, error) {
		*got = dsn
		return nil, err
	}
}

// Only the query side asks PostgreSQL to refuse writes. Everything else in the
// connection string is the same, and NewStore is unchanged.
func TestSql_OnlyTheQuerySideIsReadOnly(t *testing.T) {
	boom := errors.New("stop")
	for name, withSsl := range map[string]bool{"ssl off": false, "ssl on": true} {
		t.Run(name, func(t *testing.T) {
			cfg := baseConfig(DbTypeSql)
			cfg.SslMode = withSsl
			var dsn string
			stubSqlDriver(t, &dsn, boom)

			_, err := NewStore(cfg)
			assert.ErrorIs(t, err, boom)
			plain := dsn
			_, err = NewCommandStore(cfg)
			assert.ErrorIs(t, err, boom)
			assert.Equal(t, plain, dsn, "the command side connects as NewStore does")
			_, err = NewQueryStore(cfg)
			assert.ErrorIs(t, err, boom)

			assert.Equal(t, QueryConnString(cfg), dsn)
			plainURL, err := url.Parse(plain)
			require.NoError(t, err)
			queryURL, err := url.Parse(dsn)
			require.NoError(t, err)
			assert.Empty(t, plainURL.Query().Get("default_transaction_read_only"))
			assert.Equal(t, "on", queryURL.Query().Get("default_transaction_read_only"))
			assert.Equal(t, plainURL.Query().Get("sslmode"), queryURL.Query().Get("sslmode"))
			assert.Equal(t, plainURL.User, queryURL.User)
			assert.Equal(t, plainURL.Host, queryURL.Host)
			assert.Equal(t, plainURL.Path, queryURL.Path)
		})
	}
}

func TestQueryConnString_KeepsTheRest(t *testing.T) {
	cfg := baseConfig(DbTypeSql)
	cfg.Host, cfg.Username, cfg.Password, cfg.Db = "::1", "us er", "p@ss/w:rd?", "my db"
	u, err := url.Parse(QueryConnString(cfg))
	require.NoError(t, err)
	pw, _ := u.User.Password()
	assert.Equal(t, "us er", u.User.Username())
	assert.Equal(t, "p@ss/w:rd?", pw)
	assert.Equal(t, "[::1]:1", u.Host)
	assert.Equal(t, "my db", u.Path[1:])
	assert.Equal(t, "on", u.Query().Get("default_transaction_read_only"))
	assert.Equal(t, "disable", u.Query().Get("sslmode"))
}

func TestSql_DriverErrorsPassThroughBothSides(t *testing.T) {
	boom := errors.New("no server")
	var dsn string
	stubSqlDriver(t, &dsn, boom)
	_, err := NewQueryStore(baseConfig(DbTypeSql))
	assert.ErrorIs(t, err, boom)
	_, err = NewCommandStore(baseConfig(DbTypeSql))
	assert.ErrorIs(t, err, boom)
}

// The wrappers pass arguments and results straight through, and touch nothing else.
func TestQueryOnly_Delegates(t *testing.T) {
	rows := []any{map[string]any{"id": 1}}
	spy := &spyStore{rows: rows}
	q := queryOnly{spy}

	got, err := q.Read("accounts", map[string]any{"id": 1})
	assert.NoError(t, err)
	assert.Equal(t, rows, got)
	assert.NoError(t, q.Ping())
	assert.NoError(t, q.Disconnect())
	assert.Equal(t, []string{"Read", "Ping", "Disconnect"}, spy.calls)
	assert.Equal(t, []any{"accounts", map[string]any{"id": 1}}, spy.args)

	boom := errors.New("boom")
	spy.err = boom
	_, err = q.Read("accounts", nil)
	assert.ErrorIs(t, err, boom)
	assert.ErrorIs(t, q.Ping(), boom)
	assert.ErrorIs(t, q.Disconnect(), boom)
}

func TestCommandOnly_Delegates(t *testing.T) {
	spy := &spyStore{}
	c, err := newCommandOnly(spy)
	require.NoError(t, err)
	data, filter := map[string]any{"a": 1}, map[string]any{"id": 2}

	assert.NoError(t, c.Create("accounts", data))
	assert.NoError(t, c.Update("accounts", filter, data))
	assert.NoError(t, c.Delete("accounts", filter))
	_, err = c.BeginTransaction(context.Background())
	assert.NoError(t, err)
	assert.NoError(t, c.Ping())
	assert.NoError(t, c.Disconnect())
	assert.Equal(t, []string{"Create", "Update", "Delete", "BeginTransaction", "Ping", "Disconnect"}, spy.calls)
	assert.Equal(t, []any{"accounts", data, "accounts", filter, data, "accounts", filter}, spy.args)

	boom := errors.New("boom")
	spy.err = boom
	assert.ErrorIs(t, c.Create("a", nil), boom)
	assert.ErrorIs(t, c.Update("a", nil, nil), boom)
	assert.ErrorIs(t, c.Delete("a", nil), boom)
	_, err = c.BeginTransaction(context.Background())
	assert.ErrorIs(t, err, boom)
	assert.ErrorIs(t, c.Ping(), boom)
	assert.ErrorIs(t, c.Disconnect(), boom)
}

// A store without transactions cannot be a command store.
func TestNewCommandOnly_NeedsTransactions(t *testing.T) {
	c, err := newCommandOnly(&noTxStore{})
	assert.ErrorContains(t, err, "no transactions")
	assert.Nil(t, c)
}
