package bot

import (
	"fmt"
	"time"

	"github.com/chzyer/readline"
	"github.com/rs/zerolog"
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
	})).With().Timestamp().Logger()

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

func (m *Matrix) InviteHandler() (event.Type, mautrix.EventHandler) {
	return event.StateMember, func(source mautrix.EventSource, evt *event.Event) {
		if evt.GetStateKey() == m.client.UserID.String() && evt.Content.AsMember().Membership == event.MembershipInvite {
			_, err := m.client.JoinRoomByID(evt.RoomID)
			if err == nil {
				m.client.Log.Info().
					Str("room_id", evt.RoomID.String()).
					Str("inviter", evt.Sender.String()).
					Msg("Joined room after invite")
			} else {
				m.client.Log.Error().Err(err).
					Str("room_id", evt.RoomID.String()).
					Str("inviter", evt.Sender.String()).
					Msg("Failed to join room after invite")
			}
		}
	}
}

func (m *Matrix) RespondHandler() (event.Type, mautrix.EventHandler) {
	return event.EventMessage, func(source mautrix.EventSource, evt *event.Event) {
		m.client.Log.Info().
			Str("body", evt.Content.AsMessage().Body).
			Msg("Received message")

		if evt.Sender != id.UserID(m.config.UserID) {
			msgBody := evt.Content.AsMessage().Body
			resp, err := m.gptClient.Complete(NewConversation(msgBody))

			if err != nil {
				fmt.Println("OpenAI API returned with ", err)
				return
			}

			// Send the OpenAI response back to the chat
			formattedResp := format.RenderMarkdown(resp, true, false)
			m.client.SendMessageEvent(evt.RoomID, event.EventMessage, &formattedResp)
		}
	}
}
