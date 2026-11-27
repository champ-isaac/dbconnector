// Command caller drives dbconnector the way an outside program would: only the public API
// (factory, abstract, drivers/*, replication), configured from the environment.
//
//	VERIFY_RUN=<id> PG_HOST=127.0.0.1 PG_DB=data_privacy PG_USER=user PG_PASSWORD='user!@#' \
//	  go run ./.claude/skills/verify-dbconnector/caller <pg|mongo|repl> <write|cleanup>
//
// "write" leaves marker rows/documents tagged with VERIFY_RUN so verify.sh can check them
// from outside the library (psql / mongosh). "cleanup" deletes them. repl has no cleanup
// phase (its slot is temporary). Exit 1 if any step fails. It lives under .claude/, which
// `./...` skips, so it never counts toward the repo's coverage.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"time"

	"github.com/jackc/pglogrepl"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/champ-isaac/dbconnector/abstract"
	"github.com/champ-isaac/dbconnector/drivers/mongo"
	sqldrv "github.com/champ-isaac/dbconnector/drivers/sql"
	"github.com/champ-isaac/dbconnector/factory"
	"github.com/champ-isaac/dbconnector/replication"
)

var failed bool

func step(name string, err error) {
	if err != nil {
		failed = true
		fmt.Printf("FAIL %s: %v\n", name, err)
		return
	}
	fmt.Printf("OK   %s\n", name)
}

// str prints a column value; text-like columns can arrive as []byte.
func str(v any) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}

func expect(name string, got, want any) {
	if fmt.Sprint(got) != fmt.Sprint(want) {
		step(name, fmt.Errorf("got %v, want %v", got, want))
		return
	}
	step(name+" = "+fmt.Sprint(want), nil)
}

// rows turns the result of Read into maps, for both drivers ([]any of map, or []bson.M).
func rows(res any, err error) []map[string]any {
	step("read", err)
	var out []map[string]any
	if rv := reflect.ValueOf(res); rv.Kind() == reflect.Slice {
		for i := 0; i < rv.Len(); i++ {
			switch m := rv.Index(i).Interface().(type) {
			case map[string]any:
				out = append(out, m)
			case bson.M:
				out = append(out, m)
			}
		}
	}
	return out
}

func main() {
	run := os.Getenv("VERIFY_RUN")
	if len(os.Args) != 3 || run == "" {
		fmt.Println("usage: VERIFY_RUN=<id> caller <pg|mongo|repl> <write|cleanup>")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	switch os.Args[1] {
	case "pg":
		pg(ctx, run, os.Args[2])
	case "mongo":
		mg(ctx, run, os.Args[2])
	case "repl":
		repl(ctx, run)
	default:
		fmt.Println("unknown backend", os.Args[1])
		os.Exit(2)
	}
	if failed {
		os.Exit(1)
	}
}

func pg(ctx context.Context, run, phase string) {
	cfg, err := factory.FromEnv(ctx, "PG_")
	step("factory.FromEnv(PG_)", err)
	if err != nil {
		return
	}
	store, err := factory.NewStore(cfg)
	step("factory.NewStore(sql)", err)
	if err != nil {
		return
	}
	defer func() { _ = store.Disconnect() }()
	flt := map[string]any{"last_name": run}
	count := func() int { return len(rows(store.Read("accounts", flt))) }

	if phase == "cleanup" {
		step("delete", store.Delete("accounts", flt))
		expect("rows left", count(), 0)
		return
	}

	step("ping", store.Ping())
	step("create", store.Create("accounts", map[string]any{"first_name": "verify", "last_name": run, "age": 1, "amount": 12.34}))
	r := rows(store.Read("accounts", flt))
	expect("rows after create", len(r), 1)
	step("update", store.Update("accounts", flt, map[string]any{"age": 2}))
	r = rows(store.Read("accounts", flt))
	if len(r) == 1 {
		expect("age after update", r[0]["age"], 2)
	}
	refused := func(name string, err, want error) {
		if errors.Is(err, want) {
			step(name, nil)
		} else {
			step(name, fmt.Errorf("got %v, want %v", err, want))
		}
	}
	refused("Update with an empty filter is refused", store.Update("accounts", map[string]any{}, map[string]any{"age": 0}), sqldrv.ErrEmptyFilter)
	refused("Delete with an empty filter is refused", store.Delete("accounts", map[string]any{}), sqldrv.ErrEmptyFilter)
	_, err = store.Read("accounts; drop table accounts", flt)
	refused("a table name with SQL in it is refused", err, sqldrv.ErrInvalidIdentifier)
	res, err := store.ExecWithSql("select count(*) as n from accounts where last_name = $1", run)
	step("ExecWithSql select", err)
	if l, ok := res.([]any); ok && len(l) == 1 {
		expect("ExecWithSql count", l[0].(map[string]any)["n"], 1)
	} else {
		step("ExecWithSql result shape", fmt.Errorf("got %T", res))
	}

	tm := store.(abstract.TransactionManager)
	tx, err := tm.BeginTransaction(ctx)
	step("begin tx (rollback case)", err)
	if err == nil {
		step("tx create", tx.Create("accounts", map[string]any{"first_name": "verify-rollback", "last_name": run, "age": 3}))
		expect("tx sees its own write", len(rows(tx.Read("accounts", flt))), 2)
		step("rollback", tx.Rollback())
		step("tx disconnect", tx.Disconnect())
	}
	expect("rows after rollback", count(), 1)

	tx, err = tm.BeginTransaction(ctx)
	step("begin tx (commit case)", err)
	if err == nil {
		step("tx create", tx.Create("accounts", map[string]any{"first_name": "verify-commit", "last_name": run, "age": 4}))
		step("commit", tx.Commit())
		step("tx disconnect", tx.Disconnect())
	}
	expect("rows after commit", count(), 2)

	q, err := factory.NewQueryStore(cfg)
	step("factory.NewQueryStore", err)
	if err == nil {
		_, isWriter := any(q).(abstract.Writer)
		expect("query store is a Writer", isWriter, false)
		expect("query store reads", len(rows(q.Read("accounts", flt))), 2)
		_ = q.Disconnect()
	}
	drv, err := sqldrv.NewDriver(ctx, factory.QueryConnString(cfg), 1, 1, time.Second, time.Minute)
	step("sql.NewDriver(QueryConnString)", err)
	if err == nil {
		ro := sqldrv.NewStore(drv)
		err = ro.Create("accounts", map[string]any{"first_name": "verify-ro", "last_name": run, "age": 9})
		if err == nil {
			step("read-only connection refuses a write", errors.New("write was accepted"))
		} else {
			step("read-only connection refuses a write", nil)
		}
		_ = ro.Disconnect()
	}
	// ponytail: marker rows stay until `cleanup`; verify.sh checks them with psql in between.
}

func mg(ctx context.Context, run, phase string) {
	cfg := factory.Config{ // the docker-compose.yaml mongodb service
		Type: factory.DbTypeMongo, Ctx: ctx, Schema: "mongodb", Db: "data_privacy",
		Username: "user", Password: "user!@#", Host: "127.0.0.1", Port: 27017,
		MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: 5 * time.Second, ConnMaxIdleTime: 5 * time.Second,
	}
	store, err := factory.NewStore(cfg)
	step("factory.NewStore(mongo)", err)
	if err != nil {
		return
	}
	defer func() { _ = store.Disconnect() }()
	filter := func(id string) map[string]any { return map[string]any{"verify_run": run, "id": id} }
	balance := func(id string) any {
		r := rows(store.Read("accounts", filter(id)))
		if len(r) != 1 {
			return fmt.Sprintf("%d docs", len(r))
		}
		return r[0]["balance"]
	}

	if phase == "cleanup" {
		step("delete", store.Delete("accounts", map[string]any{"verify_run": run}))
		expect("docs left", len(rows(store.Read("accounts", map[string]any{"verify_run": run}))), 0)
		return
	}

	step("ping", store.Ping())
	step("create A", store.Create("accounts", map[string]any{"verify_run": run, "id": "A", "balance": 100.0}))
	step("create B", store.Create("accounts", map[string]any{"verify_run": run, "id": "B", "balance": 50.0}))
	expect("A balance", balance("A"), 100.0)
	step("update $set", store.Update("accounts", filter("A"), map[string]any{"note": "updated"}))
	err = store.Update("accounts", filter("A"), mongo.Inc{"balance": -1})
	if errors.Is(err, mongo.ErrIncNeedsTransaction) {
		step("Inc outside a transaction is refused", nil)
	} else {
		step("Inc outside a transaction is refused", fmt.Errorf("got %v", err))
	}

	tm := store.(abstract.TransactionManager)
	tx, err := tm.BeginTransaction(ctx)
	step("begin tx (commit case)", err)
	if err == nil {
		step("tx Inc A -10", tx.Update("accounts", filter("A"), mongo.Inc{"balance": -10}))
		step("tx Inc B +10", tx.Update("accounts", filter("B"), mongo.Inc{"balance": 10}))
		step("commit", tx.Commit())
		step("tx disconnect", tx.Disconnect())
	}
	expect("A after commit", balance("A"), 90.0)
	expect("B after commit", balance("B"), 60.0)

	tx, err = tm.BeginTransaction(ctx)
	step("begin tx (rollback case)", err)
	if err == nil {
		step("tx Inc A -5", tx.Update("accounts", filter("A"), mongo.Inc{"balance": -5}))
		step("rollback", tx.Rollback())
		step("tx disconnect", tx.Disconnect())
	}
	expect("A after rollback", balance("A"), 90.0)

	tx, err = tm.BeginTransaction(ctx)
	step("begin tx (no-match case)", err)
	if err == nil {
		err = tx.Update("accounts", filter("nope"), mongo.Inc{"balance": 1})
		if errors.Is(err, mongo.ErrNoMatch) {
			step("Inc on a missing doc is ErrNoMatch", nil)
		} else {
			step("Inc on a missing doc is ErrNoMatch", fmt.Errorf("got %v", err))
		}
		_ = tx.Rollback()
		_ = tx.Disconnect()
	}

	q, err := factory.NewQueryStore(cfg)
	step("factory.NewQueryStore", err)
	if err == nil {
		_, isWriter := any(q).(abstract.Writer)
		expect("query store is a Writer", isWriter, false)
		expect("query store reads A", len(rows(q.Read("accounts", filter("A")))), 1)
		_ = q.Disconnect()
	}
}

func repl(ctx context.Context, run string) {
	cfg, err := factory.FromEnv(ctx, "PG_")
	step("factory.FromEnv(PG_)", err)
	if err != nil {
		return
	}
	store, err := factory.NewStore(cfg)
	step("factory.NewStore(sql)", err)
	if err != nil {
		return
	}
	defer func() { _ = store.Disconnect() }()
	conn, err := replication.Connect(ctx, cfg)
	step("replication.Connect", err)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close(ctx) }()
	sys, err := pglogrepl.IdentifySystem(ctx, conn)
	step("IDENTIFY_SYSTEM", err)
	if err == nil && sys.SystemID == "" {
		step("system id is set", errors.New("empty"))
	}
	slot := "dbconnector_verify_" + regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(run, "_")
	_, err = pglogrepl.CreateReplicationSlot(ctx, conn, slot, "pgoutput",
		pglogrepl.CreateReplicationSlotOptions{Temporary: true, Mode: pglogrepl.LogicalReplication})
	step("create temporary logical slot (needs wal_level=logical)", err)
	if err != nil {
		return
	}
	res, err := store.ExecWithSql("select slot_type, temporary::text as t from pg_replication_slots where slot_name = $1", slot)
	step("slot visible through the store", err)
	if l, ok := res.([]any); ok && len(l) == 1 {
		m := l[0].(map[string]any)
		expect("slot type", str(m["slot_type"]), "logical")
		expect("slot temporary", str(m["t"]), "true")
	} else {
		step("slot row shape", fmt.Errorf("got %T %v", res, res))
	}
}
