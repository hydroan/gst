package redis

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/model"
)

// sampleModel is a minimal types.Model for the model-typed helpers.
type sampleModel struct {
	Name string `json:"name"`

	model.Base
}

func (*sampleModel) TableName() string { return "samples" }

// withoutClient drops the client for the duration of the test while leaving
// the configuration claiming Redis is enabled. That combination is the real
// deployment state after Init failed or Close ran, and it is what separates
// the two properties asserted here: the helpers must answer from the handle
// they actually hold, not from what configuration promises.
func withoutClient(t *testing.T) context.Context {
	t.Helper()

	oldClient, oldEnabled := cli, config.App.Redis.Enabled
	cli = nil
	config.App.Redis.Enabled = true
	t.Cleanup(func() {
		cli = oldClient
		config.App.Redis.Enabled = oldEnabled
	})
	return context.Background()
}

// TestHelpersWithoutClientReportDisabled asserts that every helper reports the
// missing client instead of answering with a zero value a caller reads as
// success, and that none of them dereferences the absent client.
func TestHelpersWithoutClientReportDisabled(t *testing.T) {
	ctx := withoutClient(t)
	sample := &sampleModel{Name: "sample"}

	cases := []struct {
		name string
		call func() error
	}{
		{"Set", func() error { return Set(ctx, "key", "value") }},
		{"SetM", func() error { return SetM(ctx, "key", sample) }},
		{"SetML", func() error { return SetML(ctx, "key", []*sampleModel{sample}) }},
		{"Get", func() error { _, err := Get(ctx, "key"); return err }},
		{"GetInt", func() error { _, err := GetInt(ctx, "key"); return err }},
		{"GetM", func() error { _, err := GetM[*sampleModel](ctx, "key"); return err }},
		{"GetML", func() error { _, err := GetML[*sampleModel](ctx, "key"); return err }},
		{"Del", func() error { return Del(ctx, "key") }},
		{"SetNX", func() error { _, err := SetNX(ctx, "key", "value", time.Minute); return err }},
		{"Expire", func() error { return Expire(ctx, "key", time.Minute) }},
		{"Incr", func() error { _, err := Incr(ctx, "key"); return err }},
		{"TTL", func() error { _, err := TTL(ctx, "key"); return err }},
		{"ZAdd", func() error { return ZAdd(ctx, "key", 1, "member") }},
		{"ZRange", func() error { _, err := ZRange(ctx, "key", 0, -1); return err }},
		{"ZRangeByScore", func() error { _, err := ZRangeByScore(ctx, "key", "-inf", "+inf"); return err }},
		{"ZRem", func() error { return ZRem(ctx, "key", "member") }},
		{"ZRemRangeByScore", func() error { return ZRemRangeByScore(ctx, "key", "-inf", "+inf") }},
		{"RemovePrefix", func() error { return RemovePrefix(ctx, "prefix") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, ErrRedisIsDisabled) {
				t.Fatalf("want ErrRedisIsDisabled, got %v", err)
			}
		})
	}
}

// TestSetNXWithoutClientDoesNotReportAcquired covers the most dangerous shape
// of a silent no-op: SetNX answering true means "the key was mine to write",
// which callers building a lock or a replay marker on it read as exclusive
// access. Without a client, every caller would be told exactly that.
func TestSetNXWithoutClientDoesNotReportAcquired(t *testing.T) {
	ctx := withoutClient(t)

	acquired, err := SetNX(ctx, "key", "value", time.Minute)
	if !errors.Is(err, ErrRedisIsDisabled) {
		t.Fatalf("want ErrRedisIsDisabled, got %v", err)
	}
	if acquired {
		t.Fatal("want acquired false without a client")
	}
}
