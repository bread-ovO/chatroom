package store

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"chatroom/models"
	_ "github.com/mattn/go-sqlite3" // SQLite 驱动
)

type SQLiteMessageStore struct {
	db *sql.DB
}

// NewSQLiteMessageStore 创建并返回一个新的 SQLiteMessageStore 实例
func NewSQLiteMessageStore(dataSourceName string) (*SQLiteMessageStore, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// ping the database to ensure connection is established
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	return &SQLiteMessageStore{db: db}, nil
}

// Init 初始化数据库，创建 messages 表
func (s *SQLiteMessageStore) Init() error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		username TEXT,
		content TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	_, err := s.db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("创建 messages 表失败: %w", err)
	}
	log.Println("SQLite 数据库表初始化成功。")
	return nil
}

func (s *SQLiteMessageStore) SaveMessage(msg models.Message) error {
	if msg.Type != "chat" && msg.Type != "join" && msg.Type != "leave" {
		return nil
	}

	// 将 time.Time 格式化为数据库能接受的字符串格式，通常推荐 ISO 8601 或 RFC3339
	// SQLite 的 CURRENT_TIMESTAMP 默认是 "YYYY-MM-DD HH:MM:SS" 或 "YYYY-MM-DD HH:MM:SS.SSS"
	// 为了兼容，我们存入数据库时使用 time.RFC3339Nano 格式，这是最完整的格式
	insertSQL := `INSERT INTO messages(type, username, content, timestamp) VALUES(?, ?, ?, ?)`
	_, err := s.db.Exec(insertSQL, msg.Type, msg.Username, msg.Content, msg.Timestamp.Format(time.RFC3339Nano)) // <--- 关键修正：存储时格式化
	if err != nil {
		return fmt.Errorf("保存消息失败: %w", err)
	}
	return nil
}

// GetMessages 获取最近的 N 条消息
func (s *SQLiteMessageStore) GetMessages(limit int) ([]models.Message, error) {
	query := `SELECT type, username, content, timestamp FROM messages ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("查询消息失败: %w", err)
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var msg models.Message
		var timestampStr string
		if err := rows.Scan(&msg.Type, &msg.Username, &msg.Content, &timestampStr); err != nil {
			return nil, fmt.Errorf("扫描消息行失败: %w", err)
		}
		// <--- 关键修正：读取时使用 time.RFC3339Nano 解析
		parsedTime, err := time.Parse(time.RFC3339Nano, timestampStr)
		if err != nil {
			log.Printf("警告: 解析时间戳 '%s' 失败: %v", timestampStr, err)
			parsedTime = time.Now() // 回退到当前时间
		}
		msg.Timestamp = parsedTime
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("行迭代错误: %w", err)
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// Close 关闭数据库连接
func (s *SQLiteMessageStore) Close() error {
	return s.db.Close()
}
