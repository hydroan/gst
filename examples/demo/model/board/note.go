package board

import (
	"time"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// Note is served over gRPC as well as HTTP: GRPC() has gg gen derive
// pb/board/note.proto from this type and the actions below, compile it and
// generate the gRPC service beside the HTTP routes, both driven by the same
// service code. Every field carries a pb tag numbering it in the message;
// the fields of model.Base take 1 to 10, so a model's own start at 11. The
// standard actions become the rpcs CreateNote, GetNote, ListNote and so on,
// and the publish action PublishNote; over HTTP they are /api/board/notes.
type Note struct {
	Title       string     `json:"title" query:"title" pb:"11"`
	Body        string     `json:"body" pb:"12"`
	Published   bool       `json:"published" query:"published" pb:"13"`
	PublishedAt *time.Time `json:"published_at,omitempty" pb:"14"`

	model.Base
}

func (Note) TableName() string { return "notes" }

// NotePublishReq names the channel a note is published to.
type NotePublishReq struct {
	Channel string `json:"channel" pb:"1"`
}

// NotePublishRsp reports a publication.
type NotePublishRsp struct {
	ID          string    `json:"id" pb:"1"`
	Channel     string    `json:"channel" pb:"2"`
	PublishedAt time.Time `json:"published_at" pb:"3"`
}

func (Note) Design() {
	GRPC()
	Migrate()
	Endpoint("notes")

	Create(func() {
		Service()
	})
	Delete(func() {})
	Update(func() {})
	Patch(func() {})
	List(func() {})
	Get(func() {})

	Route("board/notes/:id/publish", func() {
		Create(func() {
			Service("publish")
			Payload[*NotePublishReq]()
			Result[*NotePublishRsp]()
		})
	})
}
