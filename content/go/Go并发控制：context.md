+++
date = '2024-11-29T20:26:13+08:00'
title = "Go 并发控制：context"
tags = ["go", "并发"]
categories = ["go"]
+++

今天想分享一下context包，相信很多学习go的入门开发者，在一些开源的项目中，或者一些视频中，都听过一句话，就是把ctx，也就是context.Context类型的参数作为方法的第一个参数，他们会讲是用来做上下文的，然后我们就是一脸懵，上下文是什么？加了会发生什么？然后我们知道了这个参数是用来向下透传的，会携带一些信息，同时可以做到超时控制等作用，但是我们还是不清楚是如何起作用的。

单纯的认为只要函数中带着context参数往下传递就可以做到超时取消，快速返回。相信大多数初学者也都是和我一个想法，其实这是一个错误的思想，其取消机制采用的也是通知机制，单纯的透传并不会起作用，比如你这样写代码：

```go
func main()  {
 ctx,cancel := context.WithTimeout(context.Background(),10 * time.Second)
 defer cancel()
 go Monitor(ctx)

 time.Sleep(20 * time.Second)
}

func Monitor(ctx context.Context)  {
 for {
  fmt.Print("monitor")
 }
}
```
即使context透传下去了，没有监听取消信号也是不起任何作用的。所以了解context的使用还是很有必要的，本文以 Go 1.23.0 版本源码为基础，逐步解析Go语言的context包。

## 1. context设计

context 被设计为一个接口，名为 Context。为了支持不同特性，这个接口有多种结构体实现。而每个结构体又提供了一个或多个 exported 函数（大写字母开头的公开函数）作为构造函数来实例化 context 对象。

![context 设计架构](/go/images/context.png)
这张图包含了 context 中最核心的对象和它们之间的关系，我们来简单梳理下这张图:

- **Context 接口：** 这是 context 最基本的抽象，定义了一个 context 对象应该支持哪些行为。它被设计为 exported，所以我们也可以实现自定义的 context 对象。

- **实现 Context 接口的结构体：** 为了实现 context 的控制链路和安全传值两大特性，context 包提供了多种 Context 接口的实现。
    - `emptyCtx` 表示一个空的 context 实现，没有控制链路的能力，也没有安全传值的功能。不过它作为最基础的 context 实现，可以算是其他 context 实现的“基类”了。`backgroundCtx` 和 `todoCtx` 包装了 `emptyCtx`，不过二者并没有扩展什么功能，只是表明了语义，它们通常作为整个 context 链路的起点。
    
    - `cancelCtx` 是一个带有取消功能的 context 实现，所以它拥有控制链路的能力。`timerCtx` 和 `afterFuncCtx` 都是在 `cancelCtx` 的基础上来实现的。

    - `withoutCancelCtx` 从命名上也能看出，它和 `cancelCtx` 正相反，没有取消功能，在实现上与 `emptyCtx` 差不多。

    - `valueCtx` 见名之意，是用来进行安全传值的。

    - `stopCtx` 实现，它比较特殊，没有提供构造函数，目前来看并不是 context 的核心对象。

- **exported 函数：** 因为所有的 context 实现都是 unexported 类型，所以就需要 exported 类型的函数来创建 context 对象供我们使用。
    - `Background()` 是使用的最多的函数了，它构造一个 `backgroundCtx` 对象并返回，通常作为 context 树的根节点。

    - `TODO()` 函数当然就是用来构造 `todoCtx` 对象的构造函数了，同样会作为 context 树的根节点。当我们不知道该用哪个 context 对象时，就用它。

    - `WithCancel()` 和 `WithCancelCause()` 都用来构造并返回 `cancelCtx` 对象，二者唯一的区别就是构造对象时是否传入根因。

    - `WithDeadline()` 和 `WithDeadlineCause()` 用于构造一个 `cancelCtx` 或 `timerCtx` 对象。它们可以接收一个 time.Time 用来指定 context 对象被取消的时间，到期时会被自动取消。

    - `WithTimeout()` 和` WithTimeoutCause()` 都接收一个 time.Duration 来指定多长时间之后 context 对象被取消。`WithTimeout()` 内部调用了 `WithDeadline()`，而 `WithTimeoutCause()` 内部则调用了 `WithDeadlineCause()`。

    - `WithoutCancel()` 用于构造并返回 `withoutCancelCtx` 对象。

    - `WithValue()`用于构造并返回 `valueCtx` 对象。

    - `AfterFunc()` 用于在 context 过期时异步的执行一个任务，它会构造一个 `afterFuncCtx` 对象，但不返回它，而是返回一个停止函数，可以阻止异步任务执行。

## 2. Context接口

Context 作为 context 包最核心的接口，其定义如下：

```go
type Context interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}
```
可以看到 Context 只有 4 个方法:

- `Deadline()` 方法返回该 context 应该被取消的截止时间，如果此 context 没有设置截止时间，则返回的 ok 值为 false。
- `Done()` 返回一个只读的 channel 作为取消信号，当 context 被取消时，此 channel 会被 close 掉。
- `Err()` 方法返回 context 被取消的原因，如果 context 还未取消，返回 nil；如果调用 cancel() 主动取消了 context，返回 Canceled 错误；如果是截止时间到了自动取消了 context，返回 DeadlineExceeded 错误。
- `Value()` 方法返回与给定键（key）关联的值（value），如果没有与该 key 关联的 value，则返回 nil。

## 3. context实现

接下来我们对 `Context` 接口的具体实现进行逐一讲解。

### 3.1 emptyCtx

`emptyCtx` 是最基础的 context 实现，定义如下：
```go
type emptyCtx struct{}

func (emptyCtx) Deadline() (deadline time.Time, ok bool) {
	return
}

func (emptyCtx) Done() <-chan struct{} {
	return nil
}

func (emptyCtx) Err() error {
	return nil
}

func (emptyCtx) Value(key any) any {
	return nil
}
```
它确实“基础”，也确实 “empty”，所有实现都为空，没有代码逻辑，仅是一个 context 架子。

#### 3.1.1 backgroundCtx 和 todoCtx

`backgroundCtx` 定义如下：

```go
type backgroundCtx struct{ emptyCtx }

func (backgroundCtx) String() string {
	return "context.Background"
}
```

`todoCtx` 定义如下 ：
```go
type todoCtx struct{ emptyCtx }

func (todoCtx) String() string {
	return "context.TODO"
}
```

可以看出来二者只是内嵌了 emptyCtx，也仅比 emptyCtx 多实现了一个 String() 方法。

#### 3.1.2 exporter方法：Background() 和 TODO()

我们在使用 context 时，往往会用 `context.Background()` 或 context.TODO()` 来定义了最顶层 context，这两个方法实现如下：

```go
func Background() Context {
	return backgroundCtx{}
}

func TODO() Context {
	return todoCtx{}
}
```

### 3.2 cancelCtx

`cancelCtx` 结构体定义如下：

```go
type cancelCtx struct {
	Context // “继承”的父 Context

	mu       sync.Mutex            // 持有锁保护下面这些字段
	done     atomic.Value          // 值为 chan struct{} 类型，会被懒惰创建，在第一次调用取消函数 cancel() 时被关闭，表示 Context 已被取消
	children map[canceler]struct{} // 所有可以被取消的子 Context 集合，它们在第一次调用取消函数 cancel() 时被级联取消，然后置为 nil
	err      error                 // 取消原因，在第一次调用取消函数 cancel() 时被设置值
	cause    error                 // 取消根因，在第一次调用取消函数 cancel() 时被设置值
}

type canceler interface {
	cancel(removeFromParent bool, err, cause error) // 取消函数
	Done() <-chan struct{}                          // 通过返回的 channel 能够知道是否被取消
}
```

- `cancelCtx` 直接内嵌了 `Context` 接口，也就是说，它支持任意其他类型的 context 实现作为父上下文（`parent context`）。

- context 是并发安全的，所以 cancelCtx 内部持有一把互斥锁，保证安全的操作结构体属性。

- `done` 属性为 `atomic.Value` 类型，是为了支持原子操作，使用它可以减少互斥锁的使用频率，稍后你将在 `Done()` 方法中看到。它的值是 chan struct{} 类型。

- `children` 属性是一个集合，记录了当前 context 的所有子上下文（`child context`）。这样，父子 context 就产生了链路关系，以此为基础实现父 context 取消时，级联的取消所有子 context。
    - `canceler`是一个接口，表示一个可以被取消的对象。也就是说，在 context 包中设计的支持取消的 context 类型都需要提供这两个方法。
    - 父 context 取消时会调用子 context 的 `cancel()` 方法进行级联取消；并且有取消功能的 context 必须要实现 `Done()` 方法，这样使用者才能通过监听 done channel 知道这个 context 是否被取消。

- `err` 和 `cause` 分别记录了 context 被取消的原因和根因，`err` 是 context 包内部产生的，`cause` 则是我们在使用 `WithXxxCause()` 方法构造 context 对象时传入的。

`cancelCtx` 的方法实现如下：
```go
func (c *cancelCtx) Done() <-chan struct{} {
	// 使用 double-check 来提升性能
    // 无锁的快路径
	d := c.done.Load() // 原子操作，比互斥锁更加轻量
	if d != nil {      // 如果存在 channel 直接返回
		return d.(chan struct{})
	}
    // 加锁的慢路径
    // 如果不存在 channel，则要先加锁，然后创建 channel 并返回
	c.mu.Lock() 
	defer c.mu.Unlock()
	d = c.done.Load()
	if d == nil { // 为保证并发安全，再做一次检查
		d = make(chan struct{})
		c.done.Store(d)
	}
	return d.(chan struct{})
}

var cancelCtxKey int

func (c *cancelCtx) Value(key any) any {
	// 使用 &cancelCtxKey 标记需要返回自身
	// 这是一个未导出的（unexported）类型，所以仅作为 context 包内部实现的一个“协议”，对用户不可见
	if key == &cancelCtxKey {
		return c
	}
	// 接着向上遍历父 Context 链路，查询 key
	return value(c.Context, key)
}

func (c *cancelCtx) Err() error {
	c.mu.Lock()
	err := c.err
	c.mu.Unlock()
	return err
}

func (c *cancelCtx) String() string {
	return contextName(c.Context) + ".WithCancel"
}

type stringer interface {
	String() string
}

func contextName(c Context) string {
	if s, ok := c.(stringer); ok {
		return s.String()
	}
	return reflectlite.TypeOf(c).String()
}


```

`cancelCtx` 的 `cancel()` 方法实现如下：

```go
func (c *cancelCtx) cancel(removeFromParent bool, err, cause error) {
	if err == nil {
		panic("context: internal error: missing cancel error")
	}
	if cause == nil { // 如果没有设置根因，取 err
		cause = err
	}
	c.mu.Lock()
	if c.err != nil { // 如果 err 不为空，说明已经被取消，直接返回
		c.mu.Unlock()
		return
	}

	// NOTE: 只有第一次调用 cancel 才会执行之后的代码

	// 记录错误和根因
	c.err = err
	c.cause = cause
	d, _ := c.done.Load().(chan struct{})
	if d == nil { // 如果 done 为空，直接设置一个已关闭的 channel
		c.done.Store(closedchan)
	} else { // 如果 done 有值，将其关闭
		close(d)
	}
	// 级联取消所有子 Context
	for child := range c.children {
		// NOTE: 获取子 Context 的锁，同时持有父 Context 的锁
		child.cancel(false, err, cause)
	}
	c.children = nil // 清空子 Context 集合，因为已经完成了 Context 树整个链路的取消操作
	c.mu.Unlock()

	if removeFromParent { // 从父 Context 的 children 集合中移除当前 Context
		removeChild(c.Context, c)
	}
}


// closedchan 表示一个已关闭的 channel
var closedchan = make(chan struct{})

// 导入 context 包时直接关闭 closedchan
func init() {
	close(closedchan)
}
```

这个方法用来取消 `cancelCtx`，它接收 3 个参数，`removeFromParent` 表示是否要从父 context 的 children 属性集合中移除当前的 cancelCtx；`err` 和 `cause` 则分别表示取消的错误原因和根因。

其中的`removeChlid`路线如下：

```go
func removeChild(parent Context, child canceler) {
	// 判断父 context 是否为 stopCtx 类型，如果是，则调用其 s.stop() 方法。
    if s, ok := parent.(stopCtx); ok {
		s.stop()
		return
	}
    // 
	p, ok := parentCancelCtx(parent)
	if !ok {
		return
	}
	p.mu.Lock()
	if p.children != nil {
		delete(p.children, child)
	}
	p.mu.Unlock()
}

// 给定一个 parent，沿着 Context 的包装链向上查找，找到第一个真正具备取消能力的 *cancelCtx 节点。
func parentCancelCtx(parent Context) (*cancelCtx, bool) {
	done := parent.Done() // parent自己的Done channel
    // closedchan 表示已经被取消了 || nil 表示永远不会被取消
	if done == closedchan || done == nil {
		return nil, false
	}
    // 
	p, ok := parent.Value(&cancelCtxKey).(*cancelCtx) // 通过value找到的cancelCtx
	if !ok {
		return nil, false
	}
    // 进一步进行并发安全与隔离性检查：
	// 获取查找到的 *cancelCtx 内部实际的 done channel
	pdone, _ := p.done.Load().(chan struct{}) // 通过cancelCtx找到的Done channel
	if pdone != done {
		return nil, false
	}
	return p, true
}
```
> 为什么需要这个函数？
>
> ctx1, cancel := context.WithCancel(context.Background()) // ctx1 是 *cancelCtx
>
> ctx2 := context.WithValue(ctx1, "key", "value")          // ctx2 是 *valueCtx
> 
> 如果现在有一个子 Context 的 parent 是 ctx2。当子 Context 想要从 parent 的 children 中移除自己时，ctx2 (valueCtx) 本身根本没有 children 属性。我们必须向上穿透，找到真正的 ctx1 (*cancelCtx)，才能操作它的 children Map。

> pdone和done 这一部分的理解
>
> `done` 是当前 parent 的 Done channel，`pdone` 是顺着 Value 找到的底层 cancelCtx 的 Done channel。在标准库的正常用法中，它俩永远是同一个 channel。
>
> 什么时候会不相等？用户自定义了一个context，
>
>   - 他重写了 Done() 方法，返回了一个自己新造的 channel。
>
>   -  但他偷懒了，Value() 方法依然原封不动地透传了祖辈的 cancelCtx。
>
> 比如：
> ```
> // 用户自定义的 Context
>type myCtx struct {
>    parent context.Context
>    myDone chan struct{}  // 它自己独有的 done channel！
>}
>
>func (m *myCtx) Deadline() (time.Time, bool) { return m.parent.Deadline() }
>
>// 关键：Done() 返回的是自己的 channel，而不是透传 parent 的
>func (m *myCtx) Done() <-chan struct{} { return m.myDone }
>
>func (m *myCtx) Err() error { return nil }
>
>// 关键：Value() 完全透传给 parent，parent 里能找到 cancelCtx
>func (m *myCtx) Value(key interface{}) interface{} {
>    return m.parent.Value(key)
>}
> ```

#### 3.2.1 exporter方法：WithCancel() 和 WithCancelCause()
看完了 `cancelCtx` 的实现，接下来看下我们如何构造一个 `cancelCtx`。

context 包提供了两种构造 `cancelCtx` 的方法，分别是 `WithCancel()` 和 `WithCancelCause()`。

`WithCancel()` 和 `WithCancelCause()`实现如下：

```go
type CancelFunc func()

func WithCancel(parent Context) (ctx Context, cancel CancelFunc) {
	c := withCancel(parent)
	return c, func() { c.cancel(true, Canceled, nil) }
}

type CancelCauseFunc func(cause error)

func WithCancelCause(parent Context) (ctx Context, cancel CancelCauseFunc) {
	c := withCancel(parent)
	return c, func(cause error) { c.cancel(true, Canceled, cause) }
}

var Canceled = errors.New("context canceled")
```

`WithCancel()`和`WithCancelCause()` 根据给定的父 context 构造一个新的具有取消功能的 cancelCtx 并返回，其核心逻辑是代理给 withCancel() 函数去实现的。唯一区别就是返回的函数是否支持设置 context 被取消的根因 cause

那么接下来就看看 withCancel() 函数是如何实现的：

```go
func withCancel(parent Context) *cancelCtx {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	c := &cancelCtx{}            // 带取消功能的 Context
	c.propagateCancel(parent, c) // 将新构造的 Context 向上传播挂载到父 Context 的 children 属性中，这样当父 Context 取消时子 Context 对象 c 也会级联取消
	return c
}

func (c *cancelCtx) propagateCancel(parent Context, child canceler) {
	c.Context = parent // “继承”父 Context，这里可以是任何实现了 Context 接口的类型

	// NOTE: 父 Context 没有实现取消功能
	done := parent.Done()
	if done == nil { // 如果父 Context 的 Done() 方法返回 nil，说明父 Context 没有取消的功能，那么无需传播子 Context 的 cancel 功能到父 Context
		return
	}

	// NOTE: 父 Context 已经被取消
	select {
	case <-done: // 直接取消子 Context，且取消原因设置为父 Context 的取消原因
		child.cancel(false, parent.Err(), Cause(parent))
		return
	default:
	}

	// NOTE: 父 Context 还未取消
	if p, ok := parentCancelCtx(parent); ok { // 如果父 Context 是 *cancelCtx 或者从 *cancelCtx 派生而来
		p.mu.Lock()
		if p.err != nil {
			// 如果父 Context 的 err 属性有值，说明已经被取消，直接取消子 Context
			child.cancel(false, p.err, p.cause)
		} else {
			if p.children == nil { // 延迟创建父 Context 的 children 属性
				p.children = make(map[canceler]struct{})
			}
			p.children[child] = struct{}{} // 将 child 加入到这个 *cancelCtx 的 children 集合中
		}
		p.mu.Unlock()
		return
	}

	// NOTE: 父 Context 实现了 afterFuncer 接口
	if a, ok := parent.(afterFuncer); ok { // 测试文件 afterfunc_test.go 中 *afterFuncCtx 实现了 afterFuncer 接口
		c.mu.Lock()
		stop := a.AfterFunc(func() { // 注册子 Context 取消功能到父 Context，当父 Context 取消时，能级联取消子 Context
			child.cancel(false, parent.Err(), Cause(parent))
		})
		c.Context = stopCtx{ // 将当前 *cancelCtx 的直接父 Context 设置为 stopCtx
			Context: parent, // stopCtx 的父 Context 设置为 parent
			stop:    stop,
		}
		c.mu.Unlock()
		return
	}

	// NOTE: 父 Context 不是已知类型，但实现了取消功能
	goroutines.Add(1) // 记录下开启了几个 goroutine，用于测试代码
	go func() {       // 开起一个 goroutine，监听父 Context 是否被取消，如果取消则级联取消子 Context
		select {
		case <-parent.Done(): // 父 Context 被取消
			child.cancel(false, parent.Err(), Cause(parent))
		case <-child.Done(): // 自己被取消
		}
	}()
}
```


其中提到了`afterFuncer`和`stopCtx`：
```go
type afterFuncer interface {
	AfterFunc(func()) func() bool
}

type stopCtx struct {
	Context
	stop func() bool
}
```
在 Go 1.23.0 版本 context 包的源码中，并没有一个 context 实现了 afterFuncer 接口。所以 stopCtx 也并没有被真正使用。所以我才在前文讲解 removeChild() 函数时说 stopCtx 类型不必深究。

针对 cancelCtx 类型的源码讲解就到这里，可以说 cancelCtx 是最复杂的 context 实现了，后文中要讲解的 timerCtx 和 afterFuncCtx 都是基于它实现的。


### 3.3 timeCtx

timerCtx 结构体定义如下：

```go
type timerCtx struct {
	cancelCtx             // “继承”了 cancelCtx
	timer     *time.Timer // Under cancelCtx.mu.

	deadline time.Time
}
```
`timerCtx` 内部嵌入了 `cancelCtx` 以“继承” `Done()` 和 `Err()` 方法。并且它还关联了一个定时器 `timer` 和截止时间 `deadline`，以此来实现在截止时间到期时，自动取消 context。

timerCtx 实现的方法如下：

```go
func (c *timerCtx) Deadline() (deadline time.Time, ok bool) {
	return c.deadline, true
}

func (c *timerCtx) String() string {
	return contextName(c.cancelCtx.Context) + ".WithDeadline(" +
		c.deadline.String() + " [" +
		time.Until(c.deadline).String() + "])"
}

func (c *timerCtx) cancel(removeFromParent bool, err, cause error) {
	c.cancelCtx.cancel(false, err, cause)
	if removeFromParent {
		// 将此 *timerCtx 从其父 *cancelCtx 的 children 集合中删除
		removeChild(c.cancelCtx.Context, c)
	}
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()
}
```

`timerCtx` 并没有直接使用 `cancelCtx` 的`cancel`方法，而是自己也实现了 `cancel()` 方法。内部调用的 removeChild() 函数我们在前文讲解 cancelCtx 时已经见过了。这里唯一需要注意的一点是，如果 timer 属性不为 nil 则调用 timer.Stop() 将其停止，并将属性值置为 nil，以此让 timer 对象尽早被 GC 回收

#### 3.3.1 exporter方法：WithDeadline() 和 WithDeadlineCause()

```go
func WithDeadline(parent Context, d time.Time) (Context, CancelFunc) {
	return WithDeadlineCause(parent, d, nil)
}

func WithDeadlineCause(parent Context, d time.Time, cause error) (Context, CancelFunc) {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	// 如果父 Context 的截止时间已经比传入的 d 更早，直接返回一个 *cancelCtx（无需构造 *timerCtx 等待定时器判断截止时间到了才取消 Context）
	if cur, ok := parent.Deadline(); ok && cur.Before(d) {
		return WithCancel(parent)
	}
	c := &timerCtx{ // 构造一个带有定时器和截止时间功能的 Context
		deadline: d,
	}
	// 这里使用 cancelCtx 结构体默认值，初始化 timerCtx 时没有显式初始化 cancelCtx 字段
	c.cancelCtx.propagateCancel(parent, c) // 向父 Context 传播 cancel 功能，这样当父 Context 取消时当前 Context 也会被级联取消
	dur := time.Until(d)
	if dur <= 0 { // 截止日期已过，直接取消
		c.cancel(true, DeadlineExceeded, cause)
		return c, func() { c.cancel(false, Canceled, nil) }
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.timer = time.AfterFunc(dur, func() { // 等待截止时间到期，自动调用 cancel 取消 Context
			c.cancel(true, DeadlineExceeded, cause)
		})
	}
	return c, func() { c.cancel(true, Canceled, nil) }
}

var DeadlineExceeded error = deadlineExceededError{}

type deadlineExceededError struct{}

func (deadlineExceededError) Error() string   { return "context deadline exceeded" }
func (deadlineExceededError) Timeout() bool   { return true }
func (deadlineExceededError) Temporary() bool { return true }
```
`WithDeadlineCause()` 实现代码不多，首先对 `parent` 是否为 nil 做了检查。接着检查父 context 的截止时间是否比传入的 `d` 更早，如果是，则直接创建一个 `*cancelCtx` 并返回，无需创建 `*timerCtx`。这是因为 context 具有级联取消的能力，既然父 context 的截止时间更早，则父 context 一定先于子 context 取消，所以子 context 会被级联取消，这就没必要再大费周章的构造 `*timerCtx` 来定时取消子 context 了。

如果上述条件不成立，则构造一个带有定时器和截止时间功能的 `*timerCtx`。并且，同样需要调用 `cancelCtx.propagateCancel()` 向上传播取消功能。

接着判断是否已到截止时间，如果到了，则直接取消 context。否则使用 time.AfterFunc() 来实现延迟取消 context。

#### 3.3.2 exporter方法：WithTimeout() 和 WithTimeoutCause()

```go
func WithTimeout(parent Context, timeout time.Duration) (Context, CancelFunc) {
	return WithDeadline(parent, time.Now().Add(timeout))
}

func WithTimeoutCause(parent Context, timeout time.Duration, cause error) (Context, CancelFunc) {
	return WithDeadlineCause(parent, time.Now().Add(timeout), cause)
}
```

`WithTimeout()` 内部调用了 `WithDeadline()`，而 `WithTimeoutCause()` 内部则调用了 `WithDeadlineCause()`。


### 3.4 withoutCancelCtx

`withoutCancelCtx` 故名思义，是没有取消功能的 context，它可以打断 context 控制链路中级联取消的能力。

withoutCancelCtx 结构体定义非常简单，只有一个属性 c 用来保存父 context：

```go
type withoutCancelCtx struct {
	c Context
}

func (withoutCancelCtx) Deadline() (deadline time.Time, ok bool) {
	return
}

func (withoutCancelCtx) Done() <-chan struct{} {
	return nil
}

func (withoutCancelCtx) Err() error {
	return nil
}

func (c withoutCancelCtx) Value(key any) any {
	return value(c, key)
}

func (c withoutCancelCtx) String() string {
	return contextName(c.c) + ".WithoutCancel"
}
```
`withoutCancelCtx` 虽然没有取消功能，但实现了 `Value` 方法，可以根据 `key` 查询 `value`。这样才能保证整个 context 链路中传值的能力不被中断。

#### 3.4.1 exporter方法：WithoutCancel()
```go
func WithoutCancel(parent Context) Context {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	return withoutCancelCtx{parent}
}
```
### 3.5 valueCtx
我们前面介绍的 context 从设计上来说都是为了实现控制链路的，与其他 context 不同，valueCtx 用于实现在 context 链路中进行安全传值。

```go
type valueCtx struct {
	Context
	key, val any // 存储的键值对，注意一个 Context 仅能保存一对 key/value，这样就能实现并发读的安全，copy-on-write
}

func (c *valueCtx) Value(key any) any {
	if c.key == key { // 在自己的键值对中查找
		return c.val
	}
	return value(c.Context, key) // 沿着父 Context 向上查找
}
```

`valueCtx` 结构体内部嵌入了 Context 接口，这样可以直接复用父 context 实现的方法。key 和 value 字段则用于存储键值对。可以发现，一个 `valueCtx` 对象只能存储一对 `key/value`。

在用户调用 `Value()` 方法查找给定 key 关联的 value 时，首先判断是否在当前 context 中，如果不在，则交给 `value()` 函数来处理。

在介绍 `*cancelCtx.Value()` 方法时，我们并没有深入讲解 `value()` 函数，那么现在是时候看下 `value()` 函数是如何实现的了：

```go
func value(c Context, key any) any {
	for {
		switch ctx := c.(type) { // 断言 Context 类型
		case *valueCtx: // 表示一个用于安全传递数据的 Context
			if key == ctx.key { // 与当前 Context 的 key 匹配，直接返回对应的值 val
				return ctx.val
			}
			c = ctx.Context // key 不匹配，继续向上遍历父 Context
		case *cancelCtx: // 表示一个带有取消功能的 Context
			if key == &cancelCtxKey { // 检查 key 是否等于 &cancelCtxKey（这是一个指向 *cancelCtx 的特殊键），如果匹配，就返回自身（即 c 对象）
				return c
			}
			c = ctx.Context // key 不匹配，继续向上遍历父 Context
		case withoutCancelCtx: // 表示一个不带取消功能的 Context（使用 WithoutCancel() 创建出来的 Context 类型）
			if key == &cancelCtxKey { // 检查 key 是否等于 &cancelCtxKey，如果匹配，说明要查找的是取消信号的特殊键，就返回 nil，因为这种 Context 没有取消信号
				return nil
			}
			c = ctx.c // 如果 key 不匹配，则继续向上遍历父 Context
		case *timerCtx: // 表示一个带有定时器的 Context
			if key == &cancelCtxKey { // 检查 key 是否等于 &cancelCtxKey，如果匹配，返回其包装的 *cancelCtx
				return &ctx.cancelCtx
			}
			c = ctx.Context // key 不匹配，继续向上遍历父 Context
		case backgroundCtx, todoCtx: // 这两个类型是无值的 Context（通常这是 Context 树的根），所以直接返回 nil
			return nil
		default: // 如果没有匹配任何已知的 Context 类型，则调用 Context 的 Value 方法去查找 key 对应的值
			return c.Value(key)
		}
	}
}
```

#### 3.5.1 exporter 方法：WithValue()
```go
func WithValue(parent Context, key, val any) Context {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	if key == nil {
		panic("nil key")
	}
	if !reflectlite.TypeOf(key).Comparable() {
		panic("key is not comparable")
	}
	return &valueCtx{parent, key, val}
}
```
这里对 parent 和 key 都做了检查，注意 key 一定是可比较类型。

可以发现，valueCtx 并没有使用互斥锁，这是因为每次新增 key/value 时，都会新建一个新的 valueCtx，并将 parent 赋值给 valueCtx。这种 copy-on-write 的思想，保证绝不修改现有的 context 对象，那么程序中并发读取值时就不会产生 data race，同时也能保证并发安全。

### 3.6 afterFuncCtx

```go
type afterFuncCtx struct {
	cancelCtx           // “继承”了 cancelCtx
	once      sync.Once // 要么用来开始执行 f，要么用来阻止 f 被执行
	f         func()
}

func (a *afterFuncCtx) cancel(removeFromParent bool, err, cause error) {
	a.cancelCtx.cancel(false, err, cause) // 取消 cancelCtx
	if removeFromParent {
		removeChild(a.Context, a) // 将当前 *afterFuncCtx 从 cancelCtx 的父 Context 的 children 属性中移除
	}
	a.once.Do(func() { // 确保仅执行一次
		go a.f() // 开启新的 goroutine 执行 f，如果在调用 a.cancel() 之前 stop 函数被调用，stop 函数中的 a.once.Do 优先被执行，则此处就不会执行
	})
}
```

与 `timerCtx` 一样，`afterFuncCtx` 内部也嵌入了 `cancelCtx`。此外它还有两个属性 `once` 和 `f`，`once` 保证一个操作仅执行一次，要么用来开始执行 `f`，要么用来阻止 `f` 被执行，`f` 是一个延迟函数。

`afterFuncCtx` 在取消时，首先会取消父 `cancelCtx`。然后根据参数 removeFromParent 决定是否从父 context 的 children 属性中移除。最后使用 once.Do() 确保 f 函数仅执行一次。

#### 3.6.1 exporter 方法：AfterFunc()

```go
func AfterFunc(ctx Context, f func()) (stop func() bool) {
	a := &afterFuncCtx{
		f: f,
	}
	// 调用 cancelCtx 的向上传播方法，将 a 的取消功能挂载到父 ctx 的 children 属性中，实现级联取消
	a.cancelCtx.propagateCancel(ctx, a)
	return func() bool { // 返回一个停止函数，用于阻止 f 被执行
		stopped := false
		a.once.Do(func() { // 确保仅执行一次
			stopped = true // 如果此处被执行，则 a.cancel 方法内部的 a.once.Do 就不会重复执行，即阻止 f 被执行
		})
		if stopped { // 第一次调用，取消 Context
			a.cancel(true, Canceled, nil)
		}
		return stopped
	}
}
```

与其他 context 构造函数不同，`AfterFunc()` 并不会返回构造的 `afterFuncCtx 对象`，而是返回一个 `stop() 函数`。

其实 AfterFunc() 的功能是为 context 注册一个延迟函数，当 context 被取消时，开启新的 goroutine 异步执行 f()。而 stop() 函数的作用则是用来阻止 f() 被执行。

- 如果先执行 `cancel()`，则 f() 必然执行。无论之后是否调用了 stop()。
- 如果先执行 `stop()`，则 f() 必然不会被执行。无论之后是否调用了 cancel()。

## 4. 总结
context 包的核心功能是`控制链路`和`安全传值`，且并发安全。

context 被设计为一个 Context 接口和多个实现了此接口的结构体。

一切 context 链路都会从一个空的 emptyCtx 开始，由 context.Background() 或 context.TODO() 来定义了最顶层 context，接着使用 WithXxx() 方法在原有的 context 基础上附加新的功能，形成 context 链路。

context 链路最终可能发展成一个树形结构，不过你要清楚，控制链路是从上到下的，父 context 取消，则会及联的取消所有带有取消功能的子孙 context；但通过给定 key 查找 value 则是自下而上的，而这就会导致从不同的起点出发，查找 context 中相同key 对应的 value 可能不同。