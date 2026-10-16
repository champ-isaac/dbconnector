package sql

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

// sqlOpen is a variable so tests can replace it, pq does not validate the DSN before connecting.
var sqlOpen = sql.Open

type Driver struct {
	Ctx context.Context
	Db  *sql.DB
}

func NewDriver(ctx context.Context, conn string, maxOpenConns, maxIdleConns int, connMaxIdleTime, connMaxLifeTime time.Duration) (*Driver, error) {
	db, err := sqlOpen("postgres", conn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifeTime)
	db.SetConnMaxIdleTime(connMaxIdleTime)
	return &Driver{Ctx: ctx, Db: db}, nil
}

func (d *Driver) Exec(query string, args ...any) (sql.Result, error) {
	return d.Db.Exec(query, args...)
}

func (d *Driver) Query(query string, args ...any) (*sql.Rows, error) {
	return d.Db.Query(query, args...)
}

func (d *Driver) Close() error {
	return d.Db.Close()
}
