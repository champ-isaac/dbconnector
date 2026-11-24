package factory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// FromEnv reads a PostgreSQL Config from environment variables. Every name is the
// prefix plus the suffix below, so one process can hold several servers
// (FromEnv(ctx, "PG_") and FromEnv(ctx, "PG_REPLICA_1_")).
//
//	HOST (required), PORT (5432), DB (required), USER (required), PASSWORD,
//	SSLMODE (disable | require, default disable),
//	MAX_OPEN_CONNS (10, at least 1), MAX_IDLE_CONNS (5, at most MAX_OPEN_CONNS), CONN_MAX_LIFETIME (30m), CONN_MAX_IDLE_TIME (5m)
//
// Values are trimmed of white space (PASSWORD too) and an empty value counts as
// unset. The durations use time.ParseDuration syntax. Errors name the variable and
// never include its value, because PASSWORD is one of them.
func FromEnv(ctx context.Context, prefix string) (Config, error) {
	return fromLookup(ctx, prefix, os.Getenv)
}

func fromLookup(ctx context.Context, prefix string, getenv func(string) string) (Config, error) {
	var errs []error
	get := func(name, def string) string {
		if v := strings.TrimSpace(getenv(prefix + name)); v != "" {
			return v
		}
		return def
	}
	num := func(name string, def int) int {
		v := get(name, "")
		if v == "" {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			errs = append(errs, fmt.Errorf("%s%s must be a non-negative integer", prefix, name))
			return def
		}
		return n
	}
	dur := func(name string, def time.Duration) time.Duration {
		v := get(name, "")
		if v == "" {
			return def
		}
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			errs = append(errs, fmt.Errorf("%s%s must be a duration like 30m", prefix, name))
			return def
		}
		return d
	}

	maxOpen := num("MAX_OPEN_CONNS", 10)
	if maxOpen < 1 { // database/sql treats 0 as unlimited
		errs = append(errs, fmt.Errorf("%sMAX_OPEN_CONNS must be at least 1", prefix))
	}
	maxIdle := num("MAX_IDLE_CONNS", min(5, maxOpen))
	if maxIdle > maxOpen {
		errs = append(errs, fmt.Errorf("%sMAX_IDLE_CONNS must not exceed %sMAX_OPEN_CONNS", prefix, prefix))
	}

	cfg := Config{
		Type:            DbTypeSql,
		Ctx:             ctx,
		Schema:          "postgres",
		Host:            get("HOST", ""),
		Port:            num("PORT", 5432),
		Db:              get("DB", ""),
		Username:        get("USER", ""),
		Password:        get("PASSWORD", ""),
		MaxOpenConns:    maxOpen,
		MaxIdleConns:    maxIdle,
		ConnMaxLifetime: dur("CONN_MAX_LIFETIME", 30*time.Minute),
		ConnMaxIdleTime: dur("CONN_MAX_IDLE_TIME", 5*time.Minute),
	}
	if cfg.Host == "" {
		errs = append(errs, errors.New(prefix+"HOST is required"))
	}
	if cfg.Db == "" {
		errs = append(errs, errors.New(prefix+"DB is required"))
	}
	// An empty user makes the driver fall back to the operating-system user.
	if cfg.Username == "" {
		errs = append(errs, errors.New(prefix+"USER is required"))
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		errs = append(errs, fmt.Errorf("%sPORT must be 1-65535", prefix))
	}
	switch get("SSLMODE", "disable") {
	case "disable":
	case "require":
		cfg.SslMode = true
	default:
		errs = append(errs, fmt.Errorf("%sSSLMODE must be disable or require", prefix))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
