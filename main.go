package main

import (
	"context"
	"fmt"
	"github.com/matrix-org/gomatrix"
	"github.com/russross/blackfriday/v2"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"os"
)

func main() {
	MatrixHomeserver, ok := os.LookupEnv("MATRIX_HOMESERVER")
	if !ok {
		slog.Error("MATRIX_HOME_SERVER is not set")
		os.Exit(1)
	}
	MatrixUserID, ok := os.LookupEnv("MATRIX_USER_ID")
	if !ok {
		slog.Error("MATRIX_USER_ID is not set")
		os.Exit(1)
	}
	MatrixAccessKey, ok := os.LookupEnv("MATRIX_ACCESS_KEY")
	if !ok {
		slog.Error("MATRIX_ACCESS_KEY is not set")
		os.Exit(1)
	}
	OpenaiAPIKey, ok := os.LookupEnv("OPENAI_API_KEY")
	if !ok {
		slog.Error("OPENAI_API_KEY is not set")
		os.Exit(1)
	}

	// Create new OpenAI client
	openaiClient := openai.NewClient(OpenaiAPIKey)

	matrixClient, _ := gomatrix.NewClient(MatrixHomeserver, MatrixUserID, MatrixAccessKey)

	syncer := matrixClient.Syncer.(*gomatrix.DefaultSyncer)
	syncer.OnEventType("m.room.member", func(ev *gomatrix.Event) {
		if ev.Content["membership"] == "invite" && *ev.StateKey == MatrixUserID {
			_, err := matrixClient.JoinRoom(ev.RoomID, "", nil)
			if err != nil {
				fmt.Println("Failed to join room:", err)
			}
		}
	})

	syncer.OnEventType("m.room.message", func(ev *gomatrix.Event) {
		if ev.Sender != MatrixUserID {
			msgBody, _ := ev.Body()

			// Generate a message with OpenAI API
			openAiResp, err := openaiClient.CreateChatCompletion(
				context.Background(),
				openai.ChatCompletionRequest{
					Model: openai.GPT4,
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleSystem,
							Content: "You are a chatbot that helps people by responding to their questions with short messages.",
						},

						{
							Role:    openai.ChatMessageRoleUser,
							Content: msgBody,
						},
					},
				})

			if err != nil {
				fmt.Println("OpenAI API returned with ", err)
				return
			}

			// Send the OpenAI response back to the chat
			responseText := openAiResp.Choices[len(openAiResp.Choices)-1].Message.Content
			formattedResponse := blackfriday.Run([]byte(responseText))
			_, _ = matrixClient.SendFormattedText(ev.RoomID, responseText, string(formattedResponse))
		}
	})

	for {
		if err := matrixClient.Sync(); err != nil {
			fmt.Println("Sync() returned with ", err)
		}
	}
}
