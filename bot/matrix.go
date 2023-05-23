package bot

import (
	"time"

	"github.com/chzyer/readline"
	"github.com/rs/zerolog"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
)

type Config struct {
	Homeserver    string
	UserID        string
	UserAccessKey string
	UserPassword  string
	DBPath        string
	Pickle        string
	OpenAIKey     string
}

type Matrix struct {
	config        Config
	readline      *readline.Instance
	client        *mautrix.Client
	cryptoHelper  *cryptohelper.CryptoHelper
	conversations Conversations
	gptClient     *GPT
}

func New(cfg Config) *Matrix {
	return &Matrix{
		config: cfg,
	}
}

func (m *Matrix) Init() error {
	client, err := mautrix.NewClient(m.config.Homeserver, id.UserID(m.config.UserID), m.config.UserAccessKey)
	if err != nil {
		return err
	}
	var oei mautrix.OldEventIgnorer
	oei.Register(client.Syncer.(mautrix.ExtensibleSyncer))
	m.client = client

	m.client.Log = zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.TimeFormat = time.Stamp
	})).With().Timestamp().Logger().Level(zerolog.InfoLevel)

	m.cryptoHelper, err = cryptohelper.NewCryptoHelper(client, []byte(m.config.Pickle), m.config.DBPath)
	if err != nil {
		return err
	}
	m.cryptoHelper.LoginAs = &mautrix.ReqLogin{
		Type:       mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: m.config.UserID},
		Password:   m.config.UserPassword,
	}
	if err := m.cryptoHelper.Init(); err != nil {
		return err
	}
	m.client.Crypto = m.cryptoHelper

	m.gptClient = NewGPT(m.config.OpenAIKey)

	m.conversations = make(Conversations, 0)

	return nil
}

func (m *Matrix) Run() error {
	if err := m.client.Sync(); err != nil {
		return err
	}

	return nil
}

func (m *Matrix) Close() error {
	if err := m.client.Sync(); err != nil {
		return err
	}
	if err := m.cryptoHelper.Close(); err != nil {
		return err
	}

	return nil
}

func (m *Matrix) AddEventHandler(eventType event.Type, handler mautrix.EventHandler) {
	syncer := m.client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(eventType, handler)
}

func (m *Matrix) InviteHandler(logger *slog.Logger) (event.Type, mautrix.EventHandler) {
	return event.StateMember, func(source mautrix.EventSource, evt *event.Event) {
		if evt.GetStateKey() == m.client.UserID.String() && evt.Content.AsMember().Membership == event.MembershipInvite {
			_, err := m.client.JoinRoomByID(evt.RoomID)
			if err != nil {
				logger.Error("failed to join room after invite", slog.String("err", err.Error()), slog.String("room_id", evt.RoomID.String()), slog.String("inviter", evt.Sender.String()))
				return
			}

			logger.Info("Joined room after invite", slog.String("room_id", evt.RoomID.String()), slog.String("inviter", evt.Sender.String()))
		}
	}
}

func (m *Matrix) ResponseHandler(logger *slog.Logger) (event.Type, mautrix.EventHandler) {
	return event.EventMessage, func(source mautrix.EventSource, evt *event.Event) {
		content := evt.Content.AsMessage()
		eventID := evt.ID
		logger.Info("received message", slog.String("content", content.Body))

		conv := m.conversations.FindByEventID(eventID)
		if conv != nil {
			logger.Info("known message, ignoring", slog.String("event_id", eventID.String()))
			return
		}

		parentID := id.EventID("")
		if relatesTo := content.GetRelatesTo(); relatesTo != nil {
			parentID = relatesTo.GetReplyTo()
		}
		if parentID != "" {
			logger.Info("parent found, looking for conversation", slog.String("parent_id", parentID.String()))
			conv = m.conversations.FindByEventID(parentID)
		}
		if conv != nil {
			conv.Add(Message{
				EventID:  eventID,
				ParentID: parentID,
				Role:     openai.ChatMessageRoleUser,
				Content:  content.Body,
			})
			logger.Info("found parent, appending message to conversation", slog.String("event_id", eventID.String()))
		} else {
			conv = NewConversation(eventID, content.Body)
			m.conversations = append(m.conversations, conv)
			logger.Info("no parent found, starting new conversation", slog.String("event_id", eventID.String()))
		}

		if evt.Sender != id.UserID(m.config.UserID) {
			// get reply from GPT
			reply, err := m.gptClient.Complete(conv)
			if err != nil {
				logger.Error("failed to get reply from openai", slog.String("err", err.Error()))
				return
			}

			formattedReply := format.RenderMarkdown(reply, true, false)
			formattedReply.RelatesTo = &event.RelatesTo{
				InReplyTo: &event.InReplyTo{
					EventID: eventID,
				},
			}
			if _, err := m.client.SendMessageEvent(evt.RoomID, event.EventMessage, &formattedReply); err != nil {
				logger.Error("failed to send message", slog.String("err", err.Error()))
				return
			}

			if len(reply) > 30 {
				reply = reply[:30] + "..."
			}
			logger.Info("sent reply", slog.String("parent_id", eventID.String()), slog.String("content", reply))
		}
	}
}
