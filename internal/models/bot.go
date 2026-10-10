package models

import (
	"database/sql"
	"time"
)

type Bot struct {
	ID               int64
	Is_active        bool
	Bot_token        string
	Inactive_message sql.NullString
	Main_admin_id    int64
	CreatedAt        time.Time
}
