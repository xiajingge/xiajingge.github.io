+++

date = '2025-06-12T13:43:52+08:00'
title = "Go 并发控制：sync.Pool"
tags = ["go", "并发","sync"]
categories = ["go","后端开发"]

+++

# Go 并发控制：sync.Pool

## 1. 简介	

`sync.Pool` 是 Go 并发原语中用于对象池化的工具，主要用于缓存和复用临时对象，以减少内存分配和垃圾回收的压力。

`sync.Pool` 是一个结构体，其全部属性如下：

```go
// Pool 是一组临时对象的集合，这些对象可以被单独保存和获取。
//
// 存储在 Pool 中的任何对象都可能随时被自动移除，且不会发出通知。
// 如果 Pool 持有该对象的唯一引用时发生了这种情况，该对象可能会被释放。
//
// Pool 可以安全地被多个 goroutine 同时使用。
//
// Pool 的目的是缓存已分配但未使用的对象以供后续复用，从而减轻垃圾回收器的压力。
// 也就是说，它使得构建高效、线程安全的空闲列表变得容易。然而，它并不适用于所有的空闲列表。
//
// Pool 的适当用途是管理一组在包的并发独立客户端之间静默共享且可能被复用的临时对象。
// Pool 提供了一种将分配开销分摊到多个客户端的方式。
//
// Pool 的一个良好使用示例是 fmt 包，它维护了一个动态大小的临时输出缓冲区存储。
// 该存储在负载下（当许多 goroutine 活跃地进行打印时）会扩展，在闲置时会收缩。
//
// 另一方面，作为短生命周期对象一部分维护的空闲列表不适合使用 Pool，
// 因为在这种情况下开销无法很好地分摊。让此类对象实现自己的空闲列表会更高效。
//
// Pool 在首次使用后不得被复制。
//
// 在 [Go 内存模型] 的术语中，对 Put(x) 的调用"先于同步"于返回相同值 x 的 [Pool.Get] 调用。
// 类似地，返回 x 的 New 调用"先于同步"于返回相同值 x 的 Get 调用。
// [the Go memory model]: https://go.dev/ref/mem
type Pool struct {
	noCopy noCopy

	local     unsafe.Pointer // local fixed-size per-P pool, actual type is [P]poolLocal
	localSize uintptr        // size of the local array

	victim     unsafe.Pointer // local from previous cycle
	victimSize uintptr        // size of victims array

	// New optionally specifies a function to generate
	// a value when Get would otherwise return nil.
	// It may not be changed concurrently with calls to Get.
	New func() any
}

func (p *Pool) Get() any
func (p *Pool) Put(x any)
func (p *Pool) getSlow(pid int) any
func (p *Pool) pin() (*poolLocal, int)
func (p *Pool) pinSlow() (*poolLocal, int)
```

这一部分的解释就是：

> ### 核心概念
>
> `sync.Pool` 是 Go 语言提供的一个临时对象池，用于缓存已分配但未使用的对象，供后续复用。
>
> ### 公开方法
>
> - `New` 字段：当池中没有可用对象时，调用 `New` 函数创建一个新对象。
> - `Get` 方法：从池中获取一个对象。如果池为空，则调用 `New` 创建新对象。
> - `Put` 方法：将对象放回池中，以便复用。
>
> ### 主要特性
>
> 1. **线程安全**：可被多个 goroutine 并发使用
> 2. **自动回收**：池中对象可能随时被自动移除，不保证持久存储
> 3. **减轻 GC 压力**：通过对象复用减少内存分配和垃圾回收开销
>
> ### 适用场景
>
> - ✅ **适合**：管理跨多个并发客户端共享的临时对象（如 fmt 包的输出缓冲区）
> - ❌ **不适合**：短生命周期对象的空闲列表（应自行实现空闲列表）
>
> ### 使用注意
>
> - Pool 首次使用后不可复制
> - 内存模型保证：`Put(x)` 先于 `Get()` 返回 x；`New` 返回 x 先于 `Get()` 返回 x
>
> ### 典型用途
>
> 构建高效、线程安全的对象复用机制，将分配开销分摊到多次使用中。

## 2. 使用示例

```go
package main

import (
	"bytes"
	"io"
	"os"
	"sync"
	"time"
)

var bufPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return new(bytes.Buffer)
	},
}

// timeNow is a fake version of time.Now for tests.
func timeNow() time.Time {
	return time.Unix(1136214245, 0)
}

func Log(w io.Writer, key, val string) {
	b := bufPool.Get().(*bytes.Buffer)
	b.Reset()
	// Replace this with time.Now() in a real logger.
	b.WriteString(timeNow().UTC().Format(time.RFC3339))
	b.WriteByte(' ')
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(val)
	w.Write(b.Bytes())
	bufPool.Put(b)
}

func main() {
	Log(os.Stdout, "path", "/search?q=flowers")
}
```

这是 [sync.Pool](https://pkg.go.dev/sync@go1.25.0#Pool) 官方文档中示例代码。

首先，直接通过 `sync.Pool{}` 语法实例化一个 `Pool` 对象 `bufPool`，并且这里还初始化了 `New` 函数，其返回一个 `*bytes.Buffer` 对象。

接着，在`Log` 函数内部使用了 `bufPool`，通过 `Get` 方法得到一个 `*bytes.Buffer` 类型的对象 `b`，然后向 `b` 中写入数据，最后别忘了使用 `Put` 方法将 `*bytes.Buffer` 对象“还回去”，将其缓存在池中，以便下次使用。

最后，在 `main` 函数中调用 `Log` 函数，并将结果写入标准输出。

执行示例代码，得到输出如下：

```go
$ go run main.go
2006-01-02T15:04:05Z path=/search?q=flowers
```

根据以上使用示例，我们可以总结 `sync.Pool` 使用套路：

1. 实例化一个 `sync.Pool` 对象，并且赋值 `New` 属性，用户构造缓存对象。
2. 通过`p.Get()`取出对象`obj`使用。
   1. 如果池中有，就直接返回。
   2. 如果没有，调用 `New` 属性构造函数，构造一个新的对象并返回（如果没有 `New` 属性，则返回 `nil`）。
3. 对象使用完成后记得调用 `p.Put(obj)` 重新放入池中，以便下次使用。

由此可见，`sync.Pool` 适用于以下场景：

- 频繁创建和销毁的对象：如临时缓冲区。
- 减少内存分配：通过复用对象，减少 GC 压力。
- 无状态对象：池中的对象不应包含与特定上下文相关的状态。

此外，在使用 `sync.Pool` 时有两点需要我们特别注意：

- 对象重置：从池中获取的对象可能包含之前的状态，使用前需要**重置**。
- 对象生命周期：池中的对象可能会被 GC 回收，因此不能依赖池中的对象长期存在。

`sync.Pool` 的设计中有一个比较有意思的点，一个对象被放入池中以后，如果没被使用，则连续两次 GC 后，这个对象一定会被释放。

## 3. 源码分析

### 3.1 sync.Pool结构体

`sync.Pool` 是一个结构体，其定义如下：

```go
type Pool struct {
	// 禁止复制
	noCopy noCopy

	// 空闲对象，poolLocal 指针类型
	local unsafe.Pointer
	// 数组大小
	localSize uintptr

	// 回收站
	victim unsafe.Pointer
	// 数组大小
	victimSize uintptr

	// New 是一个可选的函数，调用 Get 方法时，如果缓存池中没有可用对象，则调用此方法生成一个新的值并返回，否则返回 nil
	// 该函数不能在并发调用 Get 时被修改
	New func() any
}
```

- `New` 属性我们已经使用过了，调用 `Get` 方法时，如果缓存池中没有可用对象，则调用此方法生成一个新的值并返回。
- `noCopy` 属性用来标记禁止复制，所以我们在拿到 `sync.Pool` 实例化对象后，记得一定不要让其产生复制操作。
- `local`和`victim`都是`poollocal`指针类型，用于存储缓存对象。`local`是当前P本地缓存的对象，而`victim`可以理解为Windows电脑的回收站。

Go 在触发垃圾回收时，`sync.Pool` 会做两件事：

1. 将所有缓存的 `victim` 中的对象移除。
2. 把所有缓存的 `local` 中对象移动到 `victim`。

从这个过程可以知道，`victim` 就是 Windows 电脑中的“回收站”，我们在电脑中删除文件时，先到回收站，然后在回收站里可以彻底删除。

### 3.2 poolLocal结构体

`poolLocal` 同样是一个结构体，其定义如下：

```go
type poolLocalInternal struct {
	// 私有对象
	private any
	// 共享队列，这是一个 lock-free 双向队列
	shared poolChain // Local P can pushHead/popHead; any P can popTail.
}

type poolLocal struct {
	poolLocalInternal

	// Prevents false sharing on widespread platforms with
	// 128 mod (cache line size) = 0 .
	pad [128 - unsafe.Sizeof(poolLocalInternal{})%128]byte
}
```

`poolLocal` 中包含了 `poolLocalInternal` 和 `pad` 两个属性。

其中 `pad` 属性并不是用来存放数据的，而是用于将 `poolLocal` 结构体所占用的内存对齐到 128 的整数倍。这是为了解决伪共享（`false sharing`）问题，以此来独占 CPU 高速缓存的 CacheLine。

而 `poolLocalInternal` 结构体内部，才是用来存储缓存数据的。其中 `private` 是一个私有对象，用于记录当前 P 下缓存的对象，`shared` 是一个双向队列（一个 `lock-free` 的双向链表结构），用于记录多个 P 中共享的缓存对象，当前 P 能够进行 `pushHead/popHead` 操作，其他 P 能够进行 `popTail` 操作，从而在当前 P 中窃取缓存对象。



这里所说的 P 是指 Go GMP 模型中的处理器（P），之所以设计为当前 P 从队头进行读写，其他 P 从队尾进行获取操作，目的是在不加锁的情况下保证并发安全。

`sync.Pool` 为每个处理器（P）维护一个本地的 `poolLocal` 结构，其中包含一个 `shared` 队列。这个 `shared` 队列的类型是 `poolChain`，它是一个由多个 `poolDequeue` 节点组成的双向链表结构。每个 `poolDequeue` 都是一个固定大小的环形队列（r`ing buffer`），并且每个新节点的容量通常是前一个节点的两倍。

`poolDequeue` 被设计为一个单生产者（`single-producer`）/多消费者（`multi-consumer`） 的无锁队列（`lock-free`）：

- 生产者：即当前 `P`，可以执行 `pushHead`（在头部添加）和 `popHead`（从头部弹出）操作。
- 消费者：包括当前 `P`（也可以消费）和其他 `P`。其他 `P`只能执行 `popTail`（从尾部弹出）操作。

对于 `poolChain` 的介绍就到这里，不再继续深入，避免陷入其中，我们应该继续回到 `sync.Pool` 本身方法的学习

![syncPool](/images/syncPool.png)



### 3.3 Put方法

`Put`方法用于添加一个对象到池中吗，其实现如下：

```go
// Put 添加一个元素到池中
func (p *Pool) Put(x any) {
	if x == nil { // x 为 nil 直接返回
		return
	}

	// pin() 把当前 goroutine 固定在当前的 P 上
	// 同时返回 local 对象（*poolLocal）和当前 P id
	l, _ := p.pin()

	if l.private == nil {
		l.private = x // 如果 private 为 nil，则直接将 x 赋值给它
	} else {
		l.shared.pushHead(x) // 否则，将 x push 到共享队列队头
	}

	// 将当前 goroutine 从当前 P 上解除固定
	runtime_procUnpin()
}
```

可以发现，`Put` 方法实现逻辑相当简单。

其中 `p.pin()` 和 `runtime_procUnpin()` 是必须成对出现的调用，有点类似互斥锁的加锁/解锁操作，并且同样是用来解决并发问题的。不同的是，`pin` 操作更加轻量，`p.pin()` 能够将当前 goroutine 固定在当前的 P 上。因为在一个 P 上，同一时刻只会运行一个 goroutine，所以，接下来在当前 goroutine 中操作当前 P 上的任何对象都无需加锁，从而避免的并发问题。

调用 `p.pin()` 能够拿到存储在当前 P 中的 `*poolLocal` 对象和当前 P `ID`，有了 `*poolLocal` 对象，就可以判断 `l.private` 是否为空，如果值为 `nil`，那么直接将对象 `x` 赋值到 `private` 属性中缓存起来。否则，将对象 `x` 存储到共享队列 `l.shared` 中。

### 3.4 Get方法

`Get` 方法用于从池中获取一个对象，其实现如下：

```go
// Get 从 [Pool] 中选择一个任意项，将其从 Pool 中移除，然后返回给调用者。
// Get 可以选择忽略池并将其视为空。
// 调用者不应假设传递给 [Pool.Put] 的值与 Get 返回的值之间存在任何关系。
//
// 如果 Get 返回 nil 且 p.New 非零，则 Get 返回调用 p.New 的结果。
func (p *Pool) Get() any {
	// 把当前 goroutine 固定在当前的 P 上
	// 拿到 local 对象（*poolLocal，该 P 的本地池）和当前 P id
	l, pid := p.pin()

	// 获取当前 P 中的 private
	x := l.private
	l.private = nil
	if x == nil { // private 不存在
		// 尝试从当前 P 的共享队列中弹出空闲对象
		// 因为 shared 队列只有所属的 P 会操作头部（生产者），所以 popHead 操作也无需加锁
		x, _ = l.shared.popHead()
		if x == nil { // 触发慢路径
			// 当前 P 的本地池为空，则尝试从其他 P 窃取或从 victim 缓存获取
			x = p.getSlow(pid)
		}
	}

	// 解除 pin
	runtime_procUnpin()

	if x == nil && p.New != nil {
		x = p.New() // 如果所有缓存都未找到对象，且用户提供了 New 函数，则创建一个新对象
	}
	return x
}
```

与 `Put` 方法一样，`Get` 方法的逻辑也通过 `p.pin()` 和 `runtime_procUnpin()` 进行保护。

Get 方法在缓存中获取空闲对象的搜索路径如下：

1. 从 `l.private` 中获取对象。
2. 从本地共享队列 `l.shared` 中获取对象。
3. 慢路径（尝试从其他 P 窃取或从 `victim` 回收站中获取）。

### 3.5 执行流程图

`Put`方法的执行流是：

![Put](https://jianghushinian.cn/2025/09/07/sync-pool/Pool_Put.png)

`Get`的执行流是：

![Get](https://jianghushinian.cn/2025/09/07/sync-pool/Pool_Get.png)

## 4. 总结

`sync.Pool` 的核心场景是：**复用“短生命周期、可重复使用、分配成本高”的临时对象，降低 GC 压力**。

- 典型用途：`[]byte` 缓冲区、`bytes.Buffer`、临时结构体、编解码中反复创建的中间对象
- 最常见场景：高并发请求里每次都要 `new` 一堆小对象，导致频繁 GC；用 `Pool` 可明显减少分配
- 适合“借用-归还”模型：`Get()` 取对象，用完后 `Reset` 再 `Put()` 回去

不适合的场景：

- 需要长期持有或强一致缓存（`sync.Pool` 里的对象**随时可能被 GC 清掉**）
- 资源对象（文件句柄、连接等），这类应使用专门连接池
- 对象很小且分配不频繁时，收益不明显，反而增加复杂度

## 5. 引申及思考

通过源码解读，我们知道 `sync.Pool` 默认缓存数据会存储在 `local` 中，在触发 GC 时则被移动到`victim`,`victim` 就像一个回收站，其内部的数据要被重新利用，要么被彻底删除。



但是`sync.Pool` 为什么要设计成调用两次 GC 才会回收对象呢？



​	其实这是为了防止 GC 引起的性能抖动。如果只调用一次 GC，就回收对象，则可能导致对象被频繁的创建和回收，并不能有效起到缓存的作用。那如果调用 3 次 GC 再回收行不行呢？理论上可以，但不建议这样做，其实这是一个内存和性能之间的取舍问题，如果缓存数据没有被使用，还长期存放在内存中，则势必会造成内存的浪费。两次 GC 才回收对象，应该是一个比较合理的经验值。