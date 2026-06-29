package serviceiamprofile

import (
	"github.com/hydroan/gst/database"
	modeliamprofile "github.com/hydroan/gst/internal/model/iam/profile"
	"github.com/hydroan/gst/types"
)

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
