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

	matrixClient := bot.New(bot.Config{
		Homeserver:    getParam("MATRIX_HOMESERVER", "http://localhost"),
		UserID:        getParam("MATRIX_USER_ID", "@bot:localhost"),
		UserPassword:  getParam("MATRIX_PASSWORD", "secret"),
		UserAccessKey: getParam("MATRIX_ACCESS_KEY", "secret"),
		DBPath:        getParam("BOT_DB_PATH", "bot.db"),
		Pickle:        getParam("BOT_PICKLE", "scrambled"),
		OpenAIKey:     getParam("OPENAI_API_KEY", "no key"),
	})

	if err := matrixClient.Init(); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	go matrixClient.Run()

	matrixClient.AddEventHandler(matrixClient.InviteHandler(logger))
	matrixClient.AddEventHandler(matrixClient.ResponseHandler(logger))

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
