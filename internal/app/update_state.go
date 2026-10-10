package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	jira "github.com/andygrunwald/go-jira"
	"github.com/redis/go-redis/v9"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
)

// previousMessageID reads the message id stashed for the user's current flow.
func (b *Bot) previousMessageID(user models.User) (int, error) {
	val, err := b.rdb.Get(context.Background(), messageIDKey(user)).Result()
	if err != nil {
		return 0, err
	}
	id, _ := strconv.Atoi(val)
	return id, nil
}

func HandleWaitingForToken(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	log.Println("HandleWaitingForToken")

	token, err := b.handleGettingToken(b.db, update, user)
	if err != nil {
		log.Printf("token invalid: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := repositories.InsertToken(b.db, user.ID, token); err != nil {
		log.Printf("error inserting token: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := b.rdb.Set(context.Background(), fmt.Sprintf("user:%d:token", user.ID), token, 0).Err(); err != nil {
		log.Printf("Error setting user token in Redis: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{"", "authorized"}, "status", "stat"); err != nil {
		log.Println(err)
		return http.StatusBadRequest, err
	}

	if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, "✅ توکن شما با موفقیت ثبت شد و به جیراتون وصل شدید.")); err != nil {
		log.Printf("Error sending message to user %v: %v\n", user.ChatID, err)
		return http.StatusBadRequest, err
	}
	if err := b.resetState(user, chatID); err != nil {
		return http.StatusBadRequest, err
	}

	msg := tgbotapi.NewMessage(chatID, startMessage)
	msg.ReplyMarkup = tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("assign")),
	)
	if _, err := b.bot.Send(msg); err != nil {
		log.Printf("error sending the message: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleWaittingForReminderText(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID

	if err := b.rdb.Set(context.Background(), fmt.Sprintf("user_%v_reminderText", user.ID), update.Message.Text, 0).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error setting reminder text in Redis: %v\n", err)
		return http.StatusBadRequest, err
	}
	b.deleteMessage(chatID, update.Message.MessageID)

	previousID, err := b.previousMessageID(user)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error getting message id in Redis: %v\n", err)
		return http.StatusBadRequest, err
	}

	issueID := strings.Split(user.Message.String, "_")[1]
	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, previousID, "⏰ یکی از زمان‌های زیر رو انتخاب کنید یا زمان دلخواهتون رو به این شکل بفرستید: عدد + حرف واحد، مثلا 5m (۵ دقیقه)، 4h (۴ ساعت)، 5d (۵ روز) یا 2w (۲ هفته).", reminderTimeKeyboard(issueID))
	if _, err := b.bot.Send(editMsg); err != nil {
		log.Printf("error sending message in reminder: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForReminderTime}, "stat"); err != nil {
		log.Printf("error updating user: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleWaittingForReminderTime(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID

	if !helpers.ValidateTimeInput(update.Message.Text) {
		return b.rejectReminderTime(update, user)
	}

	if err := b.resetState(user, chatID); err != nil {
		return http.StatusBadRequest, err
	}
	b.deleteMessage(chatID, update.Message.MessageID)

	previousID, err := b.previousMessageID(user)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error checking message id in Redis: %v\n", err)
		return http.StatusBadRequest, err
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

	text, err := b.rdb.Get(context.Background(), fmt.Sprintf("user_%v_reminderText", user.ID)).Result()
	if err != nil && err != redis.Nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error checking reminderText in Redis: %v\n", err)
		return http.StatusBadRequest, err
	}

	issueID := strings.Split(user.Message.String, "_")[1]
	url := jiraReminderURL(issueID, update.Message.Text, neturl.QueryEscape(text))
	if err := b.authorizedPostWithRetry(url, token, user); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error setting reminder: %v\n", err)
		return http.StatusBadRequest, err
	}

	value, unit := reminderValueUnit(update.Message.Text)
	confirm := fmt.Sprintf("⏰ یادآوری شما برای %v %v دیگه تنظیم شد.", value, unit)
	if text != "" {
		confirm = fmt.Sprintf("⏰ یادآوری شما با عنوان «<b>%v</b>» برای %v %v دیگه تنظیم شد.", text, value, unit)
	}
	editMsg := tgbotapi.NewEditMessageText(chatID, previousID, confirm)
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println(err)
	}
	return http.StatusOK, nil
}

// rejectReminderTime re-prompts the user after an invalid duration input.
func (b *Bot) rejectReminderTime(update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	b.deleteMessage(chatID, update.Message.MessageID)

	previousID, err := b.previousMessageID(user)
	if err != nil {
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e
		}
		log.Printf("Error checking message id in Redis: %v\n", err)
		return http.StatusBadRequest, err
	}
	b.deleteMessage(chatID, previousID)

	issueID := strings.Split(user.Message.String, "_")[1]
	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, previousID, "⛔ این فرمت قابل قبول نیست. زمان رو به شکل عدد + حرف واحد بفرستید؛ مثلا 5m (۵ دقیقه)، 4h (۴ ساعت)، 5d (۵ روز) یا 2w (۲ هفته) — یا یکی از دکمه‌های زیر رو انتخاب کنید.", reminderTimeKeyboard(issueID))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending message in reminder: %v\n", err)
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e
		}
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleWaittingForCommentText(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID

	if err := b.resetState(user, chatID); err != nil {
		return http.StatusBadRequest, err
	}

	issueKey := strings.Split(user.Message.String, "_")[1]
	token, err := repositories.GetUserToken(b.db, user.ID)
	if err == sql.ErrNoRows {
		helpers.MarkTokenExpired(b.db, user, token)
		log.Println("the user is unauthorized")
		return http.StatusBadRequest, err
	} else if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusBadRequest, err
	}

	// Encoded with json.Marshal rather than hand-built with fmt.Sprintf: a comment
	// containing a quote, backslash or newline used to produce invalid JSON (or
	// worse, let the user's text alter the request's structure).
	url := fmt.Sprintf("%s/rest/api/2/issue/%v/comment", jiraURL, issueKey)
	payload := struct {
		Body string `json:"body"`
	}{Body: update.Message.Text}
	if err := b.postJSONWithRetry(url, payload, http.StatusCreated, token, user); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error posting comment: %v\n", err)
		return http.StatusBadRequest, err
	}

	previousID, err := b.previousMessageID(user)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error getting previous message ID from Redis: %v\n", err)
		return http.StatusBadRequest, err
	}
	b.deleteMessage(chatID, update.Message.MessageID)

	editMsg := tgbotapi.NewEditMessageText(chatID, previousID, fmt.Sprintf(`✅ کامنت شما با موفقیت برای <a href="%s">%v</a> ثبت شد`, issueLink(issueKey), issueKey))
	editMsg.ReplyMarkup = nil
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error updating comment message: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleWaittingForUnsubUserUsername(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	username := update.Message.Text

	if err := b.resetState(user, chatID); err != nil {
		return http.StatusBadRequest, err
	}

	filtered, err := repositories.GetFilteredUsers(b.db, user.ID)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching filtered users: %v\n", err)
		return http.StatusBadRequest, err
	}
	if helpers.Contains(filtered, username) {
		if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, "این کاربر از قبل تو لیست فیلترشده‌هاست و اعلان‌هاش رو دریافت نمی‌کنید.")); err != nil {
			return http.StatusBadRequest, err
		}
		return http.StatusOK, nil
	}

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err == sql.ErrNoRows {
		helpers.MarkTokenExpired(b.db, user, token)
		log.Println("the user is unauthorized")
		return http.StatusBadRequest, err
	} else if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusBadRequest, err
	}

	isValid, err := helpers.IsValidJiraUsername(jiraURL, username, token.Token)
	if err != nil {
		return http.StatusBadRequest, err
	}
	reply := "❌ این نام کاربری تو جیرا پیدا نشد. لطفا username دقیق کاربر (نه نام نمایشی) رو بفرستید."
	if isValid {
		if err := repositories.InsertFilteredUser(b.db, user.ID, username); err != nil {
			return http.StatusBadRequest, err
		}
		reply = fmt.Sprintf("✅ کاربر %v به لیست فیلترشده‌ها اضافه شد؛ از این به بعد اعلان‌هاش رو دریافت نمی‌کنید.", username)
	}
	if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, reply)); err != nil {
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleWaittingForAssignTaskUserSummary(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	b.deleteMessage(chatID, update.Message.MessageID)

	if err := b.resetState(user, chatID); err != nil {
		return http.StatusBadRequest, err
	}
	if err := b.rdb.LPush(context.Background(), assignTaskKey(user), fmt.Sprintf("summary_%v", update.Message.Text)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing summary to Redis: %v\n", err)
		return http.StatusBadRequest, err
	}

	previousID, err := b.previousMessageID(user)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error getting previous message ID from Redis: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForAssignTaskUserDescription}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error updating user: %v\n", err)
		return http.StatusBadRequest, err
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, previousID, "📝 حالا توضیحات تسک رو بنویسید؛ یعنی دقیقا چیکار باید انجام بشه:")
	editMsg.ReplyMarkup = nil
	message, err := b.bot.Send(editMsg)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error updating message: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := b.rdb.Set(context.Background(), messageIDKey(user), message.MessageID, 0).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error storing message id: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleWaittingForAssignTaskUserDescription(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	b.deleteMessage(chatID, update.Message.MessageID)

	if err := b.resetState(user, chatID); err != nil {
		return http.StatusBadRequest, err
	}

	values, err := b.rdb.LRange(context.Background(), assignTaskKey(user), 0, -1).Result()
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error retrieving list from Redis: %v\n", err)
		return http.StatusBadRequest, err
	}
	if len(values) < 3 {
		b.sendErrorMessage(chatID)
		log.Printf("Expected at least 3 values in the list, got %d\n", len(values))
		return http.StatusBadRequest, fmt.Errorf("incomplete assign-task data")
	}

	issueKey := redisFind(values, "projectId_")
	userAccountName := redisFind(values, "userAccountId_")
	summary := redisFind(values, "summary_")
	priority := redisFind(values, "priority_")
	description := update.Message.Text

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err == sql.ErrNoRows {
		helpers.MarkTokenExpired(b.db, user, token)
		log.Println("the user is unauthorized")
		return http.StatusBadRequest, err
	} else if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusBadRequest, err
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err
	}

	previousID, err := b.previousMessageID(user)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error getting previous message ID from Redis: %v\n", err)
		return http.StatusBadRequest, err
	}

	issueKeyID, err := helpers.CreateIssue(client, issueKey, summary, description, "Task", userAccountName, priority)
	if err != nil {
		log.Println("error creating issue", err)
		editMsg := tgbotapi.NewEditMessageText(chatID, previousID, "🚫 شما دسترسی ساخت تسک برای این پروژه رو ندارید. با ادمین پروژه یا پشتیبانی جیراتون تماس بگیرید.")
		editMsg.ReplyMarkup = nil
		if _, err := b.bot.Send(editMsg); err != nil {
			b.sendErrorMessage(chatID)
			log.Printf("Error updating message: %v\n", err)
			return http.StatusBadRequest, err
		}
		if err := repositories.UpdateUser(b.db, user, []interface{}{""}, "stat"); err != nil {
			b.sendErrorMessage(chatID)
			log.Println("error updating user", err)
			return http.StatusBadRequest, err
		}
		return http.StatusBadRequest, err
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, previousID, fmt.Sprintf(`✅ <a href="%s">تسک</a> شما با موفقیت ساخته شد`, issueLink(issueKeyID)))
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = nil
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error updating message: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleWaittingForLogtimeTimeSpent(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID

	if err := b.rdb.LPush(context.Background(), logtimeKey(user), fmt.Sprintf("timeSpent_%v", update.Message.Text)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing to Redis: %v\n", err)
		return http.StatusBadRequest, err
	}

	message, err := b.bot.Send(tgbotapi.NewMessage(chatID, "📝 توضیحات کار انجام‌شده روی این تسک رو بنویسید:"))
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending message: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := b.rdb.Set(context.Background(), messageIDKey(user), message.MessageID, 0).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error storing message id: %v\n", err)
		return http.StatusBadRequest, err
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForLogtimeDescription}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error updating user: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func HandleWaittingForLogtimeDescription(b *Bot, update tgbotapi.Update, user models.User) (int, error) {
	chatID := update.Message.Chat.ID
	b.deleteMessage(chatID, update.Message.MessageID)

	if err := b.resetState(user, chatID); err != nil {
		return http.StatusBadRequest, err
	}

	values, err := b.rdb.LRange(context.Background(), logtimeKey(user), 0, -1).Result()
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error retrieving list from Redis: %v\n", err)
		return http.StatusBadRequest, err
	}
	if len(values) < 2 {
		b.sendErrorMessage(chatID)
		log.Printf("Expected at least 2 values in the list, got %d\n", len(values))
		return http.StatusBadRequest, fmt.Errorf("incomplete logtime data")
	}

	issueKey := redisFind(values, "taskId_")
	timeSpent := redisFind(values, "timeSpent_")
	fmtDay := redisFind(values, "logTimeDay_")
	description := update.Message.Text

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err == sql.ErrNoRows {
		helpers.MarkTokenExpired(b.db, user, token)
		log.Println("the user is unauthorized")
		return http.StatusBadRequest, err
	} else if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusBadRequest, err
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err
	}

	previousID, err := b.previousMessageID(user)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error getting previous message ID from Redis: %v\n", err)
		return http.StatusBadRequest, err
	}

	if issueKey == "" || timeSpent == "" || description == "" {
		b.sendErrorMessage(chatID)
		log.Println("Missing required fields for logging work time.")
		return http.StatusBadRequest, fmt.Errorf("missing required fields: issueKey=%s, timeSpent=%s, description=%s", issueKey, timeSpent, description)
	}

	parsedTime, err := time.Parse("2006-01-02", fmtDay)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error parsing date: %v\n", err)
		return http.StatusBadRequest, err
	}
	jiraTime := jira.Time(parsedTime)

	worklog, _, err := client.Issue.AddWorklogRecord(issueKey, &jira.WorklogRecord{
		Comment:   description,
		TimeSpent: timeSpent,
		Started:   &jiraTime,
	})
	if err != nil {
		log.Printf("Error logging work time in Jira: %v\n", err)
		if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, "⚠️ ثبت لاگ‌تایم برای این تسک با خطا مواجه شد. لطفا دوباره امتحان کنید یا با پشتیبانی تماس بگیرید.")); err != nil {
			b.sendErrorMessage(chatID)
			log.Printf("Error sending failure message: %v\n", err)
			return http.StatusBadRequest, err
		}
		return http.StatusBadRequest, err
	}

	// The callback prefix here must match the "deleteWorklog" key registered in
	// privateCallbackHandlers (router.go) / handled by HandleDeleteWorklog. It
	// used to read "deleteWorkfmt", so this button silently did nothing when
	// pressed. There is no "Edit" handler implemented anywhere, so that button
	// is dropped rather than shipping a control that can't do anything.
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑️ حذف", fmt.Sprintf("deleteWorklog_%v_%v", worklog.IssueID, worklog.ID)),
		),
	)
	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, previousID,
		fmt.Sprintf(`✅ <a href="%s">لاگ‌تایم</a> شما با موفقیت به مدت زمان <b>%v</b> ثبت شد.`, issueLink(issueKey), timeSpent), keyboard)
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error updating success message: %v\n", err)
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}
