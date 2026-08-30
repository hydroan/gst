package redis_test

import (
	"testing"
	"time"

	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/redis"
)

// Sample is the types.Model the model helpers are exercised with.
type Sample struct {
	Name  string `json:"name,omitempty" query:"name"`
	Desc  string `json:"desc,omitempty" query:"desc"`
	Count int    `json:"count" gorm:"default:0"`

	model.Base
}

func TestModelHelpersRoundtrip(t *testing.T) {
	ctx := t.Context()
	want := &Sample{Name: "sample", Desc: "one", Count: 3}

	if err := redis.SetM(ctx, "redis-test:model", want, time.Minute); err != nil {
		t.Fatalf("setm: %v", err)
	}
	got, err := redis.GetM[*Sample](ctx, "redis-test:model")
	if err != nil {
		t.Fatalf("getm: %v", err)
	}
	if got.Name != want.Name || got.Count != want.Count {
		t.Fatalf("want %+v, got %+v", want, got)
	}

	if err = redis.SetML(ctx, "redis-test:model-list", []*Sample{want}, time.Minute); err != nil {
		t.Fatalf("setml: %v", err)
	}
	list, err := redis.GetML[*Sample](ctx, "redis-test:model-list")
	if err != nil {
		t.Fatalf("getml: %v", err)
	}
	if len(list) != 1 || list[0].Name != want.Name {
		t.Fatalf("want one %q back, got %+v", want.Name, list)
	}
}
