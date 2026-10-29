package preference

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"demo/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
)

// Set answers POST /api/preferences/set: a custom action with request and
// response types of its own, whose write is an upsert.
type Set struct {
	service.Base[*model.Preference, *model.PreferenceSetReq, *model.PreferenceSetRsp]
}

// Create sets the key. The id is derived from the owner and the key, so the
// same two name the same row and Upsert merges on the primary key: a key
// set again has its value replaced in place and the row keeps its id. A
// primary-key collision is the one Upsert merges on every dialect; on MySQL
// a collision on the unique index would merge too, on PostgreSQL and SQLite
// it would fail (see database.Upsert). Upsert runs no model hook, which is
// why the owner is stamped here.
func (s *Set) Create(ctx *gst.ServiceContext, req *model.PreferenceSetReq) (*model.PreferenceSetRsp, error) {
	preference := &model.Preference{UserID: ctx.UserID(), Key: req.Key, Value: req.Value}
	preference.ID = preferenceID(preference.UserID, preference.Key)
	if err := database.Database[*model.Preference](ctx).Upsert(preference); err != nil {
		return nil, gst.NewErrorWithCause(http.StatusInternalServerError, "failed to set the preference", err)
	}
	return preference, nil
}

// preferenceID is the id of the owner's preference under key, the same for
// the same two values: the first 16 bytes of the SHA-256 of the values
// joined by ":", in hex.
func preferenceID(userID, key string) string {
	hash := sha256.Sum256([]byte(userID + ":" + key))
	return hex.EncodeToString(hash[:16])
}
