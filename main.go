package main

import (
	"flag"
	"log"
	"net/http"
	"os"        // 用于处理信号
	"os/signal" // 用于处理信号
	"syscall"   // 用于处理信号
	"text/template"

	"chatroom/client"
	"chatroom/hub"
	"chatroom/store"
	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", ":8080", "http 服务地址")
var dbPath = flag.String("db", "./chat.db", "SQLite 数据库文件路径") // 数据库路径参数

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，方便开发
	},
}

// serveHome 处理根路径 "/" 的 HTTP 请求，通常用于提供 HTML 页面。
func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	if r.URL.Path != "/" {
		http.Error(w, "未找到", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "方法不被允许", http.StatusMethodNotAllowed)
		return
	}
	homeTemplate.Execute(w, r.Host) // 渲染 HTML 模板
}

// serveWs 处理 WebSocket 连接升级请求。
func serveWs(myHub *hub.Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		username = "游客"
	}

	cl := client.NewClient(myHub, conn, username)
	// <--- 关键修正：将客户端实例发送到 Hub 的注册通道
	myHub.Register(cl) // 调用 Hub 的 Register 方法

	// cl.RunPumps() 不在这里调用了，因为它现在由 Hub 在注册成功后调用。
	// 这解决了循环依赖问题，也确保了只有成功注册的客户端才启动泵。
}

func main() {
	flag.Parse() // 解析命令行参数

	// --- 初始化数据库存储 ---
	// 创建 SQLiteMessageStore 实例
	messageStore, err := store.NewSQLiteMessageStore(*dbPath)
	if err != nil {
		log.Fatalf("创建消息存储失败: %v", err)
	}
	defer messageStore.Close() // 确保在程序退出时关闭数据库连接

	// 初始化数据库表
	if err := messageStore.Init(); err != nil {
		log.Fatalf("初始化消息存储失败: %v", err)
	}

	// 创建聊天室的 Hub 实例，并将消息存储传递给它
	myHub := hub.NewHub(messageStore)
	go myHub.Run() // 启动 Hub 的主循环协程，处理注册、注销和广播消息

	// 注册 HTTP 路由处理器
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(myHub, w, r) // 将 Hub 实例传递给 WebSocket 处理器
	})

	// --- 优雅关闭服务器 ---
	// 创建一个通道用于接收操作系统信号
	quit := make(chan os.Signal, 1)
	// 监听中断信号 (Ctrl+C) 和终止信号
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 在一个单独的协程中启动 HTTP 服务器
	go func() {
		if err := http.ListenAndServe(*addr, nil); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe 失败: %v", err)
		}
	}()
	log.Printf("服务器已在 %s 启动", *addr)

	<-quit // 阻塞主协程，直到接收到终止信号
	log.Println("收到终止信号，正在关闭服务器...")
	// 在这里可以添加清理资源的代码，例如关闭所有 WebSocket 连接。
	// 对于这个简单的应用，defer messageStore.Close() 已经处理了数据库关闭。
	log.Println("服务器已优雅关闭。")
}

// home.html 模板（与 main.go 在同一目录或调整路径）
var homeTemplate = template.Must(template.ParseFiles("home.html"))
