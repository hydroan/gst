package serviceiamprofile

import (
	"github.com/hydroan/gst/database"
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ProfilePatchService handles partial updates to the current user's profile.
type ProfilePatchService struct {
	service.Base[*modeliamprofile.Profile, *modeliamprofile.ProfilePatchReq, *modeliamprofile.ProfilePatchRsp]
}

// Patch creates or updates the current user's profile with only the requested fields.
func (p *ProfilePatchService) Patch(ctx *types.ServiceContext, req *modeliamprofile.ProfilePatchReq) (rsp *modeliamprofile.ProfilePatchRsp, err error) {
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
		applyProfilePatch(record, req)
		if err = database.Database[*modeliamprofile.Profile](ctx).Create(record); err != nil {
			log.Error("failed to create profile", err)
			return nil, err
		}
		return record, nil
	}

	columns := applyProfilePatch(record, req)
	if len(columns) == 0 {
		return record, nil
	}
	if err = updateProfileColumns(ctx, record, columns); err != nil {
		log.Error("failed to update profile", err)
		return nil, err
	}

	return record, nil
}
