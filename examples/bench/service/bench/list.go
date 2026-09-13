package bench

import (
	"bench/model/bench"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*bench.Bench, *bench.Bench, *bench.Bench]
}

func (b *Lister) ListBefore(ctx *gst.ServiceContext, benches *[]*bench.Bench) error {
	return nil
}

func (b *Lister) ListAfter(ctx *gst.ServiceContext, benches *[]*bench.Bench) error {
	return nil
}
