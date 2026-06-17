package helloworld

import (
	"github.com/hydroan/gst/types"
)

var counter = 0

func (s *Service) Create(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module create")
	defer func() {
		counter++
	}()

	return &Rsp{
		Field3: "create hello world",
		Field4: counter,
	}, nil
}

func (s *Service) Delete(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module delete")
	defer func() {
		counter--
	}()

	return &Rsp{
		Field3: "delete hello world",
		Field4: counter,
	}, nil
}

func (s *Service) Update(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module update")
	counter = req.Field2

	return &Rsp{
		Field3: "update hello world",
		Field4: counter,
	}, nil
}

func (s *Service) Patch(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module patch")
	counter = req.Field2

	return &Rsp{
		Field3: "patch hello world",
		Field4: counter,
	}, nil
}

func (s *Service) List(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module list")

	return &Rsp{
		Field3: "list hello world",
		Field4: counter,
	}, nil
}

func (s *Service) Get(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module get")

	return &Rsp{
		Field3: "get hello world",
		Field4: counter,
	}, nil
}

func (s *Service) CreateMany(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module many creator")
	counter = counter + req.Field2*req.Field2

	return &Rsp{
		Field3: "batch create hello world",
		Field4: counter,
	}, nil
}

func (s *Service) DeleteMany(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module many deleter")
	counter = counter - req.Field2*req.Field2

	return &Rsp{
		Field3: "batch delete hello world",
		Field4: counter,
	}, nil
}

func (s *Service) UpdateMany(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module many updater")
	counter = req.Field2

	return &Rsp{
		Field3: "batch update hello world",
		Field4: counter,
	}, nil
}

func (s *Service) PatchMany(ctx *types.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithServiceContext(ctx, ctx.GetPhase())

	log.Info("helloworld module many patcher")
	counter = req.Field2

	return &Rsp{
		Field3: "batch patch hello world",
		Field4: counter,
	}, nil
}
