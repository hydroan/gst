package note_test

import (
	"testing"

	"demo/internal/testsupport"
	"demo/model/board"
	pbboard "demo/pb/board"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestCreate covers POST /api/board/notes and the CreateNote rpc, served by
// Creator in create.go: a note starts unpublished whatever the request
// says, over either transport, and a gRPC call names its session or is
// refused.
func TestCreate(t *testing.T) {
	t.Run("over HTTP", func(t *testing.T) {
		account := testsupport.Login(t)
		note, err := account.Client.Post[board.Note](t.Context(), "/api/board/notes", &board.Note{Title: "hello", Body: "over http", Published: true})
		require.NoError(t, err)
		require.Equal(t, "hello", note.Title)
		require.False(t, note.Published)
		require.Nil(t, note.PublishedAt)
	})

	t.Run("over gRPC", func(t *testing.T) {
		notes := pbboard.NewNoteServiceClient(testsupport.Dial(t))
		ctx := testsupport.Authorized(t.Context(), testsupport.Session(t))
		rsp, err := notes.CreateNote(ctx, &pbboard.CreateNoteRequest{Note: &pbboard.Note{Title: "hello", Body: "over grpc", Published: true}})
		require.NoError(t, err)
		require.Equal(t, "hello", rsp.GetNote().GetTitle())
		require.False(t, rsp.GetNote().GetPublished())
		require.NotEmpty(t, rsp.GetNote().GetId())
	})

	t.Run("over gRPC without a session", func(t *testing.T) {
		notes := pbboard.NewNoteServiceClient(testsupport.Dial(t))
		_, err := notes.CreateNote(t.Context(), &pbboard.CreateNoteRequest{Note: &pbboard.Note{Title: "hello"}})
		require.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}
