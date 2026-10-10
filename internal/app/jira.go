package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	jira "github.com/andygrunwald/go-jira"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"

	"rira/internal/database/repositories"
	"rira/internal/helpers"
	"rira/internal/models"
)

// newJiraClient builds a Jira client authenticated with the given bearer token.
// The underlying HTTP client is given the same timeout as httpClient so a stuck
// Jira request can't hang a request-handling goroutine forever.
func newJiraClient(token string) (*jira.Client, error) {
	tp := jira.BearerAuthTransport{Token: token}
	client := tp.Client()
	client.Timeout = httpClient.Timeout
	return jira.NewClient(client, jiraURL)
}

// authorizedRequest performs an HTTP request with a bearer token. Callers close the body.
func authorizedRequest(method, url, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return httpClient.Do(req)
}

// authorizedPost issues an authorized POST and reports non-2xx responses as errors
// instead of silently discarding them.
func authorizedPost(url, token string) error {
	resp, err := authorizedRequest(http.MethodPost, url, token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, url, string(body))
	}
	return nil
}

// authorizedPostWithRetry issues an authorized POST, and if the token appears to have
// expired transiently, revalidates it once and retries the same request before giving up.
// This avoids the bug where a successful token-health check was mistaken for the original
// request having succeeded, even though it was never actually retried.
func (b *Bot) authorizedPostWithRetry(url string, token models.Token, user models.User) error {
	err := authorizedPost(url, token.Token)
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "unexpected status 401") {
		return err
	}
	if ok := helpers.TokenRetry(token, 5); !ok {
		helpers.MarkTokenExpired(b.db, user, token)
		return fmt.Errorf("token is no longer valid: %w", err)
	}
	return authorizedPost(url, token.Token)
}

// postJSONWithRetry POSTs payload as a properly-encoded JSON body (avoiding the
// classic bug of hand-building JSON with fmt.Sprintf, which breaks on quotes,
// backslashes or newlines in user text) and retries once on a transient 401,
// matching authorizedPostWithRetry's behaviour.
func (b *Bot) postJSONWithRetry(url string, payload interface{}, wantStatus int, token models.Token, user models.User) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode request body: %w", err)
	}

	do := func() (*http.Response, error) {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token.Token)
		req.Header.Set("Content-Type", "application/json")
		return httpClient.Do(req)
	}

	resp, err := do()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		if ok := helpers.TokenRetry(token, 5); !ok {
			helpers.MarkTokenExpired(b.db, user, token)
			return fmt.Errorf("token is no longer valid")
		}
		resp, err = do()
		if err != nil {
			return err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != wantStatus {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, url, string(respBody))
	}
	return nil
}

// callbackArg returns the nth underscore-separated segment of a callback payload.
func callbackArg(cq *tgbotapi.CallbackQuery, n int) string {
	parts := strings.Split(cq.Data, "_")
	if n < len(parts) {
		return parts[n]
	}
	return ""
}

// inboxAction runs an authorized POST against the inbox API (retrying once on a
// transient 401), shared by the vote/watch/star/read callbacks. Its caller, id,
// must be the notification's own card ID (card.ID), not the underlying issue ID:
// the Jira inbox "message" endpoints are keyed by the notification, not the issue.
func (b *Bot) inboxAction(cq *tgbotapi.CallbackQuery, user models.User, urlFn func(id string) string) (int, error, bool) {
	id := callbackArg(cq, 1)
	token, err := repositories.GetUserToken(b.db, user.ID)
	if err != nil {
		log.Printf("inboxAction: error fetching user token for user %d: %v\n", user.ID, err)
		return http.StatusUnauthorized, err, false
	}
	if err := b.authorizedPostWithRetry(urlFn(id), token, user); err != nil {
		log.Printf("inboxAction: error sending request for user %d: %v\n", user.ID, err)
		return http.StatusBadRequest, err, false
	}
	return http.StatusOK, nil, false
}

// tokenForUser returns a valid token, marking it expired and notifying the user when missing.
func (b *Bot) tokenForUser(user models.User, chatID int64) (models.Token, bool) {
	token, err := repositories.GetUserToken(b.db, user.ID)
	if err != nil {
		b.sendUnauthorizedMessage(chatID, user)
		helpers.MarkTokenExpired(b.db, user, token)
		log.Printf("tokenForUser: user %d is unauthorized: %v\n", user.ID, err)
		return token, false
	}
	if !helpers.TestToken(token) {
		if ok := helpers.TokenRetry(token, 5); !ok {
			b.sendUnauthorizedMessage(chatID, user)
			helpers.MarkTokenExpired(b.db, user, token)
			log.Printf("tokenForUser: user %d is unauthorized after retry\n", user.ID)
			return token, false
		}
	}
	return token, true
}

// jiraReminderURL builds the inbox reminder endpoint for an issue.
func jiraReminderURL(issueID any, in, text string) string {
	return fmt.Sprintf("%s/rest/inbox/1.0/reminder/%v?in=%v&text=%v", jiraURL, issueID, in, text)
}

// issueLink returns the browser URL for a Jira issue.
func issueLink(key string) string {
	return fmt.Sprintf("%s/browse/%v", jiraURL, key)
}

const pleaseWaitText = "لطفا کمی صبر کنید..."

// answerCallback shows a toast in response to a callback query.
func (b *Bot) answerCallback(cq *tgbotapi.CallbackQuery, text string) error {
	_, err := b.bot.Request(tgbotapi.NewCallback(cq.ID, text))
	return err
}

// redisFind returns the value stored under the given prefix in the user's assignTask list.
func redisFind(values []string, prefix string) string {
	for _, v := range values {
		if strings.HasPrefix(v, prefix) {
			return strings.TrimPrefix(v, prefix)
		}
	}
	return ""
}

// resetState clears the user's conversation state, notifying them on failure.
func (b *Bot) resetState(user models.User, chatID int64) error {
	if err := helpers.ResetState(b.db, user); err != nil {
		b.sendErrorMessage(chatID)
		log.Printf("resetState: error resetting stat for user %d: %v\n", user.ID, err)
		return err
	}
	return nil
}
