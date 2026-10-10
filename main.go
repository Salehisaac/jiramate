package main

import (
	"log"
	"rira/internal/app"
)

func main() {
	log.Println("Initializing bot...")
	bot, err := app.NewBot()
	if err != nil {
		log.Fatalf("Error initializing bot: %v", err)
	}

	log.Println("Starting bot...")
	if err := bot.Start(); err != nil {
		log.Fatalf("Error starting bot: %v", err)
	}
}
