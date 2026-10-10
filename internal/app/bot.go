package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"rira/internal/database"
)

const jiraURL = "https://pm.rso-co.ir"

// notificationPollInterval controls both the inbox-polling ticker and the JQL
// "updatedDate >= -Nm" window used to detect Done transitions, so the two
// stay in sync (a mismatch here would create gaps or duplicate group posts).
const notificationPollInterval = 1 * time.Minute

// httpClient is shared by every outbound HTTP call (Jira, Telegram file
// downloads, OpenAI) so requests can never hang forever and leak goroutines.
var httpClient = &http.Client{Timeout: 30 * time.Second}

var (
	botToken    string
	openaiToken string
)

// chatTokenSecret signs the JWT used to attach a project to a group chat.
// It is read from CHAT_TOKEN_SECRET; falling back to a fixed value is only
// safe for local development, so we log loudly when that happens.
var chatTokenSecret = loadChatTokenSecret()

func loadChatTokenSecret() []byte {
	if secret := os.Getenv("CHAT_TOKEN_SECRET"); secret != "" {
		return []byte(secret)
	}
	log.Println("WARNING: CHAT_TOKEN_SECRET is not set, falling back to an insecure default. Set it in production.")
	return []byte("your_secret_key_here")
}

type Bot struct {
	bot *tgbotapi.BotAPI
	rdb *redis.Client
	db  *sql.DB
}

func NewBot() (*Bot, error) {
	if err := godotenv.Load(); err != nil {
		log.Printf("Error loading .env file: %v\n", err)
	}
	botToken = os.Getenv("BOT_TOKEN")
	openaiToken = os.Getenv("OPENAI_API_KEY")

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, fmt.Errorf("error creating telegram bot api client: %w", err)
	}
	bot.Debug = os.Getenv("BOT_DEBUG") == "true"

	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "شروع کار با ربات و راهنمای اولیه"},
		tgbotapi.BotCommand{Command: "menu", Description: "نمایش منو و امکانات ربات"},
		tgbotapi.BotCommand{Command: "assign", Description: "ساخت یک تسک جدید در جیرا"},
		tgbotapi.BotCommand{Command: "connect", Description: "اتصال یا اتصال مجدد حساب جیرا"},
		tgbotapi.BotCommand{Command: "setToken", Description: "تنظیم توکن جیرا برای این گروه (فقط گروه‌ها)"},
	)
	if _, err := bot.Request(commands); err != nil {
		log.Printf("Failed to set bot commands: %v\n", err)
	}
	log.Printf("Authorized on account %s\n", bot.Self.UserName)

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		os.Getenv("MYSQL_USER"), os.Getenv("MYSQL_PASSWORD"),
		os.Getenv("MYSQL_HOST"), os.Getenv("MYSQL_PORT"), os.Getenv("MYSQL_DB"))

	if err := database.ConnectDB(dsn); err != nil {
		return nil, fmt.Errorf("error connecting to database: %w", err)
	}
	db := database.GetDB()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := database.InitDB(db); err != nil {
		return nil, fmt.Errorf("error initializing database: %w", err)
	}
	log.Println("database has been built successfully")

	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_HOST"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("error connecting to redis: %w", err)
	}

	return &Bot{
		bot: bot,
		rdb: rdb,
		db:  db,
	}, nil
}

func (b *Bot) Start() error {
	go b.handleNotifications(b.db)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	for update := range b.bot.GetUpdatesChan(u) {
		go b.handleUpdate(update)
	}
	return nil
}
