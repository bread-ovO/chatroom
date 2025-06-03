package client

import (
	"encoding/json"
	"log"
	"time"

	"chatroom/models"
	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

// Hub 是 Client 期望的 Hub 接口，它定义了客户端如何与 Hub 交互的方法。
// hub.Hub (具体的结构体) 将隐式地实现这个接口。
type Hub interface {
	Register(c *Client)
	Unregister(c *Client)
	Broadcast(message []byte)
}

// Client 代表一个连接到聊天室的用户
type Client struct {
	hub      Hub // 小写开头，私有字段
	conn     *websocket.Conn
	send     chan []byte // 小写开头，私有字段
	username string
}

// GetUsername 返回客户端的用户名。
// 这是公共方法，供其他包访问私有字段 username。
func (c *Client) GetUsername() string {
	return c.username
}

// SendMessage 发送消息到客户端的发送通道。
// 这是公共方法，供其他包（如 Hub）向此客户端发送消息。
func (c *Client) SendMessage(message []byte) {
	select {
	case c.send <- message:
	default:
		// 如果通道满了，通常表示客户端处理缓慢或已断开。
		// 这里可以根据需要添加更复杂的错误处理或日志。
	}
}

// readPump 从 WebSocket 连接读取消息并将其发送到 Hub。
// 这是一个内部方法（小写开头），只在 client 包内部使用。
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c) // 使用 Hub 接口的公共方法
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("错误: %v", err)
			}
			break
		}
		var msg models.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("解析消息失败: %v", err)
			continue
		}
		msg.Username = c.username // 在同一包内，可以访问私有字段
		msg.Timestamp = time.Now()
		msg.Type = "chat"

		parsedMessage, err := json.Marshal(msg)
		if err != nil {
			log.Printf("序列化解析后的消息失败: %v", err)
			continue
		}

		c.hub.Broadcast(parsedMessage) // 使用 Hub 接口的公共方法
	}
}

// writePump 将从 Hub 接收到的消息写入 WebSocket 连接。
// 这是一个内部方法（小写开头），只在 client 包内部使用。
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send: // 在同一包内，可以访问私有字段
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub 关闭了通道，发送一个 WebSocket 关闭消息并返回
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 将通道中所有排队的消息也写入
			n := len(c.send) // 在同一包内，可以访问私有字段
			for i := 0; i < n; i++ {
				w.Write([]byte("\n"))
				w.Write(<-c.send) // 在同一包内，可以访问私有字段
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// NewClient 是 Client 结构体的构造函数。
// **重要改变：** 它现在负责将客户端注册到 Hub 并启动其内部的读写协程。
func NewClient(h Hub, conn *websocket.Conn, username string) *Client {
	c := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256), // 缓冲通道，防止发送过快
		username: username,
	}

	// 将客户端注册到 Hub
	// Hub 会将这个客户端添加到其管理的客户端列表中
	c.hub.Register(c)

	// 启动客户端的读写协程。
	// 这些协程会持续监听WebSocket连接和内部消息通道。
	go c.writePump() // 启动写入协程
	go c.readPump()  // 启动读取协程

	return c // 返回新创建的客户端实例
}
