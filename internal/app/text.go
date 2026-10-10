package app

import (
	"log"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"

	"rira/internal/models"
)

const WelcomeMessage = `سلام 👋 به JiraMate خوش اومدید!

برای شروع، باید حساب جیرای خودتون رو به این ربات وصل کنید. کافیه این ۳ مرحله رو انجام بدید:

1️⃣ با دکمه «پنل کاربری» زیر همین پیام، وارد پروفایل جیراتون بشید.
2️⃣ از بخش Personal Access Tokens (تنظیمات حساب کاربری)، یک توکن جدید بسازید.
3️⃣ توکن ساخته‌شده رو کپی کنید و همینجا برای من به صورت یک پیام متنی ساده بفرستید (بدون هیچ توضیح اضافه‌ای).

به محض دریافت توکن، اتصال برقرار میشه و می‌تونید از همه‌ی امکانات ربات استفاده کنید. اگه در هر مرحله سوالی داشتید، کافیه بپرسید 🙂`

const startMessage = `سلام 👋

به JiraMate خوش اومدید! این ربات کمکتون میکنه بدون باز کردن جیرا، از همینجا با پروژه‌هاتون در ارتباط باشید.

با این ربات می‌تونید:
• اعلان‌های جیرا (کامنت، تغییر وضعیت و ...) رو همینجا دریافت کنید و مستقیم روشون رأی بدید، دنبالشون کنید، ستاره‌دارشون کنید یا کامنت بذارید.
• اگه دسترسی داشته باشید، تسک جدید برای یک پروژه یا یک همکار بسازید.
• برای هر اعلان یادآوری تنظیم کنید؛ کافیه روی پیام ریپلای بزنید و با صدا بگید چه زمانی و با چه توضیحی یادتون بندازه.
• با فوروارد کردن پیام یک نفر، مستقیم براش تسک بسازید.
• زمانی که روی هر تسک صرف کردید رو ثبت و بررسی کنید (لاگ‌تایم).

راهنمای سریع:
👉 دستور /menu رو بفرستید تا منوی اصلی و همه‌ی دکمه‌ها رو ببینید.
👉 برای ساختن تسک جدید، دستور /assign رو بفرستید.

اگه هر جای کار گیر کردید، همین پیام رو دوباره بخونید یا با پشتیبانی تماس بگیرید. موفق باشید! ✅`

const genericErrorText = "⚠️ مشکلی پیش اومد. لطفا چند لحظه دیگه دوباره امتحان کنید؛ اگه باز هم تکرار شد، با پشتیبانی تماس بگیرید."

func (b *Bot) sendErrorMessage(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, genericErrorText)
	if _, err := b.bot.Send(msg); err != nil {
		log.Printf("Error sending message to user %v: %v\n", chatID, err)
	}
}

func (b *Bot) sendErrorCustomMessage(chatID int64, message error) {
	msg := tgbotapi.NewMessage(chatID, message.Error())
	if _, err := b.bot.Send(msg); err != nil {
		log.Printf("Error sending message to user %v: %v\n", chatID, err)
	}
}

func (b *Bot) sendUnauthorizedMessage(chatID int64, user models.User) {
	msg := tgbotapi.NewMessage(chatID, "🔌 اتصال شما به جیرا قطع شده و دیگه نمی‌تونم اعلان‌هاتون رو دریافت کنم.\n\nبرای وصل کردن دوباره، دستور /connect رو بفرستید یا روی دکمه‌ی زیر بزنید.")

	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton("اتصال مجدد")),
	)
	keyboard.OneTimeKeyboard = true
	keyboard.ResizeKeyboard = true
	msg.ReplyMarkup = keyboard

	if _, err := b.bot.Send(msg); err != nil {
		log.Printf("error sending the message: %v\n", err)
	}
}

func (b *Bot) sendExpiredMessage(chatID int64, messageID int) {
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "⏳ این عملیات منقضی شده. لطفا با دستور /menu دوباره از اول شروع کنید.")
	editMsg.ReplyMarkup = nil
	if _, err := b.bot.Send(editMsg); err != nil {
		log.Printf("Error sending message to user %v: %v\n", chatID, err)
	}
}

// deleteMessage removes a message. Most callers treat this as fire-and-forget,
// so the failure is logged here too - otherwise it would go unnoticed entirely.
func (b *Bot) deleteMessage(chatID int64, messageID int) error {
	_, err := b.bot.Request(tgbotapi.NewDeleteMessage(chatID, messageID))
	if err != nil {
		log.Printf("deleteMessage: failed to delete message %d in chat %d: %v\n", messageID, chatID, err)
	}
	return err
}
