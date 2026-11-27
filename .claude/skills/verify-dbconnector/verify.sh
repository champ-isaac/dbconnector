#!/usr/bin/env bash
# Verification harness for dbconnector. See SKILL.md.
#   verify.sh up                      start postgres + mongodb (compose project dbconnector-verify)
#   verify.sh doctor                  read-only: is the stack worth driving?
#   verify.sh drive [unit|pg|mongo|repl|tests|all]   default all
#   verify.sh down                    remove OUR containers, network and volumes
#   verify.sh all                     up, doctor, drive all, down (down runs even on failure)
# Env: VERIFY_RUN=<id> reuse an evidence id (default v<timestamp>); KEEP=1 skip teardown in `all`.
# Evidence: .claude/skills/verify-dbconnector/evidence/<id>/ (gitignored, never removed by down).
set -uo pipefail

SKILL_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$SKILL_DIR/../../.." && pwd)
cd "$ROOT" || exit 1

PROJECT=dbconnector-verify
PG_C=$PROJECT-postgres
MG_C=$PROJECT-mongodb
PW='user!@#'
export VERIFY_RUN=${VERIFY_RUN:-v$(date +%Y%m%d-%H%M%S)}
EV=$SKILL_DIR/evidence/$VERIFY_RUN
mkdir -p "$EV"
export PG_HOST=127.0.0.1 PG_PORT=5432 PG_DB=data_privacy PG_USER=user PG_PASSWORD=$PW

dc() { docker compose -p $PROJECT -f docker-compose.yaml -f "$SKILL_DIR/compose.verify.yaml" "$@"; }
pgq() { docker exec -e PGPASSWORD="$PW" $PG_C psql -h 127.0.0.1 -U user -d data_privacy -tAc "$1"; }
mq() { docker exec $MG_C mongosh -u user -p "$PW" --authenticationDatabase admin --quiet --eval "$1"; }

FAILS=0
record() { # record <name> <ok 0/1>
  if [ "$2" = 0 ]; then echo "PASS $1" | tee -a "$EV/summary.txt"; else echo "FAIL $1" | tee -a "$EV/summary.txt"; FAILS=$((FAILS + 1)); fi
}
expect() { # expect <name> <got> <want>
  echo "$1: got '$2' want '$3'" >>"$EV/db-state.txt"
  [ "$2" = "$3" ]; record "$1 ($2)" $?
}

# Who owns a host port? ours | free | foreign:<what>
port_owner() {
  local names
  names=$(docker ps --filter "publish=$1" --format '{{.Names}}')
  if [ -n "$names" ]; then
    case "$names" in "$2") echo ours ;; *) echo "foreign:container $names" ;; esac
  elif lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "foreign:non-docker listener"
  else echo free; fi
}

cmd_up() {
  docker info >/dev/null 2>&1 || { echo "docker daemon not reachable (colima start)"; return 1; }
  local p c o
  for p in "5432 $PG_C" "27017 $MG_C"; do
    set -- $p; o=$(port_owner "$1" "$2")
    case "$o" in foreign:*) echo "port $1 is taken ($o). Tests hardcode 127.0.0.1:$1, so two stacks cannot run side by side. Stop it first, or ask the user."; return 1 ;; esac
  done
  # amd64 images run under emulation on arm64 hosts: slow first start, hence the long waits.
  dc up -d postgres mongodb >"$EV/up.log" 2>&1 || { cat "$EV/up.log"; return 1; }
  echo "waiting for postgres (init.sql ran, TCP up, wal_level=logical) ..."
  for _ in $(seq 1 90); do
    [ "$(pgq "select to_regclass('accounts') || '|' || current_setting('wal_level')" 2>/dev/null)" = "accounts|logical" ] && break
    sleep 2
  done
  [ "$(pgq "select to_regclass('accounts') || '|' || current_setting('wal_level')" 2>/dev/null)" = "accounts|logical" ] \
    || { echo "postgres not ready"; docker logs $PG_C 2>&1 | tail -30; return 1; }
  echo "waiting for mongodb (primary, init.js seed, data_privacy user) ..."
  for _ in $(seq 1 90); do
    [ "$(mq "JSON.stringify([db.hello().isWritablePrimary, db.getSiblingDB('data_privacy').accounts.countDocuments({first_name:'Isaac'}) > 0, db.getSiblingDB('data_privacy').getUser('user') !== null])" 2>/dev/null)" = "[true,true,true]" ] && { echo ready; return 0; }
    sleep 2
  done
  echo "mongodb not ready"; docker logs $MG_C 2>&1 | tail -30; return 1
}

cmd_doctor() {
  local bad=0
  chk() { if [ "$2" = "$3" ]; then echo "ok   $1 ($2)"; else echo "BAD  $1: got '$2', want '$3'"; bad=1; fi; }
  chk "docker daemon" "$(docker info >/dev/null 2>&1 && echo up || echo down)" up
  chk "port 5432 owner" "$(port_owner 5432 $PG_C)" ours
  chk "port 27017 owner" "$(port_owner 27017 $MG_C)" ours
  chk "postgres schema + wal_level" "$(pgq "select to_regclass('accounts') || '|' || current_setting('wal_level')" 2>&1)" "accounts|logical"
  chk "postgres role can replicate" "$(pgq "select rolreplication or rolsuper from pg_roles where rolname='user'" 2>&1)" t
  chk "mongodb primary" "$(mq 'db.hello().isWritablePrimary' 2>&1)" true
  chk "mongodb seed (init.js)" "$(mq "db.getSiblingDB('data_privacy').accounts.countDocuments({first_name:'Isaac'}) > 0" 2>&1)" true
  chk "go build ./..." "$(go build ./... >/dev/null 2>&1 && echo ok || echo fail)" ok
  chk "leftover verify rows" "$(pgq "select count(*) from accounts where first_name like 'verify%'" 2>&1)" 0
  return $bad
}

caller() { go run ./.claude/skills/verify-dbconnector/caller "$@"; }

drive_unit() {
  go test -race -count=1 -cover ./factory ./drivers/sql ./replication ./drivers/mongo >"$EV/unit.txt" 2>&1; record "unit tests" $?
  go vet -tags integration ./... >>"$EV/unit.txt" 2>&1; record "go vet -tags integration" $?
}

drive_pg() {
  caller pg write >"$EV/caller-pg.txt" 2>&1; record "caller pg write" $?
  expect "pg marker rows (count|sum(age)|first_names)" \
    "$(pgq "select count(*) || '|' || sum(age) || '|' || string_agg(first_name, ',' order by first_name) from accounts where last_name='$VERIFY_RUN'")" "2|6|verify,verify-commit"
  caller pg cleanup >>"$EV/caller-pg.txt" 2>&1; record "caller pg cleanup" $?
  expect "pg rows after cleanup" "$(pgq "select count(*) from accounts where last_name='$VERIFY_RUN'")" 0
}

drive_mongo() {
  caller mongo write >"$EV/caller-mongo.txt" 2>&1; record "caller mongo write" $?
  expect "mongo marker docs (A|B|note|count)" \
    "$(mq "const c=db.getSiblingDB('data_privacy').accounts, a=c.findOne({verify_run:'$VERIFY_RUN',id:'A'}), b=c.findOne({verify_run:'$VERIFY_RUN',id:'B'}); a.balance+'|'+b.balance+'|'+a.note+'|'+c.countDocuments({verify_run:'$VERIFY_RUN'})")" "90|60|updated|2"
  caller mongo cleanup >>"$EV/caller-mongo.txt" 2>&1; record "caller mongo cleanup" $?
  expect "mongo docs after cleanup" "$(mq "db.getSiblingDB('data_privacy').accounts.countDocuments({verify_run:'$VERIFY_RUN'})")" 0
}

drive_repl() {
  caller repl write >"$EV/caller-repl.txt" 2>&1; record "caller repl" $?
  expect "temporary slot gone after the connection closed" \
    "$(pgq "select count(*) from pg_replication_slots where slot_name like 'dbconnector_verify_%'")" 0
}

# The repo's own integration suite. A skipped test would leave `go test` green, so require a PASS line for each.
drive_tests() {
  local re='InPostgres|InMongo|^TestReplicationSlotCreateAndDrop$' t missing=0
  go test -tags integration -list "$re" ./tests | grep '^Test' >"$EV/tests-want.txt"
  [ -s "$EV/tests-want.txt" ] || { record "integration tests found" 1; return; }
  go test -race -count=1 -tags integration -run "$re" -v ./tests >"$EV/tests.txt" 2>&1; record "go test -tags integration" $?
  while read -r t; do
    grep -q -- "--- PASS: $t (" "$EV/tests.txt" || { echo "not PASS: $t" | tee -a "$EV/summary.txt"; missing=1; }
  done <"$EV/tests-want.txt"
  record "every listed integration test PASSed ($(wc -l <"$EV/tests-want.txt" | tr -d ' ') tests)" $missing
}

cmd_drive() {
  case "${1:-all}" in
    unit) drive_unit ;; pg) drive_pg ;; mongo) drive_mongo ;; repl) drive_repl ;; tests) drive_tests ;;
    all) drive_unit; drive_pg; drive_mongo; drive_repl; drive_tests ;;
    *) echo "usage: verify.sh drive [unit|pg|mongo|repl|tests|all]"; return 2 ;;
  esac
  echo "evidence: $EV  (failures: $FAILS)"
  [ "$FAILS" = 0 ]
}

cmd_down() { dc down -v --remove-orphans; }

case "${1:-}" in
  up) cmd_up ;;
  doctor) cmd_doctor ;;
  drive) shift; cmd_drive "$@" ;;
  down) cmd_down ;;
  all)
    [ "${KEEP:-0}" = 1 ] || trap cmd_down EXIT
    cmd_up && cmd_doctor | tee "$EV/doctor.txt" && [ "${PIPESTATUS[0]}" = 0 ] && cmd_drive all
    ;;
  *) sed -n 2,9p "$0"; rc=2 ;;
esac
rc=${rc:-$?}
rmdir "$EV" 2>/dev/null # doctor/down write no evidence; keep the folder only if something was captured
exit "$rc"
