package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"strconv"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
	"rira/static/keyboards"
)

func userPanelButton() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonURL("پنل کاربری", jiraURL+"/secure/ViewProfile.jspa"),
	})
}

func HandleUnauthorized(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID, _ := strconv.Atoi(user.ChatID)

	msg := tgbotapi.NewMessage(int64(chatID), "🔑 برای اتصال به جیرا، لطفا توکن دسترسی (Personal Access Token) خودتون رو همینجا به صورت پیام متنی بفرستید.")
	msg.ReplyMarkup = userPanelButton()
	if _, err := b.bot.Send(msg); err != nil {
		log.Printf("HandleUnauthorized: error sending the message: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForToken}, "stat"); err != nil {
		log.Println("HandleUnauthorized:", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleUnauthorizedMessage(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID, _ := strconv.Atoi(user.ChatID)
	b.sendUnauthorizedMessage(int64(chatID), user)
	return http.StatusOK, nil
}

func HandleStart(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID, _ := strconv.Atoi(user.ChatID)
	if _, err := b.bot.Send(tgbotapi.NewMessage(int64(chatID), startMessage)); err != nil {
		log.Printf("error sending the message: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleMenu(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	msg := tgbotapi.NewMessage(chatID, "سلام 👋 یکی از گزینه‌های زیر رو انتخاب کنید:")
	msg.ReplyMarkup = &keyboards.MainMenuInlineButton
	if _, err := b.bot.Send(msg); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending message in HandleMenu: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleSettings(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	msg := tgbotapi.NewMessage(chatID, "چطور می‌خواید اعلان‌های خودتون رو مدیریت کنید؟ می‌تونید پروژه یا فرد خاصی رو فیلتر یا دنبال کنید.")
	msg.ReplyMarkup = &keyboards.SettingsInlineButton
	if _, err := b.bot.Send(msg); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending message in HandleSettings: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleAssign(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	if err := b.rdb.Del(context.Background(), assignTaskKey(user)).Err(); err != nil {
		log.Printf("Error deleting list from Redis: %v\n", err)
		return http.StatusBadRequest, err
	}

	token, ok := b.tokenForUser(user, chatID)
	if !ok {
		return http.StatusBadRequest, nil
	}
	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err
	}
	projects, err := helpers.GetAllProjects(client, jiraURL)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching projects: %v\n", err)
		return http.StatusBadRequest, err
	}
	if len(projects) == 0 {
		if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, "😕 در حال حاضر به هیچ پروژه‌ای در جیرا دسترسی ندارید. برای دسترسی به یک پروژه، با ادمین جیرا تماس بگیرید.")); err != nil {
			log.Printf("Error sending message to user %v: %v\n", user.ChatID, err)
			return http.StatusBadRequest, err
		}
		return http.StatusBadRequest, nil
	}

	msg := tgbotapi.NewMessage(chatID, "📁 برای ساخت تسک جدید، یکی از پروژه‌های زیر رو انتخاب کنید:")
	msg.ReplyMarkup = projectListKeyboard(projects, "createTask")
	if _, err := b.bot.Send(msg); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending inline keyboard message: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleForwardAssign(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("می‌خواید این پیام رو به عنوان یک تسک ثبت کنید؟\n\n«%v»\n\nاین تسک برای %v ثبت میشه. برای تایید روی دکمه‌ی زیر بزنید.", update.Message.Text, update.Message.ForwardFrom.UserName))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ تایید و ثبت تسک", fmt.Sprintf("assignTaskFromForwardedMessage_%v", update.Message.ForwardFrom.ID)),
		),
	)

	if err := b.rdb.LPush(context.Background(), assignTaskKey(user), fmt.Sprintf("summary_%v", update.Message.Text)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing summary to Redis: %v\n", err)
		return http.StatusBadRequest, err
	}

	message, err := b.bot.Send(msg)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending message in HandleForwardAssign: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := b.rdb.Set(context.Background(), messageIDKey(user), message.MessageID, 0).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error setting message id in Redis: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleVoice(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	if update.Message.ReplyToMessage == nil {
		msg := tgbotapi.NewMessage(chatID, "🎙️ برای تنظیم یادآوری با پیام صوتی، اول روی یکی از پیام‌های اعلان ریپلای کنید و بعد پیام صوتی خودتون رو بفرستید.")
		if _, err := b.bot.Send(msg); err != nil {
			b.sendErrorMessage(chatID)
			log.Printf("error sending the message: %v\n", err)
			return http.StatusBadRequest, err
		}
		return http.StatusOK, nil
	}
	replyToMessageID := update.Message.ReplyToMessage.MessageID

	command, err := b.handleVoiceMessage(update)
	if err != nil {
		log.Printf("error in handling voice message: %v\n", err)
		return b.voiceError(chatID, "⚠️ مشکلی در پردازش پیام صوتی پیش اومد. لطفا دوباره تلاش کنید.", err)
	}
	response, err := processTextWithOpenAI(command)
	if err != nil {
		log.Printf("error proccessing the text with open ai: %v\n", err)
		return b.voiceError(chatID, "⚠️ مشکلی در پردازش پیام صوتی پیش اومد. لطفا دوباره تلاش کنید.", err)
	}
	event, err := unmarshalResponse(response)
	if err != nil {
		log.Printf("Failed to unmarshal response: %v\n", err)
		return b.voiceError(chatID, "⚠️ نتونستم صدای شما رو درست تحلیل کنم. لطفا دوباره تلاش کنید.", err)
	}
	if event.Time == "" {
		log.Println("error in getting time from voice")
		return b.voiceError(chatID, "🎙️ متوجه‌ی زمان یادآوری نشدم. لطفا دوباره و واضح‌تر بگید کِی یادتون بندازم (مثلا «۲ ساعت دیگه» یا «فردا صبح»).", fmt.Errorf("empty reminder time"))
	}

	// Notifications are keyed by (user_id, message_id): Telegram message IDs are
	// only unique within a single chat, so scoping by message_id alone risked
	// resolving to a completely different user's notification.
	notif, err := repositories.GetNotification(b.db, user.ID, replyToMessageID)
	if err != nil {
		log.Printf("error getting notification: %v\n", err)
		return b.voiceError(chatID, "مشکلی در تحلیل صوت به وجود امده لطفا دوباره تلاش کنید", err)
	}

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err == sql.ErrNoRows {
		helpers.MarkTokenExpired(b.db, user, token)
		log.Printf("the user is unauthorized: %v\n", err)
		return http.StatusBadRequest, err
	} else if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusBadRequest, err
	}

	url := jiraReminderURL(notif.Issue.ID, event.Time, neturl.QueryEscape(event.Description))
	if err := b.authorizedPostWithRetry(url, token, user); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error setting reminder from voice: %v\n", err)
		return http.StatusBadRequest, err
	}

	confirm := fmt.Sprintf("⏰ یادآوری شما برای %v دیگه تنظیم شد.", event.Time)
	if event.Description != "" {
		confirm = fmt.Sprintf("⏰ یادآوری شما با عنوان «<b>%v</b>» برای %v دیگه تنظیم شد.", event.Description, event.Time)
	}
	if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, confirm)); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending message: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

// voiceError replies with a user-facing message and reports the underlying failure.
func (b *Bot) voiceError(chatID int64, text string, cause error) (int, error) {
	if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		log.Printf("error sending the message: %v\n", err)
	}
	return http.StatusBadRequest, cause
}
