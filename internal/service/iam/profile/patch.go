package serviceiamprofile

import (
	"net/http"

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
	_, session, err := serviceiamsession.CurrentSession(ctx)
	if err != nil {
		return nil, err
	}

	record, found, err := loadProfileByUserID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if !found {
		record = &modeliamprofile.Profile{UserID: session.UserID}
		applyProfilePatch(record, req)
		if err = database.Database[*modeliamprofile.Profile](ctx).Create(record); err != nil {
			return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to create profile", err)
		}
		return record, nil
	}

	columns := applyProfilePatch(record, req)
	if len(columns) == 0 {
		return record, nil
	}
	if err = updateProfileColumns(ctx, record, columns); err != nil {
		return nil, err
	}

	return record, nil
}
