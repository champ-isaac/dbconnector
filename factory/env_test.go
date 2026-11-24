package factory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lookup(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestFromEnv_Defaults(t *testing.T) {
	ctx := context.Background()
	cfg, err := fromLookup(ctx, "PG_", lookup(map[string]string{"PG_HOST": "h", "PG_DB": "d", "PG_USER": "u"}))
	require.NoError(t, err)
	assert.Equal(t, Config{
		Type: DbTypeSql, Ctx: ctx, Schema: "postgres", Host: "h", Port: 5432, Db: "d", Username: "u",
		MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: 30 * time.Minute, ConnMaxIdleTime: 5 * time.Minute,
	}, cfg)
}

func TestFromEnv_AllSet(t *testing.T) {
	cfg, err := fromLookup(context.Background(), "R1_", lookup(map[string]string{
		"R1_HOST": "h", "R1_PORT": "5433", "R1_DB": "d", "R1_USER": "u", "R1_PASSWORD": "p", "R1_SSLMODE": "require",
		"R1_MAX_OPEN_CONNS": "3", "R1_MAX_IDLE_CONNS": "2", "R1_CONN_MAX_LIFETIME": "1h", "R1_CONN_MAX_IDLE_TIME": "1m",
	}))
	require.NoError(t, err)
	assert.Equal(t, 5433, cfg.Port)
	assert.Equal(t, "u", cfg.Username)
	assert.Equal(t, "p", cfg.Password)
	assert.True(t, cfg.SslMode)
	assert.Equal(t, 3, cfg.MaxOpenConns)
	assert.Equal(t, 2, cfg.MaxIdleConns)
	assert.Equal(t, time.Hour, cfg.ConnMaxLifetime)
	assert.Equal(t, time.Minute, cfg.ConnMaxIdleTime)
}

func TestFromEnv_Errors(t *testing.T) {
	cfg, err := fromLookup(context.Background(), "PG_", lookup(map[string]string{
		"PG_PORT": "x", "PG_MAX_OPEN_CONNS": "-1", "PG_MAX_IDLE_CONNS": "x", "PG_CONN_MAX_LIFETIME": "soon", "PG_SSLMODE": "maybe", "PG_PASSWORD": "secret",
	}))
	require.Error(t, err)
	assert.Equal(t, Config{}, cfg, "no half-filled Config (it would hold the password) comes back with an error")
	for _, want := range []string{"PG_HOST is required", "PG_DB is required", "PG_USER is required", "PG_PORT must be a non-negative integer",
		"PG_MAX_OPEN_CONNS must be", "PG_CONN_MAX_LIFETIME must be a duration", "PG_SSLMODE must be"} {
		assert.ErrorContains(t, err, want)
	}
	assert.NotContains(t, err.Error(), "secret")

	_, err = fromLookup(context.Background(), "PG_", lookup(map[string]string{"PG_HOST": "h", "PG_DB": "d", "PG_USER": "u", "PG_PORT": "70000"}))
	assert.ErrorContains(t, err, "PG_PORT must be 1-65535")

	_, err = fromLookup(context.Background(), "PG_", lookup(map[string]string{"PG_HOST": "h", "PG_DB": "d", "PG_USER": "u", "PG_CONN_MAX_IDLE_TIME": "-1m"}))
	assert.ErrorContains(t, err, "PG_CONN_MAX_IDLE_TIME must be a duration")
}

func TestFromEnv_Process(t *testing.T) {
	t.Setenv("DBC_HOST", "h")
	t.Setenv("DBC_DB", "d")
	t.Setenv("DBC_USER", "u")
	cfg, err := FromEnv(context.Background(), "DBC_")
	require.NoError(t, err)
	assert.Equal(t, "h", cfg.Host)
}

func TestFromEnv_Pool(t *testing.T) {
	base := map[string]string{"PG_HOST": " h ", "PG_DB": "d", "PG_USER": "u"}
	with := func(kv ...string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	cfg, err := fromLookup(context.Background(), "PG_", lookup(with("PG_MAX_OPEN_CONNS", "3")))
	require.NoError(t, err)
	assert.Equal(t, "h", cfg.Host, "values are trimmed")
	assert.Equal(t, 3, cfg.MaxIdleConns, "default idle is capped by open")

	_, err = fromLookup(context.Background(), "PG_", lookup(with("PG_MAX_OPEN_CONNS", "0")))
	assert.ErrorContains(t, err, "PG_MAX_OPEN_CONNS must be at least 1")
	_, err = fromLookup(context.Background(), "PG_", lookup(with("PG_MAX_OPEN_CONNS", "2", "PG_MAX_IDLE_CONNS", "3")))
	assert.ErrorContains(t, err, "PG_MAX_IDLE_CONNS must not exceed PG_MAX_OPEN_CONNS")
}

func TestFromEnv_PasswordTrimmedEmptyIsUnset(t *testing.T) {
	cfg, err := fromLookup(context.Background(), "PG_", lookup(map[string]string{
		"PG_HOST": "h", "PG_DB": "d", "PG_USER": "u", "PG_PASSWORD": " p w\n", "PG_PORT": "  ", "PG_SSLMODE": "",
	}))
	require.NoError(t, err)
	assert.Equal(t, "p w", cfg.Password)
	assert.Equal(t, 5432, cfg.Port)
	assert.False(t, cfg.SslMode)
}

func TestFromEnv_ErrorsNeverEchoValues(t *testing.T) {
	_, err := fromLookup(context.Background(), "PG_", lookup(map[string]string{
		"PG_HOST": "h", "PG_DB": "d", "PG_USER": "u",
		"PG_PORT": "tok-port", "PG_SSLMODE": "tok-ssl", "PG_CONN_MAX_LIFETIME": "tok-life",
		"PG_MAX_OPEN_CONNS": "tok-open", "PG_MAX_IDLE_CONNS": "tok-idle",
	}))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "tok-")
}

// One process can hold several servers: each prefix reads only its own names.
func TestFromEnv_PrefixesAreIsolated(t *testing.T) {
	env := lookup(map[string]string{
		"PG_HOST": "primary", "PG_DB": "d", "PG_USER": "u", "PG_PORT": "5432",
		"PG_REPLICA_1_HOST": "replica", "PG_REPLICA_1_DB": "d", "PG_REPLICA_1_USER": "ru", "PG_REPLICA_1_PORT": "5433",
	})
	primary, err := fromLookup(context.Background(), "PG_", env)
	require.NoError(t, err)
	replica, err := fromLookup(context.Background(), "PG_REPLICA_1_", env)
	require.NoError(t, err)
	assert.Equal(t, []any{"primary", "u", 5432}, []any{primary.Host, primary.Username, primary.Port})
	assert.Equal(t, []any{"replica", "ru", 5433}, []any{replica.Host, replica.Username, replica.Port})
}

// The edges of each accepted range: one value inside, one just outside.
func TestFromEnv_Boundaries(t *testing.T) {
	cases := []struct {
		name, key, val, wantErr string // wantErr "" means the value is accepted
	}{
		{"port zero", "PG_PORT", "0", "PG_PORT must be 1-65535"},
		{"port max", "PG_PORT", "65535", ""},
		{"port over max", "PG_PORT", "65536", "PG_PORT must be 1-65535"},
		{"port negative", "PG_PORT", "-1", "PG_PORT must be a non-negative integer"},
		{"open negative", "PG_MAX_OPEN_CONNS", "-1", "PG_MAX_OPEN_CONNS must be a non-negative integer"},
		{"lifetime zero", "PG_CONN_MAX_LIFETIME", "0", ""},
		{"lifetime just below zero", "PG_CONN_MAX_LIFETIME", "-1ns", "PG_CONN_MAX_LIFETIME must be a duration like 30m"},
		{"idle just below zero", "PG_CONN_MAX_IDLE_TIME", "-1ns", "PG_CONN_MAX_IDLE_TIME must be a duration like 30m"},
		// The README says verify-ca and verify-full are not supported: they must fail, never run unverified.
		{"sslmode disable", "PG_SSLMODE", "disable", ""},
		{"sslmode require", "PG_SSLMODE", "require", ""},
		{"sslmode verify-ca", "PG_SSLMODE", "verify-ca", "PG_SSLMODE must be disable or require"},
		{"sslmode verify-full", "PG_SSLMODE", "verify-full", "PG_SSLMODE must be disable or require"},
		{"sslmode prefer", "PG_SSLMODE", "prefer", "PG_SSLMODE must be disable or require"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := fromLookup(context.Background(), "PG_", lookup(map[string]string{
				"PG_HOST": "h", "PG_DB": "d", "PG_USER": "u", c.key: c.val,
			}))
			if c.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, c.wantErr)
		})
	}
}
