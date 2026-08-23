package serviceiamsession

import (
	"net/http"

	"github.com/cockroachdb/errors"
	modeliamsession "github.com/hydroan/gst/internal/model/iam/session"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

// AdminSessionGetService handles retrieval of a specified session for privileged administrators.
type AdminSessionGetService struct {
	service.Base[*modeliamsession.AdminSession, *model.Empty, *modeliamsession.AdminSessionGetRsp]
}

// Get returns the detail of a specified session for a privileged administrator.
func (a *AdminSessionGetService) Get(ctx *types.ServiceContext, req *model.Empty) (rsp *modeliamsession.AdminSessionGetRsp, err error) {
	currentSessionID, _, err := CurrentSession(ctx)
	if err != nil {
		return nil, err
	}
	if err = ensureAdminSessionActor(ctx); err != nil {
		return nil, err
	}

	targetSessionID := ctx.Param("id")
	if targetSessionID == "" {
		return nil, service.NewError(http.StatusBadRequest, "session id is required")
	}

	targetSession, err := Store.LoadSession(ctx, targetSessionID)
	if err != nil {
		if errors.Is(err, types.ErrEntryNotFound) {
			return nil, service.NewError(http.StatusNotFound, "session not found")
		}
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load target session", err)
	}
	if err = ValidateSession(targetSessionID, targetSession); err != nil {
		_, _ = Store.DeleteSession(ctx, targetSessionID)
		return nil, service.NewError(http.StatusNotFound, "session not found")
	}

	return &modeliamsession.AdminSessionGetRsp{
		Session: buildSessionView(targetSession, currentSessionID),
	}, nil
}
