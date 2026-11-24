package mongo

import (
	"context"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Driver struct {
	Ctx    context.Context
	Client *mongo.Client
}

type DriverConfig struct {
	Conn          string
	Db            string
	Username      string
	Password      string
	ReplicaMaster string
}

// maxPoolSize caps a pool size far above any real setting, so a bad value
// cannot turn into a huge uint64.
const maxPoolSize = math.MaxInt32

// poolSize converts a pool size to the uint64 the driver takes. A negative
// value becomes 0 (the driver's own default) and a value above maxPoolSize is
// capped, so the conversion below can never overflow.
func poolSize(n int) uint64 {
	if n < 0 {
		return 0
	}
	if n > maxPoolSize {
		return maxPoolSize
	}
	return uint64(n)
}

func NewDriver(ctx context.Context, cfg DriverConfig, maxOpenConns, maxIdleConns int, connMaxIdleTime, connMaxLifeTime time.Duration) (*Driver, error) {
	minIdleSize := 1
	tmpMinIdleSize := maxIdleConns / 10
	if tmpMinIdleSize > 1 {
		minIdleSize = tmpMinIdleSize
	}
	clientOption := options.Client().ApplyURI(cfg.Conn)
	clientOption.SetMaxConnecting(poolSize(maxOpenConns))
	clientOption.SetMaxPoolSize(poolSize(maxIdleConns))
	clientOption.SetMinPoolSize(uint64(minIdleSize))
	clientOption.SetMaxConnIdleTime(connMaxIdleTime)
	clientOption.SetConnectTimeout(connMaxLifeTime)
	if cfg.ReplicaMaster != "" {
		clientOption.SetReplicaSet(cfg.ReplicaMaster)
	} else {
		clientOption.SetDirect(true)
	}
	clientOption.SetAuth(options.Credential{Username: cfg.Username, Password: cfg.Password, AuthSource: cfg.Db})
	client, err := mongo.Connect(clientOption)
	if err != nil {
		return nil, err
	}
	return &Driver{Ctx: ctx, Client: client}, nil
}

func (d *Driver) Disconnect() error {
	return d.Client.Disconnect(d.Ctx)
}
