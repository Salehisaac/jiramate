package app

import (
	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"

	"rira/internal/models"
)

// Conversation states stored on the user record / detected per update.
const (
	Unauthorized  = "unauthorized"
	Start         = "start"
	Assign        = "assign"
	Menu          = "menu"
	Settings      = "settings"
	Voice         = "voice"
	Connect       = "connect"
	ForwardAssign = "assign from forwarded message"

	WaittingForToken = "waitting for token"

	WaittingForReminderText = "waiiting for reminder text"
	WaittingForReminderTime = "waiiting for reminder time"

	WaittingForCommentText = "waiiting for comment text"

	WaittingForAssignTaskUserSummary     = "waiiting for asign task summary"
	WaittingForAssignTaskUserDescription = "waiiting for asign task description"

	WaittingForUnsubUserUsername = "waitting for usub user username"
	WaittingForSubUserUsername   = "waitting for sub user username"

	WaittingForLogtimeDescription = "waitting for logtime description"
	WaittingForLogtimeTimeSpent   = "waitting for logtime time spent"

	// Group / admin states.
	StartAdmin           = "start admin"
	SetToken             = "set token"
	MenuInGroup          = "menu in group"
	WaittingForChatToken = "waitting for chat token"
)

// Handler signatures dispatched by the router.
type (
	CallbackHandler func(*Bot, *tgbotapi.CallbackQuery, models.User) (int, error, bool)
	UpdateHandler   func(*Bot, tgbotapi.Update, models.User) (int, error)
	// ResponseCallbackHandler receives the acting user (needed to scope the
	// notification row it updates - see notification_repository.go), the chat
	// the callback came from, and the message being refreshed.
	ResponseCallbackHandler func(*Bot, models.User, int64, int) error
)
