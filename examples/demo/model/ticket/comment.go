package ticket

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// CommentKind tells which side of the conversation under a ticket a comment
// is.
type CommentKind string

const (
	CommentKindQuestion CommentKind = "question"
	CommentKindAnswer   CommentKind = "answer"
)

// Comment is one message of the conversation under a ticket, a question or
// an answer. It is a child resource of Ticket: model/ticket/comment.go sits
// in the directory named after model/ticket.go, so its routes nest under
// the parent's, /api/tickets/:ticket/comments. The nesting is the URL's;
// what the parent means to a comment is the service's: Create takes the
// parent from the route and List keeps to it, in service/ticket/comment,
// while the other actions are the framework's own. The batch actions,
// declared on a route of their own, take and answer lists of comments at
// /api/comments/batch, and the summary route counts the caller's comments
// by the status of their ticket, a query joining the parent.
type Comment struct {
	TicketID string      `json:"ticket_id" query:"ticket_id"`
	Kind     CommentKind `json:"kind" query:"kind"`
	Content  string      `json:"content" gorm:"type:text"`

	model.Base
}

func (Comment) TableName() string { return "comments" }

func (Comment) Design() {
	Migrate()
	Endpoint("comments")

	Create(func() {
		Service()
	})
	Delete(func() {})
	Update(func() {})
	Patch(func() {})
	List(func() {
		Service()
	})
	Get(func() {})

	Route("comments", func() {
		CreateMany(func() {})
		DeleteMany(func() {})
		UpdateMany(func() {})
		PatchMany(func() {})
	})
	Route("comments/summary", func() {
		List(func() {
			Service("summary")
			Result[*CommentSummaryRsp]()
		})
	})
}

// CommentSummaryRsp counts the caller's comments by the status of their
// ticket.
type CommentSummaryRsp struct {
	ByTicketStatus map[string]int `json:"by_ticket_status"`
}
