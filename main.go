package main

import (
	"os"
	"os/signal"

	"ewintr.nl/matrix-bots/bot"
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
	logger.Info("loaded config", slog.Int("bots", len(config.Bots)))

	for _, bc := range config.Bots {
		b := bot.New(config.OpenAI.APIKey, bc, logger)
		if err := b.Init(); err != nil {
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
