package helpers

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"rira/internal/database/repositories"
	"rira/internal/models"
)

// httpClient bounds every request in this file with a timeout so a stuck Jira
// call can't leak the calling goroutine forever.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// MarkTokenExpired flags the user's token as expired and, for private chats,
// flips their status to "unauthorized" so they're prompted to reconnect.
//
// token.ID may legitimately be zero when the caller never had a token row to
// begin with (e.g. GetUserToken returned sql.ErrNoRows and the zero-value
// models.Token was passed through); in that case there is nothing to update
// in the tokens table, and attempting it would silently match no row.
func MarkTokenExpired(db *sql.DB, user models.User, token models.Token) {
	if user.Type != "private" {
		return
	}

	values := []interface{}{"unauthorized"}
	columns := []string{"status"}
	if err := repositories.UpdateUser(db, user, values, columns...); err != nil {
		log.Printf("MarkTokenExpired: failed to mark user %d unauthorized: %v", user.ID, err)
	}

	if token.ID == 0 {
		return
	}
	tokenValues := []interface{}{"1"}
	tokenColumns := []string{"is_expired"}
	if err := repositories.UpdateToken(db, token, tokenValues, tokenColumns...); err != nil {
		log.Printf("MarkTokenExpired: failed to expire token %d for user %d: %v", token.ID, user.ID, err)
	}
}

// TokenRetry probes the inbox endpoint up to max times with the given token and
// reports whether it ever came back healthy. It always closes the response
// bodies itself: every caller only cares about the boolean, so returning the
// *http.Response used to leave its body unclosed at every call site.
func TokenRetry(token models.Token, max int) bool {
	for i := 0; i < max; i++ {
		req, err := http.NewRequest("GET", "https://pm.rso-co.ir/rest/inbox/1.0/message/list/0", nil)
		if err != nil {
			log.Println("error parsing the request : ", err)
			return false
		}
		req.Header.Set("Authorization", "Bearer "+token.Token)
		resp, err := httpClient.Do(req)
		if err != nil {
			log.Println("error sending the request : ", err)
			return false
		}
		resp.Body.Close()

		if resp.StatusCode == 200 {
			return true
		}
	}
	return false
}

func TestToken(token models.Token) bool {
	req, err := http.NewRequest("GET", "https://pm.rso-co.ir/rest/api/2/myself", nil)
	if err != nil {
		log.Println("error parsing the request : ", err)
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token.Token)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println("error sending the request : ", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}
