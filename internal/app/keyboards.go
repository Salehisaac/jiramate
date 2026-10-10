package app

import (
	"fmt"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	jira "github.com/andygrunwald/go-jira"

	"rira/internal/models"
)

const backLabel = "بازگشت ⬅️"

func cancelRow() []tgbotapi.InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(backLabel, "cancelRequest"),
	)
}

// projectListKeyboard renders one button per project plus a cancel row.
// Each button carries the callback "<prefix>_<projectKey>".
func projectListKeyboard(projects []models.Project, prefix string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, p := range projects {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(p.Name, fmt.Sprintf("%s_%v", prefix, p.Key)),
		))
	}
	rows = append(rows, cancelRow())
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// stringListKeyboard renders one button per item plus a cancel row.
func stringListKeyboard(items []string, prefix string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, item := range items {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(item, fmt.Sprintf("%s_%v", prefix, item)),
		))
	}
	rows = append(rows, cancelRow())
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// reminderTimeKeyboard offers the preset reminder intervals for an issue.
func reminderTimeKeyboard(issueID any) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("1 ساعت", fmt.Sprintf("reminderHoure_%v_1h", issueID))),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("4 ساعت", fmt.Sprintf("reminderHoure_%v_4h", issueID))),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("1 روز", fmt.Sprintf("reminderHoure_%v_1d", issueID))),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("1 هفته", fmt.Sprintf("reminderHoure_%v_1w", issueID))),
		cancelRow(),
	)
}

// priorityKeyboard offers the Jira priority levels.
func priorityKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Lowest", "priority_Lowest"),
			tgbotapi.NewInlineKeyboardButtonData("Low", "priority_Low"),
			tgbotapi.NewInlineKeyboardButtonData("Medium", "priority_Medium"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("High", "priority_High"),
			tgbotapi.NewInlineKeyboardButtonData("Highest", "priority_Highest"),
		),
	)
}

// notificationKeyboard builds the interactive controls attached to a notification card.
func notificationKeyboard(card models.Card) tgbotapi.InlineKeyboardMarkup {
	toggle := func(on bool, onLabel, onData, offLabel, offData string) tgbotapi.InlineKeyboardButton {
		if on {
			return tgbotapi.NewInlineKeyboardButtonData(onLabel, onData)
		}
		return tgbotapi.NewInlineKeyboardButtonData(offLabel, offData)
	}

	// Vote/watch/star/read/delete all hit Jira's /rest/inbox/1.0/message/... inbox
	// endpoints, which are keyed by the notification card's own ID (card.ID), not
	// the underlying issue's ID. Only the per-issue reminder endpoint uses
	// card.Issue.ID. Vote/watch used to send card.Issue.ID, so Jira had no
	// matching "message" to act on and those two buttons silently did nothing.
	voteButton := toggle(card.Voted,
		"❌ لغو رأی", fmt.Sprintf("unvote_%v", card.ID),
		"✅ ثبت رأی", fmt.Sprintf("vote_%v", card.ID))
	watchButton := toggle(card.Watching,
		"🔕 لغو دنبال‌کردن", fmt.Sprintf("unwatch_%v", card.ID),
		"🔔 دنبال کردن", fmt.Sprintf("watch_%v", card.ID))
	starButton := toggle(card.Starred,
		"🌟 حذف ستاره", fmt.Sprintf("unstar_%v", card.ID),
		"⭐ ستاره‌دار کردن", fmt.Sprintf("star_%v", card.ID))
	openButton := toggle(card.Opened,
		"👀 علامت‌گذاری به‌عنوان خوانده‌شده", fmt.Sprintf("read_%v", card.ID),
		"📑 علامت‌گذاری به‌عنوان خوانده‌نشده", fmt.Sprintf("unread_%v", card.ID))

	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(voteButton, watchButton),
		tgbotapi.NewInlineKeyboardRow(starButton, openButton),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑️ حذف", fmt.Sprintf("delete_%v", card.ID)),
			tgbotapi.NewInlineKeyboardButtonData("🗨️ ثبت کامنت", fmt.Sprintf("comment_%v", card.Issue.Key)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏰ تنظیم یادآوری", fmt.Sprintf("reminder_%v", card.Issue.ID)),
		),
	)
}

// roleUserKeyboard renders one button per project member: DisplayName -> "<prefix>_<name>".
func roleUserKeyboard(users map[string]*models.RoleUser, prefix string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, u := range users {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(u.DisplayName, fmt.Sprintf("%s_%v", prefix, u.Name)),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// taskKeyboard renders one button per issue: Summary -> "<prefix>_<key>".
func taskKeyboard(tasks []jira.Issue, prefix string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, t := range tasks {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(t.Fields.Summary, fmt.Sprintf("%s_%v", prefix, t.Key)),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// reminderUnitLabel maps a duration suffix (m/h/d/w) to its Persian label.
func reminderUnitLabel(unit string) string {
	switch unit {
	case "m":
		return "دقیقه"
	case "h":
		return "ساعت"
	case "d":
		return "روز"
	case "w":
		return "هفته"
	default:
		return ""
	}
}
