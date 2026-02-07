package config

import "os"

const (
	envBotToken = "BOT_TOKEN"
	envDBPath   = "DB_PATH"
	defaultDB   = "./data/reminders.db"
)

// Load reads BOT_TOKEN and DB_PATH from environment.
// DB_PATH defaults to "./data/reminders.db" when unset.
func Load() (BotToken, DBPath string) {
	botToken := os.Getenv(envBotToken)
	dbPath := os.Getenv(envDBPath)
	if dbPath == "" {
		dbPath = defaultDB
	}
	return botToken, dbPath
}
