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
	UserID         string         `json:"user_id" schema:"user_id"`
	ConversationID string         `json:"conversation_id" schema:"conversation_id"`
	Role           MessageRole    `json:"role" schema:"role"`
	Content        string         `json:"content" gorm:"type:text"`
	Sources        []SearchSource `json:"sources,omitempty" gorm:"-"`

	model.Base
}

func (Message) Design() {
	Migrate(true)
	Endpoint("messages")

	Create(func() {
		Service(true)
	})
	Patch(func() {})
	List(func() {
		Service(true)
	})
	Get(func() {})

	Route("messages", func() {
		DeleteMany(func() {
			Service(true)
		})
	})
}

func (Message) Purge() bool { return true }
