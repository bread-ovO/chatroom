package store

import (
	"chatroom/models"
)

// MessageStore 定义了消息存储的接口
type MessageStore interface {
	Init() error // 初始化存储（例如创建表）
	SaveMessage(msg models.Message) error
	GetMessages(limit int) ([]models.Message, error) // 获取最近的 N 条消息
}
