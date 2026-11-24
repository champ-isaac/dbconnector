package factory

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/drivers/mongo"
	"github.com/champ-isaac/dbconnector/drivers/sql"
)

type DbType = int

const (
	DbTypeSql DbType = iota
	DbTypeMongo
)

type Config struct {
	Type            DbType
	Ctx             context.Context
	Schema          string
	Db              string
	Username        string
	Password        string
	Host            string
	Port            int
	SslMode         bool   //for postgres
	ReplicaMaster   string //for mongo
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// newSqlDriver is a variable so tests can replace it, pq never fails before connecting.
var newSqlDriver = sql.NewDriver

func NewStore(config Config) (abstract.Store, error) { return newStore(config, false) }

// newStore builds the store. readOnly is for the query side: a PostgreSQL connection then
// refuses writes (see QueryConnString).
func newStore(config Config, readOnly bool) (abstract.Store, error) {
	switch config.Type {
	case DbTypeSql:
		dsn := ConnString(config)
		if readOnly {
			dsn = QueryConnString(config)
		}
		driver, err := newSqlDriver(config.Ctx, dsn, config.MaxOpenConns, config.MaxIdleConns, config.ConnMaxIdleTime, config.ConnMaxLifetime)
		if err != nil {
			return nil, err
		}
		return sql.NewStore(driver), nil
	case DbTypeMongo:
		drvCfg := mongo.DriverConfig{
			Conn:          fmt.Sprintf("%s://%s:%d", config.Schema, config.Host, config.Port),
			Db:            config.Db,
			Username:      config.Username,
			Password:      config.Password,
			ReplicaMaster: config.ReplicaMaster,
		}
		driver, err := mongo.NewDriver(config.Ctx, drvCfg, config.MaxOpenConns, config.MaxIdleConns, config.ConnMaxIdleTime, config.ConnMaxLifetime)
		if err != nil {
			return nil, err
		}
		return mongo.NewStore(driver, config.Db), nil
	default:
		return nil, fmt.Errorf("unknown db type: %v", config.Type)
	}
}

// ConnURL is the connection URL for a PostgreSQL Config. The password is
// percent-encoded, so strange characters in it are safe. It contains the
// password: do not log it.
func ConnURL(cfg Config) *url.URL {
	u := &url.URL{
		Scheme: cfg.Schema,
		User:   url.UserPassword(cfg.Username, cfg.Password),
		Host:   net.JoinHostPort(strings.Trim(cfg.Host, "[]"), strconv.Itoa(cfg.Port)), // brackets an IPv6 host
		Path:   cfg.Db,
	}
	q := u.Query()
	flag := "disable"
	if cfg.SslMode {
		flag = "require"
	}
	q.Set("sslmode", flag)
	u.RawQuery = q.Encode()
	return u
}

// ConnString is ConnURL as text. It contains the password: do not log it.
func ConnString(cfg Config) string { return ConnURL(cfg).String() }

// QueryConnString is ConnString for a connection that only queries: it starts read-only
// (default_transaction_read_only=on), so PostgreSQL refuses a plain write. That stops
// accidents only: a session can still ask for BEGIN READ WRITE. Use a read-only role or a
// standby to enforce it. It contains the password: do not log it.
func QueryConnString(cfg Config) string {
	u := ConnURL(cfg)
	q := u.Query()
	q.Set("default_transaction_read_only", "on")
	u.RawQuery = q.Encode()
	return u.String()
}
