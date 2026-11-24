package replication

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/champ-isaac/dbconnector/factory"
)

func cfg() factory.Config {
	return factory.Config{Type: factory.DbTypeSql, Schema: "postgres", Db: "d", Username: "u", Password: "p!@#", Host: "h", Port: 5432}
}

func TestConnString(t *testing.T) {
	s, err := ConnString(cfg())
	require.NoError(t, err)
	assert.Equal(t, "postgres://u:p%21%40%23@h:5432/d?replication=database&sslmode=disable", s)
}

// A replication stream carries every change, so TLS must not be dropped on the way.
func TestConnString_KeepsSslMode(t *testing.T) {
	c := cfg()
	c.SslMode = true
	s, err := ConnString(c)
	require.NoError(t, err)
	assert.Equal(t, "postgres://u:p%21%40%23@h:5432/d?replication=database&sslmode=require", s)
}

func TestConnString_NotPostgres(t *testing.T) {
	for _, typ := range []factory.DbType{factory.DbTypeMongo, factory.DbType(99)} { // an unknown type is refused too
		c := cfg()
		c.Type = typ
		_, err := ConnString(c)
		assert.ErrorContains(t, err, "only PostgreSQL", typ)
	}
}

func TestConnect(t *testing.T) {
	old := connect
	defer func() { connect = old }()
	var got string
	want := errors.New("boom")
	connect = func(_ context.Context, s string) (*pgconn.PgConn, error) { got = s; return nil, want }

	_, err := Connect(context.Background(), cfg())
	assert.Equal(t, want, err)
	wantConn, cerr := ConnString(cfg())
	require.NoError(t, cerr)
	assert.Equal(t, wantConn, got, "Connect passes the ConnString to pgconn unchanged")

	c := cfg()
	c.Type = factory.DbTypeMongo
	_, err = Connect(context.Background(), c)
	assert.ErrorContains(t, err, "only PostgreSQL")
}

// The real pgconn must not echo the password, in a parse error or a dial error.
func TestConnect_ErrorHidesPassword(t *testing.T) {
	for _, host := range []string{"bad host", "127.0.0.1"} { // unparsable, then connection refused on port 1
		c := cfg()
		c.Host, c.Port, c.Password = host, 1, "S3CRET!@#"
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := Connect(ctx, c)
		cancel()
		require.Error(t, err, host)
		for _, leak := range []string{"S3CRET", "S3CRET%21%40%23"} {
			assert.NotContains(t, err.Error(), leak, host)
		}
	}
}

// Connect uses the ctx it is given, not cfg.Ctx (the README says so).
func TestConnect_UsesItsOwnCtx(t *testing.T) {
	old := connect
	defer func() { connect = old }()
	type key struct{}
	var got context.Context
	connect = func(ctx context.Context, _ string) (*pgconn.PgConn, error) { got = ctx; return nil, errors.New("stop") }

	own := context.WithValue(context.Background(), key{}, "own")
	c := cfg()
	c.Ctx = context.WithValue(context.Background(), key{}, "cfg")
	_, _ = Connect(own, c)
	require.NotNil(t, got)
	assert.Equal(t, "own", got.Value(key{}))
}
