package main

import (
	"fmt"
	"os"
	"os/signal"

	"go-mod.ewintr.nl/matrix-bots/bot"
	"github.com/BurntSushi/toml"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/exp/slog"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	var config bot.Config
	if _, err := toml.DecodeFile(getParam("CONFIG_PATH", "conf.toml"), &config); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	type Credentials struct {
		Password  string
		AccessKey string
	}
	credentials := make(map[string]Credentials)
	for i := 0; i < len(config.Bots); i++ {
		user := getParam(fmt.Sprintf("MATRIX_BOT%d_ID", i), "")
		if user == "" {
			logger.Error("missing user id", slog.Int("user", i))
			os.Exit(1)
		}
		credentials[user] = Credentials{
			Password:  getParam(fmt.Sprintf("MATRIX_BOT%d_PASSWORD", i), ""),
			AccessKey: getParam(fmt.Sprintf("MATRIX_BOT%d_ACCESSKEY", i), ""),
		}
	}
	for i, bc := range config.Bots {
		creds, ok := credentials[bc.UserID]
		if !ok {
			logger.Error("missing credentials", slog.Int("user", i))
			os.Exit(1)
		}
		config.Bots[i].UserPassword = creds.Password
		config.Bots[i].UserAccessKey = creds.AccessKey
	}

	config.OpenAI = bot.ConfigOpenAI{
		APIKey: getParam("OPENAI_API_KEY", ""),
	}

	var acceptInvites bool
	if getParam("MATRIX_ACCEPT_INVITES", "false") == "true" {
		acceptInvites = true
	}

	logger.Info("loaded config", slog.Int("bots", len(config.Bots)))

	for _, bc := range config.Bots {
		b := bot.New(config.OpenAI.APIKey, bc, logger)
		if err := b.Init(acceptInvites); err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}
		go b.Run()
		logger.Info("started bot", slog.String("name", bc.UserDisplayName))
	}

	done := make(chan os.Signal)
	signal.Notify(done, os.Interrupt)
	<-done

	logger.Info("service stopped")
}

func getParam(name, def string) string {
	val, ok := os.LookupEnv(name)
	if !ok {
		return def
	}
	return val
}
