+++

date = '2025-11-29T17:36:22+08:00' 

title = "模型上下文协议（MCP）的 Go 实现，实现了 LLM 应用程序与外部数据源和工具之间的无缝集成" 

tags = ["AI", "mcp","go"] 

categories = ["AI","agent"]

+++

# 模型上下文协议（MCP）的 Go 实现，实现了 LLM 应用程序与外部数据源和工具之间的无缝集成

如果你最近在看 AI 应用、Agent、工具调用或者 LLM 集成，大概率已经绕不过一个词：`MCP`。

MCP，全称 `Model Context Protocol`，本质上是在定义一套标准接口，让模型能够以更统一、更可控的方式访问外部世界。工具、资源、提示词模板、会话上下文，这些原本散落在不同系统里的能力，开始可以用一套更稳定的协议来描述和暴露。

问题在于，协议本身只是开始。真正落地的时候，你还得处理一堆工程细节：

- JSON-RPC 消息怎么组织
- 工具参数怎么描述
- 资源怎么暴露
- 多种传输方式怎么接
- 会话状态怎么管理
- 长任务怎么异步返回
- 错误、恢复、钩子、中间件怎么做

如果这些都手写，你会很快发现：写的不是业务，而是在重复造一层协议框架。

这也是我觉得 [`mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) 值得写一篇文章的原因。它不是在 Go 里“又造了一个神秘框架”，而是在一个非常务实的位置上，把 MCP 服务端和客户端开发里的重复劳动，整理成了可用、可读、可维护的 API。

## 一、概要

很多人第一次接触 MCP SDK，会先去找一个 hello world，然后很快得出一个结论：不就是把工具包装成几个 handler 吗？

这个理解只对了一半。

真正的问题不是“把一个函数暴露成 MCP Tool 难不难”，而是当你的能力越来越多时，系统还能不能保持整洁。

一个真实的 MCP 服务，通常不会只有一个工具。它往往会同时包含：

- 一组工具：给模型执行动作
- 一组资源：给模型读取上下文
- 一组提示词模板：给模型构造交互模式
- 一套传输层：本地 stdio、SSE、HTTP，甚至嵌入式场景
- 一套会话机制：识别不同客户端、维护每个会话的状态
- 一套治理能力：恢复、日志、钩子、中间件、并发限制

如果你直接围绕协议报文去写，很快就会进入“每个功能都能做，但每个功能都写得很散”的状态。

`mcp-go` 的价值，就在于它提供的是一种**高层抽象**：

1. 你主要关注“我要暴露什么能力”。
2. 协议细节、路由、消息格式、能力协商这些工作由库来兜底。
3. 最终保留下来的代码，仍然很符合 Go 的习惯：显式、直白、少魔法。

这个定位我很喜欢，因为它没有试图把开发者绑进一个重框架里。它更像是一个“协议层工具箱”，而不是一个“统治一切的应用框架”。

## 二、详细介绍

从官方 README 和文档来看，`mcp-go` 当前已经覆盖了 MCP 的几块核心能力：

- `Tools`
- `Resources`
- `Prompts`
- 多传输层支持
- 客户端实现
- 会话管理
- 请求钩子
- Tool Handler Middleware
- 长任务工具

这几个关键词看起来很普通，但组合起来，意味着它已经不只是一个“给 demo 用的 SDK”。

### 1. Tools、Resources、Prompts 三件套

很多库会先把 Tool 做起来，因为 Tool 最容易展示效果：模型调用函数，函数返回结果，闭环就成立了。

但如果你真的要做一个靠谱的 MCP 服务，只支持 Tool 远远不够。你还需要：

- `Resource`：让模型读取结构化或半结构化内容
- `Prompt`：让模型在特定任务里复用一致的交互模板

这意味着你不是在暴露“单个函数”，而是在暴露一套**可被模型消费的能力系统**。

比如在一个代码助手场景里：

- Tool 可以是“读取 PR 数据”“执行静态检查”“触发构建”
- Resource 可以是“项目规范”“接口文档”“最近构建日志”
- Prompt 可以是“代码审查模板”“回归分析模板”“发布前检查模板”

当你同时具备这三种能力时，MCP 服务才真正从“工具接口”变成“上下文能力层”。

下面这段示例把三者放在同一个 `MCP Server` 里，比较适合理解它们各自的职责边界：

```go
package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"capability-demo",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, true),
		server.WithPromptCapabilities(true),
	)

	s.AddTool(
		mcp.NewTool("build_project",
			mcp.WithDescription("触发项目构建"),
			mcp.WithString("branch", mcp.Required(), mcp.Description("要构建的分支")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			branch, err := req.RequireString("branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return mcp.NewToolResultText("已触发构建，分支：" + branch), nil
		},
	)

	s.AddResource(
		mcp.NewResource(
			"docs://engineering/conventions",
			"工程规范",
			mcp.WithMIMEType("text/markdown"),
			mcp.WithResourceDescription("团队统一的 Go 工程规范"),
		),
		func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					URI:      "docs://engineering/conventions",
					MIMEType: "text/markdown",
					Text:     "# Go 工程规范\n\n- service 不直接依赖 handler\n- 错误统一包装\n",
				},
			}, nil
		},
	)

	s.AddPrompt(
		mcp.NewPrompt("code_review",
			mcp.WithPromptDescription("代码审查提示词模板"),
			mcp.WithArgument("language", mcp.ArgumentDescription("编程语言"), mcp.RequiredArgument()),
		),
		func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			lang := req.Params.Arguments["language"]
			if lang == "" {
				lang = "go"
			}

			return mcp.NewGetPromptResult(
				"代码审查模板",
				[]mcp.PromptMessage{
					mcp.NewPromptMessage(
						mcp.RoleUser,
						mcp.NewTextContent(fmt.Sprintf("请按 %s 项目的最佳实践审查以下代码。", lang)),
					),
				},
			), nil
		},
	)

	_ = s
}
```

### 2. 服务端与客户端

这是 `mcp-go` 很容易被低估的一点。

很多人提到 MCP SDK，只会想到“我怎么用它暴露一个服务”。但在真实系统里，客户端能力同样重要。你可能需要：

- 在 Go 程序里连接别人的 MCP Server
- 动态发现对方提供了哪些工具和资源
- 代替 LLM Host 主动发起调用
- 在测试里用 in-process 方式直接连本地 Server

`mcp-go` 官方文档明确把客户端建设列成独立模块，而且给了 `stdio`、`StreamableHTTP`、`SSE`、`In-Process` 几类客户端方式。

这意味着你既可以用它“提供能力”，也就是Server，也可以用它“消费能力”，就是Client。

如果你想快速理解这件事，最简单的方式就是直接用 `In-Process` client 在同一个进程里调用本地 server：

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"in-process-demo",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(
		mcp.NewTool("hello",
			mcp.WithDescription("返回问候语"),
			mcp.WithString("name", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText("Hello, " + name), nil
		},
	)

	c, err := client.NewInProcessClient(s)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		log.Fatal(err)
	}
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		log.Fatal(err)
	}

	tools, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("发现工具数量：%d\n", len(tools.Tools))

	result, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolRequestParams{
			Name: "hello",
			Arguments: map[string]any{
				"name": "mcp-go",
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("工具返回：%+v\n", result.Content)
}
```

### 3. 传输层

MCP 不是只跑在一种环境里。本地桌面工具、CLI 插件、云端服务、浏览器侧应用，它们适合的传输方式完全不同。

`mcp-go` 当前支持的几种方式基本覆盖了常见部署路径：

- `stdio`：最适合本地工具、CLI 集成、单客户端场景
- `StreamableHTTP`：更适合服务化部署、网关接入、负载均衡
- `SSE`：更适合实时通知和长连接式交互
- `In-Process`：很适合测试、嵌入式集成和本地组合

这里最关键的不是“支持得多”，而是它让你可以在**同一套能力定义**上切换不同传输方式，而不是每种传输重写一套服务逻辑。

这对 Go 工程很重要，因为 Go 项目天然喜欢把核心逻辑和外层接入层解耦。`mcp-go` 这个设计方向是顺着 Go 开发者直觉走的。

这一点可以直接体现在启动代码上。能力定义不动，只切换 transport：

```go
package main

import (
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := buildServer()

	switch os.Getenv("MCP_TRANSPORT") {
	case "http":
		httpServer := server.NewStreamableHTTPServer(s)
		log.Fatal(httpServer.Start(":8080"))
	case "sse":
		sseServer := server.NewSSEServer(s)
		log.Fatal(sseServer.Start(":8080"))
	default:
		log.Fatal(server.ServeStdio(s))
	}
}

func buildServer() *server.MCPServer {
	return server.NewMCPServer(
		"transport-demo",
		"1.0.0",
		server.WithToolCapabilities(true),
	)
}
```

### 4. 扩展能力

如果一个 SDK 只能跑 hello world，它通常在暴露问题时就很难发现并定位问题。而 `mcp-go` 真正让我觉得“它不是停留在展示层”的，是下面这些能力。

先看一个总览式写法。你会发现这些扩展能力本质上都围绕 `server.NewMCPServer(...)` 的 option 组合展开：

```go
hooks := &server.Hooks{}
hooks.AddBeforeAny(func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
	log.Printf("method=%s id=%v", method, id)
})

s := server.NewMCPServer(
	"advanced-demo",
	"1.0.0",
	server.WithToolCapabilities(true),
	server.WithTaskCapabilities(true, true, true),
	server.WithHooks(hooks),
	server.WithToolHandlerMiddleware(loggingMiddleware),
	server.WithRecovery(),
	server.WithMaxConcurrentTasks(8),
)
```

#### 1. Session Management 会话管理

官方文档里专门给了 Session Management 的章节，支持：

- 为不同客户端维护独立状态
- 注册和跟踪 session
- 向特定客户端发送通知
- 按 session 定制工具

这个能力在实际场景里非常关键。

比如你做一个企业内部助手，不同用户看到的工具权限、资源范围、租户数据都可能不同。如果没有 session 维度，你最后只能把权限判断全部塞进 handler；这样当然能写，但会越写越乱。

而支持“按 session 挂工具”“按 session 发通知”，就说明这个库已经意识到：MCP 不只是工具调用，还有连接上下文和用户边界的问题。

下面这个示例演示了两件事：注册一个自定义 session，以及给特定 session 挂一把专属工具：

```go
package main

import (
	"context"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type MySession struct {
	id           string
	notifChannel chan mcp.JSONRPCNotification
	initialized  bool
	tools        map[string]server.ServerTool
}

func (s *MySession) SessionID() string { return s.id }
func (s *MySession) NotificationChannel() chan<- mcp.JSONRPCNotification { return s.notifChannel }
func (s *MySession) Initialize() { s.initialized = true }
func (s *MySession) Initialized() bool { return s.initialized }
func (s *MySession) GetSessionTools() map[string]server.ServerTool { return s.tools }
func (s *MySession) SetSessionTools(tools map[string]server.ServerTool) { s.tools = tools }

func main() {
	s := server.NewMCPServer(
		"session-demo",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	session := &MySession{
		id:           "user-123",
		notifChannel: make(chan mcp.JSONRPCNotification, 8),
		tools:        map[string]server.ServerTool{},
	}

	if err := s.RegisterSession(context.Background(), session); err != nil {
		log.Fatal(err)
	}

	userTool := mcp.NewTool("user_profile",
		mcp.WithDescription("读取当前用户的资料"),
	)

	if err := s.AddSessionTool(
		session.SessionID(),
		userTool,
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("只对 user-123 可见的资料"), nil
		},
	); err != nil {
		log.Fatal(err)
	}

	if err := s.SendNotificationToSpecificClient(
		session.SessionID(),
		"notification/update",
		map[string]any{"message": "你的私有工具已经就绪"},
	); err != nil {
		log.Fatal(err)
	}
}
```

#### 2. 长任务工具

MCP Tool 并不总是瞬时返回。

真实系统里经常有这种调用：

- 跑批处理
- 调用外部 API 并等待
- 生成文件
- 发起异步分析任务
- 执行长时间扫描

`mcp-go` 对这类场景提供了 `task-augmented tools`。从官方 README 来看，它支持：

- 禁止任务模式
- 可选同步 / 异步双模式
- 强制异步任务模式

并且支持：

- 立即返回任务 ID
- 后台执行
- 轮询结果
- 任务完成通知
- 并发任务数量限制

这件事的工程意义很大。因为很多 demo 型 SDK 一旦遇到长任务，就会让你自己在外面另起一套任务系统；而 `mcp-go` 至少已经把协议层和任务交互的入口铺好了。

一个典型写法是把长耗时处理注册成 `AddTaskTool`，并开启 task capabilities：

```go
package main

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"task-demo",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithTaskCapabilities(true, true, true),
		server.WithMaxConcurrentTasks(10),
	)

	processTool := mcp.NewTool(
		"process_batch",
		mcp.WithDescription("异步处理一批文件"),
		mcp.WithTaskSupport(mcp.TaskSupportRequired),
		mcp.WithArray("files", mcp.Required(), mcp.WithStringItems()),
	)

	s.AddTaskTool(processTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CreateTaskResult, error) {
		files := req.GetStringSlice("files", []string{})

		for _, file := range files {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				_ = file
				time.Sleep(500 * time.Millisecond)
			}
		}

		return &mcp.CreateTaskResult{
			Task: mcp.Task{},
		}, nil
	})

	_ = s
}
```

#### 3. Hooks 和 Middleware 的治理能力

一个库能不能进生产，很多时候不是看它能不能“完成功能”，而是看它能不能“被治理”。

`mcp-go` 的文档里给了 request hooks、lifecycle hooks、tool handler middleware 这些机制。对工程团队来说，这意味着你可以比较优雅地挂这些横切能力：

- 日志
- 审计
- 指标
- 权限校验
- tracing
- panic recovery
- 请求前后观测

这才是 SDK 从“可玩”走向“可管”的分水岭。

这一块比较实用的组合是：`Hooks` 负责观测请求生命周期，`Middleware` 负责包装 tool handler：

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func loggingMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		result, err := next(ctx, req)
		log.Printf("tool=%s cost=%s err=%v", req.Params.Name, time.Since(start), err)
		return result, err
	}
}

func main() {
	hooks := &server.Hooks{}
	hooks.AddBeforeAny(func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
		log.Printf("收到请求 method=%s id=%v", method, id)
	})
	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		log.Printf("请求失败 method=%s id=%v err=%v", method, id, err)
	})

	s := server.NewMCPServer(
		"governance-demo",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(hooks),
		server.WithToolHandlerMiddleware(loggingMiddleware),
		server.WithRecovery(),
	)

	s.AddTool(
		mcp.NewTool("ping", mcp.WithDescription("健康检查")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("pong"), nil
		},
	)

	_ = s
}
```

## 三、自我想法

`mcp-go` 的设计我觉得比较稳的一点是：它提供了高层抽象，但没有强行制造太多神秘机制。

比如构建一个 Tool，大体就是：

1. 创建一个 `MCP Server`
2. 定义工具名、描述、参数 Schema
3. 给它挂一个 Go handler
4. 启动对应 transport

这个流程的搭建非常直观。下面是一个基于官方 API 风格改写的极简示例：

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"blog-demo",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	sumTool := mcp.NewTool(
		"sum_numbers",
		mcp.WithDescription("对两个数字求和"),
		mcp.WithNumber("a", mcp.Required(), mcp.Description("第一个数字")),
		mcp.WithNumber("b", mcp.Required(), mcp.Description("第二个数字")),
	)

	s.AddTool(sumTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a, err := req.RequireFloat("a")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		b, err := req.RequireFloat("b")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("结果是：%.2f", a+b)), nil
	})

	if err := server.ServeStdio(s); err != nil {
		log.Fatal(err)
	}
}
```

这个例子最重要的不是“它很短”，而是它把 MCP Server 的核心思路表达得很清楚：

- 能力定义和 handler 很贴近
- 参数是显式 schema，不是随手塞 `map[string]any`
- 错误处理仍然是 Go 风格
- transport 和业务能力没有缠在一起

## 四、适合场景

我觉得 `mcp-go` 最适合下面几类项目。

### 1. 你要快速把已有 Go 服务能力暴露成 MCP

这是最直接的场景。

如果你的系统里已经有很多现成能力，比如：

- 文件处理
- 数据查询
- 内部 API 封装
- 工作流触发
- 代码分析
- 企业内部服务编排

那么 `mcp-go` 很适合作为一层 MCP 暴露层，直接把已有逻辑包装成 Tool / Resource / Prompt。

### 2. 你要在 Go 里写自己的 MCP Host 或桥接程序

因为它连 client 侧也做了，所以你完全可以拿它来做：

- MCP 聚合器
- 多服务桥接器
- 测试用 Host
- 内部 Agent Runtime 的连接层

### 3. 你需要的不只是 demo，而是可运维的 MCP 服务

如果你的诉求包括：

- 错误恢复
- 会话隔离
- 钩子埋点
- 中间件治理
- 异步长任务
- HTTP / SSE / stdio 多种接入方式

那 `mcp-go` 明显比“自己手搓一个 JSON-RPC 服务”更现实。

## 五、注意事项

一篇认真的技术文章，不该只讲优点。

从官方 README 和文档的表述来看，`mcp-go` 仍然有几个边界值得提前说明。

### 1. MCP 规范本身就在持续演进

README 里明确提到：项目仍在活跃开发，MCP 规范本身也在变化。

这意味着什么？

意味着你在选型时，要把它当成一个**高速演进中的协议生态组件**，而不是一个十年不动的稳定底层库。好消息是它当前已经支持 **2025-11-25** 版规范，并对 **2025-06-18、2025-03-26、2024-11-05** 保持向后兼容；坏消息是，未来升级时你仍然需要关注 release note。

### 2. SDK 解决的是协议层，不是你的领域建模

`mcp-go` 能帮你优雅地暴露工具和资源，但它不会替你做好这些事情：

- 权限模型
- 多租户边界
- 业务幂等
- 任务补偿
- 领域错误分类
- 外部依赖限流

如果这些问题本身没设计好，再好的 MCP SDK 也只是把混乱包装得更体面一点。

### 3. 数量问题

这类库一旦用顺手，很容易出现一个副作用：团队开始把任何内部能力都想暴露成 MCP Tool。

真正适合 MCP 的，通常是：

- 对模型有明确价值的能力
- 参数边界清晰的能力
- 可审计、可解释、可治理的能力

这一点可以看`langchain`的介绍，我记得说过tool的封装不要太多，最多是30个左右，但是实践中10个一下是最好的。

因为`mcp`的信息本身还是会存在在上下文信息中给`LLM`调用

## 参考资料

- 官方仓库：<https://github.com/mark3labs/mcp-go>
- 官方文档 Getting Started：<https://mcp-go.dev/getting-started/>
- 官方文档 Building MCP Clients：<https://mcp-go.dev/clients/>
- GitHub Releases：<https://github.com/mark3labs/mcp-go/releases>
