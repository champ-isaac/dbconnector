package mongo

import (
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Memory benchmarks: go test ./drivers/mongo -run '^$' -bench . -benchmem
// Only client-side code is measured. The in-process fake server in the unit tests would
// put its own allocations into B/op and the heap profile, so Store.Read and Update over
// the wire are not benchmarked here; run them against a real server for that.

func BenchmarkCheckFinite(b *testing.B) {
	for _, n := range []int{1, 4, 16} {
		inc := make(Inc, n)
		for i := range n {
			inc[fmt.Sprintf("field_%02d", i)] = float64(i) + 0.5
		}
		b.Run(fmt.Sprintf("fields=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := checkFinite(inc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkDecodeDocument is the per-document cost of the loop in Store.Read and
// TxStore.Read (cursor.Decode into a bson.M). It measures the driver's bson package on a
// typical document, as a stand-in for what each returned document costs.
func BenchmarkDecodeDocument(b *testing.B) {
	raw, err := bson.Marshal(bson.D{
		{Key: "_id", Value: bson.NewObjectID()},
		{Key: "name", Value: "account-42"},
		{Key: "balance", Value: 12.5},
		{Key: "active", Value: true},
		{Key: "created_at", Value: bson.NewDateTimeFromTime(time.Unix(1_700_000_000, 0))},
		{Key: "tags", Value: bson.A{"a", "b", "c"}},
		{Key: "address", Value: bson.D{{Key: "city", Value: "Taipei"}, {Key: "zip", Value: "100"}}},
		{Key: "note", Value: nil},
	})
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		var doc bson.M
		if err := bson.Unmarshal(raw, &doc); err != nil {
			b.Fatal(err)
		}
	}
}
