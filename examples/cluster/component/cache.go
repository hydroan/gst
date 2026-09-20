package component

import (
	"context"

	"cluster/dao"

	"github.com/hydroan/gst/component"
)

func init() {
	component.Register(holdCache, "cache")
}

// holdCache opens the replicated cache while the replica starts, and keeps it
// for the life of the process. Opening a cache waits for the consumer group
// to hand it the topic's partitions, so whoever holds the cache is listening
// to its peers; opening it here rather than on the first request is what
// moves that wait into the start, where it runs beside the listener coming
// up, instead of onto the first request that caches anything.
func holdCache(ctx context.Context) error {
	if err := dao.OpenCache(); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}
