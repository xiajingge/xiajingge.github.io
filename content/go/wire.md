+++

date = '2025-02-09T16:42:35+08:00'
title = "Go Wire 依赖注入"
tags = ["go", "wire"]
categories = ["go","后端开发"]

+++

# Go Wire 依赖注入

今天要学习的内容是依赖注入，主要的目的就是简化main文件的内容，以及避免过多的全局变量带来的维护困难。

对于main文件而言，在小项目里，手动 new 结构体、逐层传依赖，完全够用；但一旦项目从 `handler -> service -> repository -> db/cache/config/logger` 逐渐扩展，初始化代码就会开始失控。你会发现：

- `main.go` 越来越长，启动代码像一条没有尽头的装配流水线
- 依赖关系只能靠人脑维护，改一个构造函数，常常要改一串调用点
- 测试时想替换一个实现，初始化过程变得很笨重
- 某个依赖漏初始化、顺序不对、错误没透传，排查起来很痛苦

这时候，依赖注入的价值才真正体现出来。它不是为了“炫技”，而是为了把**对象创建**和**业务逻辑**拆开，让依赖关系变得显式、稳定、可维护。

而在 Go 生态里，`Wire` 是一个非常典型的选择。

## 一、什么是 Wire

`Wire` 是一个 **编译期依赖注入工具**。它不靠运行时反射去动态组装对象，而是在编译前根据你声明的 provider 关系，生成一份普通 Go 代码。

Wire 走的是：**你写依赖规则，Wire 帮你生成初始化代码，但最后运行的仍然是普通 Go 代码。**

所以它的核心优势不是“神奇”，而是：

- 依赖关系清晰
- 编译期发现装配错误
- 不引入运行时容器
- 生成代码可读、可调试
- 保持 Go 风格的显式与简单

你可以把它理解成一句话：

> Wire 不是帮你“隐藏依赖”，而是帮你“自动生成手写装配代码”。

## 二、先看没有 Wire 时的问题

假设我们有一个很常见的 Web 服务：

- `Config` 负责配置
- `DB` 负责数据库连接
- `UserRepository` 依赖 `DB`
- `UserService` 依赖 `UserRepository`
- `UserHandler` 依赖 `UserService`

手写初始化大概是这样：

```go
func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	db, err := NewDB(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	userRepo := NewUserRepository(db)
	userService := NewUserService(userRepo)
	userHandler := NewUserHandler(userService)

	server := NewHTTPServer(userHandler)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
```

这段代码在小项目里没问题，但随着系统扩大，很快会出现几个现象：

1. `main.go` 变成“启动垃圾场”，所有依赖都堆在一起。
2. 构造函数一旦变化，入口代码要跟着改。
3. 你很难把“模块装配”沉淀成统一规范。
4. 多套环境、测试替身、不同实现切换时，初始化逻辑会越来越丑。

本质上，问题不是“new 太多”，而是**依赖图越来越复杂，但你仍在靠手工维护整张图**。

## 三、Wire 的核心概念

想用好 Wire，只需要先理解 3 个核心概念。

### 1. Provider

Provider 就是“构造依赖的函数”。例如：

```go
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func NewUserService(repo *UserRepository) *UserService {
	return &UserService{repo: repo}
}
```

Wire 会根据函数的入参与返回值，推导依赖关系。

### 2. Provider Set

Provider Set 是一组 provider 的集合，用来描述某个模块的装配能力。

```go
var UserSet = wire.NewSet(
	NewUserRepository,
	NewUserService,
	NewUserHandler,
)
```

它的意义不是“凑一起”，而是把依赖组织成可复用模块。大型项目里，通常会按 `infra`、`repository`、`service`、`transport` 去拆 set。

### 3. Injector

Injector 是最终入口。你告诉 Wire：**我想要什么对象**，Wire 根据 provider 图生成对应的初始化代码。

```go
func InitUserHandler() (*UserHandler, error) {
	wire.Build(UserSet, DBSet, ConfigSet)
	return nil, nil
}
```

这里返回的 `nil, nil` 不是真正逻辑，它只是告诉 Wire 最终签名长什么样。执行 `wire` 命令后，会生成真正的装配代码。

## 四、一个完整示例：用 Wire 装配 HTTP 服务

下面给一个稍微完整一点的例子。假设我们有如下目录：

```text
.
├── cmd/server
│   ├── main.go
│   ├── wire.go
│   └── wire_gen.go
├── internal/config
├── internal/repository
├── internal/service
└── internal/transport/http
```

### 1. 定义配置与基础设施

```go
package config

type Config struct {
	DSN string
}

func Load() (*Config, error) {
	return &Config{
		DSN: "root:root@tcp(127.0.0.1:3306)/demo",
	}, nil
}
```

```go
package infra

import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
)

func NewDB(cfg *config.Config) (*sql.DB, func(), error) {
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		_ = db.Close()
	}

	return db, cleanup, nil
}
```

这里有一个很实用的点：provider 可以返回 `func()` 作为清理函数，Wire 会把多个 cleanup 自动串起来。

### 2. 定义业务层

```go
package repository

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}
```

```go
package service

type UserService struct {
	repo *repository.UserRepository
}

func NewUserService(repo *repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}
```

```go
package http

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}
```

### 3. 定义 Provider Set

```go
package provider

var ConfigSet = wire.NewSet(config.Load)

var InfraSet = wire.NewSet(infra.NewDB)

var UserSet = wire.NewSet(
	repository.NewUserRepository,
	service.NewUserService,
	http.NewUserHandler,
)
```

### 4. 编写 injector

`wire.go` 一般这样写：

```go
//go:build wireinject

package main

import "github.com/google/wire"

func InitUserHandler() (*http.UserHandler, func(), error) {
	wire.Build(
		provider.ConfigSet,
		provider.InfraSet,
		provider.UserSet,
	)
	return nil, nil, nil
}
```

这里的几个点要注意：

- `//go:build wireinject` 用于避免这份“占位 injector”参与正常编译
- `wire.Build(...)` 里放的是装配依赖所需的 provider 集合
- 返回值签名里如果包含 cleanup 和 error，Wire 会自动生成对应链路

### 5. 生成代码

安装命令：

```bash
go install github.com/google/wire/cmd/wire@latest
```

然后在对应目录执行：

```bash
wire
```

执行后会生成 `wire_gen.go`，内容类似下面这样：

```go
func InitUserHandler() (*http.UserHandler, func(), error) {
	configConfig, err := config.Load()
	if err != nil {
		return nil, nil, err
	}

	db, cleanup, err := infra.NewDB(configConfig)
	if err != nil {
		return nil, nil, err
	}

	userRepository := repository.NewUserRepository(db)
	userService := service.NewUserService(userRepository)
	userHandler := http.NewUserHandler(userService)

	return userHandler, func() {
		cleanup()
	}, nil
}
```

你会发现，Wire 生成的其实就是你本来会手写的代码，只是它帮你完成了“依赖推导”和“装配生成”。

这也是为什么很多 Go 开发者能接受它：**它没有把程序变魔法，只是把重复劳动自动化了。**

## 五、Wire 最值得掌握的几个能力

Wire 不只是“自动 new 对象”，它还有几个在实际项目里非常常用的能力。

### 1. 接口绑定

如果你的 service 依赖的是接口，而不是具体实现，可以用 `wire.Bind`：

```go
type UserRepo interface {
	FindByID(ctx context.Context, id int64) (*User, error)
}

type MySQLUserRepo struct {
	db *sql.DB
}

func NewMySQLUserRepo(db *sql.DB) *MySQLUserRepo {
	return &MySQLUserRepo{db: db}
}

var RepoSet = wire.NewSet(
	NewMySQLUserRepo,
	wire.Bind(new(UserRepo), new(*MySQLUserRepo)),
)
```

这在做测试替换、多实现切换时特别有用。

### 2. 注入结构体字段

如果某个结构体就是简单聚合，也可以用 `wire.Struct`：

```go
type Application struct {
	UserHandler *http.UserHandler
	Logger      *zap.Logger
}

var AppSet = wire.NewSet(
	provider.UserSet,
	NewLogger,
	wire.Struct(new(Application), "*"),
)
```

不过我个人建议谨慎使用。`wire.Struct` 很方便，但滥用后容易让依赖来源不够直观。对核心链路，显式构造函数通常更稳。

### 3. 常量值注入

有些值不是函数产物，而是静态值，可以用 `wire.Value`：

```go
var AppNameSet = wire.NewSet(
	wire.Value("user-service"),
)
```

这个场景不算高频，但在少量配置装配里确实有用。

### 4. Cleanup 统一回收

前面 `NewDB` 返回 `(db, cleanup, error)` 就属于这一类。对于数据库、消息队列、文件句柄、外部客户端，这种模式非常适合。

Wire 能把 cleanup 自动汇总，避免资源释放漏掉。

## 六、项目里该怎么组织 Wire

其实看上面的内容之后，就是猜出来，最容易犯的错就是：**把整个项目所有 provider 全塞进一个大 set 里。**

这样短期看省事，长期一定失控。

更推荐的做法是按层或按模块拆分：

- `ConfigSet`：配置相关
- `InfraSet`：数据库、Redis、Logger、MQ
- `RepoSet`：仓储层
- `ServiceSet`：业务层
- `HandlerSet`：接口层
- `AppSet`：最终应用聚合

这样做有几个好处：

1. 每个模块边界清晰。
2. 依赖问题更容易定位。
3. 某个模块替换实现时影响范围更小。
4. 测试环境可以重组不同的 set。

我比较推荐一个实践：**让 Wire 只负责装配，不负责业务判断。**

也就是说：

- provider 里可以做初始化
- 可以返回错误
- 可以做必要的资源创建
- 但不要把复杂业务逻辑塞进 provider

一旦 provider 里出现大量条件分支、业务策略、环境判断，DI 本身就开始变味了。

## 七、常见坑与经验总结

### 1. 以为 Wire 能解决“设计问题”

不能。

Wire 只能帮你管理依赖装配，不能帮你设计好模块边界。如果你的包结构混乱、接口设计拧巴、依赖循环严重，引入 Wire 只会更早暴露问题，不会自动治好问题。

### 2. Provider 粒度过细

有些人会把每一个字段、每一个小对象都拆成 provider，结果 provider 数量爆炸，维护成本反而更高。

原则是：**只为真正值得复用、值得组合、值得替换的构造过程定义 provider。**

### 3. 过度依赖接口

不是所有层都必须抽象成接口。Go 里接口应该长在“使用方”这一侧，而不是为了 DI 强行制造。

如果一个实现短期内不会替换，直接依赖具体类型通常更简单。

### 4. 忘记生成代码

团队协作里，这个问题非常常见：改了 provider，忘了重新执行 `wire`，导致本地和 CI 行为不一致。

一个常见做法是在 `wire.go` 里加：

```go
//go:generate wire
```

然后通过：

```bash
go generate ./...
```

统一生成。

### 5. 把 `wire_gen.go` 当黑盒

不要这样做。Wire 生成的是普通 Go 代码，出了问题就直接看生成结果。很多时候，读一遍 `wire_gen.go`，你马上就知道依赖链哪里断了。
