package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"telegram-timer/config"
	"telegram-timer/db"
	"telegram-timer/handler"
	"telegram-timer/service"
	"telegram-timer/telegram"
)

const defaultAddr = ":8080"

func main() {
	botToken, dbPath := config.Load()
	if botToken == "" {
		log.Fatal("BOT_TOKEN is required")
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer database.Close()

	reminderSvc := service.NewReminderService(database)
	tgClient := telegram.NewClient(botToken)
	webhookHandler := handler.NewTelegram(reminderSvc, tgClient)
	sched := service.NewScheduler(reminderSvc, tgClient)

	go sched.Start()
	defer sched.Stop()

	http.HandleFunc("/telegram/webhook", webhookHandler.ServeHTTP)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	go func() {
		log.Printf("server listening on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down")
}
