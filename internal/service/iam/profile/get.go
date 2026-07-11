package serviceiamprofile

import (
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ProfileGetService handles retrieval of the current user's profile.
type ProfileGetService struct {
	service.Base[*modeliamprofile.Profile, *model.Empty, *modeliamprofile.ProfileGetRsp]
}

// Get returns the current user's profile. Missing profiles are represented by an
// empty profile payload and are not persisted until PATCH.
func (p *ProfileGetService) Get(ctx *types.ServiceContext, req *model.Empty) (rsp *modeliamprofile.ProfileGetRsp, err error) {
	log := p.WithContext(ctx, ctx.Phase())

	_, session, err := serviceiamsession.SessionManager.Current(ctx)
	if err != nil {
		log.Error("failed to get current session", err)
		return nil, err
	}

	record, found, err := loadProfileByUserID(ctx, session.UserID)
	if err != nil {
		log.Error("failed to load profile", err)
		return nil, err
	}
	if !found {
		record = &modeliamprofile.Profile{UserID: session.UserID}
	}

	return record, nil
}
