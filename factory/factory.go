package factory

import (
	"context"
	"fmt"
	"net/url"
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
	ctx             context.Context
	conn            string
	db              string
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
	connMaxIdleTime time.Duration
}

type SqlConfig struct {
	schema   string
	db       string
	username string
	password string
	host     string
	port     int
	sslMode  bool
}

func NewStore(config Config) (abstract.Store, error) {
	switch config.Type {
	case DbTypeSql:
		driver, err := sql.NewDriver(config.ctx, config.conn, config.maxOpenConns, config.maxIdleConns, config.connMaxIdleTime, config.connMaxLifetime)
		if err != nil {
			return nil, err
		}
		return sql.NewStore(driver), nil
	case DbTypeMongo:
		driver, err := mongo.NewDriver(config.ctx, config.conn, config.maxOpenConns, config.maxIdleConns, config.connMaxIdleTime, config.connMaxLifetime)
		if err != nil {
			return nil, err
		}
		return mongo.NewStore(driver, config.db), nil
	default:
		return nil, fmt.Errorf("unknown db type: %v", config.Type)
	}
}

// SqlConnStr fix the issue of password whose strange characters
func SqlConnStr(sqlConfig SqlConfig) string {
	u := &url.URL{
		Scheme: sqlConfig.schema,
		User:   url.UserPassword(sqlConfig.username, sqlConfig.password),
		Host:   fmt.Sprintf("%s:%d", sqlConfig.host, sqlConfig.port),
		Path:   sqlConfig.db,
	}
	q := u.Query()
	flag := "disable"
	if sqlConfig.sslMode {
		flag = "require"
	}
	q.Set("sslmode", flag)
	u.RawQuery = q.Encode()

	return u.String()
}
