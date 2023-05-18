package bot

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

type GPT struct {
	client *openai.Client
}

func NewGPT(apiKey string) *GPT {
	return &GPT{
		client: openai.NewClient(apiKey),
	}
}

func (g GPT) Complete(conv *Conversation) (string, error) {
	ctx := context.Background()
	msg := []openai.ChatCompletionMessage{}
	for _, m := range conv.Messages {
		msg = append(msg, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	req := openai.ChatCompletionRequest{
		Model:    openai.GPT3Dot5Turbo,
		Messages: msg,
	}

	resp, err := g.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Choices[len(resp.Choices)-1].Message.Content, nil
}
