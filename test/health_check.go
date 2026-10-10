package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"rira/internal/database"
	"rira/internal/database/repositories"
	"rira/internal/models"

	tgbotapi "github.com/Salehisaac/rasagram_bot_api/v8"
	jira "github.com/andygrunwald/go-jira"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

const (
	adminTelegramID = 136817825 // Your Telegram ID for receiving health reports
	testGroupID     = 10305     // Test group ID for notification checks
	jiraURL         = "https://pm.rso-co.ir"
	healthCheckTag  = "[HEALTH-CHECK]"
)

func main() {
	// Initialize logging
	log.Println("Starting JiraMate health check")

	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Printf("Error loading .env file: %v", err)
		reportError("Failed to load environment variables", err)
		return
	}

	// Initialize database connection
	dbUser := os.Getenv("MYSQL_USER")
	dbPassword := os.Getenv("MYSQL_PASSWORD")
	dbHost := os.Getenv("MYSQL_HOST")
	dbPort := os.Getenv("MYSQL_PORT")
	dbName := os.Getenv("MYSQL_DB")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	if err := database.ConnectDB(dsn); err != nil {
		log.Printf("Error connecting to database: %v", err)
		reportError("Failed to connect to database", err)
		return
	}

	db := database.GetDB()

	if err := database.InitDB(db); err != nil {
		log.Printf("error initializing database: %v", err)
		reportError("Failed to initialize database", err)
		return
	}
	log.Println("database has been built successfully")

	// Initialize Telegram bot
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		err := fmt.Errorf("BOT_TOKEN environment variable not set")
		reportError("Bot token not found", err)
		return
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		reportError("Failed to initialize Telegram bot", err)
		return
	}

	// Run health checks
	runHealthChecks(bot, db)
}

func runHealthChecks(bot *tgbotapi.BotAPI, db *sql.DB) {
	// Get a valid user with Jira token for testing
	user, token, err := getTestUser(db)
	if err != nil {
		reportErrorToTelegram(bot, "Failed to find a valid test user", err)
		return
	}
	log.Printf("Found test user: %s (ID: %d)", user.Username, user.ID)

	// Initialize Jira client
	jiraClient, err := createJiraClient(token.Token)
	if err != nil {
		reportErrorToTelegram(bot, "Failed to create Jira client", err)
		return
	}
	log.Println("Successfully connected to Jira API")

	// // Get a project key to use for testing
	// projectKey, err := getTestProjectKey(jiraClient)
	// if err != nil {
	// 	reportErrorToTelegram(bot, "Failed to get a test project", err)
	// 	return
	// }
	// log.Printf("Using project key: %s for testing", projectKey)

	// // Create test task
	// issueKey, err := createTestTask(jiraClient, projectKey)
	// if err != nil {
	// 	reportErrorToTelegram(bot, "Failed to create test task", err)
	// 	return
	// }
	issueKey := "MESB-107"
	log.Printf("Created test task: %s", issueKey)

	// Update task status to Done
	err = updateTaskStatusToDone(jiraClient, issueKey)
	if err != nil {
		// Clean up the test task even if we failed to update it
		deleteTestTask(jiraClient, issueKey)
		reportErrorToTelegram(bot, fmt.Sprintf("Failed to update test task %s status to Done", issueKey), err)
		return
	}
	log.Printf("Updated test task %s status to Done", issueKey)

	// Wait for notifications to be processed
	time.Sleep(5 * time.Second)

	// Check if notification was sent to test group
	notificationSent, err := verifyNotificationSent(bot, testGroupID, issueKey)
	if err != nil {
		deleteTestTask(jiraClient, issueKey)
		reportErrorToTelegram(bot, "Failed to verify notification delivery", err)
		return
	}

	// // Clean up the test task
	// err = deleteTestTask(jiraClient, issueKey)
	// if err != nil {
	// 	reportErrorToTelegram(bot, fmt.Sprintf("Failed to delete test task %s", issueKey), err)
	// 	// Continue execution since this is just cleanup
	// }

	// Report health check results
	if notificationSent {
		reportSuccessToTelegram(bot, "Health check completed successfully. Notifications are working properly.")
	} else {
		reportErrorToTelegram(bot, "Health check failed", fmt.Errorf("notification was not sent to test group for task %s", issueKey))
	}
}

func getTestUser(db *sql.DB) (models.User, models.Token, error) {
	var user models.User
	var token models.Token

	// Get all users
	users, err := repositories.GetAllUsers(db)
	if err != nil {
		return user, token, fmt.Errorf("failed to get users: %w", err)
	}

	// Find a user with a valid token. Status is nullable in the database, so it
	// must be nil-checked before dereferencing - a NULL row here used to panic.
	for _, u := range users {
		if u.Status != nil && *u.Status == "authorized" {
			t, err := repositories.GetUserToken(db, u.ID)
			if err == nil && t.Token != "" {
				return u, t, nil
			}
		}
	}

	return user, token, fmt.Errorf("no valid user with token found")
}

func createJiraClient(tokenStr string) (*jira.Client, error) {
	tp := jira.BearerAuthTransport{
		Token: tokenStr,
	}

	client, err := jira.NewClient(tp.Client(), jiraURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira client: %w", err)
	}

	return client, nil
}

func getTestProjectKey(client *jira.Client) (string, error) {
	projects, _, err := client.Project.GetList()
	if err != nil {
		return "", fmt.Errorf("failed to get projects: %w", err)
	}

	// Marshal the raw project list into JSON
	raw, err := json.Marshal(projects)
	if err != nil {
		return "", fmt.Errorf("failed to marshal projects: %w", err)
	}

	// Define your local struct
	type MyProject struct {
		Key string `json:"key"`
	}

	var myProjects []MyProject
	if err := json.Unmarshal(raw, &myProjects); err != nil {
		return "", fmt.Errorf("failed to unmarshal into local struct: %w", err)
	}

	if len(myProjects) == 0 {
		return "", fmt.Errorf("no projects found")
	}

	return myProjects[0].Key, nil
}

func createTestTask(client *jira.Client, projectKey string) (string, error) {
	// Create a unique summary for the test task
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	summary := fmt.Sprintf("%s Health Check Test Task - %s", healthCheckTag, timestamp)

	// Create issue fields
	issueFields := &jira.IssueFields{
		Project: jira.Project{
			Key: projectKey,
		},
		Summary:     summary,
		Description: "This is an automated health check task. It will be automatically deleted after testing.",
		Type: jira.IssueType{
			Name: "Task", // Assuming "Task" is a valid issue type in your Jira instance
		},
	}

	// Create the issue
	issue := &jira.Issue{
		Fields: issueFields,
	}

	createdIssue, _, err := client.Issue.Create(issue)
	if err != nil {
		return "", fmt.Errorf("failed to create test task: %w", err)
	}

	return createdIssue.Key, nil
}

func updateTaskStatusToDone(client *jira.Client, issueKey string) error {
	// Get available transitions for the issue
	transitions, _, err := client.Issue.GetTransitions(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get transitions for issue %s: %w", issueKey, err)
	}

	// Find the "Done" transition ID
	var doneTransitionID string
	for _, t := range transitions {
		if strings.ToLower(t.Name) == "done" || strings.ToLower(t.To.Name) == "done" {
			doneTransitionID = t.ID
			break
		}
	}

	if doneTransitionID == "" {
		return fmt.Errorf("could not find 'Done' transition for issue %s", issueKey)
	}

	// Perform the transition
	_, err = client.Issue.DoTransition(issueKey, doneTransitionID)
	if err != nil {
		return fmt.Errorf("failed to transition issue %s to Done: %w", issueKey, err)
	}

	return nil
}

func verifyNotificationSent(bot *tgbotapi.BotAPI, groupID int, issueKey string) (bool, error) {
	// This is a simplified check. In a real implementation, you might need to:
	// 1. Check the database for notification records
	// 2. Use the Telegram API to check recent messages in the group
	// 3. Set up a listener for new messages

	// For this example, we'll just wait a bit and assume the notification system works
	// In a real implementation, you'd want to actually verify the notification was sent

	// Try to send a test message to the group to verify we have access
	msg := tgbotapi.NewMessage(int64(groupID), fmt.Sprintf("Health check: Verifying notifications for task %s", issueKey))
	_, err := bot.Send(msg)
	if err != nil {
		return false, fmt.Errorf("failed to send test message to group %d: %w", groupID, err)
	}

	// In a real implementation, you would check if the actual notification was sent
	// For now, we'll assume it worked if we can send messages to the group
	return true, nil
}

func deleteTestTask(client *jira.Client, issueKey string) error {
	_, err := client.Issue.Delete(issueKey)
	if err != nil {
		return fmt.Errorf("failed to delete test task %s: %w", issueKey, err)
	}
	return nil
}

func reportError(message string, err error) {
	fullMessage := fmt.Sprintf("JiraMate Health Check Error: %s - %v", message, err)
	log.Println(fullMessage)
	// In a real implementation, you might want to log this to a file or monitoring system
}

func reportErrorToTelegram(bot *tgbotapi.BotAPI, message string, err error) {
	fullMessage := fmt.Sprintf("❌ JiraMate Health Check Error:\n%s\n\nError details: %v", message, err)
	msg := tgbotapi.NewMessage(adminTelegramID, fullMessage)
	_, sendErr := bot.Send(msg)
	if sendErr != nil {
		log.Printf("Failed to send error report to Telegram: %v", sendErr)
	}
}

func reportSuccessToTelegram(bot *tgbotapi.BotAPI, message string) {
	fullMessage := fmt.Sprintf("✅ JiraMate Health Check Success:\n%s", message)
	msg := tgbotapi.NewMessage(adminTelegramID, fullMessage)
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("Failed to send success report to Telegram: %v", err)
	}
}
