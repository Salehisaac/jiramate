package keyboards

import tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"

var CustomerMainMessageInlibeKeyboard = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("ثبت درخواست", "Request_registration"),
		tgbotapi.NewInlineKeyboardButtonData("پیگیری درخواست", "tracking_request"),
	),
)

var ReturnToMainButton = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("بازگشت ⬅️", "cancelRequest"),
	),
)

var WaitButton = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⌛️ لطفا کمی صبر کنید...", "wait"),
	),
)

var MainMenuInlineButton = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔨 ایجاد تسک جدید", "createTaskBtn"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("💬 ارسال وضعیت پروژه در گروه", "attachGroupProjectSelect"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🕝 ثبت زمان تسک ها", "log time"),
	),
	// tgbotapi.NewInlineKeyboardRow(
	// 	tgbotapi.NewInlineKeyboardButtonData("بررسی  زمان تسک ها ⏳", "worklogs"),
	// ),
	// Notification management is temporarily disabled.
	// tgbotapi.NewInlineKeyboardRow(
	// 	tgbotapi.NewInlineKeyboardButtonData("⚙️ مدیریت اعلان ها", "settings"),
	// ),
)

var GroupMainMenuInlineButton = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("انتخاب پروژه ی جدید⚙️", "attachGroupProjectSelect"),
	),
)

var UnsubescribeInlineButton = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("فیلتر کردن پروژه", "projectUnsub"),
		tgbotapi.NewInlineKeyboardButtonData("فیلتر کردن فرد", "userUnsub"),
	),
)

var SubescribeInlineButton = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("دنبال کردن پروژه", "projectSub"),
		tgbotapi.NewInlineKeyboardButtonData("دنبال کردن فرد", "userSub"),
	),
)

var SettingsInlineButton = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("حذف کردن", "unsubscribe"),
		tgbotapi.NewInlineKeyboardButtonData("اضافه کردن", "subscribe"),
	),
)
