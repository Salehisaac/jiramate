package app

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
)

var privateUpdateHandlers = map[string]UpdateHandler{
	Unauthorized:  HandleUnauthorizedMessage,
	Start:         HandleStart,
	Assign:        HandleAssign,
	Voice:         HandleVoice,
	Connect:       HandleUnauthorized,
	ForwardAssign: HandleForwardAssign,
	Menu:          HandleMenu,
	// Settings: disabled for now (notification management).
	// Settings:                            HandleSettings,
	WaittingForToken:                     HandleWaitingForToken,
	WaittingForReminderText:              HandleWaittingForReminderText,
	WaittingForReminderTime:              HandleWaittingForReminderTime,
	WaittingForCommentText:               HandleWaittingForCommentText,
	WaittingForAssignTaskUserSummary:     HandleWaittingForAssignTaskUserSummary,
	WaittingForAssignTaskUserDescription: HandleWaittingForAssignTaskUserDescription,
	WaittingForUnsubUserUsername:         HandleWaittingForUnsubUserUsername,
	WaittingForLogtimeTimeSpent:          HandleWaittingForLogtimeTimeSpent,
	WaittingForLogtimeDescription:        HandleWaittingForLogtimeDescription,
}

var groupUpdateHandlers = map[string]UpdateHandler{
	StartAdmin:           HandleStartAdmin,
	SetToken:             HandleSetToken,
	MenuInGroup:          HandleMenuInGroup,
	WaittingForChatToken: HandleWaitingForChatToken,
}

var privateCallbackHandlers = map[string]CallbackHandler{
	"vote":                           HandleVote,
	"unvote":                         HandleUnvote,
	"watch":                          HandleWatch,
	"unwatch":                        HandleUnwatch,
	"star":                           HandleStar,
	"unstar":                         HandleUnstar,
	"unread":                         HandleUnread,
	"read":                           HandleRead,
	"reminder":                       HandleReminder,
	"reminderHoure":                  HandleReminderHoure,
	"noText":                         HandleNoText,
	"comment":                        HandleComment,
	"delete":                         HandleDelete,
	"createTask":                     HandleAssingTask,
	"existedTask":                    HandleExistingTask,
	"selectUserForTask":              HandleSelectUserForTask,
	"assignTaskFromForwardedMessage": HandleAssignTaskFromForwardedMessage,
	"assignTaskForwardedMessage":     HandleAssignTaskForwardedMessage,
	"priority":                       HandleAssignTaskPriority,
	"selectUserForExistingTask":      HandleSelectUserForExistingTask,
	"assignUserToTask":               HandleAssignUserToTask,
	"cancelRequest":                  HandleCancelToMainMenu,
	"worklogs":                       HandleWorklogTime,
	"speceficWorkLog":                HandleWorklogSpeceficTime,
	"createTaskBtn":                  HandleAssignInlineBtn,
	"returnToWorklogTime":            HandleWorklogTime,
	"unsubscribe":                    HandleUnsubscribe,
	"subscribe":                      HandleSubscribe,
	"projectUnsub":                   HandleUnsubProject,
	"projectSub":                     HandleSubProject,
	"unsubProject":                   HandleUnsubProjectInsert,
	"subProject":                     HandleSubProjectInsert,
	"userUnsub":                      HandleUnsubUser,
	"userSub":                        HandleSubUser,
	"subUser":                        HandleSubUserInsert,
	// "settings": disabled for now (notification management).
	// "settings":                       HandleSettingsInline,
	"log time":                 HandleSelectProjectForLogTime,
	"selectTaskForLogTime":     HandleSelectTaskForLogTime,
	"getLogtimeDay":            HandleGetTaskLogTimeDay,
	"getLogtimeTime":           HandleGetTaskLogTime,
	"createLogTime":            HandleGetTaskLogTime,
	"logtimePage":              HandleCalenderPagintation,
	"taskPaginationNext":       HandleSelectTaskPaginationNext,
	"taskPaginationPre":        HandleSelectTaskPaginationPre,
	"deleteWorklog":            HandleDeleteWorklog,
	"attachGroupProjectSelect": HandleAttachGroupProjectSelect,
	"selectProjectForChat":     HandleSelectProjectForChat,
}

var groupCallbackHandlers = map[string]CallbackHandler{}

var responseCallbackHandlers = map[string]ResponseCallbackHandler{
	"vote":          HandleVoteResponse,
	"unvote":        HandleUnvoteResponse,
	"watch":         HandleWatchResponse,
	"unwatch":       HandleUneatchResponse,
	"star":          HandleStarResponse,
	"unstar":        HandleUnstarResponse,
	"read":          HandleReadResponse,
	"unread":        HandleUreadResponse,
	"reminderHoure": HandleReminderHoureResponse,
}

func statIs(user models.User, s string) bool   { return user.Stat != nil && *user.Stat == s }
func statusIs(user models.User, s string) bool { return user.Status != nil && *user.Status == s }

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	switch {
	case update.Message != nil:
		b.handleMessage(update)
	case update.CallbackQuery != nil:
		b.handleCallback(update)
	}
}

func (b *Bot) handleMessage(update tgbotapi.Update) {
	if update.Message == nil || update.Message.Chat == nil {
		return
	}
	chatID := update.Message.Chat.ID
	user, err := repositories.GetUserByChatID(b.db, chatID)
	if err != nil {
		if err != sql.ErrNoRows {
			b.sendErrorMessage(chatID)
			log.Printf("Error getting user from database: %v\n", err)
			return
		}
		if !helpers.IsGroupMessage(&update) {
			b.welcomeNewUser(update)
			return
		}
		// Unknown user in a group: proceed with a zero-value user.
	}

	if update.Message.Chat.Type == "private" {
		b.dispatchUpdate(privateUpdateHandlers, b.privateCommand(update, user), update, user)
	} else {
		b.dispatchUpdate(groupUpdateHandlers, b.groupCommand(update, user), update, user)
	}
}

func (b *Bot) dispatchUpdate(handlers map[string]UpdateHandler, command string, update tgbotapi.Update, user models.User) {
	handler, ok := handlers[command]
	if !ok {
		log.Printf("Unknown command: %v\n", command)
		return
	}
	if status, err := handler(b, update, user); status != http.StatusOK {
		log.Printf("error handling update: %v\n", err)
	}
}

func (b *Bot) privateCommand(update tgbotapi.Update, user models.User) string {
	text := update.Message.Text
	switch {
	case text == "/start":
		return Start
	case text == "/assign" || text == "assign":
		return Assign
	case text == "/connect":
		return Connect
	case statIs(user, WaittingForToken):
		return WaittingForToken
	case statusIs(user, "unauthorized"):
		return Unauthorized
	case text == "/menu" || text == "menu":
		return Menu
	// Settings: disabled for now (notification management).
	// case text == "/settings" || text == "settings":
	// 	return Settings
	case statIs(user, WaittingForUnsubUserUsername):
		return WaittingForUnsubUserUsername
	case update.Message.Voice != nil:
		return Voice
	case update.Message.ForwardFrom != nil && update.Message.ForwardFrom.ID != 0:
		return ForwardAssign
	case statIs(user, WaittingForReminderText):
		return WaittingForReminderText
	case statIs(user, WaittingForReminderTime):
		return WaittingForReminderTime
	case statIs(user, WaittingForCommentText):
		return WaittingForCommentText
	case statIs(user, WaittingForAssignTaskUserSummary):
		return WaittingForAssignTaskUserSummary
	case statIs(user, WaittingForAssignTaskUserDescription):
		return WaittingForAssignTaskUserDescription
	case statIs(user, WaittingForLogtimeTimeSpent):
		return WaittingForLogtimeTimeSpent
	case statIs(user, WaittingForLogtimeDescription):
		return WaittingForLogtimeDescription
	}
	return ""
}

func (b *Bot) groupCommand(update tgbotapi.Update, user models.User) string {
	self := b.bot.Self.UserName
	text := update.Message.Text
	switch {
	case text == "/start" || text == "/start@"+self:
		return StartAdmin
	case text == "/setToken" || text == "/setToken@"+self || text == "/setToken@":
		return SetToken
	case text == "/menu" || text == "/menu@"+self:
		return MenuInGroup
	case statIs(user, WaittingForChatToken):
		return WaittingForChatToken
	}
	return ""
}

func (b *Bot) welcomeNewUser(update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	msg := tgbotapi.NewMessage(chatID, WelcomeMessage)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonURL("پنل کاربری", jiraURL+"/secure/ViewProfile.jspa"),
	})
	if _, err := b.bot.Send(msg); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending the message: %v\n", err)
		return
	}
	if err := repositories.InsertUserIntoDatabase(update.Message.Chat, b.db); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error inserting user into database: %v\n", err)
	}
}

func (b *Bot) handleCallback(update tgbotapi.Update) {
	cq := update.CallbackQuery
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	command := strings.Split(cq.Data, "_")[0]

	user, err := repositories.GetUserByChatID(b.db, chatID)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error getting user from database: %v\n", err)
		return
	}

	handlers := privateCallbackHandlers
	if user.Type != "private" {
		handlers = groupCallbackHandlers
	}

	handler, ok := handlers[command]
	if !ok {
		log.Printf("Unknown callback query: %v\n", command)
		return
	}
	status, err, needReturn := handler(b, cq, user)
	if status != http.StatusOK {
		log.Printf("error handling callback query %q for user %d: %v\n", command, user.ID, err)
		return
	}
	if needReturn {
		return
	}

	b.refreshNotificationKeyboard(command, user, chatID, messageID)
}

// refreshNotificationKeyboard runs the response handler for vote/watch/star/read
// style callbacks and re-renders the notification's inline keyboard.
func (b *Bot) refreshNotificationKeyboard(command string, user models.User, chatID int64, messageID int) {
	handler, ok := responseCallbackHandlers[command]
	if !ok {
		log.Printf("Unknown callback query: %v\n", command)
		return
	}
	if err := handler(b, user, chatID, messageID); err != nil {
		log.Printf("error handling callback query: %v\n", err)
		return
	}

	// Scoped by user_id: message IDs are only unique within a single chat, so
	// looking this up by messageID alone could return a different user's card.
	notification, err := repositories.GetNotification(b.db, user.ID, messageID)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error getting notification for user %d: %v\n", user.ID, err)
		return
	}

	editMsg := tgbotapi.NewEditMessageReplyMarkup(chatID, messageID, notificationKeyboard(notification))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error updating message: %v\n", err)
	}
}
