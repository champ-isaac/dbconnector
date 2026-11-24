package factory

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/champ-isaac/dbconnector/drivers/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseConfig(t DbType) Config {
	return Config{
		Type: t, Ctx: context.Background(), Schema: "postgres", Db: "data_privacy",
		Username: "user", Password: "user!@#", Host: "127.0.0.1", Port: 1,
		MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxIdleTime: time.Minute, ConnMaxLifetime: time.Hour,
	}
}

func TestNewStore_Sql(t *testing.T) {
	// the driver connects lazily, so no server is needed to build the store
	store, err := NewStore(baseConfig(DbTypeSql))
	require.NoError(t, err)
	assert.NotNil(t, store)
	assert.NoError(t, store.Disconnect())
}

func TestNewStore_SqlDriverError(t *testing.T) {
	old := newSqlDriver
	defer func() { newSqlDriver = old }()
	newSqlDriver = func(context.Context, string, int, int, time.Duration, time.Duration) (*sql.Driver, error) {
		return nil, errors.New("boom")
	}
	_, err := NewStore(baseConfig(DbTypeSql))
	assert.EqualError(t, err, "boom")
}

func TestNewStore_Mongo(t *testing.T) {
	cfg := baseConfig(DbTypeMongo)
	cfg.Schema = "mongodb"
	store, err := NewStore(cfg)
	require.NoError(t, err)
	assert.NotNil(t, store)
	assert.NoError(t, store.Disconnect())

	cfg.ReplicaMaster = "rs0"
	store, err = NewStore(cfg)
	require.NoError(t, err)
	assert.NoError(t, store.Disconnect())

	cfg.Schema = "not-a-mongo-scheme"
	_, err = NewStore(cfg)
	assert.Error(t, err)
}

func TestNewStore_UnknownType(t *testing.T) {
	_, err := NewStore(baseConfig(DbType(99)))
	assert.ErrorContains(t, err, "unknown db type")
}

func TestConnString(t *testing.T) {
	cfg := baseConfig(DbTypeSql)
	assert.Equal(t, "postgres://user:user%21%40%23@127.0.0.1:1/data_privacy?sslmode=disable", ConnString(cfg))
	cfg.SslMode = true
	assert.Equal(t, "postgres://user:user%21%40%23@127.0.0.1:1/data_privacy?sslmode=require", ConnString(cfg))
}

func TestConnString_IPv6(t *testing.T) {
	cfg := baseConfig(DbTypeSql)
	want := "postgres://user:user%21%40%23@[::1]:1/data_privacy?sslmode=disable"
	for _, host := range []string{"::1", "[::1]"} {
		cfg.Host = host
		assert.Equal(t, want, ConnString(cfg))
	}
}

// User and database name are escaped too, so they cannot add URL parts.
func TestConnString_EscapesUserAndDb(t *testing.T) {
	cfg := baseConfig(DbTypeSql)
	cfg.Username, cfg.Db = "a@b:c/d", "my db?x=1#f"
	u, err := url.Parse(ConnString(cfg))
	require.NoError(t, err)
	pw, _ := u.User.Password()
	assert.Equal(t, "a@b:c/d", u.User.Username())
	assert.Equal(t, cfg.Password, pw)
	assert.Equal(t, "/my db?x=1#f", u.Path)
	assert.Equal(t, "sslmode=disable", u.RawQuery, "a db name must not add query parameters")
	assert.Empty(t, u.Fragment)
}

// NewStore must hand the driver the config's own connection string and pool settings, in order.
func TestNewStore_SqlDriverArguments(t *testing.T) {
	old := newSqlDriver
	defer func() { newSqlDriver = old }()
	type args struct {
		ctx                   context.Context
		dsn                   string
		open, idle            int
		idleTime, maxLifetime time.Duration
	}
	var got args
	newSqlDriver = func(ctx context.Context, dsn string, open, idle int, idleTime, lifetime time.Duration) (*sql.Driver, error) {
		got = args{ctx, dsn, open, idle, idleTime, lifetime}
		return nil, errors.New("stop")
	}
	type key struct{}
	cfg := baseConfig(DbTypeSql)
	cfg.Ctx = context.WithValue(context.Background(), key{}, "v")
	cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxIdleTime, cfg.ConnMaxLifetime = 7, 3, 2*time.Minute, 9*time.Minute

	_, _ = NewStore(cfg)
	assert.Equal(t, args{cfg.Ctx, ConnString(cfg), 7, 3, 2 * time.Minute, 9 * time.Minute}, got)
}

// Both PostgreSQL scheme spellings are valid; the URL keeps the one in the config.
func TestConnString_KeepsSchema(t *testing.T) {
	cfg := baseConfig(DbTypeSql)
	cfg.Schema = "postgresql"
	assert.Equal(t, "postgresql://user:user%21%40%23@127.0.0.1:1/data_privacy?sslmode=disable", ConnString(cfg))
}
