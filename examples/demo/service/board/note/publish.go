package note

import (
	"net/http"
	"time"

	"demo/model/board"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
)

// Publish serves POST /api/board/notes/:id/publish and the PublishNote rpc:
// a custom action with request and response types of its own. The note's
// id is a route parameter over HTTP and a request field over gRPC, and
// ctx.Param reads it either way.
type Publish struct {
	service.Base[*board.Note, *board.NotePublishReq, *board.NotePublishRsp]
}

// Create marks the note published on the channel, now.
func (p *Publish) Create(ctx *gst.ServiceContext, req *board.NotePublishReq) (*board.NotePublishRsp, error) {
	if req.Channel == "" {
		return nil, service.NewError(http.StatusBadRequest, "channel is required")
	}
	note := new(board.Note)
	if err := database.Database[*board.Note](ctx).Get(note, ctx.Param("id")); err != nil {
		return nil, service.NewErrorWithCause(http.StatusNotFound, "note not found", err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	note.Published = true
	note.PublishedAt = &now
	if err := database.Database[*board.Note](ctx).Update(note); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to publish the note", err)
	}
	return &board.NotePublishRsp{ID: note.ID, Channel: req.Channel, PublishedAt: now}, nil
}
