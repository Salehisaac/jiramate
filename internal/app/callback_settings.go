package app

import (
	"fmt"
	"log"
	"net/http"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
	"rira/static/keyboards"
)

func HandleCancelToMainMenu(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	if err := b.resetState(user, chatID); err != nil {
		return http.StatusBadRequest, err, false
	}
	editMsg := tgbotapi.NewEditMessageText(chatID, cq.Message.MessageID, "سلام 👋 یکی از گزینه‌های زیر رو انتخاب کنید:")
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message in HandleCancelToMainMenu", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleSettingsInline(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, cq.Message.MessageID, "چطور می‌خواید اعلان‌های خودتون رو مدیریت کنید؟ می‌تونید پروژه یا فرد خاصی رو فیلتر یا دنبال کنید.", keyboards.SettingsInlineButton)
	if _, err := b.bot.Send(msg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message in HandleSettings", err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, false
}

func HandleUnsubscribe(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.editSettingsMenu(cq, user, keyboards.UnsubescribeInlineButton)
}

func HandleSubscribe(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	return b.editSettingsMenu(cq, user, keyboards.SubescribeInlineButton)
}

func (b *Bot) editSettingsMenu(cq *tgbotapi.CallbackQuery, user models.User, markup tgbotapi.InlineKeyboardMarkup) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	if err := b.resetState(user, chatID); err != nil {
		return http.StatusBadRequest, err, true
	}
	msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, cq.Message.MessageID, "پروژه یا فرد مورد نظر رو انتخاب کنید:", markup)
	if _, err := b.bot.Send(msg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending settings menu", err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}

func HandleUnsubProject(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID

	if err := b.answerCallback(cq, pleaseWaitText); err != nil {
		if e := b.resetState(user, chatID); e != nil {
			return http.StatusBadRequest, e, false
		}
		b.sendErrorMessage(chatID)
		log.Printf("Failed to send callback query response: %v\n", err)
		return http.StatusBadRequest, err, false
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

	filtered, _ := repositories.GetFilteredProjects(b.db, user.ID)
	var available []models.Project
	for _, p := range projects {
		if !helpers.Contains(filtered, p.Key) {
			available = append(available, p)
		}
	}

	msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "پروژه‌ای که می‌خواید اعلان‌هاش رو فیلتر کنید (متوقف کنید) رو انتخاب کنید:", projectListKeyboard(available, "unsubProject"))
	if _, err := b.bot.Send(msg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message in HandleUnsubProject", err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}

func HandleSubProject(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID

	filtered, err := repositories.GetFilteredProjects(b.db, user.ID)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching projects: %v\n", err)
		return http.StatusBadRequest, err, true
	}
	if len(filtered) == 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "در حال حاضر هیچ پروژه‌ی فیلترشده‌ای ندارید که بخواید دوباره فعالش کنید.")
		if _, err := b.bot.Send(editMsg); err != nil {
			log.Printf("Error sending message to user %v: %v\n", user.ChatID, err)
		}
		return http.StatusBadRequest, nil, true
	}

	msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "پروژه‌ای که می‌خواید اعلان‌هاش رو دوباره دریافت کنید رو انتخاب کنید:", stringListKeyboard(filtered, "subProject"))
	if _, err := b.bot.Send(msg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message in HandleSubProject", err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}

func HandleUnsubUser(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	editMsg := tgbotapi.NewEditMessageText(chatID, cq.Message.MessageID, "نام کاربری (username) کاربری که می‌خواید اعلان‌هاش رو فیلتر کنید رو به صورت متن بفرستید:")
	if _, err := b.bot.Send(editMsg); err != nil {
		return http.StatusBadRequest, err, true
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForUnsubUserUsername}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusUnauthorized, err, false
	}
	return http.StatusOK, nil, true
}

func HandleSubUser(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID

	filtered, err := repositories.GetFilteredUsers(b.db, user.ID)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error fetching filterd users: %v\n", err)
		return http.StatusBadRequest, err, true
	}
	if len(filtered) == 0 {
		editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "در حال حاضر هیچ کاربر فیلترشده‌ای ندارید که بخواید دوباره دنبالش کنید.")
		if _, err := b.bot.Send(editMsg); err != nil {
			log.Printf("Error sending message to user %v: %v\n", user.ChatID, err)
		}
		return http.StatusOK, nil, true
	}

	msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "کاربری که می‌خواید اعلان‌هاش رو دوباره دریافت کنید رو انتخاب کنید:", stringListKeyboard(filtered, "subUser"))
	if _, err := b.bot.Send(msg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message in HandleSubUser", err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}

func HandleSubUserInsert(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	username := callbackArg(cq, 1)

	owner, err := repositories.GetUserByChatID(b.db, chatID)
	if err != nil {
		return http.StatusBadRequest, err, true
	}
	if err := repositories.DeleteFilteredUser(b.db, owner.ID, username); err != nil {
		return http.StatusBadRequest, err, true
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, cq.Message.MessageID, fmt.Sprintf(`✅ انجام شد. از این پس اعلان‌های <b>%v</b> دوباره برای شما ارسال میشه.`, username))
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}

func HandleUnsubProjectInsert(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	projectKey := callbackArg(cq, 1)

	owner, err := repositories.GetUserByChatID(b.db, chatID)
	if err != nil {
		return http.StatusBadRequest, err, true
	}
	if err := repositories.InsertFilteredProject(b.db, owner.ID, projectKey); err != nil {
		return http.StatusBadRequest, err, true
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, cq.Message.MessageID, fmt.Sprintf(`✅ انجام شد. از این پس اعلان‌های پروژه‌ی <a href="%s">%v</a> برای شما ارسال نمیشه.`, issueLink(projectKey), projectKey))
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}

func HandleSubProjectInsert(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	projectKey := callbackArg(cq, 1)

	owner, err := repositories.GetUserByChatID(b.db, chatID)
	if err != nil {
		return http.StatusBadRequest, err, true
	}
	if err := repositories.DeleteFilteredProject(b.db, owner.ID, projectKey); err != nil {
		return http.StatusBadRequest, err, true
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, cq.Message.MessageID, fmt.Sprintf(`✅ انجام شد. از این پس اعلان‌های پروژه‌ی <a href="%s">%v</a> دوباره برای شما ارسال میشه.`, issueLink(projectKey), projectKey))
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}
