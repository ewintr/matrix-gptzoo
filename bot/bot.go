package bot

import (
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
)

type Config struct {
	Homeserver      string
	UserID          string
	UserAccessKey   string
	UserPassword    string
	UserDisplayName string
	DBPath          string
	Pickle          string
	OpenAIKey       string
	SystemPrompt    string
}

type Bot struct {
	config        Config
	client        *mautrix.Client
	cryptoHelper  *cryptohelper.CryptoHelper
	characters    []Character
	conversations Conversations
	gptClient     *GPT
	logger        *slog.Logger
}

func New(cfg Config, logger *slog.Logger) *Bot {
	return &Bot{
		config: cfg,
		logger: logger,
	}
}

func (m *Bot) Init() error {
	client, err := mautrix.NewClient(m.config.Homeserver, id.UserID(m.config.UserID), m.config.UserAccessKey)
	if err != nil {
		return err
	}
	var oei mautrix.OldEventIgnorer
	oei.Register(client.Syncer.(mautrix.ExtensibleSyncer))
	m.client = client
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
	m.AddEventHandler(m.InviteHandler())
	m.AddEventHandler(m.ResponseHandler())

	return nil
}

func (m *Bot) Run() error {
	if err := m.client.Sync(); err != nil {
		return err
	}

	return nil
}

func (m *Bot) Close() error {
	if err := m.client.Sync(); err != nil {
		return err
	}
	if err := m.cryptoHelper.Close(); err != nil {
		return err
	}

	return nil
}

func (m *Bot) AddEventHandler(eventType event.Type, handler mautrix.EventHandler) {
	syncer := m.client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(eventType, handler)
}

func (m *Bot) InviteHandler() (event.Type, mautrix.EventHandler) {
	return event.StateMember, func(source mautrix.EventSource, evt *event.Event) {
		if evt.GetStateKey() == m.client.UserID.String() && evt.Content.AsMember().Membership == event.MembershipInvite {
			_, err := m.client.JoinRoomByID(evt.RoomID)
			if err != nil {
				m.logger.Error("failed to join room after invite", slog.String("err", err.Error()), slog.String("room_id", evt.RoomID.String()), slog.String("inviter", evt.Sender.String()))
				return
			}

			m.logger.Info("joined room after invite", slog.String("room_id", evt.RoomID.String()), slog.String("inviter", evt.Sender.String()))
		}
	}
}

func (m *Bot) ResponseHandler() (event.Type, mautrix.EventHandler) {
	return event.EventMessage, func(source mautrix.EventSource, evt *event.Event) {
		content := evt.Content.AsMessage()
		eventID := evt.ID
		m.logger.Info("received message", slog.String("content", content.Body))

		// ignore if the message is already recorded
		if conv := m.conversations.FindByEventID(eventID); conv != nil {
			m.logger.Info("known message, ignoring", slog.String("event_id", eventID.String()))
			return
		}

		// ignore if the message is sent by the bot itself
		if evt.Sender == id.UserID(m.config.UserID) {
			m.logger.Info("message sent by bot itself, ignoring", slog.String("event_id", eventID.String()))
			return
		}

		var conv *Conversation
		// find out if it is a reply to a known conversation
		parentID := id.EventID("")
		if relatesTo := content.GetRelatesTo(); relatesTo != nil {
			if parentID = relatesTo.GetReplyTo(); parentID != "" {
				m.logger.Info("message is a reply", slog.String("parent_id", parentID.String()))
				if c := m.conversations.FindByEventID(parentID); c != nil {
					m.logger.Info("found parent, appending message to conversation", slog.String("event_id", eventID.String()))
					c.Add(Message{
						EventID:  eventID,
						ParentID: parentID,
						Role:     openai.ChatMessageRoleUser,
						Content:  content.Body,
					})
					conv = c
				}
			}
		}

		// find out if message is a new question addressed to the bot
		m.logger.Info(content.Body)
		if conv == nil && strings.HasPrefix(strings.ToLower(content.Body), strings.ToLower(fmt.Sprintf("%s: ", m.config.UserDisplayName))) {
			m.logger.Info("message is addressed to bot", slog.String("event_id", eventID.String()))
			conv = NewConversation(eventID, m.config.SystemPrompt, content.Body)
			m.conversations = append(m.conversations, conv)
		}

		if conv == nil {
			m.logger.Info("apparently not for us, ignoring", slog.String("event_id", eventID.String()))
			return
		}

		// get reply from GPT
		reply, err := m.gptClient.Complete(conv)
		if err != nil {
			m.logger.Error("failed to get reply from openai", slog.String("err", err.Error()))
			return
		}

		formattedReply := format.RenderMarkdown(reply, true, false)
		formattedReply.RelatesTo = &event.RelatesTo{
			InReplyTo: &event.InReplyTo{
				EventID: eventID,
			},
		}
		res, err := m.client.SendMessageEvent(evt.RoomID, event.EventMessage, &formattedReply)
		if err != nil {
			m.logger.Error("failed to send message", slog.String("err", err.Error()))
			return
		}
		conv.Add(Message{
			EventID:  res.EventID,
			ParentID: eventID,
			Role:     openai.ChatMessageRoleAssistant,
			Content:  reply,
		})

		if len(reply) > 30 {
			reply = reply[:30] + "..."
		}
		m.logger.Info("sent reply", slog.String("parent_id", eventID.String()), slog.String("content", reply))
	}
}
