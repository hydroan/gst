package redis

import (
	"context"
	"reflect"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/types"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// SetM set types.Model into redis with specific key.
func SetM[M types.Model](ctx context.Context, key string, m M, expiration ...time.Duration) error {
	client, err := Client()
	if err != nil {
		return err
	}
	ttl := config.App.Redis.Expiration
	if len(expiration) > 0 {
		ttl = expiration[0]
	}
	return client.Set(ctx, redisKey(key), modelMarshaler[M]{Model: m}, ttl).Err()
}

// SetML set one or multiple types.Model into redis with specific key.
func SetML[M types.Model](ctx context.Context, key string, ml []M, expiration ...time.Duration) error {
	client, err := Client()
	if err != nil {
		return err
	}
	ttl := config.App.Redis.Expiration
	if len(expiration) > 0 {
		ttl = expiration[0]
	}
	bl := make([]modelMarshaler[M], 0)
	for i := range ml {
		bl = append(bl, modelMarshaler[M]{Model: ml[i]})
	}
	return client.Set(ctx, redisKey(key), modelMarshalerList[M](bl), ttl).Err()
}

// GetM will get cache from redis and decode into types.Model.
func GetM[M types.Model](ctx context.Context, key string) (M, error) {
	client, err := Client()
	if err != nil {
		return *new(M), err
	}
	data, err := client.Get(ctx, redisKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return *new(M), ErrKeyNotExists
		}
		zap.S().Error(err)
		return *new(M), err
	}
	typ := reflect.TypeOf(*new(M)).Elem()
	val := reflect.New(typ).Interface().(M) //nolint:errcheck
	if err := json.Unmarshal(data, val); err != nil {
		zap.S().Error(err)
		return *new(M), err
	}
	return val, nil
}

// GetML will get cache from redis and decode into []types.Model.
func GetML[M types.Model](ctx context.Context, key string) ([]M, error) {
	client, err := Client()
	if err != nil {
		return nil, err
	}
	data, err := client.Get(ctx, redisKey(key)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrKeyNotExists
		}
		zap.S().Error(err)
		return nil, err
	}

	dest := make([]modelMarshaler[M], 0)
	if err := json.Unmarshal(data, &dest); err != nil {
		zap.S().Error(err)
		return nil, err
	}
	ml := make([]M, 0)
	for i := range dest {
		ml = append(ml, dest[i].Model)
	}
	return ml, nil
}

// modelMarshaler adapts one types.Model to the encoding the client stores
// values with.
//
// The receiver of MarshalBinary and UnmarshalBinary must not be a pointer,
// otherwise redis reports:
//
//	redis: can't marshal redis.modelMarshaler[*myproject/model.Sample] (implement encoding.BinaryMarshaler)
//
// The receiver of MarshalJSON and UnmarshalJSON must be a pointer, otherwise
// it panics.
type modelMarshaler[M types.Model] struct {
	Model M
}

func (b modelMarshaler[M]) MarshalBinary() ([]byte, error) {
	return json.Marshal(b.Model)
}

func (b *modelMarshaler[M]) UnmarshalJSON(data []byte) error {
	if reflect.DeepEqual(b.Model, *new(M)) {
		b.Model = reflect.New(reflect.TypeOf(*new(M)).Elem()).Interface().(M) //nolint:errcheck
	}
	if err := json.Unmarshal(data, &b.Model); err != nil {
		zap.S().Error(err)
		return err
	}
	return nil
}

// modelMarshalerList adapts a slice of types.Model the same way
// modelMarshaler adapts one.
//
// The receiver of MarshalBinary must never be a pointer.
type modelMarshalerList[M types.Model] []modelMarshaler[M]

func (bl modelMarshalerList[M]) MarshalBinary() ([]byte, error) {
	ml := make([]types.Model, len(bl))
	for i := range bl {
		ml[i] = bl[i].Model
	}
	return json.Marshal(ml)
}
