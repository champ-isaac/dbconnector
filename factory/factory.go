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

func NewStore(config Config) (abstract.Store, error) {
	switch config.Type {
	case DbTypeSql:
		driver, err := sql.NewDriver(config.Ctx, sqlConnStr(config), config.MaxOpenConns, config.MaxIdleConns, config.ConnMaxIdleTime, config.ConnMaxLifetime)
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

// SqlConnStr fix the issue of password whose strange characters
func sqlConnStr(cfg Config) string {
	u := &url.URL{
		Scheme: cfg.Schema,
		User:   url.UserPassword(cfg.Username, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   cfg.Db,
	}
	q := u.Query()
	flag := "disable"
	if cfg.SslMode {
		flag = "require"
	}
	q.Set("sslmode", flag)
	u.RawQuery = q.Encode()

	return u.String()
}
