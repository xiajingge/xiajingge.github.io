+++
date = '2026-03-11T21:24:45+08:00'
title = "Eino源码学习(二)--ChatModel和ChatTemplate"
tags = ["Agent","AI"]
categories = ["AI"]
+++

## ChatModel概念

ChatModel 是 Eino 中用于与大语言模型交互的核心组件。它提供了统一的接口来调用不同的大模型服务（如 OpenAI、Claude、ARK 等）。抽离出统一的接口进行模型的调用，这里就体现出框架所带来的一大优势。

使用场景：

* 自然语言对话

* 文本生成和补全

* 工具调用的参数生成

* 多模态交互（文本、图片、音频等）

## 组件定义

### 接口定义

> 代码位置：eino/components/model/interface.go

```go
type BaseChatModel interface {
    Generate(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.Message, error)
    Stream(ctx context.Context, input []*schema.Message, opts ...Option) (
        *schema.StreamReader[*schema.Message], error)
}
type ChatModel interface {
    BaseChatModel

    // BindTools bind tools to the model.
    // BindTools before requesting ChatModel generally.
    // notice the non-atomic problem of BindTools and Generate.
    BindTools(tools []*schema.ToolInfo) error
}
// ToolCallingChatModel extends BaseChatModel with tool calling capabilities.
type ToolCallingChatModel interface {
    BaseChatModel

    // WithTools returns a new ToolCallingChatModel instance with the specified tools bound.
    // This method does not modify the current instance, making it safer for concurrent use.
    WithTools(tools []*schema.ToolInfo) (ToolCallingChatModel, error)
}
```

这两个接口 `ChatModel` 和 `ToolCallingChatModel` 都扩展了 `BaseChatModel`，用于支持工具调用（Tool Calling/Function Calling），但它们在**设计模式、并发安全性和使用方式**上有区别。

以下是核心差异对比：

1. 状态管理 vs 不可变性 (Mutability vs Immutability)
*   **`ChatModel` (可变/有状态):**
    *   `BindTools` 直接修改当前实例的内部状态。
    *   这是一个 **Mutation** 操作。一旦调用，该模型实例后续的所有 `Generate/Stream` 请求都会携带这些工具。
*   **`ToolCallingChatModel` (不可变/无状态):**
    *   `WithTools` **不修改**当前实例，而是返回一个绑定了工具的**新实例**。
    *   这是一个 **Functional/Immutable** 操作。原始模型实例保持不变，可以安全地被多个 goroutine 共享作为“基础模板”。

2. 并发安全性 (Concurrency Safety)
*   **`ChatModel`:**  **不安全**
    *   正如注释中提到的 *"notice the non-atomic problem of BindTools and Generate"*，如果在多线程环境下，一个 goroutine 调用 `BindTools` 的同时另一个 goroutine 调用 `Generate`，会导致竞态条件（Race Condition）。
    *   使用时通常需要加锁，或者确保每个请求使用独立的模型实例。
*   **`ToolCallingChatModel`:**  **安全**
    *   注释明确说明 *"making it safer for concurrent use"*。
    *   全局共享一个基础的 `ToolCallingChatModel`，在每个请求处理函数中通过 `WithTools` 派生出带特定工具的临时实例，无需任何锁机制。

3. API 风格与链式调用
*   **`ChatModel`:** 命令式风格
    ```go
    model.BindTools(tools)      // 先绑定
    resp, err := model.Generate(ctx, msgs) // 再调用
    ```
*   **`ToolCallingChatModel`:** 声明式/链式风格
    ```go
    // 一行完成，原始 model 不受影响
    resp, err := model.WithTools(tools).Generate(ctx, msgs) 
    ```


> 在新版本中， ChatModel 目前正在逐渐废弃，后续统一用 ToolCallingChatModel 来代替。

### 公共Option

Model 组件提供了一组公共 Option 用于配置模型行为：

> 代码位置：eino/components/model/option.go

```go
type Options struct {
    // Temperature 控制输出的随机性
    Temperature *float32
    // MaxTokens 控制生成的最大 token 数量
    MaxTokens *int
    // Model 指定使用的模型名称
    Model *string
    // TopP 控制输出的多样性
    TopP *float32
    // Stop 指定停止生成的条件
    Stop []string
}
```

可以通过以下方式设置 Option：

```go
// 设置温度
WithTemperature(temperature float32) Option

// 设置最大 token 数
WithMaxTokens(maxTokens int) Option

// 设置模型名称
WithModel(name string) Option

// 设置 top_p 值
WithTopP(topP float32) Option

// 设置停止词
WithStop(stop []string) Option
```

## 实现案例

[Eino: ChatModel 使用说明](https://cloudwego.cn/zh/docs/eino/core_modules/components/chat_model_guide)官方文档中给出了不少关于ChatModel模块组件中的使用案例，由于官方内容很详细，这里就不再过多进行赘述，而是拿一个完整的代码事例对上述内容进行详细的解释和总结，以便读者理解。

```go
import (
    "context"
    "errors"
    "net/http"
    "time"

    "github.com/cloudwego/eino/callbacks"
    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/schema"
)

// 定义 ChatModel 模块
type MyChatModel struct {
    client     *http.Client
    apiKey     string
    baseURL    string
    model      string
    timeout    time.Duration
    retryCount int
}

// 定义 ChatModel 模块配置
type MyChatModelConfig struct {
    APIKey string
}

// 初始化 ChatModel 模块
func NewMyChatModel(config *MyChatModelConfig) (*MyChatModel, error) {
    if config.APIKey == "" {
       return nil, errors.New("api key is required")
    }

    return &MyChatModel{
       client: &http.Client{},
       apiKey: config.APIKey,
    }, nil
}

// 包含 Options、callback、Stream、WithTools 具体实践
func (m *MyChatModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
    // 1. 处理选项
    options := &MyChatModelOptions{
       Options: &model.Options{
          Model: &m.model,
       },
       RetryCount: m.retryCount,
       Timeout:    m.timeout,
    }
    options.Options = model.GetCommonOptions(options.Options, opts...)
    options = model.GetImplSpecificOptions(options, opts...)

    // 2. 开始生成前的回调
    ctx = callbacks.OnStart(ctx, &model.CallbackInput{
       Messages: messages,
       Config: &model.Config{
          Model: *options.Options.Model,
       },
    })

    // 3. 执行生成逻辑
    response, err := m.doGenerate(ctx, messages, options)

    // 4. 处理错误和完成回调
    if err != nil {
       ctx = callbacks.OnError(ctx, err)
       return nil, err
    }

    ctx = callbacks.OnEnd(ctx, &model.CallbackOutput{
       Message: response,
    })

    return response, nil
}

// 流式输出
func (m *MyChatModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
    // 1. 处理选项
    options := &MyChatModelOptions{
       Options: &model.Options{
          Model: &m.model,
       },
       RetryCount: m.retryCount,
       Timeout:    m.timeout,
    }
    options.Options = model.GetCommonOptions(options.Options, opts...)
    options = model.GetImplSpecificOptions(options, opts...)

    // 2. 开始流式生成前的回调
    ctx = callbacks.OnStart(ctx, &model.CallbackInput{
       Messages: messages,
       Config: &model.Config{
          Model: *options.Options.Model,
       },
    })

    // 3. 创建流式响应
    // Pipe产生一个StreamReader和一个StreamWrite，向StreamWrite中写入可以从StreamReader中读到，二者并发安全。
    // 实现中异步向StreamWrite中写入生成内容，返回StreamReader作为返回值
    // ***StreamReader是一个数据流，仅可读一次，组件自行实现Callback时，既需要通过OnEndWithCallbackOutput向callback传递数据流，也需要向返回一个数据流，需要对数据流进行一次拷贝
    // 考虑到此种情形总是需要拷贝数据流，OnEndWithCallbackOutput函数会在内部拷贝并返回一个未被读取的流
    // 以下代码演示了一种流处理方式，处理方式不唯一
    sr, sw := schema.Pipe[*model.CallbackOutput](1)

    // 4. 启动异步生成
    go func() {
       defer sw.Close()

       // 流式写入
       m.doStream(ctx, messages, options, sw)
    }()

    // 5. 完成回调
    _, nsr := callbacks.OnEndWithStreamOutput(ctx, sr)

    return schema.StreamReaderWithConvert(nsr, func(t *model.CallbackOutput) (*schema.Message, error) {
       return t.Message, nil
    }), nil
}

// 工具绑定、参数配置
func (m *MyChatModel)  WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
    // 实现工具绑定逻辑
    return nil, nil
}

func (m *MyChatModel) doGenerate(ctx context.Context, messages []*schema.Message, opts *MyChatModelOptions) (*schema.Message, error) {
    // 实现生成逻辑
    return nil, nil
}

func (m *MyChatModel) doStream(ctx context.Context, messages []*schema.Message, opts *MyChatModelOptions, sr *schema.StreamWriter[*model.CallbackOutput]) {
    // 流式生成文本写入sr中
    return
}
```

## ChatTemplate概念
ChatTemplate 是 Eino 中用于管理和构建提示词（Prompt）的组件。它提供了变量替换、消息模板化等功能，让 Prompt 管理更加规范和可维护。

使用场景：

* 构建结构化的系统提示。

* 处理多轮对话的模板 (包括 history)。

* 实现可复用的提示模式。

## 组件定义

### 接口定义

> 代码位置：eino/components/prompt/interface.go

```go
type ChatTemplate interface {
    Format(ctx context.Context, vs map[string]any, opts ...Option) ([]*schema.Message, error)
}
```

* `prompt.FromMessages()`

  * 用于把多个 message 变成一个 chat template。

* `schema.Message{}`

  * schema.Message 是实现了 Format 接口的结构体，因此可直接构建 `schema.Message{}` 作为 template

* `schema.SystemMessage()`

  * 此方法是构建 role 为 “system” 的 message 快捷方法

* `schema.AssistantMessage()`

  * 此方法是构建 role 为 “assistant” 的 message 快捷方法

* `schema.UserMessage()`

  * 此方法是构建 role 为 “user” 的 message 快捷方法

* `schema.ToolMessage()`

  * 此方法是构建 role 为 “tool” 的 message 快捷方法

* `schema.MessagesPlaceholder()`

  * 可用于把一个 `[]*schema.Message` 插入到 message 列表中，常用于插入历史对话

## 使用方式

ChatTemplate 支持多种模板化方式，最常见的包括：

1. FString 格式：使用 `{variable}` 语法进行变量替换

2. GoTemplate 格式：使用 Go 标准库的 text/template 语法，支持条件判断、循环等复杂逻辑

3. Jinja2 格式：使用 Jinja2 模板语法

基本使用思路：

1. 使用prompt.FromMessages以及提供的schema.FString格式化

2. 构造params参数

3. 使用template自带的Format构建即可

ChatTemplate 一般用于 ChatModel 之前做上下文准备的

### FString

```go
func main() {
        ctx := context.Background()
        apiKey := os.Getenv("DEEPSEEK_API_KEY")
        if apiKey == "" {
                log.Fatal("缺少环境变量 DEEPSEEK_API_KEY")
        }
        model, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
                APIKey:  apiKey,
                Model:   "deepseek-chat",
                BaseURL: "https://api.deepseek.com",
        })
        if err != nil {
                panic(err)
        }
        template := prompt.FromMessages(
                schema.FString,
                schema.SystemMessage("你是一个{role}，请用{tone}的语气回答问题。"),
                schema.UserMessage("{question}"),
        )
        vars := map[string]any{
                "role":     "篮球体能教练",
                "tone":     "专业但易懂",
                "question": "我每周打球2次，想提升第一步爆发和上篮终结，给我一周训练计划。",
        }
        messages, err := template.Format(ctx, vars)
        if err != nil {
                panic(err)
        }
        stream, err := model.Stream(ctx, messages)
        if err != nil {
                panic(err)
        }
        defer stream.Close()
        for {
                chunk, err := stream.Recv()
                if err != nil {
                        break
                }
                print(chunk.Content)
        }
        println()
}
```

### GoTemplate

```go
func main() {
        ctx := context.Background()
        apiKey := os.Getenv("DEEPSEEK_API_KEY")
        if apiKey == "" {
                log.Fatal("缺少环境变量 DEEPSEEK_API_KEY")
        }
        model, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
                APIKey:  apiKey,
                Model:   "deepseek-chat",
                BaseURL: "https://api.deepseek.com",
        })
        if err != nil {
                panic(err)
        }
        template := prompt.FromMessages(
                schema.GoTemplate,
                schema.SystemMessage(`{{if .isExpert}}你是一个专家级{{.domain}}教练。{{else}}你是一个入门级{{.domain}}助教。{{end}}
{{if .isFormal}}请使用正式、结构化的语言风格。{{else}}请使用轻松、友好的语言风格。{{end}}
你的任务是：{{.task}}`),
                schema.UserMessage("{{.question}}"),
        )
        vars := map[string]any{
                "isExpert": false,
                "domain":   "篮球基础",
                "isFormal": false,
                "task":     "帮助初学者理解基本概念并给出练习方法",
                "question": "什么是“挡拆”（Pick and Roll）？我该如何在野球局里用好它？",
        }
        messages, err := template.Format(ctx, vars)
        if err != nil {
                panic(err)
        }
        // 可选：把渲染后的 prompt 打印出来，便于教学
        for _, m := range messages {
                println("[rendered]", m.Role, ":", m.Content)
        }
        stream, err := model.Stream(ctx, messages)
        if err != nil {
                panic(err)
        }
        defer stream.Close()

        for {
                chunk, err := stream.Recv()
                if err != nil {
                        break
                }
                print(chunk.Content)
        }
        println()
}
```

### Jinja2

```go
func main() {
        ctx := context.Background()
        apiKey := os.Getenv("DEEPSEEK_API_KEY")
        if apiKey == "" {
                log.Fatal("缺少环境变量 DEEPSEEK_API_KEY")
        }
        model, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
                APIKey:  apiKey,
                Model:   "deepseek-chat",
                BaseURL: "https://api.deepseek.com",
        })
        if err != nil {
                panic(err)
        }
        template := prompt.FromMessages(
                schema.Jinja2,
                schema.SystemMessage(`{% if level == 'expert' %}你是一名专家级篮球训练顾问。{% else %}你是一名入门篮球助教。{% endif %}
{% if domain %}你的专长方向是：{{ domain }}。{% endif %}
请用{% if formal %}正式{% else %}友好{% endif %}的语气回答，并给出可执行步骤清单。`),
                schema.UserMessage("{{ question }}"),
        )
        vars := map[string]any{
                "level":  "expert",
                "domain": "持球进攻与终结",
                "formal": true,
                "question": "请讲解“欧洲步”的关键细节，给一个30分钟训练流程，并指出最常见的3个错误。",
        }
        messages, err := template.Format(ctx, vars)
        if err != nil {
                panic(err)
        }
        for _, m := range messages {
                println("[rendered]", m.Role, ":", m.Content)
        }
        stream, err := model.Stream(ctx, messages)
        if err != nil {
                panic(err)
        }
        defer stream.Close()
        for {
                chunk, err := stream.Recv()
                if err != nil {
                        break
                }
                print(chunk.Content)
        }
        println()
}
```

### 小结

1. FString 格式 (schema.FString)

   * 使用 `{variable}` 语法进行变量替换

   * 简单直观，适合基础文本替换场景

   * 示例：`"你是一个{role}，请帮我{task}。"`

2. GoTemplate 格式 (schema.GoTemplate)

   * 使用 Go 标准库的 text/template 语法

   * 支持条件判断、循环等复杂逻辑

   * 示例：`"{{if .expert}}作为专家{{end}}请{{.action}}"`

3. Jinja2 格式 (schema.Jinja2)

   * 使用 Jinja2 模板语法

   * 示例：`"{% if level == 'expert' %}以专家的角度{% endif %}分析{{topic}}"`