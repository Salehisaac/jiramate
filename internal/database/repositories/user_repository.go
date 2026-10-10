package repositories

import (
	"database/sql"
	"fmt"
	"log"
	"rira/internal/models"
	"strings"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
)

func InsertUserIntoDatabase(chat *tgbotapi.Chat, db *sql.DB) error {
	query := `
		INSERT INTO users (chat_id, username, name,status, stat)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, chat.ID, chat.UserName, fullName(chat.FirstName, chat.LastName), "unauthorized", "waitting for token")
	return err
}

func fullName(firstName, lastName string) string {
	if firstName == "" {
		return lastName
	}
	if lastName == "" {
		return firstName
	}
	return firstName + " " + lastName
}

// UpsertGroupChatState creates the users row for a group/supergroup chat if it
// doesn't exist yet, or updates its stat (and refreshes its name/type) if it
// does. Group chats only ever gain a users row through this path or through
// InsertChatIntoDatabase - unlike private chats, nothing auto-creates one on
// first contact - so callers that need to track per-group conversation state
// (e.g. "waiting for chat token") must use this instead of UpdateUser, which
// silently affects zero rows for a chat_id that isn't in the table yet.
func UpsertGroupChatState(db *sql.DB, chat *tgbotapi.Chat, stat string) (models.User, error) {
	query := `
		INSERT INTO users (chat_id, username, name, status, stat, type)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		username = VALUES(username),
		name = VALUES(name),
		type = VALUES(type),
		stat = VALUES(stat)
	`
	if _, err := db.Exec(query, chat.ID, chat.UserName, fullName(chat.FirstName, chat.LastName), "authorized", stat, chat.Type); err != nil {
		return models.User{}, fmt.Errorf("failed to upsert group chat state: %w", err)
	}
	return GetUserByChatID(db, chat.ID)
}

func InsertChatIntoDatabase(Message *tgbotapi.Message, db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	userQuery := `
		INSERT INTO users (chat_id, username, name, status, type)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		username = VALUES(username),
		name = VALUES(name),
		type = VALUES(type)
	`

	log.Println("inserting chat, type:", Message.Chat.Type)
	_, err = tx.Exec(userQuery, Message.Chat.ID, Message.Chat.UserName, fullName(Message.Chat.FirstName, Message.Chat.LastName), "authorized", Message.Chat.Type)
	if err != nil {
		return fmt.Errorf("failed to insert chat: %v", err)
	}

	var userID int
	err = tx.QueryRow("SELECT id FROM users WHERE chat_id = ?", Message.From.ID).Scan(&userID)
	if err != nil {
		return fmt.Errorf("کابر شما هنوز در بات احراز نشده است ، لطفا وارد بات شده و ربات را استارت کنید.")
	}

	attachQuery := `
		INSERT INTO attach_groups (user_id, attach_group_id)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE
		updated_at = CURRENT_TIMESTAMP
	`
	_, err = tx.Exec(attachQuery, userID, Message.Chat.ID)
	if err != nil {
		return fmt.Errorf("failed to insert attach group: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func AssignProjectToGroup(db *sql.DB, groupChatID, user_id int64, projectKey string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var exists bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM attach_group_projects WHERE group_id = ? AND project_key = ?)",
		groupChatID, projectKey).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check for existing record: %w", err)
	}

	if exists {
		return fmt.Errorf("group is already exists")
	}

	query := `
		INSERT INTO attach_group_projects (group_id, project_key, user_id)
		VALUES (?, ?, ?)
	`
	_, err = tx.Exec(query, groupChatID, projectKey, user_id)
	if err != nil {
		return fmt.Errorf("failed to insert attach group project: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func GetAttachGroups(db *sql.DB, userId int64) ([]models.AttachGroupProjects, error) {
	var attachProjects []models.AttachGroupProjects
	query := `
		SELECT group_id, project_key, user_id
		FROM attach_group_projects
		WHERE user_id = ?
	`
	rows, err := db.Query(query, userId)
	if err != nil {
		return nil, fmt.Errorf("failed to query attach groups: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var attachProject models.AttachGroupProjects

		if err := rows.Scan(&attachProject.GroupID, &attachProject.ProjectKey, &attachProject.UserID); err != nil {
			return nil, fmt.Errorf("failed to scan group ID and project key: %w", err)
		}
		attachProjects = append(attachProjects, attachProject)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return attachProjects, nil
}

func GetUserByChatID(db *sql.DB, chatID int64) (models.User, error) {
	var user models.User
	query := "SELECT id, username,name , chat_id, status, stat, message, type FROM users WHERE chat_id = ?"

	err := db.QueryRow(query, chatID).Scan(&user.ID, &user.Username, &user.Name, &user.ChatID, &user.Status, &user.Stat, &user.Message, &user.Type)

	if err != nil {
		return user, err
	}
	return user, nil
}

func GetUserByID(db *sql.DB, userID int64) (models.User, error) {
	var user models.User
	query := "SELECT id, username,name , chat_id, status, stat, message, type FROM users WHERE id = ?"

	err := db.QueryRow(query, userID).Scan(&user.ID, &user.Username, &user.Name, &user.ChatID, &user.Status, &user.Stat, &user.Message, &user.Type)

	if err != nil {
		return user, err
	}

	return user, nil
}

func GetAllUsers(db *sql.DB) ([]models.User, error) {
	var users []models.User
	query := "SELECT id, username, chat_id, name, status, last_comment_id , stat, message, type FROM users"
	rows, err := db.Query(query)
	if err != nil {
		return users, err
	}
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Username, &user.ChatID, &user.Name, &user.Status, &user.LastCommentId, &user.Stat, &user.Message, &user.Type); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}
func GetUserToken(db *sql.DB, userID int64) (models.Token, error) {
	var token models.Token
	query := "SELECT id ,token FROM tokens WHERE user_id= ? AND is_expired = 0"

	err := db.QueryRow(query, userID).Scan(&token.ID, &token.Token)
	if err != nil {
		return token, err
	}

	return token, nil
}

func GetUserTokenByChatID(db *sql.DB, chatID int64) (models.Token, error) {
	var token models.Token
	query := `
		SELECT t.id, t.token 
		FROM tokens t
		INNER JOIN users u ON u.id = t.user_id
		WHERE u.chat_id = ? AND t.is_expired = 0
	`

	err := db.QueryRow(query, chatID).Scan(&token.ID, &token.Token)
	if err != nil {
		return token, err
	}

	return token, nil
}

func UpdateUser(db *sql.DB, user models.User, values []interface{}, columns ...string) error {
	var setValues []string
	for _, column := range columns {
		setValues = append(setValues, fmt.Sprintf("%s = ?", column))
	}
	setClause := strings.Join(setValues, ", ")

	query := fmt.Sprintf("UPDATE users SET %s WHERE id = ?", setClause)
	values = append(values, user.ID)

	_, err := db.Exec(query, values...)
	if err != nil {
		log.Printf("Error executing statement in transaction: %v", err)
		return err
	}

	log.Println("User Updated successfully")
	return nil
}

func InsertFilteredUser(db *sql.DB, userId int64, filterdUsername string) error {
	query := `
	INSERT INTO filterd_users (user_id, filterd_user_username)
	VALUES (?,?)
`
	_, err := db.Exec(query, userId, filterdUsername)
	return err
}

func DeleteFilteredUser(db *sql.DB, userId int64, filterdUsername string) error {
	query := `
	DELETE FROM filterd_users
	WHERE user_id = ? AND filterd_user_username = ?
	`
	_, err := db.Exec(query, userId, filterdUsername)
	return err
}

func GetFilteredUsers(db *sql.DB, userId int64) ([]string, error) {
	var users []string
	query := "SELECT filterd_user_username FROM filterd_users WHERE user_id = ?"
	rows, err := db.Query(query, userId)
	if err != nil {
		return users, err
	}
	for rows.Next() {
		var user string
		if err := rows.Scan(&user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func InsertFilteredProject(db *sql.DB, userId int64, projectKey string) error {
	query := `
	INSERT INTO filterd_projects (user_id, filterd_project_key)
	VALUES (?, ?)
`
	_, err := db.Exec(query, userId, projectKey)
	return err
}

func DeleteFilteredProject(db *sql.DB, userId int64, projectKey string) error {
	query := `
	DELETE FROM filterd_projects
	WHERE user_id = ? AND filterd_project_key = ?
	`
	_, err := db.Exec(query, userId, projectKey)
	return err
}

func GetFilteredProjects(db *sql.DB, userId int64) ([]string, error) {
	var projects []string
	query := "SELECT filterd_project_key FROM filterd_projects WHERE user_id = ?"
	rows, err := db.Query(query, userId)
	if err != nil {
		return projects, err
	}
	for rows.Next() {
		var project string
		if err := rows.Scan(&project); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return projects, nil
}

func InsertFilteredTask(db *sql.DB, userId int64, taskKey string) error {
	query := `
	INSERT INTO filterd_tasks (user_id, filterd_task_key)
	VALUES (?, ?)
`
	_, err := db.Exec(query, userId, taskKey)
	return err
}
