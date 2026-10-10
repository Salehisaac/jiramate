package repositories

import (
	"database/sql"
	"fmt"
	"log"
	"rira/internal/models"
	"strings"
)

func InsertToken(db *sql.DB, userID int64, token string) error {
	query := "INSERT INTO tokens (token, user_id) VALUES (?, ?)"
	var err error
	_, err = db.Exec(query, token, userID)
	return err
}

func UpdateToken(db *sql.DB, token models.Token, values []interface{}, columns ...string) error {
	var setValues []string
	for _, column := range columns {
		setValues = append(setValues, fmt.Sprintf("%s = ?", column))
	}
	setClause := strings.Join(setValues, ", ")

	query := fmt.Sprintf("UPDATE tokens SET %s WHERE id = ?", setClause)
	values = append(values, token.ID)

	_, err := db.Exec(query, values...)
	if err != nil {
		log.Printf("Error executing statement in transaction: %v", err)
		return err
	}

	log.Println("Token Updated successfully")
	return nil
}
