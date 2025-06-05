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
	hub      Hub
	conn     *websocket.Conn // 保持小写，私有
	send     chan []byte     // 保持小写，私有
	username string          // 保持小写，私有
}

// GetUsername 返回客户端的用户名。
// 这是一个公共方法，供其他包访问私有字段 username。
func (c *Client) GetUsername() string {
	return c.username
}

// SendMessage 发送消息到客户端的发送通道。
// 这是一个公共方法，供其他包（如 Hub）向此客户端发送消息。
func (c *Client) SendMessage(message []byte) {
	select {
	case c.send <- message:
	default:
		// 通道已满或关闭，通常表示客户端已断开或处理缓慢。
		// 这里可以根据需要添加更复杂的错误处理或日志。
	}
}

// CloseConnection 提供一个公共方法让 Hub 可以关闭连接。
func (c *Client) CloseConnection() {
	c.conn.Close()
}

// RunPumps 是一个公共方法，用于启动客户端的读写协程。
// Hub 包将调用此方法来启动客户端的内部逻辑。
func (c *Client) RunPumps() {
	go c.writePump() // 启动写入协程 (内部私有方法)
	go c.readPump()  // 启动读取协程 (内部私有方法)
}

// readPump 从 WebSocket 连接读取消息并将其发送到 Hub。
// 这是一个内部方法（小写开头），只在 client 包内部使用。
func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c) // 在 readPump 退出时，将客户端从 Hub 注销
		c.conn.Close()      // 关闭 WebSocket 连接
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
			break // 读取出错，退出循环，触发 defer
		}
		// 解析消息并添加用户名和时间戳
		var msg models.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("解析消息失败: %v", err)
			continue
		}
		msg.Username = c.username // 设置客户端的用户名 (在同一包内，可以访问私有字段)
		msg.Timestamp = time.Now()
		msg.Type = "chat" // 默认消息类型

		parsedMessage, err := json.Marshal(msg)
		if err != nil {
			log.Printf("序列化解析后的消息失败: %v", err)
			continue
		}

		c.hub.Broadcast(parsedMessage) // 将消息发送到 Hub 进行广播
	}
}

// writePump 将从 Hub 接收到的消息写入 WebSocket 连接。
// 这是一个内部方法（小写开头），只在 client 包内部使用。
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod) // 定时发送 ping 帧，保持连接活跃
	defer func() {
		ticker.Stop()  // 停止定时器
		c.conn.Close() // 关闭 WebSocket 连接
	}()
	for {
		select {
		case message, ok := <-c.send: // 从发送通道接收消息
			c.conn.SetWriteDeadline(time.Now().Add(writeWait)) // 设置写操作超时
			if !ok {
				// Hub 关闭了通道，发送一个 WebSocket 关闭消息并返回
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage) // 获取 WebSocket 写入器
			if err != nil {
				return // 写入器获取失败，退出
			}
			w.Write(message) // 写入消息

			// 将通道中所有排队的消息也写入
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte("\n")) // 消息之间添加换行符
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil { // 关闭写入器
				return
			}
		case <-ticker.C: // 定时器触发，发送 ping 帧
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // ping 失败，退出
			}
		}
	}
}

// NewClient 是 Client 结构体的构造函数。
// 它只负责创建 Client 实例，不负责启动其读写协程（由 Hub 在注册成功后启动）。
func NewClient(h Hub, conn *websocket.Conn, username string) *Client {
	c := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256), // 缓冲通道，防止发送过快导致阻塞
		username: username,
	}
	return c
}
