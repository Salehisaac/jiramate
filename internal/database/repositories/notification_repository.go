package repositories

import (
	"database/sql"
	"fmt"
	"log"
	"rira/internal/models"
	"strings"
)

func InsertNotification(card models.Card, user models.User, messageId int, db *sql.DB) error {
	query := `
		INSERT INTO notifications (notif_id, user_id , actor_username , actor_displayName , textSummary , text , issue_key , issue_id, issue_statusName , voted , watching , numComments , numUpdates , numWorkLogs , reminder , hideIssue , starred , inactive , pendingApprovalId , approvalState , approvalPending, message_id, opened )
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ? ,? ,? ,? ,? ,? ,? ,? ,? ,? ,? ,? ,?, ?, ? )
	`
	_, err := db.Exec(query, card.ID, user.ID, card.Actor.UserName, card.Actor.DisplayName, card.TextSummary, card.Text, card.Issue.Key, card.Issue.ID, card.Issue.StatusName, card.Voted, card.Watching, card.Stats.NumComments, card.Stats.NumUpdates, card.Stats.NumWorkLogs, card.Reminder, card.HideIssue, card.Starred, card.Inactive, card.PendingApprovalId, card.ApprovalState, card.ApprovalPending, messageId, card.Opened)
	return err
}

// GetNotification looks up a stored notification card by the Telegram message
// that carries it. Telegram message IDs are only unique within a single chat,
// so two different users' chats routinely reuse the same message_id - the
// query must also be scoped by user_id, or it can resolve to a different
// user's notification (and, previously, sql.ErrNoRows was swallowed here,
// silently handing callers a zero-value Card instead of a "not found" error).
func GetNotification(db *sql.DB, userID int64, messageId int) (models.Card, error) {
	var card models.Card
	query := `
		SELECT notif_id, actor_username, actor_displayName, textSummary, text, issue_key, issue_id, issue_statusName, voted, watching, numComments, numUpdates, numWorkLogs, reminder, hideIssue, starred, inactive, pendingApprovalId, approvalState, approvalPending, opened
		FROM notifications WHERE user_id = ? AND message_id = ?
	`
	err := db.QueryRow(query, userID, messageId).Scan(
		&card.ID,
		&card.Actor.UserName,
		&card.Actor.DisplayName,
		&card.TextSummary,
		&card.Text,
		&card.Issue.Key,
		&card.Issue.ID,
		&card.Issue.StatusName,
		&card.Voted,
		&card.Watching,
		&card.Stats.NumComments,
		&card.Stats.NumUpdates,
		&card.Stats.NumWorkLogs,
		&card.Reminder,
		&card.HideIssue,
		&card.Starred,
		&card.Inactive,
		&card.PendingApprovalId,
		&card.ApprovalState,
		&card.ApprovalPending,
		&card.Opened,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return card, err
		}
		log.Printf("Error querying notification for user %d messageId %d: %v", userID, messageId, err)
		return card, err
	}
	return card, nil
}

// UpdateNotification updates the given columns of the notification that belongs
// to userID and was delivered as messageId. Scoping by user_id prevents one
// user's vote/watch/star/read toggle from silently mutating a different
// user's row when their Telegram message IDs happen to coincide.
func UpdateNotification(db *sql.DB, userID int64, messageId int, values []interface{}, columns ...string) error {
	var setValues []string
	for _, column := range columns {
		setValues = append(setValues, fmt.Sprintf("%s = ?", column))
	}
	setClause := strings.Join(setValues, ", ")

	query := fmt.Sprintf("UPDATE notifications SET %s WHERE user_id = ? AND message_id = ?", setClause)
	values = append(values, userID, messageId)

	_, err := db.Exec(query, values...)
	if err != nil {
		log.Printf("Error updating notification for user %d messageId %d: %v", userID, messageId, err)
		return err
	}
	return nil
}

// DeleteNotification removes the notification that belongs to userID and has
// the given Jira notif_id. notif_id is not unique per user - the same Jira
// notification event is frequently stored once per watcher - so scoping by
// user_id here prevents one user deleting their own inbox card from also
// deleting every other user's copy of the same notification.
func DeleteNotification(db *sql.DB, userID int64, notificationId string) error {
	query := "DELETE FROM notifications WHERE user_id = ? AND notif_id = ?"
	_, err := db.Exec(query, userID, notificationId)
	if err != nil {
		log.Printf("Error deleting notification %s for user %d: %v", notificationId, userID, err)
		return err
	}
	return nil
}
