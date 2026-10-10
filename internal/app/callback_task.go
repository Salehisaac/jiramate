package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
)

func assignTaskKey(user models.User) string { return fmt.Sprintf("user_%v_assignTask", user.ID) }
func messageIDKey(user models.User) string  { return fmt.Sprintf("user_%v_messageId", user.ID) }

func HandleAssingTask(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err != nil {
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusUnauthorized, err, false
	}
	if err := b.answerCallback(cq, pleaseWaitText); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Failed to send callback query response: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	if err := b.rdb.LPush(context.Background(), assignTaskKey(user), fmt.Sprintf("projectId_%v", id)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing to Redis: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	roles, err := helpers.FetchProjectRoles(client, jiraURL, token.Token, id)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error fetching project roles: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	users, errs := helpers.FetchProjectUsers(roles, token.Token)
	if len(errs) != 0 {
		log.Printf("Error fetching project users %v\n", errs)
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "🚫 شما اجازه‌ی ساخت تسک برای این پروژه رو ندارید. یه پروژه‌ی دیگه رو امتحان کنید یا با ارسال /menu به منوی اصلی برگردید.")
		if _, err := b.bot.Send(editMsg); err != nil {
			log.Println("error sending message", err)
		}
		return http.StatusBadRequest, fmt.Errorf("cannot fetch project users"), false
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "👤 کاربری که می‌خواید این تسک رو بهش اختصاص بدید رو از لیست زیر انتخاب کنید:", roleUserKeyboard(users, "selectUserForTask"))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleExistingTask(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err != nil {
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusUnauthorized, err, false
	}
	if err := b.answerCallback(cq, pleaseWaitText); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending toast alert: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	tasks, err := helpers.FetchProjectTasks(client, id)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error fetching project tasks: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "یکی از تسک‌های زیر رو انتخاب کنید:", taskKeyboard(tasks, "selectUserForExistingTask"))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleSelectUserForTask(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	if err := b.rdb.LPush(context.Background(), assignTaskKey(user), fmt.Sprintf("userAccountId_%v", id)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing to Redis: %v\n", err)
		return http.StatusUnauthorized, err, false
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "اهمیت تسک رو از دکمه‌های زیر انتخاب کنید:", priorityKeyboard())
	message, err := b.bot.Send(editMsg)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("Error updating message", err)
		return http.StatusUnauthorized, err, false
	}
	if err := b.rdb.Set(context.Background(), messageIDKey(user), message.MessageID, 0).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("Error storing message id", err)
		return http.StatusUnauthorized, err, false
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForAssignTaskUserSummary}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusUnauthorized, err, false
	}
	return http.StatusOK, nil, true
}

func HandleAssignTaskPriority(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	if err := b.rdb.LPush(context.Background(), assignTaskKey(user), fmt.Sprintf("priority_%v", id)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing to Redis: %v\n", err)
		return http.StatusUnauthorized, err, false
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForAssignTaskUserSummary}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusUnauthorized, err, false
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "📝 عنوان تسک رو به صورت یک پیام متنی بفرستید:")
	editMsg.ReplyMarkup = nil
	message, err := b.bot.Send(editMsg)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("Error updating message", err)
		return http.StatusUnauthorized, err, false
	}
	if err := b.rdb.Set(context.Background(), messageIDKey(user), message.MessageID, 0).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("Error storing message id", err)
		return http.StatusUnauthorized, err, false
	}
	return http.StatusOK, nil, true
}

func HandleSelectUserForExistingTask(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err == sql.ErrNoRows {
		helpers.MarkTokenExpired(b.db, user, token)
		log.Println("the user is unauthorized")
		return http.StatusBadRequest, err, false
	} else if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	if err := b.rdb.LPush(context.Background(), assignTaskKey(user), fmt.Sprintf("taskId_%v", id)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing to Redis: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	values, err := b.rdb.LRange(context.Background(), assignTaskKey(user), 0, -1).Result()
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error reading Redis list: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	projectID := redisFind(values, "projectId_")

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	roles, err := helpers.FetchProjectRoles(client, jiraURL, token.Token, projectID)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("Error fetching project roles", err)
		return http.StatusBadRequest, err, false
	}
	users, errs := helpers.FetchProjectUsers(roles, token.Token)
	if len(errs) != 0 {
		log.Println("Error fetching project users")
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "🚫 شما اجازه‌ی ساخت تسک برای این پروژه رو ندارید. یه پروژه‌ی دیگه رو امتحان کنید یا با ارسال /menu به منوی اصلی برگردید.")
		if _, err := b.bot.Send(editMsg); err != nil {
			log.Println("error sending message", err)
		}
		return http.StatusBadRequest, fmt.Errorf("cannot fetch project users"), false
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "👤 کاربری که می‌خواید این تسک رو بهش اختصاص بدید رو از لیست زیر انتخاب کنید:", roleUserKeyboard(users, "assignUserToTask"))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleAssignUserToTask(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err == sql.ErrNoRows {
		helpers.MarkTokenExpired(b.db, user, token)
		log.Println("the user is unauthorized")
		return http.StatusBadRequest, err, false
	} else if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	values, err := b.rdb.LRange(context.Background(), assignTaskKey(user), 0, -1).Result()
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error reading Redis list: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	taskID := redisFind(values, "taskId_")

	resp, err := helpers.AssignUserToTask(client, id, taskID)
	if err != nil || resp != "204 No Content" {
		b.sendErrorMessage(chatID)
		log.Println("error assigning task for user", err)
		return http.StatusBadRequest, err, false
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID,
		fmt.Sprintf(`✅ انجام شد. تسک <a href="%s">%v</a> برای <b>%v</b> ثبت شد.`, issueLink(taskID), taskID, id))
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleAssignTaskFromForwardedMessage(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	id := callbackArg(cq, 1)

	token, err := repositories.GetUserToken(b.db, user.ID)
	if err != nil {
		log.Printf("error fetching user token: %v\n", err)
		return http.StatusUnauthorized, err, false
	}
	if err := b.answerCallback(cq, pleaseWaitText); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Failed to send callback query response: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	if err := b.rdb.LPush(context.Background(), assignTaskKey(user), fmt.Sprintf("userAccountId_%v", id)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing to Redis: %v\n", err)
		return http.StatusUnauthorized, err, false
	}

	intID, err := strconv.Atoi(id)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error converting id to int")
		return http.StatusUnauthorized, err, false
	}
	targetUser, err := repositories.GetUserByChatID(b.db, int64(intID))
	if err == sql.ErrNoRows {
		msg := tgbotapi.NewMessage(chatID, "😕 این کاربر هنوز از ربات استفاده نکرده، برای همین نمی‌تونم براش تسک بسازم. ازش بخواید اول با ارسال /start ربات رو راه‌اندازی کنه.")
		if _, err := b.bot.Send(msg); err != nil {
			b.sendErrorMessage(chatID)
			log.Println("error sending message", err)
		}
		return http.StatusUnauthorized, err, false
	} else if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error getting the user from db", err)
		return http.StatusUnauthorized, err, false
	}

	// targetUserToken belongs to targetUser (the forwarded-from person), not to
	// the acting user. Marking it expired with (user, token) here used to expire
	// the acting user's own perfectly valid token and log *them* out, purely
	// because someone else hadn't linked Jira - so on ErrNoRows we only inform
	// the acting user and leave everyone's auth state untouched.
	targetUserToken, err := repositories.GetUserToken(b.db, targetUser.ID)
	if err == sql.ErrNoRows {
		msg := tgbotapi.NewMessage(chatID, "😕 این کاربر هنوز حساب جیراش رو به ربات وصل نکرده، برای همین نمی‌تونم براش تسک بسازم. ازش بخواید اول با ارسال /connect حسابش رو وصل کنه.")
		if _, err := b.bot.Send(msg); err != nil {
			log.Println("error sending message", err)
		}
		return http.StatusUnauthorized, err, false
	} else if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching target user token: %v\n", err)
		return http.StatusUnauthorized, err, false
	}

	targetClient, err := newJiraClient(targetUserToken.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("Error creating Jira client", err)
		return http.StatusUnauthorized, err, false
	}
	targetProjects, err := helpers.GetAllProjects(targetClient, jiraURL)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error getting target user projects", err)
		return http.StatusUnauthorized, err, false
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("Error creating Jira client", err)
		return http.StatusUnauthorized, err, false
	}
	projects, err := helpers.GetAllProjects(client, jiraURL)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error getting user projects", err)
		return http.StatusUnauthorized, err, false
	}

	commonProjects := intersectProjects(projects, targetProjects)

	previousMessageID, err := b.rdb.Get(context.Background(), messageIDKey(user)).Result()
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error getting message id in Redis: %v\n", err)
		return http.StatusUnauthorized, err, false
	}
	previousMessageIDInt, _ := strconv.Atoi(previousMessageID)

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, previousMessageIDInt,
		"📁 یکی از پروژه‌های زیر رو برای این تسک انتخاب کنید:", projectListKeyboard(commonProjects, "assignTaskForwardedMessage"))
	message, err := b.bot.Send(editMsg)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message in assign task", err)
		return http.StatusUnauthorized, err, false
	}
	if err := b.rdb.Set(context.Background(), messageIDKey(user), message.MessageID, 0).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error storing message id: %v\n", err)
		return http.StatusUnauthorized, err, false
	}
	return http.StatusOK, nil, true
}

func HandleAssignTaskForwardedMessage(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	if err := b.rdb.LPush(context.Background(), assignTaskKey(user), fmt.Sprintf("projectId_%v", id)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing to Redis: %v\n", err)
		return http.StatusUnauthorized, err, false
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "توضیحات این تسک رو به صورت یک پیام متنی بفرستید:")
	editMsg.ReplyMarkup = nil
	message, err := b.bot.Send(editMsg)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error updating message: %v\n", err)
		return http.StatusUnauthorized, err, false
	}
	if err := b.rdb.Set(context.Background(), messageIDKey(user), message.MessageID, 0).Err(); err != nil {
		b.sendErrorMessage(chatID)
		return http.StatusUnauthorized, err, false
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForAssignTaskUserDescription}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusUnauthorized, err, false
	}
	return http.StatusOK, nil, true
}

func HandleAssignInlineBtn(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID

	if err := b.rdb.Del(context.Background(), assignTaskKey(user)).Err(); err != nil {
		log.Printf("Error deleting list from Redis: %v\n", err)
		return http.StatusBadRequest, err, true
	}
	token, ok := b.tokenForUser(user, chatID)
	if !ok {
		return http.StatusBadRequest, nil, true
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err, true
	}
	projects, err := helpers.GetAllProjects(client, jiraURL)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching projects: %v\n", err)
		return http.StatusBadRequest, err, true
	}
	if len(projects) == 0 {
		return b.editNoProjects(chatID, messageID, user)
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "📁 برای ساخت تسک جدید، یکی از پروژه‌های زیر رو انتخاب کنید:", projectListKeyboard(projects, "createTask"))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending inline keyboard message:", err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}

// intersectProjects returns the projects present in both slices, keyed by ID.
func intersectProjects(a, b []models.Project) []models.Project {
	inA := make(map[string]bool, len(a))
	for _, p := range a {
		inA[p.ID] = true
	}
	var common []models.Project
	for _, p := range b {
		if inA[p.ID] {
			common = append(common, p)
		}
	}
	return common
}

// editNoProjects reports that the user has no accessible projects.
func (b *Bot) editNoProjects(chatID int64, messageID int, user models.User) (int, error, bool) {
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "😕 در حال حاضر به هیچ پروژه‌ای در جیرا دسترسی ندارید. برای دسترسی به یک پروژه، با ادمین جیرا تماس بگیرید.")
	if _, err := b.bot.Send(editMsg); err != nil {
		log.Printf("Error sending message to user %v: %v\n", user.ChatID, err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusBadRequest, nil, true
}
