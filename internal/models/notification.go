package models

type NotificationResponse struct {
	Page          int           `json:"page"`
	PageSize      int           `json:"pageSize"`
	Cards         []Card        `json:"cards"`
	StarredCards  []interface{} `json:"starredCards"`
	ReminderCards []interface{} `json:"reminderCards"`
}

type Card struct {
	ID                int     `json:"id"`
	Actor             Actor   `json:"actor"`
	Type              string  `json:"type"`
	Header            string  `json:"header"`
	Text              string  `json:"text"`
	TextSummary       string  `json:"textSummary"`
	Issue             Issue   `json:"issue"`
	Opened            bool    `json:"opened"`
	TimeAgo           string  `json:"timeAgo"`
	Voted             bool    `json:"voted"`
	Watching          bool    `json:"watching"`
	LastCommentId     int     `json:"lastCommentId"`
	Stats             Stats   `json:"stats"`
	JSD               bool    `json:"jsd"`
	Reminder          bool    `json:"reminder"`
	HideIssue         bool    `json:"hideIssue"`
	Starred           bool    `json:"starred"`
	Inactive          bool    `json:"inactive"`
	PendingApprovalId int     `json:"pendingApprovalId"`
	ApprovalState     *string `json:"approvalState"`
	ApprovalPending   bool    `json:"approvalPending"`
}

type Actor struct {
	UserName    string `json:"userName"`
	DisplayName string `json:"displayName"`
	AvatarUrl   string `json:"avatarUrl"`
}

type Issue struct {
	Key                     string `json:"key"`
	ID                      int    `json:"id"`
	StatusName              string `json:"statusName"`
	StatusCategoryKey       string `json:"statusCategoryKey"`
	StatusCategoryColorName string `json:"statusCategoryColorName"`
	IconUrl                 string `json:"iconUrl"`
}

type Stats struct {
	NumComments int `json:"numComments"`
	NumUpdates  int `json:"numUpdates"`
	NumWorkLogs int `json:"numWorkLogs"`
}
