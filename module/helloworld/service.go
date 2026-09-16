package helloworld

import (
	"github.com/hydroan/gst"
)

var counter = 0

func (s *Service) Create(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module create")
	defer func() {
		counter++
	}()

	return &Rsp{
		Field3: "create hello world",
		Field4: counter,
	}, nil
}

func (s *Service) Delete(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module delete")
	defer func() {
		counter--
	}()

	return &Rsp{
		Field3: "delete hello world",
		Field4: counter,
	}, nil
}

func (s *Service) Update(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module update")
	counter = req.Field2

	return &Rsp{
		Field3: "update hello world",
		Field4: counter,
	}, nil
}

func (s *Service) Patch(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module patch")
	counter = req.Field2

	return &Rsp{
		Field3: "patch hello world",
		Field4: counter,
	}, nil
}

func (s *Service) List(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module list")

	return &Rsp{
		Field3: "list hello world",
		Field4: counter,
	}, nil
}

func (s *Service) Get(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module get")

	return &Rsp{
		Field3: "get hello world",
		Field4: counter,
	}, nil
}

func (s *Service) CreateMany(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module many creator")
	counter += req.Field2 * req.Field2

	return &Rsp{
		Field3: "batch create hello world",
		Field4: counter,
	}, nil
}

func (s *Service) DeleteMany(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module many deleter")
	counter -= req.Field2 * req.Field2

	return &Rsp{
		Field3: "batch delete hello world",
		Field4: counter,
	}, nil
}

func (s *Service) UpdateMany(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module many updater")
	counter = req.Field2

	return &Rsp{
		Field3: "batch update hello world",
		Field4: counter,
	}, nil
}

func (s *Service) PatchMany(ctx *gst.ServiceContext, req *Req) (*Rsp, error) {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("helloworld module many patcher")
	counter = req.Field2

	return &Rsp{
		Field3: "batch patch hello world",
		Field4: counter,
	}, nil
}
