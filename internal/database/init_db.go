package database

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
)

func InitDB(db *sql.DB) error {

	createNotificationsTable := `
	CREATE TABLE IF NOT EXISTS notifications (
		id BIGINT(20) NOT NULL AUTO_INCREMENT PRIMARY KEY,
		notif_id BIGINT(20),
		user_id BIGINT(20),
		actor_username VARCHAR(255),
		actor_displayName VARCHAR(255),
		textSummary TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
		text TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
		issue_key VARCHAR(50),
		issue_id BIGINT(20),
		issue_statusName VARCHAR(100),
		voted TINYINT(1),
		watching TINYINT(1),
		opened TINYINT(1),
		numComments BIGINT(20),
		numUpdates BIGINT(20),
		numWorkLogs BIGINT(20),
		reminder TINYINT(1),
		hideIssue TINYINT(1),
		starred TINYINT(1),
		inactive TINYINT(1),
		pendingApprovalId BIGINT(20),
		approvalState VARCHAR(50),
		approvalPending TINYINT(1),
		message_id BIGINT(20) NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	createTokensTable := `
	CREATE TABLE IF NOT EXISTS tokens (
		id BIGINT(20) NOT NULL AUTO_INCREMENT PRIMARY KEY,
		token TEXT NOT NULL,
		user_id BIGINT(20) NOT NULL,
		is_expired TINYINT(4) DEFAULT 0
	);`

	createUsersTable := `
	CREATE TABLE IF NOT EXISTS users (
		id BIGINT(20) NOT NULL AUTO_INCREMENT,
		name VARCHAR(120) DEFAULT NULL,
		username VARCHAR(120) DEFAULT NULL,
		chat_id BIGINT(20) DEFAULT NULL,
		account_id BIGINT(20) DEFAULT NULL,
		type ENUM('group', 'private', 'supergroup') NOT NULL DEFAULT 'private',
		message VARCHAR(120) DEFAULT NULL,
		status VARCHAR(120) DEFAULT NULL,
		stat TEXT DEFAULT NULL,
		is_admin ENUM('0', '1') NOT NULL DEFAULT '0',
		command VARCHAR(120) DEFAULT NULL,
		is_verified ENUM('unchecked', 'approved', 'baned') NOT NULL DEFAULT 'unchecked',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT NULL,
		last_check_time TEXT DEFAULT NULL,
		last_comment_id BIGINT(20) DEFAULT '0',
		PRIMARY KEY (id),
		UNIQUE KEY uq_users_chat_id (chat_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	filterdUsersTable := `
	CREATE TABLE IF NOT EXISTS filterd_users (
    id BIGINT(20) NOT NULL AUTO_INCREMENT,
    user_id BIGINT(20) NOT NULL,
    filterd_user_username VARCHAR(120) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    FOREIGN KEY (user_id) REFERENCES users(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	attachGroup := `
	CREATE TABLE IF NOT EXISTS attach_groups (
    id BIGINT(20) NOT NULL AUTO_INCREMENT,
    user_id BIGINT(20) NOT NULL,
    attach_group_id BIGINT(20) NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    FOREIGN KEY (user_id) REFERENCES users(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	attachGroupProjects := `
	CREATE TABLE IF NOT EXISTS attach_group_projects (
		id BIGINT(20) NOT NULL AUTO_INCREMENT,
		group_id BIGINT(20) NOT NULL,
		user_id BIGINT(20) NOT NULL DEFAULT 0,
		project_key VARCHAR(50) NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		FOREIGN KEY (group_id) REFERENCES attach_groups(attach_group_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	filterdProjectsTable := `
	CREATE TABLE IF NOT EXISTS filterd_projects (
    id BIGINT(20) NOT NULL AUTO_INCREMENT,
    user_id BIGINT(20) NOT NULL,
    filterd_project_key VARCHAR(120) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    FOREIGN KEY (user_id) REFERENCES users(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	filterdTaskTable := `
	CREATE TABLE IF NOT EXISTS filterd_tasks (
    id BIGINT(20) NOT NULL AUTO_INCREMENT,
    user_id BIGINT(20) NOT NULL,
    filterd_issue_key VARCHAR(120) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    FOREIGN KEY (user_id) REFERENCES users(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	filterdTypeTable := `
	CREATE TABLE IF NOT EXISTS filterd_types (
    id BIGINT(20) NOT NULL AUTO_INCREMENT,
    user_id BIGINT(20) NOT NULL,
    filterd_type_name VARCHAR(120) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    FOREIGN KEY (user_id) REFERENCES users(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	botSettingsTable := `
	CREATE TABLE IF NOT EXISTS bot_settings (
		id BIGINT(20) NOT NULL AUTO_INCREMENT,
		bot_token TEXT NOT NULL,
		is_active TINYINT(1) DEFAULT 1,
		inactive_message TEXT DEFAULT NULL,
		main_admin_id BIGINT(20) DEFAULT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	for _, query := range []string{createNotificationsTable, createTokensTable, createUsersTable, filterdUsersTable, filterdProjectsTable, filterdTaskTable, filterdTypeTable, attachGroup, attachGroupProjects, botSettingsTable} {
		_, err := db.Exec(query)
		if err != nil {
			return fmt.Errorf("failed to execute query: %v", err)
		}
	}

	return nil
}
