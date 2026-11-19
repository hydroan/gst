package model

import (
	"demo/model/ticket"

	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// TicketStatus tells whether a ticket is still open.
type TicketStatus string

const (
	TicketStatusOpen   TicketStatus = "open"
	TicketStatusClosed TicketStatus = "closed"
)

// Ticket is a request for help a user files and follows up on: it has a
// title, is open or closed, and belongs to the user who filed it. It is a
// table-backed resource with the standard actions on /api/tickets and
// /api/tickets/:ticket. Every action declares Service() for the hooks in
// service/ticket, which stamp the owner, write the audit trail and keep a
// list, a read and every write to the caller's own tickets: reading,
// updating, patching or deleting someone else's ticket answers 404, as if it
// were not there. Embedding model.Query lets a list
// take the framework's query parameters: field[op]=value filters such as
// status[in]=open,closed, _sort_by, _page and _size, and cursor pagination.
// The binding tags validate a body before any hook runs, a 400 naming the
// field. The summary route counts the tickets by status in service code of
// its own, the search route is a custom List reading those parameters
// itself, and the trash route lists the tickets the caller deleted.
type Ticket struct {
	Status TicketStatus `json:"status" query:"status" binding:"omitempty,oneof=open closed"`
	Title  string       `json:"title" query:"title" binding:"max=64"`
	// UserID is the owner, taken from the session when the ticket is created.
	UserID string `json:"user_id" query:"user_id" gorm:"size:36;not null"`
	// Username is the owner's name, filled in for the client; not a column.
	Username string `json:"username,omitempty" gorm:"-"`
	// Comments are the ticket's comments, loaded when a request asks for them:
	// Expands names the association and ?_expand=comments has the framework
	// preload it on a list or a get. constraint:- keeps the migration from
	// adding a foreign key, the preload resolving the relation in Go.
	Comments []*ticket.Comment `json:"comments,omitempty" gorm:"foreignKey:TicketID;constraint:-"`
	// Version protects concurrent edits: a client sends back the version it
	// read, an update that carries a stale one answers 409 and one that
	// carries none answers 400. A create sets it to 1, and a successful
	// update or patch bumps it.
	Version model.Version `json:"version,omitempty" gorm:"not null;default:1"`

	model.Query
	model.Base
}

func (Ticket) TableName() string      { return "tickets" }
func (Ticket) Indexes() []model.Index { return []model.Index{{Fields: []string{"UserID"}}} } // the owner filter of every list relies on it
func (Ticket) Expands() []string      { return []string{"Comments"} }                        // the associations a request may ask for

func (Ticket) Design() {
	Migrate()
	Endpoint("tickets")
	Param("ticket")

	Create(func() {
		Service()
	})
	Delete(func() {
		Service()
	})
	Update(func() {
		Service()
	})
	Patch(func() {
		Service()
	})
	List(func() {
		Service()
	})
	Get(func() {
		Service()
	})

	Route("tickets/summary", func() {
		List(func() {
			Service("summary")
			Result[*TicketSummaryRsp]()
		})
	})
	Route("tickets/search", func() {
		List(func() {
			Service("search")
			Result[*TicketSearchRsp]()
		})
	})
	Route("tickets/trash", func() {
		List(func() {
			Service("trash")
			Result[*TicketTrashRsp]()
		})
	})
}

type (
	// TicketSummaryRsp counts the caller's tickets, in all and by status.
	TicketSummaryRsp struct {
		Total    int                  `json:"total"`
		ByStatus map[TicketStatus]int `json:"by_status"`
	}

	// TicketSearchRsp is a page of the caller's tickets and how many match.
	TicketSearchRsp struct {
		Items []*Ticket `json:"items"`
		Total int       `json:"total"`
	}

	// TicketTrashRsp is the caller's deleted tickets and how many there are:
	// the same shape as TicketSearchRsp, under the trash route's own name.
	TicketTrashRsp = TicketSearchRsp
)
