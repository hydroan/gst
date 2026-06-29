package serviceiamprofile

import (
	"github.com/hydroan/gst/database"
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
	serviceiamsession "github.com/hydroan/gst/internal/service/iam/session"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// ProfileGetService handles retrieval of the current user's profile.
type ProfileGetService struct {
	service.Base[*modeliamprofile.Profile, *modeliamprofile.ProfileGetReq, *modeliamprofile.ProfileGetRsp]
}

// ProfilePatchService handles partial updates to the current user's profile.
type ProfilePatchService struct {
	service.Base[*modeliamprofile.Profile, *modeliamprofile.ProfilePatchReq, *modeliamprofile.ProfilePatchRsp]
}

// Get returns the current user's profile. Missing profiles are represented by an
// empty profile payload and are not persisted until PATCH.
func (p *ProfileGetService) Get(ctx *types.ServiceContext, req *modeliamprofile.ProfileGetReq) (rsp *modeliamprofile.ProfileGetRsp, err error) {
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

func loadProfileByUserID(ctx *types.ServiceContext, userID string) (*modeliamprofile.Profile, bool, error) {
	profiles := make([]*modeliamprofile.Profile, 0, 1)
	if err := database.Database[*modeliamprofile.Profile](ctx).
		WithLimit(1).
		WithQuery(&modeliamprofile.Profile{UserID: userID}).
		List(&profiles); err != nil {
		return nil, false, err
	}
	if len(profiles) == 0 {
		return nil, false, nil
	}
	return profiles[0], true, nil
}

func updateProfileColumns(ctx *types.ServiceContext, record *modeliamprofile.Profile, columns []string) error {
	if record == nil {
		return nil
	}
	return database.Database[*modeliamprofile.Profile](ctx).TransactionFunc(func(tx any) error {
		for _, column := range columns {
			if err := database.Database[*modeliamprofile.Profile](ctx).
				WithTx(tx).
				UpdateByID(record.ID, column, profileColumnValue(record, column)); err != nil {
				return err
			}
		}
		return nil
	})
}

func applyProfilePatch(record *modeliamprofile.Profile, req *modeliamprofile.ProfilePatchReq) []string {
	if record == nil || req == nil {
		return nil
	}

	columns := make([]string, 0, 5)
	if req.DisplayName != nil {
		record.DisplayName = *req.DisplayName
		columns = append(columns, "display_name")
	}
	if req.FirstName != nil {
		record.FirstName = *req.FirstName
		columns = append(columns, "first_name")
	}
	if req.LastName != nil {
		record.LastName = *req.LastName
		columns = append(columns, "last_name")
	}
	if req.Avatar != nil {
		record.Avatar = *req.Avatar
		columns = append(columns, "avatar")
	}
	if req.Metadata != nil {
		record.Metadata = req.Metadata
		columns = append(columns, "metadata")
	}
	return columns
}

func profileColumnValue(record *modeliamprofile.Profile, column string) any {
	switch column {
	case "display_name":
		return record.DisplayName
	case "first_name":
		return record.FirstName
	case "last_name":
		return record.LastName
	case "avatar":
		return record.Avatar
	case "metadata":
		return record.Metadata
	default:
		return nil
	}
}
