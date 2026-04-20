package database

import "time"

type CreateSession struct {
	Title string `json:"title"`
	Model string `json:"model"`
}

type UpdateSession struct {
	Title    string    `json:"title"`
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type PartialSession struct {
	CreateSession
	ID        int64     `json:"id"`
	UpdatedAt time.Time `json:"updated_at"`
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

type CreateSpecialistLog struct {
	Specialist string `json:"specialist"`
	Task       string `json:"task"`
	Script     string `json:"script"`
	OK         bool   `json:"ok"`
	Output     string `json:"output"`
	Error      string `json:"error"`
}

type SpecialistLog struct {
	CreateSpecialistLog
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateSecret struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Secret struct {
	CreateSecret
	ID        int64     `json:"id"`
	UpdatedAt time.Time `json:"updated_at"`
}
