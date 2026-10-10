package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	neturl "net/url"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	"github.com/redis/go-redis/v9"

	"net/http"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
	"rira/static/keyboards"
)

func voteWatchURL(id, query string) string {
	return fmt.Sprintf("%s/rest/inbox/1.0/message/votewatch/%v?%s", jiraURL, id, query)
}

func messageFlagURL(id, path string) string {
	return fmt.Sprintf("%s/rest/inbox/1.0/message/%v/%s", jiraURL, id, path)
}

func HandleVote(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.inboxAction(cq, user, func(id string) string { return voteWatchURL(id, "vote=vote") })
}

func HandleUnvote(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.inboxAction(cq, user, func(id string) string { return voteWatchURL(id, "vote=unvote") })
}

func HandleWatch(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.inboxAction(cq, user, func(id string) string { return voteWatchURL(id, "watch=watch") })
}

func HandleUnwatch(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.inboxAction(cq, user, func(id string) string { return voteWatchURL(id, "watch=unwatch") })
}

func HandleStar(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.inboxAction(cq, user, func(id string) string { return messageFlagURL(id, "star?star=true") })
}

func HandleUnstar(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.inboxAction(cq, user, func(id string) string { return messageFlagURL(id, "star?star=false") })
}

func HandleRead(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.inboxAction(cq, user, func(id string) string { return messageFlagURL(id, "read?flag=true") })
}

func HandleUnread(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.inboxAction(cq, user, func(id string) string { return messageFlagURL(id, "read?flag=false") })
}

func HandleReminder(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	id := callbackArg(cq, 1)

	msg := tgbotapi.NewMessage(chatID, "⏰ اگه می‌خواید متنی همراه یادآوری‌تون باشه، بنویسید و بفرستید (مثلاً «پیگیری با تیم»)؛ در غیر این صورت دکمه «بدون متن» رو بزنید.")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("❌ بدون متن", fmt.Sprintf("noText_%v", id))),
		cancelRow(),
	)
	message, err := b.bot.Send(msg)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message in reminder", err)
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e, false
		}
		return http.StatusBadRequest, err, false
	}

	if err := b.rdb.Set(context.Background(), fmt.Sprintf("user_%v_messageId", user.ID), message.MessageID, 0).Err(); err != nil {
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e, false
		}
		b.sendErrorMessage(chatID)
		log.Printf("Error sending message id in Redis: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	values := []interface{}{WaittingForReminderText, fmt.Sprintf("issueId_%v", id)}
	if err := repositories.UpdateUser(b.db, user, values, "stat", "message"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleNoText(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	if del := b.rdb.Del(context.Background(), fmt.Sprintf("user_%v_reminderText", user.ID)); del.Val() != 1 && del.Val() != 0 {
		if err := b.resetState(user, chatID); err != nil {
			return http.StatusBadRequest, err, false
		}
		log.Println("error setting reminder text to null", del.Val())
		b.sendErrorMessage(chatID)
		return http.StatusUnauthorized, fmt.Errorf("error setting reminder text to null"), false
	}

	if !statIs(user, WaittingForReminderText) {
		if err := b.resetState(user, chatID); err != nil {
			return http.StatusBadRequest, err, false
		}
		b.sendExpiredMessage(chatID, messageID)
		return http.StatusBadRequest, fmt.Errorf("operation is done"), false
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID,
		"⏰ زمان یادآوری رو از دکمه‌های پایین انتخاب کنید یا خودتون با این فرمت بفرستید: عدد + یکی از حروف m (دقیقه)، h (ساعت)، d (روز) یا w (هفته) — مثلاً 2h یعنی ۲ ساعت دیگه.", reminderTimeKeyboard(id))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message in reminder", err)
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e, false
		}
	}

	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForReminderTime}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleReminderHoure(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err == sql.ErrNoRows {
		helpers.MarkTokenExpired(b.db, user, token)
		return http.StatusBadRequest, err, false
	} else if err != nil {
		return http.StatusBadRequest, err, false
	}

	wait := tgbotapi.NewEditMessageReplyMarkup(chatID, messageID, keyboards.WaitButton)
	if _, err := b.bot.Send(wait); err != nil {
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e, false
		}
		log.Printf("Failed to send callback query response: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	if !statIs(user, WaittingForReminderTime) {
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e, false
		}
		b.sendExpiredMessage(chatID, messageID)
		return http.StatusBadRequest, nil, false
	}
	if e := b.resetState(user, chatID); e != nil {
		return http.StatusBadRequest, e, false
	}

	text, err := b.rdb.Get(context.Background(), fmt.Sprintf("user_%v_reminderText", user.ID)).Result()
	if err != nil && err != redis.Nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error checking user reminder text in Redis: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	interval := callbackArg(cq, 2)
	if err := b.authorizedPostWithRetry(jiraReminderURL(id, interval, neturl.QueryEscape(text)), token, user); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error setting reminder: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	value, unit := reminderValueUnit(interval)
	confirm := fmt.Sprintf("⏰ یادآوری‌تون برای %v %v دیگه تنظیم شد.", value, unit)
	if text != "" {
		confirm = fmt.Sprintf("⏰ یادآوری‌تون با عنوان «<b>%v</b>» برای %v %v دیگه تنظیم شد.", text, value, unit)
	}
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, confirm)
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleComment(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	id := callbackArg(cq, 1)

	msg := tgbotapi.NewMessage(chatID, "🗨️ متن کامنتی که می‌خواید برای این تسک ثبت بشه رو بنویسید و بفرستید.")
	msg.ReplyMarkup = keyboards.ReturnToMainButton
	message, err := b.bot.Send(msg)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message in comment", err)
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e, false
		}
		return http.StatusBadRequest, err, false
	}

	if err := b.rdb.Set(context.Background(), fmt.Sprintf("user_%v_messageId", user.ID), message.MessageID, 0).Err(); err != nil {
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e, false
		}
		b.sendErrorMessage(chatID)
		log.Printf("Error setting variable to redis: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	values := []interface{}{WaittingForCommentText, fmt.Sprintf("issueKey_%v", id)}
	if err := repositories.UpdateUser(b.db, user, values, "stat", "message"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleDelete(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err != nil {
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusUnauthorized, err, false
	}

	if err := b.authorizedPostWithRetry(fmt.Sprintf("%s/rest/inbox/1.0/message/delete/%v", jiraURL, id), token, user); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error deleting notification %s in Jira: %v\n", id, err)
		return http.StatusBadRequest, err, false
	}

	// Scoped to this user: the same Jira notif_id is frequently shared across every
	// watcher of an issue, so deleting by notif_id alone would also delete other
	// users' copies of the same notification.
	if err := repositories.DeleteNotification(b.db, user.ID, id); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error deleting notification", err)
		return http.StatusBadRequest, err, false
	}

	if _, err := b.bot.Request(tgbotapi.NewCallback(cq.ID, "✅ اعلان با موفقیت حذف شد")); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Failed to send callback query response: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	b.deleteMessage(chatID, messageID)
	return http.StatusOK, nil, false
}

// reminderValueUnit splits a duration token like "4h" into ("4", "ساعت").
func reminderValueUnit(interval string) (value, unit string) {
	if len(interval) >= 2 {
		return string(interval[0]), reminderUnitLabel(string(interval[1]))
	}
	return "", ""
}

// --- Response handlers: persist the toggled flag after a successful inbox action. ---

// setNotificationFlag updates a single column on the notification owned by userID.
// Scoping by user_id (not just message_id) avoids mutating a different user's row
// when two chats happen to reuse the same Telegram message ID.
func (b *Bot) setNotificationFlag(userID int64, messageID int, value, column string) error {
	if err := repositories.UpdateNotification(b.db, userID, messageID, []interface{}{value}, column); err != nil {
		log.Println("error updating notification", err)
		return err
	}
	return nil
}

func HandleVoteResponse(b *Bot, user models.User, chatID int64, messageID int) error {
	return b.setNotificationFlag(user.ID, messageID, "1", "voted")
}

func HandleUnvoteResponse(b *Bot, user models.User, chatID int64, messageID int) error {
	return b.setNotificationFlag(user.ID, messageID, "0", "voted")
}

func HandleWatchResponse(b *Bot, user models.User, chatID int64, messageID int) error {
	return b.setNotificationFlag(user.ID, messageID, "1", "watching")
}

func HandleUneatchResponse(b *Bot, user models.User, chatID int64, messageID int) error {
	return b.setNotificationFlag(user.ID, messageID, "0", "watching")
}

func HandleStarResponse(b *Bot, user models.User, chatID int64, messageID int) error {
	return b.setNotificationFlag(user.ID, messageID, "1", "starred")
}

func HandleUnstarResponse(b *Bot, user models.User, chatID int64, messageID int) error {
	return b.setNotificationFlag(user.ID, messageID, "0", "starred")
}

func HandleReadResponse(b *Bot, user models.User, chatID int64, messageID int) error {
	return b.setNotificationFlag(user.ID, messageID, "0", "opened")
}

func HandleUreadResponse(b *Bot, user models.User, chatID int64, messageID int) error {
	return b.setNotificationFlag(user.ID, messageID, "1", "opened")
}

func HandleReminderHoureResponse(b *Bot, user models.User, chatID int64, messageID int) error {
	return b.deleteMessage(chatID, messageID)
}
