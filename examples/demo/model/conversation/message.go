package conversation

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

// MessageRole identifies who produced a message.
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
)

// SearchSource describes an external reference attached to a message.
type SearchSource struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// Message demonstrates a child resource with nested routes and batch actions.
type Message struct {
	UserID         string         `json:"user_id" query:"user_id"`
	ConversationID string         `json:"conversation_id" query:"conversation_id"`
	Role           MessageRole    `json:"role" query:"role"`
	Content        string         `json:"content" gorm:"type:text"`
	Sources        []SearchSource `json:"sources,omitempty" gorm:"-"`

	model.Base
}

func (Message) Design() {
	Migrate(true)
	Endpoint("messages")

	Create(func() {
		Service()
	})
	Patch(func() {})
	List(func() {
		Service()
	})
	Get(func() {})

	Route("messages", func() {
		DeleteMany(func() {
			Service()
		})
	})
}

func (Message) Purge() bool { return true }
