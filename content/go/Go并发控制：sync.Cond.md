+++

date = '2025-03-23T14:32:24+08:00'
title = "Go 并发控制：sync.Cond"
tags = ["go", "并发","sync"]
categories = ["go","后端开发"]

+++

# Go 并发控制：sync.Cond

## 1. 简介	

`sync.Cond` 是 Go 标准库中用于 **goroutine 间等待/通知** 的同步原语。一句话总结就是`一个等通知的锁`

基于某个 Locker（通常是 Mutex），让 goroutine 可以等待某个条件成立，并在条件变化时收到通知。

解决场景：**多个 goroutine 需要等待某个共享状态变化后，才能继续执行**。

## 2. 简单的使用示例

先来一个简单的示例来对`sync.Cond`做一个简单的了解，展示了如何通过条件变量让一个协程等待另一个协程完成某项任务：

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	// 创建一个 sync.Cond 对象
	var mu sync.Mutex
	cond := sync.NewCond(&mu)

	// 定义一个任务，模拟一个协程的等待
	go func() {
		// 锁定
		mu.Lock()
		fmt.Println("Goroutine: 等待条件成立...")
		// 在条件不成立时等待
		cond.Wait()
		fmt.Println("Goroutine: 条件成立，继续执行任务")
		// 解锁
		mu.Unlock()
	}()

	// 模拟主协程做一些工作，然后通知其他协程
	go func() {
		// 锁定
		mu.Lock()
		fmt.Println("主协程: 执行一些工作")
		// 模拟一些工作
		// 然后通知等待的协程
		cond.Signal() // 唤醒一个等待的协程
		fmt.Println("主协程: 条件已成立，通知等待中的协程")
		// 解锁
		mu.Unlock()
	}()

	// 等待足够时间让协程执行完
	select {}
}
```

1. 在这个示例中，`sync.NewCond(&mu)` 创建了一个条件变量 `cond`，并将一个互斥锁 `mu` 传给它。

2. 第一个 goroutine 使用 `cond.Wait()` 来等待条件成立。当它调用 `Wait()` 时，它会释放锁并进入等待状态，直到其他协程通过 `cond.Signal()` 或 `cond.Broadcast()` 通知它。

3. 第二个 goroutine 模拟了主协程的工作，并在完成工作后调用 `cond.Signal()` 来通知等待中的协程继续执行。

4. 使用 `mu.Lock()` 和 `mu.Unlock()` 来保证在操作条件变量时的互斥性。

> #### 注意事项
>
> 1. **必须先加锁，再调用 `Wait/Signal/Broadcast`**（除非要等条件成立）
> 2. **`Wait` 返回后一定要重新检查条件**（用 for 而非 if）
> 3. **`Signal` 不释放锁**，被唤醒的 goroutine 只是在等待队列里，要等当前 goroutine `Unlock` 后它才能拿到锁
> 4. **不要在临界区外调用 `Signal/Broadcast`**（除非明确知道安全）
> 5. **`Broadcast` 会唤醒所有**，慎用，可能引发惊群效应

## 3. 底层原理

Cond 结构（简化）：

```go
type Cond struct {
    L Locker          // 底层的锁（通常是 Mutex/RWMutex）
    notify  notifyList // Go runtime 的等待队列（链表）
}
```

这一部分的解释就是：

核心方法：

| 方法          | 作用                                                         |
| ------------- | ------------------------------------------------------------ |
| `Wait()`      | **解锁 → 挂起等待 → 被唤醒后重新加锁**（原子操作，中间不可分割） |
| `Signal()`    | 通知**等待队列中的第一个** goroutine 唤醒                    |
| `Broadcast()` | 通知**所有**等待中的 goroutine 唤醒                          |

其中最重要的就是Wait方法

```go
func (c *Cond) Wait() {
    c.L.Unlock()      // 先解锁，让其他 goroutine 可以拿到锁
    runtime_notifyListWait(&c.notify)  // 挂起当前 goroutine
    c.L.Lock()        // 被唤醒后重新加锁，保证Wait前后锁状态一致
}
```

可以看到会自动解锁，唤醒后自动加锁，所以外面不需要手动解/加。

## 4. 工程实践案例

### 4.1 生产者-消费者

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

var (
	buffer []int
	mu      sync.Mutex
	cond    = sync.NewCond(&mu)
)

func producer(id int) {
	for {
		mu.Lock()
		if len(buffer) >= 5 {
			// 如果缓冲区满了，生产者等待
			cond.Wait()
		}
		// 模拟生产数据
		buffer = append(buffer, id)
		fmt.Printf("生产者 %d 生产了一个数据，缓冲区: %v\n", id, buffer)
		// 唤醒消费者
		cond.Signal()
		mu.Unlock()
		time.Sleep(time.Second)
	}
}

func consumer(id int) {
	for {
		mu.Lock()
		for len(buffer) == 0 {
			// 如果缓冲区为空，消费者等待
			cond.Wait()
		}
		// 模拟消费数据
		data := buffer[0]
		buffer = buffer[1:]
		fmt.Printf("消费者 %d 消费了数据 %d，缓冲区: %v\n", id, data, buffer)
		// 唤醒生产者
		cond.Signal()
		mu.Unlock()
		time.Sleep(time.Second * 2)
	}
}

func main() {
	// 启动多个生产者和消费者
	for i := 1; i <= 3; i++ {
		go producer(i)
		go consumer(i)
	}

	// 让主程序运行足够长时间
	select {}
}
```

- 生产者在缓冲区满时会等待，消费者在缓冲区空时会等待。
- 使用 `cond.Wait()` 让协程等待条件的变化，当条件成立时通过 `cond.Signal()` 唤醒协程。
- 生产者和消费者的工作是交替进行的，确保了缓冲区不会溢出或为空。

> // 为什么用 for 而不是 if？
> for len(buffer) == 0 {
>     cond.Wait()
> }
>
> - **必须用 for**：防止**虚假唤醒（spurious wakeup）**，即使没收到 Signal 也可能随机醒来。
> - **先加锁再判断**：保证 `判断条件 → Wait` 的原子性，防止中间被其他 goroutine 插队修改。

### 4.2 资源池

在资源池中，多个协程可能会争用有限的资源。我们可以使用 `sync.Cond` 来协调对这些资源的访问，确保只有当资源可用时，协程才能获取它们。

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

type ConnectionPool struct {
	connections []int
	mu          sync.Mutex
	cond        *sync.Cond
}

func NewConnectionPool(size int) *ConnectionPool {
	pool := &ConnectionPool{
		connections: make([]int, 0, size),
	}
	pool.cond = sync.NewCond(&pool.mu)
	return pool
}

// 获取一个连接
func (p *ConnectionPool) GetConnection() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 如果连接池为空，则等待
	for len(p.connections) == 0 {
		p.cond.Wait()
	}

	// 获取连接
	conn := p.connections[0]
	p.connections = p.connections[1:]
	fmt.Printf("获取连接: %d\n", conn)
	return conn
}

// 归还一个连接
func (p *ConnectionPool) ReturnConnection(conn int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 归还连接
	p.connections = append(p.connections, conn)
	fmt.Printf("归还连接: %d\n", conn)

	// 通知等待中的获取连接的协程
	p.cond.Signal()
}

func main() {
	pool := NewConnectionPool(3)

	// 模拟多个协程获取和归还连接
	for i := 0; i < 5; i++ {
		go func(id int) {
			conn := pool.GetConnection()
			// 模拟使用连接
			time.Sleep(time.Second)
			pool.ReturnConnection(conn)
		}(i)
	}

	// 等待足够的时间，让所有协程执行完
	time.Sleep(5 * time.Second)
}
```

1. 连接池有固定数量的连接，如果没有连接可用，则协程会通过 `cond.Wait()` 等待。

2. 每次归还连接时，使用 `cond.Signal()` 唤醒等待中的协程。

3. 这种模式确保了连接不会超出池的最大限制，也保证了每个协程都能正常地从连接池中获取资源。

### 4.3 事件等待

在一些并发场景下，可能需要等待某个事件发生，然后再继续执行后续操作。例如，多个协程可能需要等到某个标志位被设置为 `true`，然后才能继续执行。

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

var mu sync.Mutex
var cond = sync.NewCond(&mu)

var eventTriggered bool = false

func worker(id int) {
	mu.Lock()
	for !eventTriggered {
		// 如果事件没有发生，等待
		cond.Wait()
	}
	fmt.Printf("Worker %d: 事件触发，开始工作\n", id)
	mu.Unlock()
}

func triggerEvent() {
	mu.Lock()
	eventTriggered = true
	fmt.Println("事件触发，唤醒等待中的工作协程")
	cond.Broadcast() // 通知所有等待的协程
	mu.Unlock()
}

func main() {
	// 启动多个工作协程
	for i := 1; i <= 3; i++ {
		go worker(i)
	}

	// 模拟一些工作，等待3秒触发事件
	time.Sleep(3 * time.Second)
	triggerEvent()

	// 等待足够时间让所有协程完成
	time.Sleep(2 * time.Second)
}
```

1. 所有工作协程都会等待 `eventTriggered` 被设置为 `true`，直到事件触发。

2. 当事件触发时，主协程调用 `cond.Broadcast()` 通知所有等待的协程，这样它们就可以继续执行。

3. 使用 `cond.Wait()` 和 `cond.Broadcast()` 实现了多个协程基于一个事件的同步。

### 4.4 小结

- **生产者-消费者模型**：适用于处理任务队列、缓冲区等资源的生产与消费。
- **资源池**：适用于有限资源的复用与管理，避免资源竞争和死锁。
- **事件等待**：适用于多个协程基于某个条件或事件的同步，确保协程之间协调一致地执行。

## 5. 一些可能的疑问

在上文中我提到一个词，**虚假唤醒**，那现在就稍微的解释一下这个问题。

1. 什么是虚假唤醒

​	**虚假唤醒（Spurious Wakeup）**：没有调用 `Signal()` 或 `Broadcast()`，等待中的 goroutine 仍然被操作系统唤醒。

2. 为什么会出现？

虚假唤醒是 **操作系统层面** 的机制，主要原因：

| 原因                 | 说明                                                        |
| -------------------- | ----------------------------------------------------------- |
| **系统唤醒信号处理** | 信号处理可能打断等待                                        |
| **调度器优化**       | 操作系统为性能，随机唤醒某些线程                            |
| **实现细节**         | 某些系统（尤其 Linux）的 pthread_cond_wait 实现允许虚假唤醒 |

这是 **POSIX 标准** 允许的行为，不是 Go 特有的。

3. 预防的措施

**用 `sync.Cond` 的地方，Wait 永远放在 `for` 循环里**。