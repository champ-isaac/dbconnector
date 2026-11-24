package mongo

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// fakeServer is a minimal in-process MongoDB. It speaks just enough of the wire
// protocol (handshake plus OP_MSG commands) for Store and TxStore to run against a
// real mongo.Client with no database installed. It records every command so tests
// can check what was really sent.
type fakeServer struct {
	addr string
	ln   net.Listener
	wg   sync.WaitGroup

	mu        sync.Mutex
	conns     []net.Conn
	recorded  []command
	fail      map[string]bool // command name -> answer ok:0
	findBatch []bson.M        // documents a find returns
	cursorID  int64           // cursor id a find returns (0 = exhausted)
	badBatch  bool            // a find returns one document the driver cannot decode
	noMatch   bool            // an update matches no document (n = 0)
	manyMatch bool            // an update matches two documents (n = 2)
}

type command struct {
	name string
	db   string
	body bson.Raw
	seqs map[string][]bson.Raw // OP_MSG document sequences, such as the "documents" of an insert
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &fakeServer{addr: ln.Addr().String(), ln: ln, fail: map[string]bool{}}
	s.wg.Add(1)
	go s.accept()
	t.Cleanup(s.close)
	return s
}

func (s *fakeServer) close() {
	_ = s.ln.Close()
	s.mu.Lock()
	for _, c := range s.conns {
		_ = c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *fakeServer) accept() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns = append(s.conns, c)
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serve(c)
		}()
	}
}

// configure changes the fake's behaviour under its lock (the server goroutines read it).
func (s *fakeServer) configure(f func(*fakeServer)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

func (s *fakeServer) setFail(names ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range names {
		s.fail[n] = true
	}
}

// commands returns the recorded commands with this name, in arrival order.
func (s *fakeServer) commands(name string) []command {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []command
	for _, c := range s.recorded {
		if c.name == name {
			out = append(out, c)
		}
	}
	return out
}

const (
	opReply = 1
	opQuery = 2004
	opMsg   = 2013
)

func (s *fakeServer) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	for {
		var lenb [4]byte
		if _, err := io.ReadFull(c, lenb[:]); err != nil {
			return
		}
		n := int(int32(binary.LittleEndian.Uint32(lenb[:])))
		if n < 16 || n > 48_000_000 {
			return
		}
		rest := make([]byte, n-4)
		if _, err := io.ReadFull(c, rest); err != nil {
			return
		}
		reqID := binary.LittleEndian.Uint32(rest[0:4])
		opcode := int32(binary.LittleEndian.Uint32(rest[8:12]))
		body := rest[12:]

		var reply []byte
		switch opcode {
		case opQuery: // the legacy handshake
			pos := 4 // flags
			pos += bytes.IndexByte(body[pos:], 0) + 1
			pos += 8 // numberToSkip, numberToReturn
			cmd := s.parseCommand(body[pos:], nil)
			reply = oldReply(reqID, s.respond(cmd))
		case opMsg:
			flags := binary.LittleEndian.Uint32(body[0:4])
			end := len(body)
			if flags&1 != 0 { // checksum present
				end -= 4
			}
			var main []byte
			seqs := map[string][]bson.Raw{}
			pos := 4
			for pos < end {
				kind := body[pos]
				pos++
				size := int(int32(binary.LittleEndian.Uint32(body[pos:])))
				if kind == 0 {
					main = body[pos : pos+size]
				} else {
					sect := body[pos+4 : pos+size]
					id := bytes.IndexByte(sect, 0)
					name := string(sect[:id])
					docs := sect[id+1:]
					for len(docs) > 0 {
						dl := int(int32(binary.LittleEndian.Uint32(docs)))
						seqs[name] = append(seqs[name], append(bson.Raw(nil), docs[:dl]...))
						docs = docs[dl:]
					}
				}
				pos += size
			}
			cmd := s.parseCommand(main, seqs)
			if flags&2 != 0 { // moreToCome: the client expects no reply
				continue
			}
			reply = msgReply(reqID, s.respond(cmd))
		default:
			return
		}
		if _, err := c.Write(reply); err != nil {
			return
		}
	}
}

func (s *fakeServer) parseCommand(doc []byte, seqs map[string][]bson.Raw) command {
	raw := append(bson.Raw(nil), doc...)
	elems, err := raw.Elements()
	if err != nil || len(elems) == 0 {
		return command{}
	}
	c := command{name: elems[0].Key(), body: raw, seqs: seqs}
	if v, err := raw.LookupErr("$db"); err == nil {
		c.db, _ = v.StringValueOK()
	}
	return c
}

func (s *fakeServer) respond(c command) any {
	switch strings.ToLower(c.name) {
	case "hello", "ismaster":
		return bson.D{
			{Key: "helloOk", Value: true},
			{Key: "ismaster", Value: true},
			{Key: "isWritablePrimary", Value: true},
			{Key: "maxBsonObjectSize", Value: int32(16777216)},
			{Key: "maxMessageSizeBytes", Value: int32(48000000)},
			{Key: "maxWriteBatchSize", Value: int32(100000)},
			{Key: "minWireVersion", Value: int32(0)},
			{Key: "maxWireVersion", Value: int32(21)},
			{Key: "logicalSessionTimeoutMinutes", Value: int32(30)},
			{Key: "ok", Value: 1.0},
		}
	}

	s.mu.Lock()
	s.recorded = append(s.recorded, c)
	fail := s.fail[c.name]
	batch, id, bad, none, many := s.findBatch, s.cursorID, s.badBatch, s.noMatch, s.manyMatch
	s.mu.Unlock()

	if fail {
		return bson.D{{Key: "ok", Value: 0.0}, {Key: "errmsg", Value: "fake failure"}, {Key: "code", Value: int32(2)}, {Key: "codeName", Value: "BadValue"}}
	}
	switch c.name {
	case "insert":
		return bson.D{{Key: "n", Value: int32(1)}, {Key: "ok", Value: 1.0}}
	case "update":
		if none {
			return bson.D{{Key: "n", Value: int32(0)}, {Key: "nModified", Value: int32(0)}, {Key: "ok", Value: 1.0}}
		}
		if many {
			return bson.D{{Key: "n", Value: int32(2)}, {Key: "nModified", Value: int32(2)}, {Key: "ok", Value: 1.0}}
		}
		return bson.D{{Key: "n", Value: int32(1)}, {Key: "nModified", Value: int32(1)}, {Key: "ok", Value: 1.0}}
	case "delete":
		return bson.D{{Key: "n", Value: int32(1)}, {Key: "ok", Value: 1.0}}
	case "find":
		if bad {
			return badBatchReply(id)
		}
		docs := bson.A{}
		for _, d := range batch {
			docs = append(docs, d)
		}
		return bson.D{
			{Key: "cursor", Value: bson.D{{Key: "id", Value: id}, {Key: "ns", Value: c.db + ".c"}, {Key: "firstBatch", Value: docs}}},
			{Key: "ok", Value: 1.0},
		}
	default: // ping, commitTransaction, abortTransaction, killCursors, endSessions, ...
		return bson.D{{Key: "ok", Value: 1.0}}
	}
}

// badBatchReply is a find reply whose one document carries an element of an unknown
// BSON type. The cursor accepts it (only the length is checked) and Decode rejects it.
func badBatchReply(cursorID int64) []byte {
	elem := func(typ byte, key string, val []byte) []byte {
		return append(append([]byte{typ}, append([]byte(key), 0)...), val...)
	}
	doc := func(elems ...[]byte) []byte {
		body := bytes.Join(elems, nil)
		out := make([]byte, 4, len(body)+5)
		binary.LittleEndian.PutUint32(out, uint32(len(body)+5))
		return append(append(out, body...), 0)
	}
	le64 := func(v uint64) []byte { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, v); return b }
	str := func(v string) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(len(v)+1))
		return append(append(b, v...), 0)
	}
	bad := doc(elem(0x20, "x", nil)) // type 0x20 is unassigned
	batch := doc(elem(0x03, "0", bad))
	cursor := doc(
		elem(0x12, "id", le64(uint64(cursorID))),
		elem(0x02, "ns", str("db.c")),
		elem(0x04, "firstBatch", batch),
	)
	return doc(elem(0x03, "cursor", cursor), elem(0x01, "ok", le64(math.Float64bits(1))))
}

func frame(reqID uint32, opcode int32, payload []byte) []byte {
	out := make([]byte, 16, 16+len(payload))
	binary.LittleEndian.PutUint32(out[0:], uint32(16+len(payload)))
	binary.LittleEndian.PutUint32(out[4:], 1) // our request id
	binary.LittleEndian.PutUint32(out[8:], reqID)
	binary.LittleEndian.PutUint32(out[12:], uint32(opcode))
	return append(out, payload...)
}

func marshal(v any) []byte {
	if b, ok := v.([]byte); ok {
		return b
	}
	b, err := bson.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func msgReply(reqID uint32, v any) []byte {
	payload := append([]byte{0, 0, 0, 0, 0}, marshal(v)...) // flagBits, section kind 0
	return frame(reqID, opMsg, payload)
}

func oldReply(reqID uint32, v any) []byte {
	head := make([]byte, 20)
	binary.LittleEndian.PutUint32(head[0:], 8) // AwaitCapable
	binary.LittleEndian.PutUint32(head[16:], 1)
	return frame(reqID, opReply, append(head, marshal(v)...))
}

// newTestClient connects a real mongo.Client to the fake server (no auth, no retries).
func newTestClient(t *testing.T, srv *fakeServer, tweak ...func(*options.ClientOptions)) *mongo.Client {
	t.Helper()
	opts := options.Client().ApplyURI("mongodb://" + srv.addr).
		SetDirect(true).SetRetryWrites(false).SetRetryReads(false).
		SetServerSelectionTimeout(5 * time.Second)
	for _, f := range tweak {
		f(opts)
	}
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	return client
}

func newTestDriver(t *testing.T, srv *fakeServer, tweak ...func(*options.ClientOptions)) *Driver {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	client := newTestClient(t, srv, tweak...)
	d := &Driver{Ctx: ctx, Client: client}
	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
		cancel()
	})
	return d
}
