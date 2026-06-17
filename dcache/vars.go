package dcache

const (
	MIN_GOROUTINES = 10000 //nolint:staticcheck
	// TOPIC_REDIS_SET_DEL is the topic name to publish the entry associated with the key should update/delete event.
	TOPIC_REDIS_SET_DEL = "core-distributed-cache-set-del" //nolint:staticcheck
	// TOPIC_REDIS_DONE is the topic name to receive the entry associated with the key was update/delete event,
	// We should update/delete the local cache now.
	TOPIC_REDIS_DONE = "core-distributed-cache-done" //nolint:staticcheck

	GROUP_REDIS_SET_DEL = "core-distributed-cache-set-del" //nolint:staticcheck
	GROUP_REDIS_DONE    = "core-distributed-cache-done"    //nolint:staticcheck
)
