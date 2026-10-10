package repositories

import (
	"database/sql"
	"fmt"
	"rira/internal/models"
	"strings"
)

func GetTheBotConfigs(db *sql.DB) (models.Bot, error) {
	var bot models.Bot

	query := "SELECT * FROM bot_settings"

	err := db.QueryRow(query).Scan(&bot.ID, &bot.Bot_token, &bot.Is_active, &bot.Inactive_message, &bot.Main_admin_id, &bot.CreatedAt)
	if err != nil {
		return bot, err
	}

	return bot, nil
}

func UpdateBotSettings(db *sql.DB, bot_settings models.Bot, values []interface{}, columns ...string) error {

	var setValues []string
	for _, column := range columns {
		setValues = append(setValues, fmt.Sprintf("%s = ?", column))
	}
	setClause := strings.Join(setValues, ", ")

	query := fmt.Sprintf("UPDATE bot_settings SET %s WHERE id = ?", setClause)

	values = append(values, bot_settings.ID)

	_, err := db.Exec(query, values...)
	if err != nil {
		return err
	}

	return nil
}

func InsertBotSettings(db *sql.DB, bot models.Bot) error {
	// Prepare the SQL statement
	query := `
        INSERT INTO bot_settings (bot_token, main_admin_id)
        VALUES (?, ?)
        ON DUPLICATE KEY UPDATE
            bot_token = VALUES(bot_token),
            main_admin_id = VALUES(main_admin_id)
    `

	// Execute the SQL statement
	_, err := db.Exec(query, bot.Bot_token, bot.Main_admin_id)
	if err != nil {
		return err
	}

	return nil
}
