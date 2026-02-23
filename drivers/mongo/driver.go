package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Driver struct {
	Ctx    context.Context
	Client *mongo.Client
}

func NewDriver(ctx context.Context, conn string, maxOpenConns, maxIdleConns int, connMaxIdleTime, connMaxLifeTime time.Duration) (*Driver, error) {
	minIdleSize := 1
	tmpMinIdleSize := maxIdleConns / 10
	if tmpMinIdleSize > 1 {
		minIdleSize = tmpMinIdleSize
	}
	clientOption := options.Client().ApplyURI(conn)
	clientOption.SetMaxConnecting(uint64(maxOpenConns))
	clientOption.SetMaxPoolSize(uint64(maxIdleConns))
	clientOption.SetMinPoolSize(uint64(minIdleSize))
	clientOption.SetMaxConnIdleTime(connMaxIdleTime)
	clientOption.SetConnectTimeout(connMaxLifeTime)
	client, err := mongo.Connect(clientOption)
	if err != nil {
		return nil, err
	}
	return &Driver{Ctx: ctx, Client: client}, nil
}

func (d *Driver) Disconnect() error {
	return d.Client.Disconnect(d.Ctx)
}
