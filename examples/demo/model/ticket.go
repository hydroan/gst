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
// table-backed resource on /api/tickets and /api/tickets/:ticket, served in
// the two ways a resource is. Filing and changing a ticket are actions with
// request types of their own, which list the fields a client may set and
// nothing else — the owner is the session's, and no body has a field to name
// it — so the services in service/ticket build the row, keep a change to
// the caller's own tickets and answer a stale version with 409. Deleting,
// listing and reading are the framework's own, with a hook each: a list is
// kept to the caller's tickets, and a read or a delete of someone else's
// ticket answers 404, as if it were not there. Embedding model.Query lets a
// list take the framework's query parameters: field[op]=value filters such
// as status[in]=open,closed, _sort_by, _page and _size, and cursor
// pagination. The summary route counts the tickets by status in service
// code of its own, the search route is a custom List reading those
// parameters itself, and the trash route lists the tickets the caller
// deleted.
type Ticket struct {
	Status TicketStatus `json:"status" query:"status"`
	Title  string       `json:"title" query:"title"`
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
	// read, a change that carries a stale one answers 409 and one that
	// carries none answers 400. A create sets it to 1, and a successful
	// change bumps it.
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
		Payload[*TicketCreateReq]()
		Result[*TicketCreateRsp]()
	})
	Delete(func() {
		Service()
	})
	Update(func() {
		Service()
		Payload[*TicketUpdateReq]()
		Result[*TicketUpdateRsp]()
	})
	Patch(func() {
		Service()
		Payload[*TicketPatchReq]()
		Result[*TicketPatchRsp]()
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
	// TicketCreateReq is what a client files a ticket with: its title and,
	// when it is not to start open, its status. The binding tags validate
	// the body before the service runs, a 400 naming the field.
	TicketCreateReq struct {
		Title  string       `json:"title" binding:"required,max=64"`
		Status TicketStatus `json:"status" binding:"omitempty,oneof=open closed"`
	}

	// TicketCreateRsp is the ticket as filed.
	TicketCreateRsp = Ticket

	// TicketUpdateReq replaces what a client may change of a ticket, its
	// title and its status, under the version the client read.
	TicketUpdateReq struct {
		Title   string        `json:"title" binding:"required,max=64"`
		Status  TicketStatus  `json:"status" binding:"required,oneof=open closed"`
		Version model.Version `json:"version,omitempty"`
	}

	// TicketUpdateRsp is the ticket as updated.
	TicketUpdateRsp = Ticket

	// TicketPatchReq changes the fields it names, under the version the
	// client read: a pointer tells a field the body leaves out, which stays
	// as stored, from one it names. A title named empty is refused, as a
	// ticket is never filed without one.
	TicketPatchReq struct {
		Title   *string       `json:"title" binding:"omitempty,min=1,max=64"`
		Status  *TicketStatus `json:"status" binding:"omitempty,oneof=open closed"`
		Version model.Version `json:"version,omitempty"`
	}

	// TicketPatchRsp is the ticket as patched.
	TicketPatchRsp = Ticket

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
