package note_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model/board"
	pbboard "demo/pb/board"

	"github.com/stretchr/testify/require"
)

// TestPublish covers POST /api/board/notes/:id/publish and the PublishNote
// rpc, served by Publish in publish.go: the note becomes published on the
// channel, over either transport.
func TestPublish(t *testing.T) {
	account := testsupport.Login(t)
	note, err := account.Client.Post[board.Note](t.Context(), "/api/board/notes", &board.Note{Title: "to publish"})
	require.NoError(t, err)

	t.Run("over HTTP", func(t *testing.T) {
		rsp, err := account.Client.Post[board.NotePublishRsp](t.Context(), "/api/board/notes/"+note.ID+"/publish", &board.NotePublishReq{Channel: "news"})
		require.NoError(t, err)
		require.Equal(t, note.ID, rsp.ID)
		require.Equal(t, "news", rsp.Channel)

		published, err := account.Client.Get[board.Note](t.Context(), "/api/board/notes/"+note.ID)
		require.NoError(t, err)
		require.True(t, published.Published)
		require.NotNil(t, published.PublishedAt)
		require.Equal(t, rsp.PublishedAt, *published.PublishedAt)
	})

	t.Run("over gRPC", func(t *testing.T) {
		notes := pbboard.NewNoteServiceClient(testsupport.Dial(t))
		ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))

		rsp, err := notes.PublishNote(ctx, &pbboard.PublishNoteRequest{Id: note.ID, Payload: &pbboard.NotePublishReq{Channel: "wire"}})
		require.NoError(t, err)
		require.Equal(t, note.ID, rsp.GetResult().GetId())
		require.Equal(t, "wire", rsp.GetResult().GetChannel())
		require.True(t, rsp.GetResult().GetPublishedAt().IsValid())
	})
}
