package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/rs/zerolog"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
	"os"
	"os/signal"
	"sync"
	"time"
)

func main() {
	MatrixHomeserver, ok := os.LookupEnv("MATRIX_HOMESERVER")
	if !ok {
		slog.Error("MATRIX_HOME_SERVER is not set")
		os.Exit(1)
	}
	MatrixUserIDStr, ok := os.LookupEnv("MATRIX_USER_ID")
	if !ok {
		slog.Error("MATRIX_USER_ID is not set")
		os.Exit(1)
	}
	MatrixUserID := id.UserID(MatrixUserIDStr)
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

	//matrixClient, _ := mautrix.NewClient(MatrixHomeserver, MatrixUserID, MatrixAccessKey)
	//
	//syncer := matrixClient.Syncer.(*mautrix.DefaultSyncer)
	//syncer.OnEventType(event.StateMember, func(source mautrix.EventSource, evt *event.Event) {
	//	if evt.Content["membership"] == "invite" && *ev.StateKey == MatrixUserID {
	//		_, err := matrixClient.JoinRoom(ev.RoomID, "", nil)
	//		if err != nil {
	//			fmt.Println("Failed to join room:", err)
	//		}
	//	}
	//})
	//
	//syncer.OnEventType("m.room.message", func(ev *gomatrix.Event) {
	//	if ev.Sender != MatrixUserID {
	//		msgBody, _ := ev.Body()
	//
	//		// Generate a message with OpenAI API
	//		openAiResp, err := openaiClient.CreateChatCompletion(
	//			context.Background(),
	//			openai.ChatCompletionRequest{
	//				Model: openai.GPT4,
	//				Messages: []openai.ChatCompletionMessage{
	//					{
	//						Role:    openai.ChatMessageRoleSystem,
	//						Content: "You are a chatbot that helps people by responding to their questions with short messages.",
	//					},
	//
	//					{
	//						Role:    openai.ChatMessageRoleUser,
	//						Content: msgBody,
	//					},
	//				},
	//			})
	//
	//		if err != nil {
	//			fmt.Println("OpenAI API returned with ", err)
	//			return
	//		}
	//
	//		// Send the OpenAI response back to the chat
	//		responseText := openAiResp.Choices[len(openAiResp.Choices)-1].Message.Content
	//		formattedResponse := blackfriday.Run([]byte(responseText))
	//		_, _ = matrixClient.SendFormattedText(ev.RoomID, responseText, string(formattedResponse))
	//	}
	//})
	//
	//for {
	//	if err := matrixClient.Sync(); err != nil {
	//		fmt.Println("Sync() returned with ", err)
	//	}
	//}

	client, err := mautrix.NewClient(MatrixHomeserver, MatrixUserID, MatrixAccessKey)
	if err != nil {
		panic(err)
	}

	var oei mautrix.OldEventIgnorer
	oei.Register(client.Syncer.(mautrix.ExtensibleSyncer))

	rl, err := readline.New("[no room]> ")
	if err != nil {
		panic(err)
	}
	defer rl.Close()
	log := zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = rl.Stdout()
		w.TimeFormat = time.Stamp
	})).With().Timestamp().Logger()
	//if !*debug {
	//	log = log.Level(zerolog.InfoLevel)
	//}
	client.Log = log

	var lastRoomID id.RoomID

	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	syncer.OnEventType(event.EventMessage, func(source mautrix.EventSource, evt *event.Event) {
		lastRoomID = evt.RoomID
		rl.SetPrompt(fmt.Sprintf("%s> ", lastRoomID))
		log.Info().
			Str("sender", evt.Sender.String()).
			Str("type", evt.Type.String()).
			Str("id", evt.ID.String()).
			Str("body", evt.Content.AsMessage().Body).
			Msg("Received message")

		if evt.Sender != MatrixUserID {
			msgBody := evt.Content.AsMessage().Body

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
			responseMarkdown := openAiResp.Choices[len(openAiResp.Choices)-1].Message.Content
			responseMessage := format.RenderMarkdown(responseMarkdown, true, false)
			client.SendMessageEvent(lastRoomID, event.EventMessage, &responseMessage)
		}

	})
	syncer.OnEventType(event.StateMember, func(source mautrix.EventSource, evt *event.Event) {
		if evt.GetStateKey() == client.UserID.String() && evt.Content.AsMember().Membership == event.MembershipInvite {
			_, err := client.JoinRoomByID(evt.RoomID)
			if err == nil {
				lastRoomID = evt.RoomID
				rl.SetPrompt(fmt.Sprintf("%s> ", lastRoomID))
				log.Info().
					Str("room_id", evt.RoomID.String()).
					Str("inviter", evt.Sender.String()).
					Msg("Joined room after invite")
			} else {
				log.Error().Err(err).
					Str("room_id", evt.RoomID.String()).
					Str("inviter", evt.Sender.String()).
					Msg("Failed to join room after invite")
			}
		}
	})

	//cryptoHelper, err := cryptohelper.NewCryptoHelper(client, []byte("meow"), "mautrix-example.db")
	//if err != nil {
	//	panic(err)
	//}
	//
	//// You can also store the user/device IDs and access token and put them in the client beforehand instead of using LoginAs.
	////client.UserID = "..."
	////client.DeviceID = "..."
	////client.AccessToken = "..."
	//// You don't need to set a device ID in LoginAs because the crypto helper will set it for you if necessary.
	////cryptoHelper.LoginAs = &mautrix.ReqLogin{
	////	Type:       mautrix.AuthTypePassword,
	////	Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: *username},
	////	Password:   *password,
	////}
	//// If you want to use multiple clients with the same DB, you should set a distinct database account ID for each one.
	////cryptoHelper.DBAccountID = ""
	//err = cryptoHelper.Init()
	//if err != nil {
	//	panic(err)
	//}
	//// Set the client crypto helper in order to automatically encrypt outgoing messages
	//client.Crypto = cryptoHelper

	log.Info().Msg("Now running")
	syncCtx, cancelSync := context.WithCancel(context.Background())
	var syncStopWait sync.WaitGroup
	syncStopWait.Add(1)

	go func() {
		err = client.SyncWithContext(syncCtx)
		defer syncStopWait.Done()
		if err != nil && !errors.Is(err, context.Canceled) {
			panic(err)
		}
	}()

	done := make(chan os.Signal)
	signal.Notify(done, os.Interrupt)
	<-done

	//for {
	//	line, err := rl.Readline()
	//	if err != nil { // io.EOF
	//		break
	//	}
	//	if lastRoomID == "" {
	//		log.Error().Msg("Wait for an incoming message before sending messages")
	//		continue
	//	}
	//	resp, err := client.SendText(lastRoomID, line)
	//	if err != nil {
	//		log.Error().Err(err).Msg("Failed to send event")
	//	} else {
	//		log.Info().Str("event_id", resp.EventID.String()).Msg("Event sent")
	//	}
	//}
	cancelSync()
	syncStopWait.Wait()
	//err = cryptoHelper.Close()
	//if err != nil {
	//	log.Error().Err(err).Msg("Error closing database")
	//}
}
