package bench

import (
	"bench/model/bench"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*bench.Bench, *bench.Bench, *bench.Bench]
}

func (b *Lister) ListBefore(ctx *types.ServiceContext, benches *[]*bench.Bench) error {
	return nil
}

func (b *Lister) ListAfter(ctx *types.ServiceContext, benches *[]*bench.Bench) error {
	return nil
}
