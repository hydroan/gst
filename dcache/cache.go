package dcache

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/redis"
	"github.com/hydroan/gst/util"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/panjf2000/ants/v2"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

var once sync.Once

// Init initializes the distributed cache system as a state node that manages Redis operations
// and coordinates cache synchronization across multiple distributed core nodes.
//
// This function serves as the central coordinator for distributed cache operations by:
//   - Consuming cache operation events (Set/Delete) from Kafka
//   - Executing Redis operations in a consistent, ordered manner
//   - Publishing completion events to notify other nodes to update their local caches
//   - Maintaining data consistency through timestamp-based ordering and deduplication
//
// Architecture Overview:
//
//	The distributed cache system consists of:
//	1. State Node (this Init function): Manages Redis and coordinates operations
//	2. Core Nodes: Maintain local secondary caches and send operation requests
//	3. Kafka: Message broker for event communication between nodes
//	4. Redis: Centralized cache storage for distributed data
//
// Key Implementation Rules:
//  1. Timestamp-based event filtering: Events with timestamps older than the recorded
//     maximum timestamp are discarded to prevent out-of-order operations
//  2. Per-key deduplication: Only the latest operation for each key is retained within
//     a batch, ensuring consistency (e.g., if Set(11:14) and Delete(11:10) exist,
//     only Set(11:14) will be executed)
//  3. Ordered execution: Operations are sorted by timestamp and executed sequentially
//     to maintain strict ordering of Redis cache operations
//  4. Batch processing: Events are processed in batches with maximum timestamp tracking
//     for efficient throughput and consistency guarantees
//
// Error Handling:
//   - Uses sync.Once to ensure single initialization
//   - Validates Redis client availability before starting
//   - Implements comprehensive error logging and metrics collection
//   - Gracefully handles Kafka connection issues and message processing failures
//
// Performance Optimizations:
//   - Utilizes goroutine pools to control Kafka consumer concurrency
//   - Implements batch processing to reduce Redis round trips
//   - Uses concurrent maps for thread-safe timestamp tracking per key
func Init() error {
	var gerr error
	once.Do(func() {
		const compKey = "comp"
		const compVal = "[DistributedCache.Init]"

		hostname, err := os.Hostname()
		if err != nil {
			gerr = err
			return
		}
		log := logger.Dcache.With("hostname", hostname, compKey, compVal)
		log.Info("distributed cache setup")

		redisCli, err := redis.New(config.App.Redis)
		if err != nil {
			gerr = err
			return
		}

		// bound the kafka concurrency manually with a goroutine pool
		gopool, err := ants.NewPool(runtime.NumCPU()*2000, ants.WithPreAlloc(false))
		if err != nil {
			gerr = err
			return
		}

		// initialize the Kafka consumer and producer
		consumer, err := newConsumer(config.App.Kafka.Brokers, TOPIC_REDIS_SET_DEL, GROUP_REDIS_SET_DEL)
		if err != nil {
			gerr = err
			return
		}
		producer, err := newProducer(config.App.Kafka.Brokers, TOPIC_REDIS_DONE)
		if err != nil {
			gerr = err
			return
		}

		var wg sync.WaitGroup
		// keep a separate maximum timestamp per key
		keyMaxTimestamps := cmap.New[int64]()

		util.SafeGo(func() {
			for {
				// base context, used for operation timeouts
				baseCtx := context.Background()
				fetches := consumer.PollFetches(context.Background())
				if fetches.IsClientClosed() {
					// A closed client cannot recover here; stop the consumer
					// instead of spinning on an immediately returning poll.
					log.Error("kafka consumer client closed, stopping the state node consumer")
					return
				}
				fetches.EachError(func(s string, i int32, err error) {
					log.Errorz(
						"failed to fetch from kafka",
						zap.Error(err),
						zap.String("topic", TOPIC_REDIS_SET_DEL),
						zap.String("s", s),
						zap.Int32("i", i),
					)
				})

				// reset the batch counters
				totalRecords := 0        // total number of messages
				var successRecords int64 // number of messages processed successfully
				var failedRecords int64  // number of messages that failed to process
				skippedRecords := 0      // number of invalid messages that were skipped

				// tracks the offsets of the messages processed in this batch
				offsets := make(map[string]map[int32]kgo.EpochOffset)

				// ---------------------------------------------------------------------
				// Phase one: collect every event and deduplicate by timestamp, keeping the latest operation per key
				// ---------------------------------------------------------------------

				// holds the latest operation per key, which implements rule one and rule three
				keyEvents := make(map[string]*event)

				begin := time.Now()
				// walk the messages of every partition
				fetches.EachPartition(func(p kgo.FetchTopicPartition) {
					if len(p.Records) == 0 {
						return // silently skip empty partitions
					}

					totalRecords += len(p.Records)

					// make sure the offset map of the topic is initialized
					if _, exists := offsets[p.Topic]; !exists {
						offsets[p.Topic] = make(map[int32]kgo.EpochOffset)
					}

					var lastOffset int64 = -1
					for _, record := range p.Records {
						lastOffset = record.Offset // remember the offset of the last message

						// parse the event
						event := new(event)
						if err := json.Unmarshal(record.Value, event); err != nil {
							log.Errorz(
								"failed to unmarshal event from kafka record",
								zap.Error(err),
								zap.Int64("offset", record.Offset),
							)
							failedRecords++
							continue
						}

						// the highest timestamp seen so far for this key
						keyMaxTS, _ := keyMaxTimestamps.Get(event.Key)

						// Rule one: drop events whose timestamp is not newer than the highest one seen for this key
						if event.TS <= keyMaxTS {
							log.Warnz(
								"skipping outdated event for key",
								zap.String("key", event.Key),
								zap.Int64("event_ts", event.TS),
								zap.Int64("key_max_ts", keyMaxTS),
								zap.String("op", event.Op.String()),
							)
							skippedRecords++
							continue
						}

						// Rule two: deduplicate by timestamp, keeping only the latest operation per key
						existingEvent, exists := keyEvents[event.Key]
						if !exists || event.TS > existingEvent.TS {
							keyEvents[event.Key] = event
						}

					}

					// update the partition offset, in case offsets are committed manually later (probably no longer needed)
					if lastOffset >= 0 {
						offsets[p.Topic][p.Partition] = kgo.EpochOffset{
							Offset: lastOffset + 1,
							Epoch:  -1,
						}
					}
				})

				// nothing to process, wait for the next batch
				if len(keyEvents) == 0 {
					log.Debugz(
						"no events to process in this batch",
						zap.Int("total_records", totalRecords),
						zap.Int("skipped_records", skippedRecords),
						zap.Int64("failed_records", failedRecords),
					)
					continue
				}

				// turn the map into a slice so it can be sorted by timestamp
				eventSlice := make([]*event, 0, len(keyEvents))
				for _, event := range keyEvents {
					eventSlice = append(eventSlice, event)
				}

				// Rule three: sort strictly by timestamp (oldest first)
				sort.Slice(eventSlice, func(i, j int) bool {
					return eventSlice[i].TS < eventSlice[j].TS
				})

				// ---------------------------------------------------------------------
				// Phase two: run the Redis operations in timestamp order, then publish a kafka message for each
				// ---------------------------------------------------------------------

				// highest timestamp per key within this batch, applied once the batch is done
				batchKeyMaxTS := make(map[string]int64)

				// run the redis and kafka operations of this batch
				wg.Add(len(eventSlice))
				for i := range eventSlice {
					evt := eventSlice[i]
					// update the highest timestamp of this key within the batch
					if ts, exists := batchKeyMaxTS[evt.Key]; !exists || evt.TS > ts {
						batchKeyMaxTS[evt.Key] = evt.TS
					}

					// TODO: lower this to the Debug level in production
					log.Infoz("process event", zap.Object("event", evt))

					submitErr := gopool.Submit(func() {
						defer wg.Done()
						switch evt.Op {
						case opSet:
							if evt.SyncToRedis {
								if err := redisCli.Set(baseCtx, evt.Key, []byte(evt.Val), evt.RedisTTL).Err(); err != nil {
									atomic.AddInt64(&failedRecords, 1)
									log.Errorz(
										"failed to set redis key",
										zap.Error(err),
										zap.String("key", evt.Key),
										zap.Object("event", evt),
									)
									return
								}
							}
							// send the done event to Kafka whether or not it was synced to Redis
							evtDone := &event{
								CacheID:     evt.CacheID,
								Typ:         evt.Typ,
								Op:          opSetDone,
								Key:         evt.Key,
								Val:         evt.Val,
								TTL:         evt.TTL,
								TS:          time.Now().UnixNano(),
								Hostname:    evt.Hostname,
								SyncToRedis: evt.SyncToRedis,
								RedisTTL:    evt.RedisTTL,
							}
							data, err := json.Marshal(evtDone)
							if err != nil {
								log.Errorz(
									"failed to marshal event in redis set",
									zap.Error(err),
									zap.Object("event", evtDone),
								)
								atomic.AddInt64(&failedRecords, 1)
							} else {
								atomic.AddInt64(&successRecords, 1)
								// publish the kafka message synchronously
								produceRecord := &kgo.Record{Topic: TOPIC_REDIS_DONE, Value: data}
								if err := producer.ProduceSync(baseCtx, produceRecord).FirstErr(); err != nil {
									log.Errorz(
										"failed to produce redis set done event",
										zap.Error(err),
										zap.Object("event", evtDone),
									)
								}
							}
						case opDel:
							if evt.SyncToRedis {
								if err := redisCli.Del(baseCtx, evt.Key).Err(); err != nil {
									log.Errorz(
										"failed to del redis key",
										zap.Error(err),
										zap.String("key", evt.Key),
										zap.Object("event", evt),
									)
									atomic.AddInt64(&failedRecords, 1)
									return
								}
							}
							// send the done event to Kafka whether or not it was synced to Redis
							evtDone := &event{
								CacheID:     evt.CacheID,
								Typ:         evt.Typ,
								Op:          opDelDone,
								Key:         evt.Key,
								TS:          time.Now().UnixNano(),
								Hostname:    evt.Hostname,
								SyncToRedis: evt.SyncToRedis,
								RedisTTL:    evt.RedisTTL,
							}
							data, err := json.Marshal(evtDone)
							if err != nil {
								log.Errorz(
									"failed to marshal event in redis del",
									zap.Error(err),
									zap.Object("event", evtDone),
								)
								atomic.AddInt64(&failedRecords, 1)
							} else {
								atomic.AddInt64(&successRecords, 1)
								// publish the kafka message synchronously
								produceRecord := &kgo.Record{Topic: TOPIC_REDIS_DONE, Value: data}
								if err := producer.ProduceSync(baseCtx, produceRecord).FirstErr(); err != nil {
									log.Errorz(
										"failed to produce redis del done event",
										zap.Error(err),
										zap.Object("event", evtDone),
									)
								}
							}
						default:
							log.Warnz("unknown operation type", zap.String("op", evt.Op.String()))
						}
					})
					if submitErr != nil {
						// The task never ran, so its wait-group slot must be
						// released here or wg.Wait would hang forever.
						wg.Done()
						log.Errorz("failed to submit event to gopool", zap.Error(submitErr), zap.Object("event", evt))
					}
				}
				wg.Wait()

				// the batch is done, update the highest timestamp of every key
				for key, ts := range batchKeyMaxTS {
					keyMaxTimestamps.Set(key, ts)
				}

				// log the processing statistics
				if totalRecords > 0 {
					log.Infoz(
						"successfully consumed events",
						zap.Int("total", totalRecords),
						zap.Int("deduplicated", len(eventSlice)),
						zap.Int64("success", successRecords),
						zap.Int64("failed", failedRecords),
						zap.Int("skipped", skippedRecords),
						util.LogDuration(time.Since(begin)),
					)
				}

				// drop the map and the slice to help the GC reclaim the memory
				keyEvents = nil
				eventSlice = nil
				batchKeyMaxTS = nil //nolint:ineffassign,wastedassign

				// // every restart consumes from the latest offset, so there is no need to persist offsets
				// if len(offsets) > 0 {
				// 	consumer.CommitOffsets(ctx, offsets, func(c *kgo.Client, ocr1 *kmsg.OffsetCommitRequest, ocr2 *kmsg.OffsetCommitResponse, err error) {
				// 		if err != nil {
				// 			fmt.Println("failed to commit offsets:", err)
				// 		} else {
				// 			fmt.Printf("successfully committed offsets: total(%d), success(%d), failed(%d), offset(%v), costed(%s)\n",
				// 				totalRecords, successRecords, failedRecords, offsets, time.Since(begin).String())
				// 		}
				// 	})
				// }

			}
		}, "DistributedCache.Init")
	})

	return gerr
}
