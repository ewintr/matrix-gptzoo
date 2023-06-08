package main

import (
	"os"
	"os/signal"

	"ewintr.nl/matrix-bots/bot"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/exp/slog"
)

func main() {

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	botConfigs := []bot.Config{{
		Homeserver:      getParam("MATRIX_HOMESERVER", "http://localhost"),
		UserID:          getParam("MATRIX_USER_ID", "@bot:localhost"),
		UserPassword:    getParam("MATRIX_PASSWORD", "secret"),
		UserAccessKey:   getParam("MATRIX_ACCESS_KEY", "secret"),
		UserDisplayName: getParam("MATRIX_DISPLAY_NAME", "Bot"),
		DBPath:          getParam("BOT_DB_PATH", "bot.db"),
		Pickle:          getParam("BOT_PICKLE", "scrambled"),
		OpenAIKey:       getParam("OPENAI_API_KEY", "no key"),
		SystemPrompt:    "You are a chatbot that helps people by responding to their questions with short messages.",
	}}

	for _, bc := range botConfigs {
		b := bot.New(bc, logger)
		if err := b.Init(); err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}
		go b.Run()
	}

	done := make(chan os.Signal)
	signal.Notify(done, os.Interrupt)
	<-done

}

func getParam(name, def string) string {
	val, ok := os.LookupEnv(name)
	if !ok {
		return def
	}
	return val
}
