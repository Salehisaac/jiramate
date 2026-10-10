package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	jira "github.com/andygrunwald/go-jira"
	persian "github.com/yaa110/go-persian-calendar"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
)

// Task is a lightweight issue summary used for log-time pagination.
type Task struct {
	Summary string
	Key     string
}

const taskPageSize = 9

func logtimeKey(user models.User) string { return fmt.Sprintf("user_%v_logtime", user.ID) }

func HandleWorklogTime(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
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

	issues, _, err := client.Issue.Search(`worklogAuthor = currentUser() AND worklogDate >= startOfMonth()`, &jira.SearchOptions{})
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Failed to search issues: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	year, month, _ := persian.New(time.Now()).Date()
	totalLoggedTime := 0
	taskTimes := make(map[string]int)
	for _, issue := range issues {
		worklogs, _, err := client.Issue.GetWorklogs(issue.ID)
		if err != nil {
			log.Printf("Failed to get worklogs for issue %s: %v\n", issue.Key, err)
			continue
		}
		for _, worklog := range worklogs.Worklogs {
			if worklog.Started == nil {
				continue
			}
			y, m, _ := persian.New(time.Time(*worklog.Started)).Date()
			if y == year && m == month {
				taskTimes[issue.Key] += worklog.TimeSpentSeconds
				totalLoggedTime += worklog.TimeSpentSeconds
			}
		}
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for issueKey, seconds := range taskTimes {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("🕒 %s | %d ساعت", issueKey, seconds/3600), fmt.Sprintf("speceficWorkLog_%v", issueKey)),
		))
	}
	rows = append(rows, cancelRow())

	// float64(totalLoggedTime/3600) truncated to whole hours *before* converting
	// to float, so e.g. 90 logged minutes rendered as "1" instead of "1.5".
	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID,
		fmt.Sprintf("⏱ <b>مجموع ساعت‌های ثبت‌شده شما در ماه %v:</b> <i>%.1f ساعت</i>\nبرای دیدن جزئیات هر تسک، روی یکی از دکمه‌های زیر بزنید.", month, float64(totalLoggedTime)/3600),
		tgbotapi.NewInlineKeyboardMarkup(rows...))
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		log.Println("error sending message", err)
		b.sendErrorMessage(chatID)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleWorklogSpeceficTime(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
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
		log.Println("Error creating Jira client", err)
		return http.StatusBadRequest, err, false
	}

	worklogs, _, err := client.Issue.GetWorklogs(id)
	if err != nil {
		log.Printf("Failed to get worklogs for issue %s: %v\n", id, err)
		return http.StatusBadRequest, err, true
	}

	year, month, _ := persian.New(time.Now()).Date()
	totalLoggedTime := 0
	logtimeText := "\n\n"
	for _, worklog := range worklogs.Worklogs {
		if worklog.Started == nil {
			continue
		}
		y, m, _ := persian.New(time.Time(*worklog.Started)).Date()
		if y == year && m == month {
			logtimeText += fmt.Sprintf("- %v: %d ساعت و %d دقیقه\n\n", worklog.Comment, worklog.TimeSpentSeconds/3600, (worklog.TimeSpentSeconds%3600)/60)
			totalLoggedTime += worklog.TimeSpentSeconds
		}
	}

	issue, _, err := client.Issue.Get(id, nil)
	if err != nil {
		log.Printf("error getting issue %s: %v\n", id, err)
		return http.StatusBadRequest, err, true
	}

	// float64(totalLoggedTime/3600) truncated to whole hours *before* converting
	// to float, silently dropping any logged minutes from the total shown here.
	text := fmt.Sprintf(`⏰ <b>مجموع ساعت‌های ثبت‌شده برای تسک:</b> <a href="%s">%v - %v</a> — %.1f ساعت`,
		issueLink(id), id, issue.Fields.Summary, float64(totalLoggedTime)/3600) + logtimeText

	returnBtn := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(backLabel, "returnToWorklogTime")),
	)
	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, returnBtn)
	editMsg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(editMsg); err != nil {
		log.Println("error sending message", err)
		b.sendErrorMessage(chatID)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleSelectProjectForLogTime(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID

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

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "📁 برای ثبت زمان، یکی از پروژه‌های زیر رو انتخاب کنید:", projectListKeyboard(projects, "selectTaskForLogTime"))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending inline keyboard message:", err)
		return http.StatusBadRequest, err, true
	}
	return http.StatusOK, nil, true
}

func HandleSelectTaskForLogTime(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
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
	if err := b.rdb.Del(context.Background(), logtimeKey(user)).Err(); err != nil {
		log.Printf("Error deleting list from Redis: %v\n", err)
		return http.StatusBadRequest, err, true
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	jiraTasks, err := helpers.FetchProjectTasks(client, id)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error fetching project tasks: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	tasks := make([]Task, 0, len(jiraTasks))
	taskMap := make(map[string]string, len(jiraTasks))
	for _, t := range jiraTasks {
		tasks = append(tasks, Task{Summary: t.Fields.Summary, Key: t.Key})
		taskMap[t.Fields.Summary] = t.Key
	}
	if err := b.rdb.HMSet(context.Background(), fmt.Sprintf("%v_tasks_%v", id, user.ID), taskMap).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error storing tasks", err)
		return http.StatusBadRequest, err, false
	}

	end := taskPageSize
	if len(tasks) < end {
		end = len(tasks)
	}
	rows := logtimeTaskButtons(tasks, 0, end)
	if len(tasks) > taskPageSize {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(">>", fmt.Sprintf("taskPaginationNext_%v_%v", id, taskPageSize)),
		))
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "🗂 تسکی که می‌خواید براش زمان ثبت کنید رو انتخاب کنید:", tgbotapi.NewInlineKeyboardMarkup(rows...))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleSelectTaskPaginationPre(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)
	offset, _ := strconv.Atoi(callbackArg(cq, 2))

	if err := b.answerCallback(cq, pleaseWaitText); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("error sending toast alert: %v\n", err)
		return http.StatusBadRequest, err, false
	}

	tasks, err := b.cachedTasks(id, user)
	if err != nil {
		return http.StatusBadRequest, err, false
	}

	end := offset + taskPageSize
	if end > len(tasks) {
		end = len(tasks)
	}
	rows := logtimeTaskButtons(tasks, offset, end)

	if offset == 0 && len(tasks) > taskPageSize {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(">>", fmt.Sprintf("taskPaginationNext_%v_%v", id, taskPageSize)),
		))
	} else {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("<<", fmt.Sprintf("taskPaginationPre_%v_%v", id, offset-taskPageSize)),
			tgbotapi.NewInlineKeyboardButtonData(">>", fmt.Sprintf("taskPaginationNext_%v_%v", id, offset+taskPageSize)),
		))
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "🗂 تسکی که می‌خواید براش زمان ثبت کنید رو انتخاب کنید:", tgbotapi.NewInlineKeyboardMarkup(rows...))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleSelectTaskPaginationNext(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)
	offset, _ := strconv.Atoi(callbackArg(cq, 2))

	tasks, err := b.cachedTasks(id, user)
	if err != nil {
		return http.StatusBadRequest, err, false
	}

	end := offset + taskPageSize
	if end >= len(tasks) {
		end = len(tasks)
	}
	rows := logtimeTaskButtons(tasks, offset, end)

	if offset+taskPageSize >= len(tasks) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("<<", fmt.Sprintf("taskPaginationPre_%v_%v", id, offset-taskPageSize)),
		))
	} else {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("<<", fmt.Sprintf("taskPaginationPre_%v_%v", id, offset-taskPageSize)),
			tgbotapi.NewInlineKeyboardButtonData(">>", fmt.Sprintf("taskPaginationNext_%v_%v", id, offset+taskPageSize)),
		))
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "🗂 تسکی که می‌خواید براش زمان ثبت کنید رو انتخاب کنید:", tgbotapi.NewInlineKeyboardMarkup(rows...))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleDeleteWorklog(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	issueID := callbackArg(cq, 1)
	worklogID := callbackArg(cq, 2)

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
	if err := b.rdb.Del(context.Background(), logtimeKey(user)).Err(); err != nil {
		log.Printf("Error deleting list from Redis: %v\n", err)
		return http.StatusBadRequest, err, true
	}

	client, err := newJiraClient(token.Token)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error creating Jira client: %v\n", err)
		return http.StatusBadRequest, err, true
	}
	req, err := client.NewRequest(http.MethodDelete, fmt.Sprintf("/rest/api/2/issue/%s/worklog/%s", issueID, worklogID), nil)
	if err != nil {
		log.Printf("Error building delete-worklog request: %v\n", err)
		return http.StatusBadRequest, err, true
	}
	resp, err := client.Do(req, nil)
	if err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error deleting worklog %s on issue %s: %v\n", worklogID, issueID, err)
		return http.StatusBadRequest, err, true
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b.sendErrorMessage(chatID)
		err := fmt.Errorf("unexpected status %d deleting worklog %s on issue %s", resp.StatusCode, worklogID, issueID)
		log.Println(err)
		return http.StatusBadRequest, err, true
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "✅ لاگ‌تایم با موفقیت حذف شد.")
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

func HandleGetTaskLogTimeDay(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	if err := b.rdb.LPush(context.Background(), logtimeKey(user), fmt.Sprintf("taskId_%v", id)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing to Redis: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "📅 روزی که می‌خواید براش زمان ثبت کنید رو انتخاب کنید:", helpers.CreateCalendarKeyboard(time.Now()))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForLogtimeTimeSpent}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusUnauthorized, err, false
	}
	return http.StatusOK, nil, true
}

func HandleGetTaskLogTime(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	id := callbackArg(cq, 1)

	if err := b.rdb.LPush(context.Background(), logtimeKey(user), fmt.Sprintf("logTimeDay_%v", id)).Err(); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("Error pushing to Redis: %v\n", err)
		return http.StatusBadRequest, err, false
	}
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, "⏱ مدت زمانی که روی این تسک کار کردید رو بفرستید. برای ساعت از h و برای دقیقه از m استفاده کنید، مثلاً 1h 30m یعنی ۱ ساعت و ۳۰ دقیقه (می‌تونید w برای هفته و d برای روز رو هم اضافه کنید).")
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	if err := repositories.UpdateUser(b.db, user, []interface{}{WaittingForLogtimeTimeSpent}, "stat"); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error updating user", err)
		return http.StatusUnauthorized, err, false
	}
	return http.StatusOK, nil, true
}

func HandleCalenderPagintation(b *Bot, cq *tgbotapi.CallbackQuery, user models.User) (int, error, bool) {
	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID

	var year, month int
	fmt.Sscanf(callbackArg(cq, 1), "%d-%02d", &year, &month)
	switch month {
	case 0:
		year, month = year-1, 12
	case 13:
		year, month = year+1, 1
	}

	editMsg := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, "📅 روزی که می‌خواید براش زمان ثبت کنید رو انتخاب کنید:",
		helpers.CreateCalendarKeyboard(time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)))
	if _, err := b.bot.Send(editMsg); err != nil {
		b.sendErrorMessage(chatID)
		log.Println("error sending message", err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, true
}

// cachedTasks loads the task list previously stored for a project's log-time flow.
func (b *Bot) cachedTasks(projectID string, user models.User) ([]Task, error) {
	result, err := b.rdb.HGetAll(context.Background(), fmt.Sprintf("%v_tasks_%v", projectID, user.ID)).Result()
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(result))
	for summary, key := range result {
		tasks = append(tasks, Task{Summary: summary, Key: key})
	}
	return tasks, nil
}

// logtimeTaskButtons builds one button per task in [start,end): Summary -> "getLogtimeDay_<key>".
func logtimeTaskButtons(tasks []Task, start, end int) [][]tgbotapi.InlineKeyboardButton {
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := start; i < end; i++ {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(tasks[i].Summary, fmt.Sprintf("getLogtimeDay_%v", tasks[i].Key)),
		))
	}
	return rows
}
