package note

import (
	"demo/model/board"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

// Creator hooks the framework's own Create of a note, reached as
// POST /api/board/notes and as the CreateNote rpc alike: the hook runs for
// both, the transports differing only in how the note arrives.
type Creator struct {
	service.Base[*board.Note, *board.Note, *board.Note]
}

// CreateBefore has every note start unpublished, whatever the request says;
// publishing is the publish action's.
func (n *Creator) CreateBefore(_ *gst.ServiceContext, note *board.Note) error {
	note.Published = false
	note.PublishedAt = nil
	return nil
}
