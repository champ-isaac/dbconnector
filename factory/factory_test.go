package factory

import (
	"context"
	"errors"
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

func TestSqlConnStr(t *testing.T) {
	cfg := baseConfig(DbTypeSql)
	assert.Equal(t, "postgres://user:user%21%40%23@127.0.0.1:1/data_privacy?sslmode=disable", sqlConnStr(cfg))
	cfg.SslMode = true
	assert.Equal(t, "postgres://user:user%21%40%23@127.0.0.1:1/data_privacy?sslmode=require", sqlConnStr(cfg))
}
