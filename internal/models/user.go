package models

import (
	"database/sql"
	"time"
)

type User struct {
	ID            int64
	Name          string
	Username      string
	Message       sql.NullString
	Status        *string
	Stat          *string
	ChatID        string
	Is_admin      int64
	Type          string
	Command       string
	Is_verified   string
	LastCommentId int64
	Created_at    *time.Time
}

type Token struct {
	ID    int64
	Token string
}
