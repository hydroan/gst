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
	"github.com/hydroan/gst/provider/redis"
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
		}
		log := logger.Dcache.With("hostname", hostname, compKey, compVal)
		log.Info("distributed cache setup")

		redisCli, err := redis.New(config.App.Redis)
		if err != nil {
			gerr = err
			return
		}

		// 手动通过线程池控制 kafka 并发量
		gopool, err := ants.NewPool(runtime.NumCPU()*2000, ants.WithPreAlloc(false))
		if err != nil {
			gerr = err
			return
		}

		// 初始化 Kafka 消费者和生产者
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
		// 为每个 key 维护独立的最大时间戳
		keyMaxTimestamps := cmap.New[int64]()

		util.SafeGo(func() {
			for {
				// 基础上下文，用于操作超时控制
				baseCtx := context.Background()
				fetches := consumer.PollFetches(context.Background())
				if fetches.IsClientClosed() {
					log.Error("fetches.IsClientClosed", zap.Error(err))
					continue
				}
				fetches.EachError(func(s string, i int32, err error) {
					log.Error(
						"failed to fetch from kafka",
						zap.Error(err),
						zap.String("topic", TOPIC_REDIS_SET_DEL),
						zap.String("s", s),
						zap.Int32("i", i),
					)
				})

				// 重置批次计数器
				totalRecords := 0        // 总消息数
				var successRecords int64 // 成功处理的消息数
				var failedRecords int64  // 处理失败的消息数
				skippedRecords := 0      // 跳过的无效的消息数

				// 用于跟踪本批次处理的消息的偏移量
				offsets := make(map[string]map[int32]kgo.EpochOffset)

				// ---------------------------------------------------------------------
				// 第一阶段：收集所有事件并按时间戳去重，保留每个键的最新操作
				// ---------------------------------------------------------------------

				// 存储每个键的最新操作，实现规则1和规则3
				keyEvents := make(map[string]*event)

				begin := time.Now()
				// 遍历所有分区的消息
				fetches.EachPartition(func(p kgo.FetchTopicPartition) {
					if len(p.Records) == 0 {
						return // 静默跳过空分区
					}

					totalRecords += len(p.Records)

					// 确保为每个主题初始化偏移量映射
					if _, exists := offsets[p.Topic]; !exists {
						offsets[p.Topic] = make(map[int32]kgo.EpochOffset)
					}

					var lastOffset int64 = -1
					for _, record := range p.Records {
						lastOffset = record.Offset // 记录最后一条消息的偏移量

						// 解析事件
						event := new(event)
						if err = json.Unmarshal(record.Value, event); err != nil {
							log.Error(
								"failed to unmarshal event from kafka record",
								zap.Error(err),
								zap.Int64("offset", record.Offset),
							)
							failedRecords++
							continue
						}

						// 获取该 key 的历史最大时间戳
						keyMaxTS, _ := keyMaxTimestamps.Get(event.Key)

						// 规则一：过滤掉时间戳小于该 key 历史最大时间戳的事件
						if event.TS <= keyMaxTS {
							log.Warn(
								"skipping outdated event for key",
								zap.String("key", event.Key),
								zap.Int64("event_ts", event.TS),
								zap.Int64("key_max_ts", keyMaxTS),
								zap.String("op", event.Op.String()),
							)
							skippedRecords++
							continue
						}

						// 规则二: 按时间戳去重：只保留每个键的最新操作
						existingEvent, exists := keyEvents[event.Key]
						if !exists || event.TS > existingEvent.TS {
							keyEvents[event.Key] = event
						}

					}

					// 更新分区偏移量，用于后续可能的手动提交偏移量(可能用不到了)
					if lastOffset >= 0 {
						offsets[p.Topic][p.Partition] = kgo.EpochOffset{
							Offset: lastOffset + 1,
							Epoch:  -1,
						}
					}
				})

				// 如果没有消息需要处理，则继续等待下一批
				if len(keyEvents) == 0 {
					log.Debug(
						"no events to process in this batch",
						zap.Int("total_records", totalRecords),
						zap.Int("skipped_records", skippedRecords),
						zap.Int64("failed_records", failedRecords),
					)
					continue
				}

				// 将map转换为切片，按照时间戳排序
				eventSlice := make([]*event, 0, len(keyEvents))
				for _, event := range keyEvents {
					eventSlice = append(eventSlice, event)
				}

				// 规则三: 严格按照时间戳排序 (从早到晚)
				sort.Slice(eventSlice, func(i, j int) bool {
					return eventSlice[i].TS < eventSlice[j].TS
				})

				// ---------------------------------------------------------------------
				// 第二阶段：按照时间戳顺序执行Redis操作, 操作完后推送 kafka 消息
				// ---------------------------------------------------------------------

				// 记录本批次处理的每个 key 的最大时间戳，用于批处理结束后更新
				batchKeyMaxTS := make(map[string]int64)

				// 批次操作 redis 和 kafka 超时控制
				wg.Add(len(eventSlice))
				for i := range eventSlice {
					evt := eventSlice[i]
					// 更新该 key 在本批次中的最大时间戳
					if ts, exists := batchKeyMaxTS[evt.Key]; !exists || evt.TS > ts {
						batchKeyMaxTS[evt.Key] = evt.TS
					}

					// TODO: 生产环境设置成 Debug 级别
					log.Info("process event", zap.Object("event", evt))

					err = gopool.Submit(func() {
						defer wg.Done()
						switch evt.Op {
						case opSet:
							if evt.SyncToRedis {
								// logger.Info("redis set", zap.Int64("event_ts", evt.TS), zap.String("key", evt.Key), zap.Any("value", evt.Val), zap.Duration("redis_ttl", evt.RedisTTL))
								if err = redisCli.Set(baseCtx, evt.Key, []byte(evt.Val), evt.RedisTTL).Err(); err != nil {
									atomic.AddInt64(&failedRecords, 1)
									log.Error(
										"failed to set redis key",
										zap.Error(err),
										zap.String("key", evt.Key),
										zap.Object("event", evt),
									)
									return
								}
							}
							// 无论是否同步到Redis，都发送完成事件到Kafka
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
							var data []byte
							if data, err = json.Marshal(evtDone); err != nil {
								log.Error(
									"failed to marshal event in redis set",
									zap.Error(err),
									zap.Object("event", evtDone),
								)
								atomic.AddInt64(&failedRecords, 1)
							} else {
								atomic.AddInt64(&successRecords, 1)
								// 同步推送 kafka 消息
								produceRecord := &kgo.Record{Topic: TOPIC_REDIS_DONE, Value: data}
								if err = producer.ProduceSync(baseCtx, produceRecord).FirstErr(); err != nil {
									log.Error(
										"failed to produce redis set done event",
										zap.Error(err),
										zap.Object("event", evtDone),
									)
								}
							}
						case opDel:
							if evt.SyncToRedis {
								if err = redisCli.Del(baseCtx, evt.Key).Err(); err != nil {
									log.Error(
										"failed to del redis key",
										zap.Error(err),
										zap.String("key", evt.Key),
										zap.Object("event", evt),
									)
									atomic.AddInt64(&failedRecords, 1)
									return
								}
							}
							// 无论是否同步到Redis，都发送完成事件到Kafka
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
							var data []byte
							if data, err = json.Marshal(evtDone); err != nil {
								log.Error(
									"failed to marshal event in redis del",
									zap.Error(err),
									zap.Object("event", evtDone),
								)
								atomic.AddInt64(&failedRecords, 1)
							} else {
								atomic.AddInt64(&successRecords, 1)
								// 同步推送 kafka 消息
								produceRecord := &kgo.Record{Topic: TOPIC_REDIS_DONE, Value: data}
								if err = producer.ProduceSync(baseCtx, produceRecord).FirstErr(); err != nil {
									log.Error(
										"failed to produce redis del done event",
										zap.Error(err),
										zap.Object("event", evtDone),
									)
								}
							}
						default:
							log.Warn("unknown operation type", zap.String("op", evt.Op.String()))
						}
					})
					if err != nil {
						log.Error("failed to submit event to gopool", zap.Error(err), zap.Object("event", evt))
					}
				}
				wg.Wait()

				// 批处理完成后，更新每个 key 的最大时间戳
				for key, ts := range batchKeyMaxTS {
					keyMaxTimestamps.Set(key, ts)
				}

				// 记录处理统计信息
				if totalRecords > 0 {
					log.Info(
						"successfully consumed events",
						zap.Int("total", totalRecords),
						zap.Int("deduplicated", len(eventSlice)),
						zap.Int64("success", successRecords),
						zap.Int64("failed", failedRecords),
						zap.Int("skipped", skippedRecords),
						zap.String("costed", util.FormatDurationSmart(time.Since(begin), 2)),
					)
				}

				// 清空 map 和 slice，帮助 GC 自动回收内存
				keyEvents = nil
				eventSlice = nil
				batchKeyMaxTS = nil //nolint:ineffassign,wastedassign

				// // 系统每次重启时，都会从最新的偏移量开始消费, 所以不需要保存偏移量
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
