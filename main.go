package main

import (
	"flag"
	"log"
	"net/http"
	"text/template"

	"chatroom/client" // 导入你的 client 包
	"chatroom/hub"    // 导入你的 hub 包
	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", ":8080", "http 服务地址")

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
	// 尝试将 HTTP 连接升级为 WebSocket 连接
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// 从 URL 查询参数中获取用户名，如果未提供则默认为“游客”
	username := r.URL.Query().Get("username")
	if username == "" {
		username = "游客"
	}

	// 调用 client.NewClient 函数来创建并启动一个新的客户端。
	// NewClient 内部会处理客户端的注册（通过 Hub 接口）和读写协程的启动。
	_ = client.NewClient(myHub, conn, username) // 使用 _ 丢弃返回值，因为在这里我们不需要 Client 实例的引用。
}

func main() {
	flag.Parse() // 解析命令行参数

	myHub := hub.NewHub() // 创建聊天室的 Hub 实例
	go myHub.Run()        // 启动 Hub 的主循环协程，处理注册、注销和广播消息

	// 注册 HTTP 路由处理器
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(myHub, w, r) // 将 Hub 实例传递给 WebSocket 处理器
	})

	log.Printf("服务器已在 %s 启动", *addr)
	// 启动 HTTP 服务器并监听指定地址
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe 失败: ", err)
	}
}

// home.html 模板（与 main.go 在同一目录或调整路径）
var homeTemplate = template.Must(template.ParseFiles("home.html"))
