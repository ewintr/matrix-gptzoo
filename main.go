package main

import (
	"os"
	"os/signal"

	"ewintr.nl/matrix-bots/matrix"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/exp/slog"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	matrixClient := matrix.New(matrix.Config{
		Homeserver:    getParam("MATRIX_HOMESERVER", "http://localhost"),
		UserID:        getParam("MATRIX_USER_ID", "@bot:localhost"),
		UserPassword:  getParam("MATRIX_PASSWORD", "secret"),
		UserAccessKey: getParam("MATRIX_ACCESS_KEY", "secret"),
		DBPath:        getParam("BOT_DB_PATH", "bot.db"),
		Pickle:        getParam("BOT_PICKLE", "scrambled"),
	})

	OpenaiAPIKey, ok := os.LookupEnv("OPENAI_API_KEY")
	if !ok {
		logger.Error("OPENAI_API_KEY is not set")
		os.Exit(1)
	}

	// Create new OpenAI client
	openaiClient := openai.NewClient(OpenaiAPIKey)

	if err := matrixClient.Init(); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	go matrixClient.Run()

	matrixClient.AddEventHandler(matrixClient.InviteHandler())
	matrixClient.AddEventHandler(matrixClient.RespondHandler(openaiClient))

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
