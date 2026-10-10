package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	jira "github.com/andygrunwald/go-jira"

	"rira/internal/models"
)

func GetAllProjects(client *jira.Client, jiraURL string) ([]models.Project, error) {
	allProjects, _, err := client.Project.GetList()
	if err != nil {
		return nil, fmt.Errorf("error fetching projects: %v", err)
	}

	var projects []models.Project
	for _, project := range *allProjects {
		projects = append(projects, models.Project{
			ID:             project.ID,
			Key:            project.Key,
			Name:           project.Name,
			CategoryName:   project.ProjectCategory.Name,
			ProjectTypeKey: project.ProjectTypeKey,
		})
	}
	return projects, nil
}

func FetchProjectRoles(client *jira.Client, jiraURL, token, projectKey string) (map[string]string, error) {
	project, resp, err := client.Project.Get(projectKey)
	if err != nil {
		return nil, fmt.Errorf("error fetching project %s: %v", projectKey, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return project.Roles, nil
}

func FetchProjectUsers(roles map[string]string, token string) (map[string]*models.RoleUser, []error) {
	users := make(map[string]*models.RoleUser)
	var mu sync.Mutex // guards users: it's written from every role's goroutine below
	var wg sync.WaitGroup
	errChan := make(chan error, len(roles))

	for roleName, roleURL := range roles {
		wg.Add(1)
		go func(roleName, roleURL string) {
			defer wg.Done()

			roleUsers, err := FetchRoleUsers(roleURL, token)
			if err != nil {
				if err.Error() == "401" {
					errChan <- fmt.Errorf("401")
					return
				}
				log.Printf("Error fetching users for role %s: %v", roleName, err)
				errChan <- fmt.Errorf("error fetching users for role %s: %v", roleName, err)
				return
			}

			mu.Lock()
			defer mu.Unlock()
			for _, actor := range roleUsers.Actors {
				if u, exists := users[actor.Name]; exists {
					u.Roles = append(u.Roles, roleName)
				} else {
					users[actor.Name] = &models.RoleUser{
						DisplayName: actor.DisplayName,
						Name:        actor.Name,
						AccountID:   actor.AccountID.String(),
						Roles:       []string{roleName},
					}
				}
			}
		}(roleName, roleURL)
	}

	wg.Wait()
	close(errChan)

	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}
	return users, errors
}

func FetchRoleUsers(roleURL, token string) (*models.RoleUsersResponse, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", roleURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("401")
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var roleUsers models.RoleUsersResponse
	if err := json.NewDecoder(resp.Body).Decode(&roleUsers); err != nil {
		return nil, err
	}
	return &roleUsers, nil
}
