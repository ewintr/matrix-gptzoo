package bot

import (
	"github.com/sashabaranov/go-openai"
	"maunium.net/go/mautrix/id"
)

const systemPrompt = "You are a chatbot that helps people by responding to their questions with short messages."

type Message struct {
	EventID  id.EventID
	Role     string
	Content  string
	ParentID id.EventID
}

type Conversation struct {
	Messages []Message
}

func NewConversation(id id.EventID, question string) *Conversation {
	return &Conversation{
		Messages: []Message{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				EventID: id,
				Role:    openai.ChatMessageRoleUser,
				Content: question,
			},
		},
	}
}

func (c *Conversation) Contains(EventID id.EventID) bool {
	for _, m := range c.Messages {
		if m.EventID.String() == EventID.String() {
			return true
		}
	}

	return false
}

func (c *Conversation) Add(msg Message) {
	c.Messages = append(c.Messages, msg)
}

type Conversations []*Conversation

func (cs Conversations) FindByEventID(EventID id.EventID) *Conversation {
	for _, c := range cs {
		if c.Contains(EventID) {
			return c
		}
	}

	return nil
}
