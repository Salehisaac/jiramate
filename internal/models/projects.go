package models

type Project struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	CategoryName   string `json:"categoryName"`
	ProjectTypeKey string `json:"projectTypeKey"`
	Name           string `json:"name"`
}

type AttachGroupProjects struct {
	ID         int64  `json:"id"`
	GroupID    int64  `json:"group_id"`
	UserID     int64  `json:"user_id"`
	ProjectKey string `json:"project_key"`
}
