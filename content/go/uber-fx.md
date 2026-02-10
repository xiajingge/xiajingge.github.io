+++
date = '2026-02-10T22:03:52+08:00'
title = "Uber-fx依赖注入"
tags = [go]
categories = [go]

+++

# fx依赖注入

## 简介

​	fx 是 uber 开源的一款依赖注入框架，依赖注入框架在之前接触过wire，目的是减少系统的启动复杂度，集中管理系统中的init功能函数。小项目引入依赖注入其实就没必要，甚至会凭空提高复杂度。

## 简单的依赖注入讲解

在我们写golang项目时，经常会遇到要使用其他包的对象。例如：

```go
func NewA() TypeA 
func NewB(TypeA) TypeB
func NewC(TypeA, TypeB) TypeC
```

当我们需要一个 TypeC 时， 需要按照顺序手动构造 TypeA， TypeB，然后在构造 TypeC。

使用fx后

```go
func main() {
    var x NewC
    fx.New(
        fx.Provide(NewA, NewB, NewC),
        fx.Populate(&x)
    )
    fmt.Println(x)
}
```

我们将各个构造函数通过 `Provide` 函数注入到 `fx.App` 后，fx就会帮我们管理构造函数的调用，并且这种调用是 lazy的，即当某一个构造函数不存在被依赖时，那么它是不会被调用的。并且，这些构造函数被调用后构造的变量，是会被缓存的，所以当其他函数存在多个对其依赖时，只会被执行一次，之后都将直接返回第一构造的变量。

所以，我们使用 `Provide` 向fx注入构造函数时，注入顺序并不重要。

## 函数签名

这里会列出fx的主要的函数和方法，并对作用做出简要解释。

```go
func New(opts ...Option) *App
func Provide(constructors ...interface{}) Option 
func Supply(values ...interface{}) Option
func Populate(targets ...interface{}) Option
func Invoke(funcs ...interface{}) Option
```

- **New**

  创建一个Fx应用，然后通过该应用进行程序的启动和管理。

- **Provide**

  将各个组件的构造器注册到Fx的应用容器中，Fx会在运行时调用它们并把返回的对象实例管理起来，供后续步骤使用。

- **Supply**

  函数传入的参数是已经构造完毕的值（value），也就是说 `Provide(NewC)` → `Supply(C)`

- **Populate**

  在 `New` 函数外部，我们先var了一个 C，并且通过 `Provide` 注入 C 的构造函数，那么外部通过植入的变量C，在初始化完成后通过 `Provide` 的构造函数完成构造。这样就可以在 `New` 函数外部使用这个经过构造的变量。

  可以把 Fx 想成一个“黑盒容器”,正常情况下：Fx 自己管理生命周期，对象只在 `Invoke` / `Provide` 链路里流转，但是有了Populate，就可以把容器里的对象 **暴露到外部世界**

- **Invoke**

  添加业务执行的函数。这些函数是我的业务逻辑的入口，它们可以接收任何在fx.Provide()提供的类型作为参数，Fx会将这些类型的实例自动注入到函数中，使得在这些函数中无需手工创建依赖对象，直接使用依赖对象进行业务处理

> 需要注意的一点是 Invoke 注册的函数的运行是**有顺序**的，而 Provide 注入的构造函数并没有顺序

## 简单使用

下面是一个官方例子

·

```go
package main

import (
	"context"
	"fmt"
	"go.uber.org/fx"
	"io"
	"net"
	"net/http"
	"os"
)

func main() {
	fx.New(
		fx.Provide(
			NewHTTPServer,
			NewEchoHandler,
			NewServeMux,
		),
		fx.Invoke(func(srv *http.Server) {}),
	).Run()
}

func NewHTTPServer(lc fx.Lifecycle, mux *http.ServeMux) *http.Server {
	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}
			fmt.Println("Starting HTTP server at", srv.Addr)
			go srv.Serve(ln)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
	return srv
}

// EchoHandler is an http.Handler that copies its request body
// back to the response.
type EchoHandler struct{}

// NewEchoHandler builds a new EchoHandler.
func NewEchoHandler() *EchoHandler {
	return &EchoHandler{}
}

// ServeHTTP handles an HTTP request to the /echo endpoint.
func (*EchoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := io.Copy(w, r.Body); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to handle request:", err)
	}
}

// NewServeMux builds a ServeMux that will route requests
// to the given EchoHandler.
func NewServeMux(echo *EchoHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/echo", echo)
	return mux
}

```

fx的启动顺序：

1. 构建依赖图（Provide阶段）

   执行所有 `fx.Provide`

2. 执行所有 `fx.Invoke`（关键点）

   按顺序执行

3. 执行 `Lifecycle.OnStart`（真正启动）

   按注册顺序

4. 阻塞运行（Run 模式）

5. 执行 `Lifecycle.OnStop`（真正启动）

   按注册顺序的逆序

## 补充信息

### fx.Anntated

#### 介绍

`fx.Annotated`fx库中的一个结构体，主要用于给提供给容器的函数或调用的函数添加元数据。

```go
type Annotated struct {
    Name   string
    Target interface{}
    Group  string
}
```

- Name字段可以指定该函数提供的结果在fx图中的名称。此名称可用于解析命名的实例。
- Target字段是你想添加注释的构造函数。
- Group字段可以指定结果应该添加到的值组。

这是如何使用`fx.Annotated`的一个例子：

```go
type Result struct{}

func NewResult() Result {
    return Result{}
}

func main() {
    fx.New(
        fx.Provide(
            fx.Annotated{
                Name:   "example",
                Target: NewResult,
            },
        ),
    )
}
```

在这个例子中，我们使用 `fx.Annotated`将 `NewResult` 函数的返回值以 “example” 的名称提供给fx容器。然后，我们可以通过该名称在其他地方获取此结果。

这种提供命名实例的能力非常有用，特别是当你有多个实现同一接口的值时，在fx中需要明确指定使用哪一个。

#### Group字段的作用

在构建Fx应用程序时，你可能会遇到一种情况，也就是你需要把多个提供者的结果（通常是具有共同类型或接口的对象）集中在一起，这就是“Group”派上用场的地方。

以下是使用值组的一些典型情况：

1. **多个提供者提供相同类型的结果**：你可能有多个提供者都返回相同的类型，这在创建插件系统或模块化设计时很常见。在这种情况下，你可能需要将所有的插件（通过各自的提供者提供）聚合到一个列表中。
2. **需要动态注入依赖项**：有时候，你可能不知道需要依赖多少或哪些具体的实例。例如，你的应用可能依赖一个插件系统，这个系统允许其他开发者添加他们自己的插件。在这种情况下，你可以使用值组来动态地收集和注入插件。
3. **需要将多个对象组合在一起进行处理**：例如，你可能想把所有实现了特定接口的对象收集起来，并在单个函数中一次处理它们。通过使用值组，你可以轻松地将这些对象注入到这个函数中。

值组允许你在不知道具体数量或具体实例的情况下注入一组对象，同时保持代码的解耦。这样可以帮助你创建更模块化和可扩展的应用程序。

```go
package main

import (
	"fmt"

	"go.uber.org/fx"
)

type ResType struct {
	Value string
}

func Provider1() ResType {
	return ResType{Value: "From Provider 1"}
}

func Provider2() ResType {
	return ResType{Value: "From Provider 2"}
}

type ResGroup struct {
	fx.In

	Group []ResType `group:"myGroup"`
}

func main() {
	fx.New(
		fx.Provide(
			fx.Annotated{
				Group:  "myGroup",
				Target: Provider1,
			},
			fx.Annotated{
				Group:  "myGroup",
				Target: Provider2,
			},
		),
		// 不要用 *ResGroup
		// fx.In 结构体 必须是值类型
		fx.Invoke(func(r ResGroup) {
			for _, res := range r.Group {
				fmt.Println(res.Value)
			}
		}),
	).Run()
}
```

### fx.in / fx.out

#### 作用

**`fx.In` / `fx.Out` 是给 Fx 用的“结构体标记”**。

用来告诉 Fx：这个 struct 不是普通数据，而是「依赖声明 / 结果声明」*

```go
package main

import (
	"fmt"

	"go.uber.org/fx"
)

type MyType1 struct {
	Value int
}

type MyType2 struct {
	Value string
}

type ModuleAParams struct {
	fx.In

	Value1 MyType1
}

type ModuleAResults struct {
	fx.Out

	Value2 MyType2
}

func ModuleA(p ModuleAParams) ModuleAResults {
	// use p.Value1
	return ModuleAResults{
		Value2: MyType2{Value: "Hello, world"},
	}
}

func main() {
	v1 := MyType1{Value: 10}

	app := fx.New(
		fx.Supply(v1),
		fx.Provide(ModuleA),
		fx.Invoke(func(v2 MyType2) {
			fmt.Println(v2.Value) // Output: Hello, world
		}),
	)

	app.Run()
}
```

#### Supply

```go
fx.Supply(v1)
```

“容器里已经有一个 `MyType1` 了”

#### Provide

```go
fx.Provide(ModuleA)
```

👉 “如果有人要 `MyType2`， 那我可以用 调 `ModuleA` 来造”

#### fx.In

在`ModuleAParams`中

Fx 看到 `fx.In`

- 扫描 struct 的每个字段
- **按字段类型去容器里找依赖**

等价于`func ModuleA(v1 MyType1) ModuleAResults`

如果没有 `fx.In` 会怎样？

❌ Fx 会认为：

> “哦，这是一个普通 struct，需要我提供一个 `ModuleAParams`”

然后报错：

```shell
missing type: ModuleAParams
```

#### fx.Out

```go
type ModuleAResults struct {
    fx.Out

    Value2 MyType2
}
```

它的作用是：

> **声明：ModuleA 要向容器“产出什么依赖”**

- `fx.Out` = “返回值拆包器”
- Fx 看到 `fx.Out`
  - 把 struct 里的每个字段**分别注册成可注入的依赖**

相当于写成：`func ModuleA(v1 MyType1) MyType2`

如果没有 `fx.Out` 会怎样？

```go
type ModuleAResults struct {
    Value2 MyType2
}
```

❌ Fx 会认为：

> “ModuleA 返回的是一个 `ModuleAResults` 类型”

而你 `Invoke` 要的是 `MyType2`：

```go
fx.Invoke(func(v2 MyType2) {})
```

于是报错：

```go
missing type: MyType2
```

#### Fx 内部流程是：

1. 发现需要 `MyType2`
2. 找到 `ModuleA`
3. 发现 `ModuleA` 需要 `MyType1`
4. 容器里已经有（`fx.Supply(v1)`）
5. 调 `ModuleA`
6. 用 `fx.Out` 把 `Value2` 注册成 `MyType2`
7. 注入到 `Invoke`

#### 实际使用例子

```go
type Params struct {
    fx.In

    DB     *sql.DB
    Logger *zap.Logger
    Cache  *redis.Client `optional:"true"`
}

type Results struct {
    fx.Out

    Handler http.Handler
    Service *Service
    Repo    *Repo
}

func NewUserModule(p Params) Results {
    repo := NewRepo(p.DB)
    svc  := NewService(repo)
    h    := NewHandler(svc)

    return Results{
        Repo:    repo,
        Service: svc,
        Handler: h,
    }
}

func main(){
   fx.New(
        fx.Provide(NewHandler),
        fx.Invoke(func(h http.Handler) {
            fmt.Println("got handler")
        }),
	).Run()

}
```

一句话理解作用：

> **fx.In：我需要什么**
> **fx.Out：我提供什么，只能写fx看的懂、容器需要的东西，不能有方法，它的作用只是一个依赖声明清单**
> **没有它们，Fx 只看类型，不懂语义**



