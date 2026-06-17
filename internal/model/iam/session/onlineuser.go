package modeliamsession

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/types"
)

type OnlineUser struct {
	// User Info
	UserID   string `json:"user_id,omitempty" schema:"user_id"`
	Username string `json:"username,omitempty" schema:"username"`
	ClientIP string `json:"client_ip,omitempty" schema:"client_ip"`

	// User Agent info.
	Source   string `json:"source" schema:"source"`
	Platform string `json:"platform" schema:"platform"`
	Engine   string `json:"engine" schema:"engine"`
	Browser  string `json:"browser" schema:"browser"`

	model.Base
}

func (ou *OnlineUser) Purge() bool                                { return true }
func (ou *OnlineUser) CreateBefore(ctx *types.ModelContext) error { return ou.validate(ctx) }
func (ou *OnlineUser) UpdateBefore(ctx *types.ModelContext) error { return ou.validate(ctx) }

func (ou *OnlineUser) validate(_ *types.ModelContext) error {
	// Uniquely identifies an active online user by combining userID, clientIP and source(UserAgent).
	sum := sha256.Sum256(fmt.Appendf(nil, "%s:%s:%s", ou.UserID, ou.ClientIP, ou.Source))
	id := hex.EncodeToString(sum[:])
	ou.SetID(id)

	return nil
}
