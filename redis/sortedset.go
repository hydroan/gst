package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

// ZAdd adds one or multiple string members with the same score into a sorted set.
func ZAdd(ctx context.Context, key string, score float64, members ...string) error {
	client, err := Client()
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	entries := make([]goredis.Z, 0, len(members))
	for i := range members {
		entries = append(entries, goredis.Z{Score: score, Member: members[i]})
	}
	return client.ZAdd(ctx, Key(key), entries...).Err()
}

// ZRange returns sorted set members in ascending score order.
func ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	client, err := Client()
	if err != nil {
		return nil, err
	}
	return client.ZRange(ctx, Key(key), start, stop).Result()
}

// ZRangeByScore returns sorted set members whose scores are between minScore and maxScore.
func ZRangeByScore(ctx context.Context, key, minScore, maxScore string) ([]string, error) {
	client, err := Client()
	if err != nil {
		return nil, err
	}
	args := goredis.ZRangeArgs{
		Key:     Key(key),
		Start:   minScore,
		Stop:    maxScore,
		ByScore: true,
	}
	return client.ZRangeArgs(ctx, args).Result()
}

// ZRem removes one or multiple members from a sorted set.
func ZRem(ctx context.Context, key string, members ...string) error {
	client, err := Client()
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	memberArgs := make([]any, 0, len(members))
	for i := range members {
		memberArgs = append(memberArgs, members[i])
	}
	return client.ZRem(ctx, Key(key), memberArgs...).Err()
}

// ZRemRangeByScore removes sorted set members whose score is between minScore and maxScore.
func ZRemRangeByScore(ctx context.Context, key, minScore, maxScore string) error {
	client, err := Client()
	if err != nil {
		return err
	}
	return client.ZRemRangeByScore(ctx, Key(key), minScore, maxScore).Err()
}
