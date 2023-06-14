package bot

import (
	"strings"
	"sync"

	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
)

var (
	botNames = []string{}
	mu       = &sync.Mutex{}
)

func BotNameAppend(name string) {
	mu.Lock()
	defer mu.Unlock()

	botNames = append(botNames, name)
}

type ConfigOpenAI struct {
	APIKey string
}

type ConfigBot struct {
	DBPath            string
	Pickle            string
	Homeserver        string
	UserID            string
	UserAccessKey     string
	UserPassword      string
	UserDisplayName   string
	SystemPrompt      string
	AnswerUnaddressed bool
}

type Config struct {
	OpenAI ConfigOpenAI `toml:"openai"`
	Bots   []ConfigBot  `toml:"bot"`
}

type Bot struct {
	openaiKey     string
	config        ConfigBot
	client        *mautrix.Client
	cryptoHelper  *cryptohelper.CryptoHelper
	characters    []Character
	conversations Conversations
	gptClient     *GPT
	logger        *slog.Logger
}

func New(openaiKey string, cfg ConfigBot, logger *slog.Logger) *Bot {
	return &Bot{
		openaiKey: openaiKey,
		config:    cfg,
		logger:    logger,
	}
}

func (m *Bot) Init(acceptInvites bool) error {
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
	m.gptClient = NewGPT(m.openaiKey)
	m.conversations = make(Conversations, 0)
	if acceptInvites {
		m.AddEventHandler(m.InviteHandler())
	}
	m.AddEventHandler(m.ResponseHandler())

	m.config.UserDisplayName = strings.ToLower(m.config.UserDisplayName)
	BotNameAppend(m.config.UserDisplayName)

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
				m.logger.Error("failed to join room after invite", slog.String("err", err.Error()), slog.String("room_id", evt.RoomID.String()), slog.String("inviter", evt.Sender.String()), slog.String("bot", m.config.UserDisplayName))
				return
			}

			m.logger.Info("joined room after invite", slog.String("room_id", evt.RoomID.String()), slog.String("inviter", evt.Sender.String()), slog.String("bot", m.config.UserDisplayName))
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
			m.logger.Info("known message, ignoring", slog.String("event_id", eventID.String()), slog.String("bot", m.config.UserDisplayName))
			return
		}

		// ignore if the message is sent by the bot itself
		if evt.Sender == id.UserID(m.config.UserID) {
			m.logger.Info("message sent by bot itself, ignoring", slog.String("event_id", eventID.String()), slog.String("bot", m.config.UserDisplayName))
			return
		}

		var conv *Conversation
		// find out if it is a reply to a known conversation
		parentID := id.EventID("")
		var hasParent bool
		if relatesTo := content.GetRelatesTo(); relatesTo != nil {
			if parentID = relatesTo.GetReplyTo(); parentID != "" {
				hasParent = true
				m.logger.Info("message is a reply", slog.String("parent_id", parentID.String()))
				if c := m.conversations.FindByEventID(parentID); c != nil {
					m.logger.Info("found parent, appending message to conversation", slog.String("event_id", eventID.String()), slog.String("bot", m.config.UserDisplayName))
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

		m.logger.Info(content.Body)
		addressedTo, _, isAddressed := strings.Cut(content.Body, ": ")
		addressedTo = strings.TrimSpace(strings.ToLower(addressedTo))
		if strings.Contains(addressedTo, " ") {
			isAddressed = false // only display names without spaces, otherwise no way to know if it's a name or not
		}

		// find out if message is a new question addressed to the bot
		if conv == nil && isAddressed && addressedTo == m.config.UserDisplayName {
			m.logger.Info("message is addressed to bot", slog.String("event_id", eventID.String()), slog.String("bot", m.config.UserDisplayName))
			conv = NewConversation(eventID, m.config.SystemPrompt, content.Body)
			m.conversations = append(m.conversations, conv)
		}
		// find out if the message is addressed to no-one and this bot answers those
		if conv == nil && !isAddressed && !hasParent && m.config.AnswerUnaddressed {
			m.logger.Info("message is addressed to no-one", slog.String("event_id", eventID.String()), slog.String("bot", m.config.UserDisplayName))
			conv = NewConversation(eventID, m.config.SystemPrompt, content.Body)
			m.conversations = append(m.conversations, conv)
		}

		if conv == nil {
			m.logger.Info("apparently not for us, ignoring", slog.String("event_id", eventID.String()), slog.String("bot", m.config.UserDisplayName))
			return
		}

		// get reply from GPT
		reply, err := m.gptClient.Complete(conv)
		if err != nil {
			m.logger.Error("failed to get reply from openai", slog.String("err", err.Error()), slog.String("bot", m.config.UserDisplayName))
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
			m.logger.Error("failed to send message", slog.String("err", err.Error()), slog.String("bot", m.config.UserDisplayName))
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
		m.logger.Info("sent reply", slog.String("parent_id", eventID.String()), slog.String("content", reply), slog.String("bot", m.config.UserDisplayName))
	}
}
