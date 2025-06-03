package hub

import (
	"encoding/json"
	"log"
	"time"

	"chatroom/client"
	"chatroom/models"
)

type Hub struct {
	clients    map[*client.Client]bool
	broadcast  chan []byte
	register   chan *client.Client
	unregister chan *client.Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *client.Client),
		unregister: make(chan *client.Client),
		clients:    make(map[*client.Client]bool),
	}
}

func (h *Hub) Register(c *client.Client) {
	h.register <- c
}

func (h *Hub) Unregister(c *client.Client) {
	h.unregister <- c
}

func (h *Hub) Broadcast(message []byte) {
	h.broadcast <- message
}

func (h *Hub) Run() {
	for {
		select {
		case cl := <-h.register: // 将 client 变量名改为 cl，避免与 client 包冲突，尽管这不是强制性的
			h.clients[cl] = true
			log.Printf("客户端 %s 加入了聊天室。", cl.GetUsername()) // <--- 修正：使用 Getter
			// 通知其他用户新用户加入
			joinMsg := models.Message{
				Type:      "join",
				Username:  cl.GetUsername(),             // <--- 修正：使用 Getter
				Content:   cl.GetUsername() + " 加入了聊天。", // <--- 修正：使用 Getter
				Timestamp: time.Now(),
			}
			jsonMsg, _ := json.Marshal(joinMsg)
			for c := range h.clients {
				if c != cl { // 不向自己发送加入消息
					// select { // 可以简化，直接调用 SendMessage
					// case c.send <- jsonMsg:
					// default:
					// 	close(c.send)
					// 	delete(h.clients, c)
					// }
					c.SendMessage(jsonMsg) // <--- 修正：使用 SendMessage 方法
				}
			}

		case cl := <-h.unregister: // 将 client 变量名改为 cl
			if _, ok := h.clients[cl]; ok {
				delete(h.clients, cl)
				// close(cl.send) // <--- 这个逻辑应该在 client.writePump 退出时处理，或者由 SendMessage 内部判断
				log.Printf("客户端 %s 离开了聊天室。", cl.GetUsername()) // <--- 修正：使用 Getter
				// 通知其他用户用户离开
				leaveMsg := models.Message{
					Type:      "leave",
					Username:  cl.GetUsername(),             // <--- 修正：使用 Getter
					Content:   cl.GetUsername() + " 离开了聊天。", // <--- 修正：使用 Getter
					Timestamp: time.Now(),
				}
				jsonMsg, _ := json.Marshal(leaveMsg)
				for c := range h.clients {
					// select { // 可以简化，直接调用 SendMessage
					// case c.send <- jsonMsg:
					// default:
					// 	close(c.send)
					// 	delete(h.clients, c)
					// }
					c.SendMessage(jsonMsg) // <--- 修正：使用 SendMessage 方法
				}
			}
		case message := <-h.broadcast:
			for cl := range h.clients { // 将 client 变量名改为 cl
				// select { // 可以简化，直接调用 SendMessage
				// case cl.send <- message:
				// default:
				// 	close(cl.send)
				// 	delete(h.clients, cl)
				// }
				cl.SendMessage(message) // <--- 修正：使用 SendMessage 方法
			}
		}
	}
}
