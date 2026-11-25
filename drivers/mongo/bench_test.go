package mongo

import (
	"fmt"
	"testing"
)

// Memory benchmark: go test ./drivers/mongo -run '^$' -bench . -benchmem (client-side code only, no server).

func BenchmarkCheckFinite(b *testing.B) {
	inc := make(Inc, 16)
	for i := range 16 {
		inc[fmt.Sprintf("field_%02d", i)] = float64(i) + 0.5
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := checkFinite(inc); err != nil {
			b.Fatal(err)
		}
	}
}
