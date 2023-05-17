package bot

import "github.com/sashabaranov/go-openai"

const systemPrompt = "You are a chatbot that helps people by responding to their questions with short messages."

type Message struct {
	EventID   string
	Role      string
	Content   string
	ReplyToID string
}

type Conversation struct {
	Messages []Message
}

func NewConversation(question string) *Conversation {
	return &Conversation{
		Messages: []Message{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: question,
			},
		},
	}
}

func (c *Conversation) Contains(EventID string) bool {
	for _, m := range c.Messages {
		if m.EventID == EventID {
			return true
		}
	}

	return false
}

func (c *Conversation) Add(msg Message) {
	c.Messages = append(c.Messages, msg)
}

type Conversations []*Conversation

func (cs Conversations) Contains(EventID string) bool {
	for _, c := range cs {
		if c.Contains(EventID) {
			return true
		}
	}

	return false
}

func (cs Conversations) Add(msg Message) {
	for _, c := range cs {
		if c.Contains(msg.EventID) {
			c.Add(msg)
			return
		}
	}

	c := NewConversation(msg.Content)
	cs = append(cs, c)
}
