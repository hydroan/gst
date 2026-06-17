package bufferpool

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuffers(t *testing.T) {
	const dummyData = "dummy data"
	p := NewPool()

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				buf := p.Get()
				assert.Zero(t, buf.Len(), "Expected truncated buffer")
				assert.NotZero(t, buf.Cap(), "Expected non-zero capacity")

				buf.AppendString(dummyData)
				assert.Len(t, dummyData, buf.Len(), "Expected buffer to contain dummy data")

				buf.Free()
			}
		})
	}
	wg.Wait()
}
