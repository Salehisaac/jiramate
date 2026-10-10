package app

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	"github.com/golang-jwt/jwt/v4"

	"rira/internal/database/repositories"
	"rira/internal/models"
)

func HandleStartAdmin(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	text := `سلام 👋
برای وصل کردن این گروه به یه پروژه‌ی جیرا، این مراحل رو دنبال کنید (این کار رو باید ادمین گروه انجام بده):
1. توی چت خصوصی با ربات (نه اینجا توی گروه) دستور /menu رو بفرستید.
2. از منو گزینه‌ی «💬 ارسال وضعیت پروژه در گروه» رو بزنید و پروژه‌ی موردنظرتون رو انتخاب کنید.
3. ربات یه توکن براتون می‌فرسته؛ اون رو کپی کنید.
4. برگردید همینجا توی گروه و دستور /setToken رو بفرستید.
5. وقتی ربات ازتون توکن خواست، همون توکن کپی‌شده رو همینجا بفرستید.`
	if _, err := b.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, text)); err != nil {
		b.sendErrorMessage(update.Message.Chat.ID)
		log.Printf("Error sending message: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleSetToken(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID

	if _, err := repositories.GetUserByChatID(b.db, update.Message.From.ID); err != nil {
		b.sendErrorCustomMessage(chatID, fmt.Errorf("کاربر شما هنوز توی ربات احراز هویت نشده. لطفا اول توی چت خصوصی با ربات دستور /start رو بزنید و مراحل اتصال به جیرا رو کامل کنید، بعد دوباره اینجا دستور /setToken رو بفرستید."))
		log.Printf("error getting user %v\n", err)
		return http.StatusBadRequest, err
	}

	if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, "🔑 لطفا توکنی که از چت خصوصی با ربات گرفتید رو کپی کنید و همینجا برام بفرستید.")); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error sending message: %v\n", err)
		return http.StatusBadRequest, err
	}

	// The group chat may not have a users row yet (e.g. this is the first time
	// /setToken has ever been run in it) - upsert it instead of blindly
	// UPDATE-ing the possibly zero-valued `user` passed in by the router, which
	// would silently affect zero rows and leave the group's state untracked.
	if _, err := repositories.UpsertGroupChatState(b.db, update.Message.Chat, WaittingForChatToken); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating group state", err)
		return http.StatusUnauthorized, err
	}
	return http.StatusOK, nil
}

func HandleMenuInGroup(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	if _, err := b.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "دستور /menu توی گروه کار نمی‌کنه. برای دیدن منوی اصلی، برید توی چت خصوصی با ربات و همونجا دستور /menu رو بفرستید.")); err != nil {
		b.sendErrorMessage(update.Message.Chat.ID)
		log.Printf("Error sending message: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleWaitingForChatToken(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID

	parsedToken, err := jwt.Parse(update.Message.Text, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return chatTokenSecret, nil
	})
	if err != nil {
		log.Println("Error parsing token:", err)
		return http.StatusBadRequest, fmt.Errorf("error %v", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if ok && parsedToken.Valid {
		tokenChatID, isStr := claims["chatId"].(string)
		id, _ := strconv.Atoi(tokenChatID)
		if !isStr || int64(id) != update.Message.From.ID {
			if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, "❌ این توکن مال حساب تلگرام شما نیست. توکن رو باید همون کسی که گرفته، خودش همینجا بفرسته. لطفا از منوی ربات گزینه‌ی «💬 ارسال وضعیت پروژه در گروه» رو دوباره بزنید و یه توکن تازه بگیرید.")); err != nil {
				return http.StatusUnauthorized, err
			}
			log.Println("chat token mismatch")
			return http.StatusUnauthorized, fmt.Errorf("invalid chat token")
		}
	} else {
		log.Println("Invalid token or claims.")
	}

	sender, err := repositories.GetUserByChatID(b.db, update.Message.From.ID)
	if err != nil {
		log.Println("error getting user", err)
		return http.StatusUnauthorized, err
	}
	if err := repositories.InsertChatIntoDatabase(update.Message, b.db); err != nil {
		b.sendErrorCustomMessage(chatID, err)
		log.Printf("Error inserting chat into database: %v\n", err)
		return http.StatusUnauthorized, err
	}

	projectKey := claims["projectKey"].(string)
	err = repositories.AssignProjectToGroup(b.db, chatID, sender.ID, projectKey)
	if err != nil {
		if err.Error() == "group is already exists" {
			if e := b.deleteMessage(chatID, update.Message.MessageID); e != nil {
				b.sendErrorMessage(chatID)
				log.Println("error deleting message", e)
				return http.StatusUnauthorized, e
			}
			if _, e := b.bot.Send(tgbotapi.NewMessage(chatID, "این گروه از قبل به یه پروژه وصل شده. اگه می‌خواید پروژه رو عوض کنید، با پشتیبانی تماس بگیرید.")); e != nil {
				b.sendErrorMessage(chatID)
				log.Println("error sending message:", e)
				return http.StatusBadRequest, e
			}
			if e := repositories.UpdateUser(b.db, user, []interface{}{""}, "stat"); e != nil {
				b.sendErrorMessage(chatID)
				log.Println("error updating user", e)
				return http.StatusUnauthorized, e
			}
			return http.StatusOK, nil
		}
		b.deleteMessage(chatID, update.Message.MessageID)
		b.sendErrorMessage(chatID)
		log.Println("error assigning project to group", err)
		return http.StatusBadRequest, err
	}

	if err := repositories.UpdateUser(b.db, user, []interface{}{""}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusUnauthorized, err
	}
	b.deleteMessage(chatID, update.Message.MessageID)

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ توکن با موفقیت ثبت شد و این گروه به پروژه‌ی %s وصل شد. از الان اعلان‌های تسک‌های این پروژه براتون همینجا ارسال میشه.", projectKey))
	if _, err := b.bot.Send(msg); err != nil {
		return http.StatusUnauthorized, err
	}
	return http.StatusOK, nil
}
