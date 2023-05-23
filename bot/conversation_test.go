package bot_test

import (
	"testing"

	"ewintr.nl/matrix-bots/bot"
)

func TestNewConversation(t *testing.T) {
	t.Parallel()

	conv := bot.NewConversation("test", "question")
	if conv == nil {
		t.Error("NewConversation returned nil")
	}
	if len(conv.Messages) != 2 {
		t.Error("NewConversation did not create 2 messages")
	}
	if conv.Messages[1].Content != "question" {
		t.Error("NewConversation did not set question")
	}
}

func TestConversation_Contains(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		conv *bot.Conversation
		exp  bool
	}{
		{
			name: "empty",
			conv: &bot.Conversation{},
			exp:  false,
		},
		{
			name: "not contains",
			conv: &bot.Conversation{
				Messages: []bot.Message{
					{
						EventID: "other",
						Content: "content",
					},
				},
			},
		},
		{
			name: "contains",
			conv: &bot.Conversation{
				Messages: []bot.Message{
					{
						EventID: "id",
						Content: "content",
					},
				},
			},
			exp: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.conv.Contains("id") != tc.exp {
				t.Errorf("expected %v, got %v", tc.exp, tc.conv.Contains("test"))
			}
		})
	}
}

func TestConversation_Add(t *testing.T) {
	conv := &bot.Conversation{}
	conv.Add(bot.Message{
		EventID: "id",
	})
	if !conv.Contains("id") {
		t.Error("Add did not add message")
	}
}
