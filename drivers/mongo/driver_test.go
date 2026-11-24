package mongo

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPoolSize(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want uint64
	}{
		{"inside the range", 25, 25},
		{"zero stays zero (driver default)", 0, 0},
		{"at the maximum", maxPoolSize, maxPoolSize},
		{"above the maximum is capped", maxPoolSize + 1, maxPoolSize},
		{"largest int is capped", math.MaxInt, maxPoolSize},
		{"below the minimum becomes zero", -1, 0},
		{"smallest int becomes zero", math.MinInt, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, poolSize(c.in))
		})
	}
}
