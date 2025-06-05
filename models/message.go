package models

import "time"

type Message struct {
	Type      string    `json:"type"` // 例如 "chat", "join", "leave"
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`

	Users []string `json:"users,omitempty"`
	Error string   `json:"error,omitempty"`
}
