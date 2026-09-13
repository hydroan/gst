package serviceiamprofile

import (
	"net/http"

	"github.com/hydroan/gst/database"
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"gorm.io/datatypes"
)

// Column references for the patch writes below; module sources carry no
// generated Cols vars, so the references are declared here.
var (
	colProfileDisplayName = types.NewColumn[*modeliamprofile.Profile, string]("display_name")
	colProfileAvatar      = types.NewColumn[*modeliamprofile.Profile, string]("avatar")
	colProfileMetadata    = types.NewColumn[*modeliamprofile.Profile, datatypes.JSONMap]("metadata")
)

func loadProfileByUserID(ctx *types.ServiceContext, userID string) (*modeliamprofile.Profile, bool, error) {
	profiles := make([]*modeliamprofile.Profile, 0, 1)
	if err := database.Database[*modeliamprofile.Profile](ctx).
		WithLimit(1).
		WithQuery(&modeliamprofile.Profile{UserID: userID}).
		List(&profiles); err != nil {
		return nil, false, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load profile", err)
	}
	if len(profiles) == 0 {
		return nil, false, nil
	}
	return profiles[0], true, nil
}

func applyProfilePatch(record *modeliamprofile.Profile, req *modeliamprofile.ProfilePatchReq) []types.Assignment {
	if record == nil || req == nil {
		return nil
	}

	assignments := make([]types.Assignment, 0, 3)
	if req.DisplayName != nil {
		record.DisplayName = *req.DisplayName
		assignments = append(assignments, colProfileDisplayName.Set(record.DisplayName))
	}
	if req.Avatar != nil {
		record.Avatar = *req.Avatar
		assignments = append(assignments, colProfileAvatar.Set(record.Avatar))
	}
	if req.Metadata != nil {
		record.Metadata = req.Metadata
		assignments = append(assignments, colProfileMetadata.Set(record.Metadata))
	}
	return assignments
}
