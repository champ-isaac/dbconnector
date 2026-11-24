// Package replication opens PostgreSQL logical replication connections.
// database/sql and lib/pq cannot do this: the replication protocol needs the
// replication=database startup parameter and a raw connection. Connect gives
// that raw connection; run the protocol on it with github.com/jackc/pglogrepl
// (IdentifySystem, CreateReplicationSlot, StartReplication, ...).
package replication

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/champ-isaac/dbconnector/factory"
)

// connect is a variable so tests can replace it, pgconn needs a server.
var connect = pgconn.Connect

// ConnString is factory.ConnString plus replication=database. It contains the
// password: do not log it.
func ConnString(cfg factory.Config) (string, error) {
	if cfg.Type != factory.DbTypeSql {
		return "", errors.New("replication: only PostgreSQL configs are supported")
	}
	u := factory.ConnURL(cfg)
	q := u.Query()
	q.Set("replication", "database")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Connect opens one logical replication connection. The caller owns it and
// must Close it. cfg.MaxOpenConns and the other pool settings do not apply.
func Connect(ctx context.Context, cfg factory.Config) (*pgconn.PgConn, error) {
	conn, err := ConnString(cfg)
	if err != nil {
		return nil, err
	}
	return connect(ctx, conn)
}
