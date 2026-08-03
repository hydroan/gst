package redis_test

import (
	"fmt"
	"testing"

	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/provider/redis"
)

func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{Redis: true})
}

func BenchmarkRedis(b *testing.B) {
	groups := make([]*Group, 0, 1000)
	for i := range 1000 {
		groups = append(groups, &Group{
			Name:        fmt.Sprintf("group-%d", i),
			Desc:        fmt.Sprintf("desc-%d", i),
			MemberCount: i,
		})
	}

	for b.Loop() {
		if err := redis.SetML(b.Context(), "groups", groups); err != nil {
			b.Fatalf("%+v\n", err)
		}
	}
}

type Group struct {
	Name        string `json:"name,omitempty" query:"name" gorm:"unique" binding:"required"`
	Desc        string `json:"desc,omitempty" query:"desc"`
	MemberCount int    `json:"member_count" gorm:"default:0"`

	model.Base
}
