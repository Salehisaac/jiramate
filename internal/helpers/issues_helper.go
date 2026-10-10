package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	jira "github.com/andygrunwald/go-jira"
)

func CreateIssue(client *jira.Client, projectKey, summary, description, issueType, assigneeAccountName, priority string) (string, error) {

	i := jira.Issue{
		Fields: &jira.IssueFields{
			Assignee: &jira.User{
				Name: assigneeAccountName,
			},
			Description: description,
			Type: jira.IssueType{
				Name: issueType,
			},
			Priority: &jira.Priority{
				Name: priority,
			},
			Project: jira.Project{
				Key: projectKey,
			},
			Summary: summary,
			Unknowns: map[string]interface{}{
				"customfield_10424": map[string]string{
					"id": "10509",
				},
			},
		},
	}

	if issueJSON, err := json.MarshalIndent(i, "", "  "); err != nil {
		log.Printf("CreateIssue: failed to marshal issue payload for logging: %v", err)
	} else {
		log.Printf("CreateIssue: creating issue in project %s:\n%s\n", projectKey, issueJSON)
	}
	issue, _, err := client.Issue.Create(&i)
	if err != nil {
		return "", fmt.Errorf("failed to create issue in project %s: %w", projectKey, err)
	}

	return issue.Key, nil
}

func FetchProjectTasks(client *jira.Client, projectKey string) ([]jira.Issue, error) {
	jql := fmt.Sprintf(`project = "%s" AND status != Done ORDER BY created DESC`, projectKey)

	jiraIssues, _, err := client.Issue.Search(jql, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to search tasks for project %s: %w", projectKey, err)
	}

	return jiraIssues, nil
}

func AssignUserToTask(client *jira.Client, assigneeAccountName, issueID string) (string, error) {

	user := jira.User{
		Name: assigneeAccountName,
	}

	resp, err := client.Issue.UpdateAssigneeWithContext(context.Background(), issueID, &user)
	if err != nil {
		return "", err
	}

	return resp.Response.Status, nil
}
