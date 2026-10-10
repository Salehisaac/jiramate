package app

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	"github.com/golang-jwt/jwt/v4"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
)

func HandleAttachGroupProjectSelect(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	groupChatID, _ := strconv.Atoi(user.ChatID)
	chatID := cq.From.ID
	messageID := cq.Message.MessageID

	token, err := repositories.GetUserTokenByChatID(b.db, chatID)
	if err == sql.ErrNoRows {
		b.sendUnauthorizedMessage(int64(groupChatID), user)
		helpers.MarkTokenExpired(b.db, user, token)
		log.Println("HandleAttachGroupProjectSelect: user is unauthorized")
		return http.StatusBadRequest, err, true
	} else if err != nil {
		log.Printf("HandleAttachGroupProjectSelect: error fetching user token: %v\n", err)
		return http.StatusBadRequest, err, true
	}

	if !helpers.TestToken(token) {
		if ok := helpers.TokenRetry(token, 5); !ok {
			b.sendUnauthorizedMessage(int64(groupChatID), user)
			helpers.MarkTokenExpired(b.db, user, token)
			log.Println("HandleAttachGroupProjectSelect: user is unauthorized")
			return http.StatusBadRequest, fmt.Errorf("the user is unauthorized"), true
		}
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(int64(groupChatID))
		log.Printf("HandleAttachGroupProjectSelect: error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err, true
	}
	projects, err := helpers.GetAllProjects(client, jiraURL)
	if err != nil {
		b.sendErrorMessage(int64(groupChatID))
		log.Printf("HandleAttachGroupProjectSelect: error fetching projects: %v\n", err)
		return http.StatusBadRequest, err, true
	}
	if len(projects) == 0 {
		// The prompt being edited lives in the group chat (groupChatID), not the
		// individual user's private chat (chatID) - editing the wrong chat here
		// used to fail silently with a "message not found" error from Telegram.
		return b.editNoProjects(int64(groupChatID), messageID, user)
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(int64(groupChatID), messageID, "📋 یکی از پروژه‌های زیر رو انتخاب کنید تا این گروه بهش وصل بشه", projectListKeyboard(projects, "selectProjectForChat"))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(int64(groupChatID))
		log.Println("HandleAttachGroupProjectSelect: error sending inline keyboard message:", err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}

func HandleSelectProjectForChat(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	projectKey := callbackArg(cq, 1)

	claims := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"projectKey": projectKey,
		"chatId":     user.ChatID,
		"exp":        time.Now().Add(24 * time.Hour).Unix(),
		"iat":        time.Now().Unix(),
	})
	tokenString, err := claims.SignedString(chatTokenSecret)
	if err != nil {
		log.Println("HandleSelectProjectForChat: error signing token:", err)
		return http.StatusBadRequest, err, true
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, fmt.Sprintf(`
🔗 برای وصل کردن ربات به گروه‌تون این مراحل رو دنبال کنید:
1. ربات رو به گروه‌تون اضافه کنید.
2. دستور /setToken رو توی گروه بفرستید.
3. توکن زیر رو کپی کنید و توی همون گروه برای ربات بفرستید:
<code>%v</code>`, tokenString))
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("HandleSelectProjectForChat: error sending inline keyboard message:", err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}
