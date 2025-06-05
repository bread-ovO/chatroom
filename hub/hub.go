package hub

import (
	"encoding/json"
	"log"
	"sort" // 用于排序用户列表
	"time" // 用于消息时间戳

	"chatroom/client" // 导入 client 包，以便引用 client.Client 类型
	"chatroom/models" // 导入 models 包，以便引用 Message 类型
	"chatroom/store"  // 导入 store 包，以便引用 MessageStore 接口
)

// Hub 是聊天室的中心，负责管理客户端连接和消息广播。
type Hub struct {
	// clients 使用 map[string]*client.Client 存储活跃的客户端连接，键为用户名。
	clients map[string]*client.Client

	// broadcast 是一个缓冲通道，用于接收来自客户端的入站消息。
	broadcast chan []byte

	// register 是一个缓冲通道，用于接收客户端的注册请求。
	register chan *client.Client

	// unregister 是一个缓冲通道，用于接收客户端的注销请求。
	unregister chan *client.Client

	// messageStore 是一个 MessageStore 接口的实例，用于消息的持久化存储。
	messageStore store.MessageStore
}

// NewHub 创建并返回一个新的 Hub 实例。
// 它需要一个 MessageStore 接口的实现，用于消息的持久化。
func NewHub(ms store.MessageStore) *Hub {
	return &Hub{
		clients:      make(map[string]*client.Client), // 初始化客户端 map
		broadcast:    make(chan []byte),
		register:     make(chan *client.Client),
		unregister:   make(chan *client.Client),
		messageStore: ms, // 赋值消息存储实例
	}
}

// Register 方法将客户端添加到注册通道。
// client.Client 会调用此方法来向 Hub 发送注册请求。
func (h *Hub) Register(c *client.Client) {
	h.register <- c
}

// Unregister 方法将客户端添加到注销通道。
// 当客户端断开连接时，client.Client 会调用此方法。
func (h *Hub) Unregister(c *client.Client) {
	h.unregister <- c
}

// Broadcast 方法将消息添加到广播通道。
// 当客户端发送消息时，会通过此方法将消息发送到 Hub 进行广播。
func (h *Hub) Broadcast(message []byte) {
	h.broadcast <- message
}

// SendUserListToAllClients 生成当前在线用户列表，并将其作为 "user_list" 类型的消息广播给所有在线客户端。
// 方法名大写开头，使其在 Hub 包内部可访问，如果需要，其他包也可以访问。
func (h *Hub) SendUserListToAllClients() {
	userList := make([]string, 0, len(h.clients))
	for username := range h.clients {
		userList = append(userList, username)
	}
	sort.Strings(userList)
	log.Printf("DEBUG: Current user list: %v (count: %d)", userList, len(userList))

	userListMsg := models.Message{
		Type:  "user_list",
		Users: userList,
		// <--- 关键修正：移除下面这三行，它们是多余的，且零值可能导致问题
		// Username:  "",
		// Content:   "",
		// Timestamp: time.Time{}, // 移除此行
	}
	jsonUserListMsg, err := json.Marshal(userListMsg)
	if err != nil {
		log.Printf("序列化用户列表消息失败: %v", err)
		return
	}
	log.Printf("DEBUG: Broadcasting user_list message: %s", string(jsonUserListMsg)) // <--- 添加这条日志

	for _, cl := range h.clients {
		log.Printf("DEBUG: Sending user_list to client: %s", cl.GetUsername()) // <--- 添加这条日志
		cl.SendMessage(jsonUserListMsg)
	}
}

// Run 启动 Hub 的主事件循环。
// 这个方法在一个单独的 goroutine 中运行，持续监听来自各个通道的事件。
func (h *Hub) Run() {
	for {
		select {
		// 处理客户端注册请求
		case cl := <-h.register:
			log.Printf("DEBUG: Hub received register request for client: %s", cl.GetUsername()) // <--- 添加 DEBUG 日志

			// 1. 检查昵称唯一性
			if _, exists := h.clients[cl.GetUsername()]; exists {
				errMsg := models.Message{
					Type:  "error",
					Error: "昵称已被占用，请尝试其他昵称。",
				}
				jsonErrMsg, _ := json.Marshal(errMsg)
				cl.SendMessage(jsonErrMsg)
				cl.CloseConnection() // 关闭连接
				log.Printf("拒绝客户端 %s: 昵称已被占用。", cl.GetUsername())
				continue // 跳过当前循环，不进行后续注册步骤
			}

			// 昵称唯一，将客户端添加到 Hub 的管理列表
			h.clients[cl.GetUsername()] = cl
			log.Printf("客户端 %s 加入了聊天室。", cl.GetUsername()) // <--- 这条日志应该出现

			// 启动新连接客户端的读写协程。
			// 这是客户端内部处理消息收发的核心逻辑。
			cl.RunPumps() // <--- 修正：Hub 在成功注册后才启动泵

			// --- 发送历史消息给新连接的客户端 ---
			historyMessages, err := h.messageStore.GetMessages(50)
			if err != nil {
				log.Printf("获取历史消息失败: %v", err)
			} else {
				for _, msg := range historyMessages {
					jsonMsg, err := json.Marshal(msg)
					if err != nil {
						log.Printf("序列化历史消息失败: %v", err)
						continue
					}
					cl.SendMessage(jsonMsg)
				}
			}

			// --- 广播用户加入通知 ---
			joinMsg := models.Message{
				Type:      "join",
				Username:  cl.GetUsername(),
				Content:   cl.GetUsername() + " 加入了聊天。",
				Timestamp: time.Now(),
			}
			jsonMsg, _ := json.Marshal(joinMsg)
			h.messageStore.SaveMessage(joinMsg)

			for _, c := range h.clients {
				c.SendMessage(jsonMsg)
			}

			// --- 更新并广播在线用户列表 ---
			h.SendUserListToAllClients()

		// 处理客户端注销请求（客户端断开连接）
		case cl := <-h.unregister:
			// 检查客户端是否存在于 Hub 的管理列表中 (通过用户名查找)
			if _, ok := h.clients[cl.GetUsername()]; ok {
				// 从管理列表中删除客户端 (通过用户名删除)
				delete(h.clients, cl.GetUsername())
				log.Printf("客户端 %s 离开了聊天室。", cl.GetUsername())

				// 构建用户离开通知消息
				leaveMsg := models.Message{
					Type:      "leave",
					Username:  cl.GetUsername(),
					Content:   cl.GetUsername() + " 离开了聊天。",
					Timestamp: time.Now(),
				}
				jsonMsg, _ := json.Marshal(leaveMsg)
				// 将用户离开消息保存到数据库
				h.messageStore.SaveMessage(leaveMsg) // h.messageStore 必须是 MessageStore 接口的实例

				// 将离开通知广播给所有剩余的在线客户端
				for _, c := range h.clients { // 遍历 map 的值
					c.SendMessage(jsonMsg)
				}
				// --- 更新并广播在线用户列表 ---
				// 调用 Hub 的公共方法 SendUserListToAllClients
				h.SendUserListToAllClients()
			}

		// 处理来自客户端的广播消息
		case message := <-h.broadcast:
			// 解码消息以便进行持久化（如果需要）
			var msg models.Message
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("广播消息解码失败: %v", err)
				continue
			}
			// 将聊天消息保存到数据库
			h.messageStore.SaveMessage(msg) // h.messageStore 必须是 MessageStore 接口的实例

			// 将原始 JSON 消息广播给所有在线客户端
			for _, cl := range h.clients { // 遍历 map 的值
				cl.SendMessage(message)
			}
		}
	}
}
