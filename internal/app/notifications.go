package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	jira "github.com/andygrunwald/go-jira"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
	"rira/static/keyboards"
)

const inboxListURL = jiraURL + "/rest/inbox/1.0/message/list/0"

var headerTitleRe = regexp.MustCompile(`title="([^"]+)"`)

// handleNotifications polls every authorized user for new inbox cards on a ticker.
func (b *Bot) handleNotifications(db *sql.DB) {
	const batchSize = 10
	var wg sync.WaitGroup

	processBatch := func(batch []models.User) {
		defer wg.Done()
		for _, user := range batch {
			if user.Status == nil || *user.Status == "unauthorized" {
				continue
			}
			notifications, err := b.getNotificationsForUser(db, user)
			if err != nil {
				log.Printf("handleNotifications: error fetching notifications for user %d: %v\n", user.ID, err)
				continue
			}
			b.processNotifications(user, notifications, db)
		}
	}

	ticker := time.NewTicker(notificationPollInterval)
	defer ticker.Stop()

	for range ticker.C {
		users, err := repositories.GetAllUsers(db)
		if err != nil {
			log.Printf("handleNotifications: error getting all users: %v\n", err)
			continue
		}
		for i := 0; i < len(users); i += batchSize {
			end := i + batchSize
			if end > len(users) {
				end = len(users)
			}
			wg.Add(1)
			go processBatch(users[i:end])
		}
		wg.Wait()
	}
}

// getNotificationsForUser fetches the inbox feed for a single user, refreshing the token on 401.
func (b *Bot) getNotificationsForUser(db *sql.DB, user models.User) (models.NotificationResponse, error) {
	var notifications models.NotificationResponse

	intChatId, err := strconv.Atoi(user.ChatID)
	if err != nil {
		return notifications, fmt.Errorf("invalid chat id %q for user %d: %w", user.ChatID, user.ID, err)
	}

	token, err := repositories.GetUserToken(db, user.ID)
	if err == sql.ErrNoRows {
		helpers.MarkTokenExpired(db, user, token)
		return notifications, fmt.Errorf("the user is unauthorized")
	} else if err != nil {
		return notifications, err
	}

	resp, err := authorizedRequest(http.MethodGet, inboxListURL, token.Token, nil)
	if err != nil {
		return notifications, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		log.Printf("getNotificationsForUser: token unauthorized for user %v, retrying\n", user.Username)
		if ok := helpers.TokenRetry(token, 5); !ok {
			b.sendUnauthorizedMessage(int64(intChatId), user)
			helpers.MarkTokenExpired(db, user, token)
			return notifications, fmt.Errorf("the user is unauthorized")
		}
		log.Printf("getNotificationsForUser: token recovered for user %v\n", user.Username)
		resp, err = authorizedRequest(http.MethodGet, inboxListURL, token.Token, nil)
		if err != nil {
			return notifications, err
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return notifications, fmt.Errorf("error reading response body: %v", err)
	}
	if err := json.Unmarshal(body, &notifications); err != nil {
		return notifications, fmt.Errorf("error unmarshalling response body: %v", err)
	}
	return notifications, nil
}

// processNotifications delivers attached-group "done" updates and new inbox cards to the user.
func (b *Bot) processNotifications(user models.User, notifications models.NotificationResponse, db *sql.DB) {
	intChatId, err := strconv.Atoi(user.ChatID)
	if err != nil {
		log.Printf("processNotifications: invalid chat id %q for user %d: %v\n", user.ChatID, user.ID, err)
		return
	}
	if reflect.DeepEqual(notifications, models.NotificationResponse{}) {
		return
	}

	allFilterdProjects, err := repositories.GetFilteredProjects(b.db, user.ID)
	if err != nil {
		log.Printf("processNotifications: error getting filtered projects for user %d: %v\n", user.ID, err)
		return
	}
	allFilterdUsers, err := repositories.GetFilteredUsers(b.db, user.ID)
	if err != nil {
		log.Printf("processNotifications: error getting filtered users for user %d: %v\n", user.ID, err)
		return
	}
	groups, err := repositories.GetAttachGroups(b.db, user.ID)
	if err != nil {
		log.Printf("processNotifications: error getting attach groups for user %d: %v\n", user.ID, err)
		return
	}

	// A broken token on one attached group must not stop this user's own
	// notification cards (below) from being delivered, so failures here are
	// only ever logged/notified, never allowed to abort processNotifications.
	b.notifyAttachedGroups(groups, int64(intChatId))

	if len(notifications.Cards) == 0 {
		return
	}

	for i := len(notifications.Cards) - 1; i >= 0; i-- {
		card := notifications.Cards[i]
		if int64(card.ID) <= user.LastCommentId {
			continue
		}

		projectKey := strings.Split(card.Issue.Key, "-")[0]
		if !helpers.Contains(allFilterdProjects, projectKey) && !helpers.Contains(allFilterdUsers, card.Actor.DisplayName) {
			b.sendNotificationCard(int64(intChatId), card, user, db)
		}

		if card.ID == notifications.Cards[0].ID {
			if err := repositories.UpdateUser(db, user, []interface{}{notifications.Cards[0].ID}, "last_comment_id"); err != nil {
				log.Printf("processNotifications: error updating last_comment_id for user %d: %v\n", user.ID, err)
				return
			}
		}
	}
}

// notifyAttachedGroups posts "done" task updates to each group attached to the user.
// A problem with one attached project (missing token, Jira error, ...) only skips
// that project; it must never abort the rest of the batch or the caller's own
// notification delivery, and any auth failure is attributed to the actual owner
// of that project's token (attachProject.UserID), not the user being polled.
func (b *Bot) notifyAttachedGroups(groups []models.AttachGroupProjects, chatID int64) {
	for _, attachProject := range groups {
		token, err := repositories.GetUserToken(b.db, attachProject.UserID)
		if err != nil {
			if err == sql.ErrNoRows {
				b.markAttachedProjectOwnerUnauthorized(attachProject)
			} else {
				log.Printf("notifyAttachedGroups: error fetching token for user %d: %v\n", attachProject.UserID, err)
			}
			continue
		}

		jiraClient, err := newJiraClient(token.Token)
		if err != nil {
			log.Printf("notifyAttachedGroups: error creating Jira client for user %d: %v\n", attachProject.UserID, err)
			continue
		}

		pollMinutes := int(notificationPollInterval / time.Minute)
		jql := fmt.Sprintf(`project = "%s" AND status changed TO Done AND status = Done AND updatedDate >= -%dm ORDER BY updated DESC`, attachProject.ProjectKey, pollMinutes)
		issues, _, err := jiraClient.Issue.Search(jql, &jira.SearchOptions{
			MaxResults: 50,
			Fields:     []string{"summary", "status", "assignee", "updated"},
		})
		if err != nil {
			log.Printf("notifyAttachedGroups: error searching issues for project [%s]: %v\n", attachProject.ProjectKey, err)
			continue
		}

		for _, issue := range issues {
			assigneeName, assigneeUsername := "Unassigned", "N/A"
			if issue.Fields.Assignee != nil {
				assigneeName = issue.Fields.Assignee.DisplayName
				assigneeUsername = issue.Fields.Assignee.Name
			}
			text := fmt.Sprintf("🎉 <b>تسک انجام شد!</b> 🎉\n\n<b>مسئول:</b> %s\n<b>نام کاربری:</b> %s\n<b>عنوان:</b> %s",
				assigneeName, assigneeUsername, issue.Fields.Summary)

			msg := tgbotapi.NewMessage(attachProject.GroupID, text)
			msg.ParseMode = tgbotapi.ModeHTML
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonURL("بررسی تسک", issueLink(issue.Key)),
			})
			if _, err := b.bot.Send(msg); err != nil {
				log.Printf("notifyAttachedGroups: error sending message to group %d: %v\n", attachProject.GroupID, err)
			}
		}
	}
}

// markAttachedProjectOwnerUnauthorized marks the actual owner of an attached
// project's Jira token as unauthorized. It must look the owner up itself:
// the caller polling for notifications is very often a different Telegram
// user than whoever originally linked the group to this project.
func (b *Bot) markAttachedProjectOwnerUnauthorized(attachProject models.AttachGroupProjects) {
	owner, err := repositories.GetUserByID(b.db, attachProject.UserID)
	if err != nil {
		log.Printf("notifyAttachedGroups: error loading token owner %d: %v\n", attachProject.UserID, err)
		return
	}
	if ownerChatID, err := strconv.ParseInt(owner.ChatID, 10, 64); err == nil {
		b.sendUnauthorizedMessage(ownerChatID, owner)
	}
	helpers.MarkTokenExpired(b.db, owner, models.Token{})
}

// sendNotificationCard renders a single inbox card and stores it. If delivery to
// Telegram fails there is no message to reference, so it must not be recorded:
// doing so used to leave behind notifications rows with message_id=0.
func (b *Bot) sendNotificationCard(chatID int64, card models.Card, user models.User, db *sql.DB) {
	explanation := ""
	if match := headerTitleRe.FindStringSubmatch(card.Header); len(match) > 1 {
		explanation = match[1]
	}

	text := fmt.Sprintf(`📢 <b>اعلان جدید!</b>

<b>کاربر:</b> %s
<b>نام کاربری:</b> %s
<b>عملیات:</b> %s
<b>عنوان:</b> %s
<b>وضعیت:</b> %s
<b>متن اعلان:</b> %s
`,
		card.Actor.DisplayName, card.Actor.UserName, notificationOperation(card.Type),
		explanation, card.Issue.StatusName, card.Header)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = notificationKeyboard(card)

	message, err := b.bot.Send(msg)
	if err != nil {
		log.Printf("sendNotificationCard: error sending message to user %v: %v\n", user.ChatID, err)
		return
	}
	if err := repositories.InsertNotification(card, user, message.MessageID, db); err != nil {
		log.Printf("sendNotificationCard: error inserting notification for user %v: %v\n", user.ChatID, err)
	}
}

// notificationOperation maps an inbox card type to a human-readable verb.
func notificationOperation(cardType string) string {
	switch cardType {
	case "REMINDER":
		return "یادآوری"
	case "COMMENTED":
		return "کامنت جدید"
	case "GENERIC":
		return "تغییر وضعیت"
	case "WORKLOG":
		return "ثبت لاگ زمان"
	case "CREATED":
		return "ایجاد تسک"
	case "UPDATED":
		return "به‌روزرسانی"
	default:
		return "نامشخص"
	}
}

// handleGettingToken validates a freshly supplied PAT and seeds the user's last seen card.
func (b *Bot) handleGettingToken(db *sql.DB, update tgbotapi.Update, user models.User) (string, error) {
	token := update.Message.Text
	chatID := update.Message.Chat.ID

	resp, err := authorizedRequest(http.MethodGet, inboxListURL, token, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		msg := tgbotapi.NewMessage(chatID, "توکن شما معتبر نیست، لطفا دوباره تلاش کنید:")
		msg.ReplyMarkup = keyboards.ReturnToMainButton
		if _, err := b.bot.Send(msg); err != nil {
			return "", err
		}
		return "", fmt.Errorf("the user is unauthorized")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var notifications models.NotificationResponse
	if err := json.Unmarshal(body, &notifications); err != nil {
		return "", err
	}

	if len(notifications.Cards) > 0 {
		if err := repositories.UpdateUser(db, user, []interface{}{notifications.Cards[0].ID}, "last_comment_id"); err != nil {
			log.Printf("handleGettingToken: error updating last_comment_id for user %d: %v\n", user.ID, err)
			return "", err
		}
	}
	return token, nil
}
