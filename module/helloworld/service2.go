package helloworld

import "github.com/hydroan/gst/types"

func (s *Service2) CreateBefore(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 create before")
	hw.Before = "hello world 2 create before"

	return nil
}

func (s *Service2) CreateAfter(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 create after")
	hw.After = "hello world 2 create after"

	return nil
}

func (s *Service2) DeleteBefore(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 delete before")
	hw.Before = "hello world 2 delete before"

	return nil
}

func (s *Service2) DeleteAfter(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 delete after")
	hw.After = "hello world 2 delete after"

	return nil
}

func (s *Service2) UpdateBefore(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 update before")
	hw.Before = "hello world 2 update before"

	return nil
}

func (s *Service2) UpdateAfter(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 update after")
	hw.After = "hello world 2 update after"

	return nil
}

func (s *Service2) PatchBefore(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 patch before")
	hw.Before = "hello world 2 patch before"

	return nil
}

func (s *Service2) PatchAfter(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 patch after")
	hw.After = "hello world 2 patch after"

	return nil
}

func (s *Service2) ListBefore(ctx *types.ServiceContext, hws *[]*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 list before")
	// no effected.
	for _, hw := range *hws {
		hw.Before = "hello world 2 list before"
	}

	return nil
}

func (s *Service2) ListAfter(ctx *types.ServiceContext, hws *[]*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 list after")
	for _, hw := range *hws {
		hw.Before = "hello world 2 list before"
		hw.After = "hello world 2 list after"
	}

	return nil
}

func (s *Service2) GetBefore(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 get before")
	// no effected
	hw.Before = "hello world 2 get before"

	return nil
}

func (s *Service2) GetAfter(ctx *types.ServiceContext, hw *Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 get after")
	hw.Before = "hello world 2 get before"
	hw.After = "hello world 2 get after"

	return nil
}

func (s *Service2) CreateManyBefore(ctx *types.ServiceContext, hws ...*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 batch create before")
	for _, hw := range hws {
		hw.Before = "hello world 2 batch create before"
	}

	return nil
}

func (s *Service2) CreateManyAfter(ctx *types.ServiceContext, hws ...*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 batch create after")
	for _, hw := range hws {
		hw.After = "hello world 2 batch create after"
	}

	return nil
}

func (s *Service2) DeleteManyBefore(ctx *types.ServiceContext, hws ...*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 batch delete before")
	for _, hw := range hws {
		hw.Before = "hello world 2 batch delete before"
	}

	return nil
}

func (s *Service2) DeleteManyAfter(ctx *types.ServiceContext, hws ...*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 batch delete after")
	for _, hw := range hws {
		hw.After = "hello world 2 batch delete after"
	}

	return nil
}

func (s *Service2) UpdateManyBefore(ctx *types.ServiceContext, hws ...*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 batch update before")
	for _, hw := range hws {
		hw.Before = "hello world 2 batch update before"
	}

	return nil
}

func (s *Service2) UpdateManyAfter(ctx *types.ServiceContext, hws ...*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 batch update after")
	for _, hw := range hws {
		hw.After = "hello world 2 batch update after"
	}

	return nil
}

func (s *Service2) PatchManyBefore(ctx *types.ServiceContext, hws ...*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 batch patch before")
	for _, hw := range hws {
		hw.Before = "hello world 2 batch patch before"
	}

	return nil
}

func (s *Service2) PatchManyAfter(ctx *types.ServiceContext, hws ...*Helloworld2) error {
	log := s.WithContext(ctx, ctx.Phase())

	log.Info("hello world 2 batch patch after")
	for _, hw := range hws {
		hw.After = "hello world 2 batch patch after"
	}

	return nil
}
