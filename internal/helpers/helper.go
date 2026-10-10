package helpers

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"

	"rira/internal/database/repositories"
	"rira/internal/models"
)

var timeInputRe = regexp.MustCompile(`^\d+[wdhm]$`)

func ValidateTimeInput(input string) bool {
	return timeInputRe.MatchString(input)
}

func ResetState(db *sql.DB, user models.User) error {
	return repositories.UpdateUser(db, user, []interface{}{""}, "stat")
}

func Contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func IsValidJiraUsername(jiraURL, username, token string) (bool, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/rest/api/2/user?username=%s", jiraURL, url.QueryEscape(username)), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return false, nil
	case http.StatusOK:
		return true, nil
	default:
		return false, fmt.Errorf("unexpected response status: %d", resp.StatusCode)
	}
}

func IsGroupMessage(update *tgbotapi.Update) bool {
	if update.Message == nil {
		return false
	}
	chatType := update.Message.Chat.Type
	return chatType == "group" || chatType == "supergroup"
}

// CreateCalendarKeyboard renders a month grid whose day buttons emit "getLogtimeTime_<date>".
func CreateCalendarKeyboard(currentTime time.Time) tgbotapi.InlineKeyboardMarkup {
	year, month, _ := currentTime.Date()

	weekRow := make([]tgbotapi.InlineKeyboardButton, 0, 7)
	for _, day := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		weekRow = append(weekRow, tgbotapi.NewInlineKeyboardButtonData(day, "noop"))
	}

	today := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("امروز", fmt.Sprintf("getLogtimeTime_%v", time.Now().Format("2006-01-02"))),
	)

	calendar := [][]tgbotapi.InlineKeyboardButton{today, weekRow}

	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	startOffset := (int(firstDay.Weekday()) + 6) % 7
	daysInMonth := firstDay.AddDate(0, 1, -1).Day()

	row := make([]tgbotapi.InlineKeyboardButton, 0, 7)
	for i := 0; i < startOffset; i++ {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("-", "noop"))
	}
	for i := 1; i <= daysInMonth; i++ {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%02d", i), fmt.Sprintf("getLogtimeTime_%d-%02d-%02d", year, month, i)))
		if len(row) == 7 {
			calendar = append(calendar, row)
			row = make([]tgbotapi.InlineKeyboardButton, 0, 7)
		}
	}
	if len(row) > 0 {
		calendar = append(calendar, row)
	}

	calendar = append(calendar, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("<<", fmt.Sprintf("logtimePage_%d-%02d", year, int(month)-1)),
		tgbotapi.NewInlineKeyboardButtonData(month.String(), "noop"),
		tgbotapi.NewInlineKeyboardButtonData(">>", fmt.Sprintf("logtimePage_%d-%02d", year, int(month)+1)),
	))

	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: calendar}
}
