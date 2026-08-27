+++

date = '2025-11-25T16:45:35+08:00'
title = "WebSocket详解"
tags = ["go", "WebSocket"]
categories = ["go"]

+++


## 1. 什么是 WebSocket

首先需要对WebSocket有一个准确的角色认知。它是和HTTP一样的**通信协议**。是用来规定两台机器应该怎么进行通信。

那么先看看HTTP是怎么样的通信方式。

浏览器访问: `http://example.com/user`

本质会产生这样的请求：
```
http://example.com/user
```

服务器返回：
```
HTTP/1.1 200 OK
Content-Type: application/json

{
    "name": "Tom"
}
```

于是可以看到网页内容或者json数据。

那WebSocket有什么不同？其实WebSocket建立连接的时候，最开始还是HTTP，这一点非常重要，因为都是基于TCP之上的协议，WebSocket的握手就是基于 HTTP 的 `Upgrade` 请求。WebSocket 借用了 HTTP 来建立连接，但建立成功之后，并不是一直按普通 HTTP 请求/响应方式通信。

客户端发出去的东西大致类似：
```
GET /chat HTTP/1.1
Host: example.com
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: xxxxx
Sec-WebSocket-Version: 13
```

服务端如果同意，就返回：
```
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: xxxxx
```

从响应中可以看到一个字段`101 Switching Protocols`，可以看出此时进行了协议转换，成了WebSocket。

这个过程就是**握手/upgrade**。

所以WebSocket和HTTP的关系可以建立这个脑图：
```
WebSocket
   │
   ├── 建立连接阶段
   │       ↓
   │     HTTP Upgrade
   │
   └── 建立成功以后
           ↓
       WebSocket 协议
```
```
WebSocket
   │
   ├── 建立连接阶段
   │       ↓
   │     HTTP Upgrade
   │
   └── 建立成功以后
           ↓
       WebSocket 协议
```

那到了现在，还是不清楚WebSocket和HTTP到底有什么不同？为什么要基于HTTP升级到WebSocket，是HTTP有什么不能解决的问题吗？

那还真对了，HTTP的问题非常明显，就是HTTP的通信模式是这样的：

```
Client                         Server
  |                              |
  | -------- HTTP Request -----> |
  | <------- HTTP Response ----- |
  |                              |
```
客户端请求一次，服务器响应一次。

HTTP的规则是：必须由客户端发起请求，服务器才能返回数据。服务器不能无缘无故主动给客户端发消息。
- HTTP的困境：如果想获取最新数据，客户端只能靠“轮询”（不停地问：“有更新吗？”）。这导致大量无用的空请求，浪费带宽和CPU，且存在延迟。

- WebSocket的解决：连接建立后，服务器拥有了“主动权”。一旦后台数据变化（如股票价格、物流轨迹、系统告警），服务器可以立即主动推送给客户端，实现真正的“零延迟”实时更新。

WebSocket的通信思路是：
```
Client                         Server
  |                              |
  | ---- HTTP Upgrade ---------> |
  | <--- 101 Switching --------- |
  |                              |
  | ===== WebSocket Conn ======= |
  |                              |
  | -------- message ----------> |
  | <------- message ----------- |
  | <------- message ----------- |
  | -------- message ----------> |
```

直接的 HTTP vs WebSocket 对比

| 对比                 | HTTP                             | WebSocket                          |
| -------------------- | -------------------------------- | ---------------------------------- |
| 通信模式             | 请求 → 响应                      | 双向通信                           |
| 是否适合长期实时通信 | 一般                             | 非常适合                           |
| 服务器能否主动发送   | 普通 HTTP 场景下不能直接主动推送 | 可以                               |
| 连接                 | 通常一次请求对应一次响应         | 可以长期保持                       |
| 常见用途             | REST API、网页请求、下载         | 聊天、实时通知、在线游戏、实时监控 |
| 地址                 | `http://` / `https://`           | `ws://` / `wss://`                 |

其中：`ws://`类似于：`http://`，而：`wss://`类似于：`https://`，`wss://` 表示加密的 WebSocket 连接。

>听到这里，还需要补充一点，或者说纠正一个可能存在的错误观念：**WebSocket 比 HTTP 更快**
>
>千万别形成这样的错误认识：WebSocket 就是“更快的 HTTP”。
>
>它们只是解决的问题不同。

## 2. quick start

现在开始代码实战部分，主要是以`github.com/gorilla/websocket`进行讲解。

先从一个最简单的Echo服务开始。

### 2.1 简单的服务端设计

```go
package main

import (
	"log"
	"net/http"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// 非浏览器客户端可能没有 Origin。
		if origin == "" {
			return true
		}
		// Demo 环境只允许自己的前端。
		return origin == "http://localhost:8080"
	},
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade websocket failed:", err)
		return
	}
	defer conn.Close()

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("read failed:", err)
			return
		}

		err = conn.WriteMessage(messageType, message)
		if err != nil {
			log.Println("write failed:", err)
			return
		}
	}
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
```

> 现在还不能直接在浏览器地址栏里输入 ws://
>
> 因为浏览器地址栏主要是拿来访问网页 HTTP/HTTPS 资源的。

### 2.2 简单的HTML客户端

前端可硬使用原生的WebSocket API进行测试。

```html
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>WebSocket Demo</title>
</head>
<body>

<button onclick="connect()">连接</button>
<button onclick="sendMessage()">发送 Hello</button>

<script>
let ws;

function connect() {
    ws = new WebSocket("ws://localhost:8080/ws");

    ws.onopen = function () {
        console.log("WebSocket 已连接");
    };

    ws.onmessage = function (event) {
        console.log("收到服务器消息：", event.data);
    };

    ws.onclose = function () {
        console.log("WebSocket 已关闭");
    };
}
    
function sendMessage() {
    ws.send("Hello");
}
</script>

</body>
</html>
```

也可以写一个go客户端。

```go
package main

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/echo", nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer conn.Close()

	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for t := range ticker.C {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(t.String())); err != nil {
			log.Println("write:", err)
			return
		}
	}
}
```

### 2.3 核心概念

对于服务端的开发，我们需要认识两个最核心的概念。
```
websocket.Upgrader
        |
        | HTTP -> WebSocket
        v
websocket.Conn
        |
        +---- ReadMessage()
        |
        +---- WriteMessage()
```
其中：

1. `Upgrader` 负责把 HTTP 请求升级成 WebSocket。

常见配置包括：

```go
websocket.Upgrader{ 
	ReadBufferSize: 1024, 
	WriteBufferSize: 1024, 
	CheckOrigin: ..., 
	EnableCompression: true,
}
```

其中 BufferSize 是 I/O 缓冲区配置，它不是 WebSocket 最大消息长度限制，读写 buffer 用于减少网络系统调用以及构建 WebSocket Frame；

如果要真正限制客户端消息大小，需要使用 `SetReadLimit`，例如`conn.SetReadLimit(1024 * 1024)`，表示最大消息1M。

对于连接数量很多但写入并不频繁的场景，还可以考虑 `WriteBufferPool` 降低每条连接长期占用写缓冲区造成的内存开销。

2. `Conn` 代表升级后的 WebSocket 长连接，读消息、写消息都在这个连接上
   
`messageType, data, err := conn.ReadMessage()`

`err := conn.WriteMessage(messageType, data)`

其中 `messageType` 最常见的是：

- websocket.TextMessage：文本消息
- websocket.BinaryMessage：二进制数据

> 千万注意：Upgrade 以后不要再写普通 HTTP 响应.
>
> 因为upgrade成功之后，这条连接已经从普通 HTTP 转换为 WebSocket。
>
> 后面的通信应该使用：conn.WriteMessage，而不是c.Json这种类似的HTTP语句

### 2.4 工程中放在哪个位置

已Gin框架为例：

```
Gin Middleware
       |
       | HTTP
       v
Authentication
Authorization
Parameter validation
Rate limiting
       |
       v
Upgrade()
       |
       | WebSocket
       v
Client
readPump
writePump
Hub
```
WebSocket 同样应该在 Upgrade 之前完成**身份确认**

```go
r.GET(
	"/ws",
	AuthMiddleware(),
	wsHandler,
)
```

### 2.5 并发模型

最需要牢记的是官方的并发约束：**一条连接允许一个并发 Reader 和一个并发 Writer。** 应用必须保证，不要让多个 goroutine 同时执行写方法，也不要让多个 goroutine 同时执行读方法；但是一个 goroutine 读、另一个 goroutine 写是允许的，也是推荐模式。

也就是说下面的代码有问题：

```go
go conn.WriteMessage(
	websocket.TextMessage,
	[]byte("A"),
)

go conn.WriteMessage(
	websocket.TextMessage,
	[]byte("B"),
)
```

不要把*websocket.Conn暴露给所有业务协程。

而是设计成：
```
+-------------------+
|     readPump()    |  --> 单独 goroutine，负责读
+-------------------+
+-------------------+
|     writePump()   |  --> 单独 goroutine，负责写
+-------------------+
```
所有消息排队，只有一个 `writePump`真正操作 Conn。

## 3. 生产问题

### 3.1 心跳保活Ping/Pong

WebSocket 是长连接，但中间可能经过代理、Nginx、负载均衡器。这些设备常常有超时机制，如果一个连接长时间没有数据流动，就会被静默断开。

WebSocket 协议内置了 Ping/Pong 控制帧：

Ping 帧：对端收到后应当立即回复 Pong；

Pong 帧：对 Ping 的响应；

Ping/Pong 不占用业务消息通道，应使用 `WriteControl` 发送。

服务端典型做法：

- 读循环设置 PongHandler，收到 Pong 后刷新读超时；

- 写循环定期发送 Ping；

- 如果读超时，说明客户端可能已经掉线，关闭连接。

```go
const (
	writeWait  = 10 * time.Second // 写超时
	pongWait   = 60 * time.Second // 等待pong的超时时间
	pingPeriod = (pongWait * 9) / 10 // Ping 周期，必须小于pongwait
)

// 设置读超时
conn.SetReadDeadline(time.Now().Add(pongWait))
// 收到Pong后刷新
conn.SetPongHandler(func(string) error {
	conn.SetReadDeadline(time.Now().Add(pongWait))
	return nil
})

ticker := time.NewTicker(pingPeriod)
defer ticker.Stop()

for {
	select {
	case message, ok := <-ch:
		// 写业务消息
	case <-ticker.C:
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
			return
		}
	}
}
```

### 3.2 连接管理

实际业务往往需要：

- 维护所有在线连接；

- 向某个用户或所有用户广播消息；

- 处理连接注册、注销。

Gorilla 官方示例提供了一套经典的 Hub 模式，核心是：

```go
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// 客户端太慢，处理策略
                    // 如果通道满了(说明该客户端网络太卡，消费不过来)，直接踢下线，防止拖垮整个系统
                    close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}
```

每个连接对应一个 Client：

```go
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}
```
每个 Client 启动两个 goroutine：

- readPump()：读取客户端消息；

- writePump()：从 send channel 取消息，写入连接。

这种模式的好处：

- 所有连接在 Hub 中统一管理；

- 通过 channel 通信，避免锁竞争；

- 读和写分离，符合 Gorilla 并发模型。

### 3.3 优雅关闭

生产环境不能直接 kill -9，需要处理：

- 停止接收新连接；

- 通知现有连接关闭；

- 清理资源；

- 退出进程。

类比HTTP等服务的优雅关闭。

```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := srv.Shutdown(ctx); err != nil {
	log.Printf("shutdown error: %v", err)
}
```

对比Hub，需要增加一个停止信号。

```go
type Hub struct {
	// ...
	stop chan struct{}
	done chan struct{}
}

func (h *Hub) Run() {
	defer close(h.done)
	for {
		select {
		case <-h.stop:
			// 关闭所有客户端
			for client := range h.clients {
				close(client.send)
				client.conn.Close()
			}
			return
		// 其他 case...
		}
	}
}

func (h *Hub) Shutdown() {
	close(h.stop)
	<-h.done
}
```

### 3.4 小结

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait       = 10 * time.Second
	pongWait        = 60 * time.Second
	pingPeriod      = (pongWait * 9) / 10
	maxMessageSize  = 1024
	sendBuffer      = 256
	broadcastBuffer = 256
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// 非浏览器客户端可能没有 Origin，按需调整
		if origin == "" {
			return true
		}
		return origin == "https://example.com" || origin == "http://localhost:3000"
	},
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	stop       chan struct{}
	done       chan struct{}
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan []byte, broadcastBuffer),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

func (h *Hub) run() {
	defer close(h.done)

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()
			log.Printf("client connected: %s", client.conn.RemoteAddr())

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Printf("client disconnected: %s", client.conn.RemoteAddr())
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					log.Printf("slow client %s, disconnecting", client.conn.RemoteAddr())
					go client.conn.Close()
				}
			}
			h.mu.RUnlock()

		case <-h.stop:
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				client.conn.Close()
			}
			h.mu.Unlock()
			return
		}
	}
}

func (h *Hub) shutdown() {
	close(h.stop)
	<-h.done
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func (c *Client) readPump() {
	defer func() {
		// 停止时避免阻塞在 unregister channel 上
		select {
		case c.hub.unregister <- c:
		case <-c.hub.stop:
		}
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		mt, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("read error: %v", err)
			}
			break
		}

		_ = mt // 这里忽略消息类型，实际可按类型处理

		// 非阻塞广播，避免拖慢读循环
		select {
		case c.hub.broadcast <- message:
		default:
			log.Printf("broadcast buffer full, drop message from %s", c.conn.RemoteAddr())
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// Hub 关闭了 send channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				return
			}
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade error:", err)
		return
	}

	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, sendBuffer),
	}

	// 停止注册新连接
	select {
	case hub.register <- client:
	case <-hub.stop:
		client.conn.Close()
		return
	}

	go client.writePump()
	go client.readPump()
}

func validSession(v string) bool {
	// TODO: 接入真实的 session / JWT 校验
	return v == "demo-session"
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil || !validSession(cookie.Value) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func main() {
	hub := newHub()
	go hub.run()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	}))

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Println("server listening on :8080")

		// 生产环境请使用 ListenAndServeTLS
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hub.shutdown()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	log.Println("server exited")
}
```

### 3.5 nginx反向代理（可选）

把 WebSocket 代码部署到服务器后，发现连不上。99% 是因为 Nginx 配置不对。
WebSocket 握手是 HTTP 请求，但 Nginx 默认不会把 HTTP 升级请求透传给后端。你必须在 Nginx 的 location 块中显式配置：

```
location /ws {
    proxy_pass http://your_backend_servers;
    
    # 必须配置 HTTP 1.1 版本，以支持长连接
    proxy_http_version 1.1;
    
    # 核心：告诉 Nginx 升级协议
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "Upgrade";
    
    # 透传真实 IP，方便服务端日志排查
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    
    # 代理超时设置 (防止 Nginx 自动掐断空闲连接)
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

