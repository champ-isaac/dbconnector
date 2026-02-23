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
	Ctx             context.Context
	Conn            string
	Db              string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

type SqlConfig struct {
	Schema   string
	Db       string
	Username string
	Password string
	Host     string
	Port     int
	SslMode  bool
}

func NewStore(config Config) (abstract.Store, error) {
	switch config.Type {
	case DbTypeSql:
		driver, err := sql.NewDriver(config.Ctx, config.Conn, config.MaxOpenConns, config.MaxIdleConns, config.ConnMaxIdleTime, config.ConnMaxLifetime)
		if err != nil {
			return nil, err
		}
		return sql.NewStore(driver), nil
	case DbTypeMongo:
		driver, err := mongo.NewDriver(config.Ctx, config.Conn, config.MaxOpenConns, config.MaxIdleConns, config.ConnMaxIdleTime, config.ConnMaxLifetime)
		if err != nil {
			return nil, err
		}
		return mongo.NewStore(driver, config.Db), nil
	default:
		return nil, fmt.Errorf("unknown db type: %v", config.Type)
	}
}

// SqlConnStr fix the issue of password whose strange characters
func SqlConnStr(sqlConfig SqlConfig) string {
	u := &url.URL{
		Scheme: sqlConfig.Schema,
		User:   url.UserPassword(sqlConfig.Username, sqlConfig.Password),
		Host:   fmt.Sprintf("%s:%d", sqlConfig.Host, sqlConfig.Port),
		Path:   sqlConfig.Db,
	}
	q := u.Query()
	flag := "disable"
	if sqlConfig.SslMode {
		flag = "require"
	}
	q.Set("sslmode", flag)
	u.RawQuery = q.Encode()

	return u.String()
}
