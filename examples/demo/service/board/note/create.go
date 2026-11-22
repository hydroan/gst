package note

import (
	"strings"

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

// CreateBefore trims the title the note arrives with. It has nothing to
// clear of the publication a request may name: the model declares those
// fields read-only, so the framework dropped them before this hook ran.
func (n *Creator) CreateBefore(_ *gst.ServiceContext, note *board.Note) error {
	note.Title = strings.TrimSpace(note.Title)
	return nil
}
