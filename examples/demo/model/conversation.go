package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// ConversationType identifies the content type handled by a conversation.
type ConversationType string

const (
	ConversationTypeChat  ConversationType = "chat"
	ConversationTypeImage ConversationType = "image"
)

// Conversation demonstrates a database-backed resource with custom service hooks.
type Conversation struct {
	Type ConversationType `json:"type" query:"type"`

	UserID string `json:"user_id" query:"user_id"`
	Title  string `json:"title" query:"title"`

	// Username is returned to clients and is not stored in the database.
	Username string `json:"username,omitempty" gorm:"-"`

	model.Base
}

func (Conversation) Design() {
	Migrate()
	Endpoint("conversations")
	Param("conv")

	Create(func() {
		Service()
	})
	Delete(func() {
		Service()
	})
	Patch(func() {
		Service()
	})
	List(func() {
		Service()
	})
	Get(func() {})
}

func (Conversation) Purge() bool { return true }
