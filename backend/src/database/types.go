package database

import "time"

type PartialSession struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
	Model     string    `json:"model"`
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

type Message struct {
	ID        int64  `json:"id"`
	Role      Role   `json:"role"`
	Content   string `json:"content"`
	SessionID int64  `json:"session_id"`
}

type Session struct {
	PartialSession
	Messages []Message `json:"messages"`
}
