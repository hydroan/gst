package redis_test

import (
	"fmt"
	"testing"

	"github.com/hydroan/gst/redis"
)

func BenchmarkSetML(b *testing.B) {
	samples := make([]*Sample, 0, 1000)
	for i := range 1000 {
		samples = append(samples, &Sample{
			Name:  fmt.Sprintf("sample-%d", i),
			Desc:  fmt.Sprintf("desc-%d", i),
			Count: i,
		})
	}

	for b.Loop() {
		if err := redis.SetML(b.Context(), "samples", samples); err != nil {
			b.Fatalf("%+v\n", err)
		}
	}
}
