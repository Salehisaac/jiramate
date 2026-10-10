package models

import "encoding/json"

type RoleUser struct {
	DisplayName string   `json:"displayName"`
	Name        string   `json:"name"`
	AccountID   string   `json:"id"`
	Roles       []string `json:"roles"`
}

type RoleUsersResponse struct {
	Actors []struct {
		DisplayName string      `json:"displayName"`
		Name        string      `json:"name"`
		AccountID   json.Number `json:"id"`
	} `json:"actors"`
}
