package mongo

import (
	"bytes"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func newTestStore(t *testing.T, srv *fakeServer, tweak ...func(*options.ClientOptions)) *Store {
	t.Helper()
	return NewStore(newTestDriver(t, srv, tweak...), "testdb").(*Store)
}

func TestNewStore(t *testing.T) {
	d := &Driver{}
	s, ok := NewStore(d, "db").(*Store)
	require.True(t, ok)
	assert.Same(t, d, s.driver)
	assert.Equal(t, "db", s.database)
}

func TestStore_Create(t *testing.T) {
	srv := newFakeServer(t)
	s := newTestStore(t, srv)

	require.NoError(t, s.Create("accounts", bson.M{"_id": 1, "name": "a"}))

	cmds := srv.commands("insert")
	require.Len(t, cmds, 1)
	assert.Equal(t, "testdb", cmds[0].db)
	assert.Equal(t, "accounts", cmds[0].body.Lookup("insert").StringValue())
	docs := cmds[0].seqs["documents"]
	require.Len(t, docs, 1)
	assert.Equal(t, "a", docs[0].Lookup("name").StringValue())
}

func TestStore_Read(t *testing.T) {
	srv := newFakeServer(t)
	srv.configure(func(s *fakeServer) { s.findBatch = []bson.M{{"_id": 1, "name": "a"}, {"_id": 2, "name": "b"}} })
	s := newTestStore(t, srv)

	got, err := s.Read("accounts", bson.M{"_id": 1})
	require.NoError(t, err)

	rows, ok := got.([]bson.M)
	require.True(t, ok)
	require.Len(t, rows, 2)
	assert.Equal(t, "a", rows[0]["name"])
	assert.Equal(t, "b", rows[1]["name"])

	cmds := srv.commands("find")
	require.Len(t, cmds, 1)
	assert.Equal(t, "accounts", cmds[0].body.Lookup("find").StringValue())
	assert.EqualValues(t, 1, cmds[0].body.Lookup("filter", "_id").AsInt64())
}

func TestStore_Read_NoRows(t *testing.T) {
	srv := newFakeServer(t)
	s := newTestStore(t, srv)

	got, err := s.Read("accounts", bson.M{})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestStore_Update(t *testing.T) {
	srv := newFakeServer(t)
	s := newTestStore(t, srv)

	require.NoError(t, s.Update("accounts", bson.M{"_id": 1}, bson.M{"name": "z"}))

	cmds := srv.commands("update")
	require.Len(t, cmds, 1)
	assert.Equal(t, "accounts", cmds[0].body.Lookup("update").StringValue())
	ups := cmds[0].seqs["updates"]
	require.Len(t, ups, 1)
	assert.EqualValues(t, 1, ups[0].Lookup("q", "_id").AsInt64())
	assert.Equal(t, "z", ups[0].Lookup("u", "$set", "name").StringValue())
	assert.True(t, ups[0].Lookup("multi").Boolean(), "UpdateMany updates every match")
}

func TestStore_Delete(t *testing.T) {
	srv := newFakeServer(t)
	s := newTestStore(t, srv)

	require.NoError(t, s.Delete("accounts", bson.M{"_id": 1}))

	cmds := srv.commands("delete")
	require.Len(t, cmds, 1)
	assert.Equal(t, "accounts", cmds[0].body.Lookup("delete").StringValue())
	dels := cmds[0].seqs["deletes"]
	require.Len(t, dels, 1)
	assert.EqualValues(t, 1, dels[0].Lookup("q", "_id").AsInt64())
	assert.EqualValues(t, 0, dels[0].Lookup("limit").AsInt64(), "DeleteMany has no limit")
}

func TestStore_Ping(t *testing.T) {
	srv := newFakeServer(t)
	s := newTestStore(t, srv)

	require.NoError(t, s.Ping())
	assert.Len(t, srv.commands("ping"), 1)
}

func TestStore_ServerErrorsAreReturned(t *testing.T) {
	for _, cmd := range []string{"insert", "find", "update", "delete", "ping"} {
		t.Run(cmd, func(t *testing.T) {
			srv := newFakeServer(t)
			s := newTestStore(t, srv)
			srv.setFail(cmd)
			var err error
			switch cmd {
			case "insert":
				err = s.Create("c", bson.M{"a": 1})
			case "find":
				var got any
				got, err = s.Read("c", bson.M{})
				assert.Nil(t, got)
			case "update":
				err = s.Update("c", bson.M{}, bson.M{"a": 1})
			case "delete":
				err = s.Delete("c", bson.M{})
			case "ping":
				err = s.Ping()
			}
			assert.ErrorContains(t, err, "fake failure")
		})
	}
}

// A document the driver cannot decode fails the whole Read. The cursor is still
// closed, and a failure to close it is logged, not returned.
func TestStore_Read_DecodeErrorClosesCursor(t *testing.T) {
	srv := newFakeServer(t)
	srv.configure(func(s *fakeServer) { s.badBatch, s.cursorID = true, 77 })
	srv.setFail("killCursors")
	s := newTestStore(t, srv)

	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	got, err := s.Read("c", bson.M{})
	assert.Nil(t, got)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "fake failure", "the cursor-close error is logged, not returned")
	assert.Len(t, srv.commands("killCursors"), 1)
	assert.Contains(t, logs.String(), "error when cursor closing")
}

func TestStore_Disconnect(t *testing.T) {
	srv := newFakeServer(t)
	s := newTestStore(t, srv)
	require.NoError(t, s.Ping())

	require.NoError(t, s.Disconnect())
	assert.Error(t, s.Ping(), "a disconnected store cannot be used")
}

func TestStore_ExecWithSql_NotSupported(t *testing.T) {
	assert.PanicsWithValue(t, "ExecWithSql does not implement for mongo yet", func() {
		_, _ = (&Store{}).ExecWithSql("select 1")
	})
}

// A relative update outside a transaction is refused and nothing is sent. Sent as a $set
// it would store the delta as the new value (a balance of 100 changed by -10 became -10).
func TestStore_Update_RefusesInc(t *testing.T) {
	srv := newFakeServer(t)
	s := newTestStore(t, srv)

	err := s.Update("accounts", bson.M{"id": "A"}, Inc{"balance": -10.0})

	assert.ErrorIs(t, err, ErrIncNeedsTransaction)
	assert.Empty(t, srv.commands("update"), "no update reaches the server")
	// a plain $set update is unaffected
	assert.NoError(t, s.Update("accounts", bson.M{"id": "A"}, bson.M{"balance": 90.0}))
	assert.Len(t, srv.commands("update"), 1)
}

// Outside a transaction every Inc is refused for the same reason, empty or not.
func TestStore_Update_RefusesEmptyInc(t *testing.T) {
	srv := newFakeServer(t)
	s := newTestStore(t, srv)

	assert.ErrorIs(t, s.Update("accounts", bson.M{"id": "A"}, Inc{}), ErrIncNeedsTransaction)
	assert.Empty(t, srv.commands("update"))
}
