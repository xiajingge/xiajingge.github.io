+++
date = '2025-06-17T21:07:15+08:00'
title = "开源系列--cron"
tags = ["go"]
categories = ["opensource"]
+++

如果你曾经在 Go 中实现过定时任务，可能会发现，原生的 time.Timer 或 time.Ticker 虽然简单易用，但在复杂的场景下（如多任务调度、时区处理、任务失败重试等）往往显得力不从心。这时，一个功能强大且灵活的定时任务库就显得尤为重要。

`github.com/robfig/cron` 正是为此而生。它不仅支持标准的 crontab 表达式，还提供了秒级精度、时区设置、任务链（Job Wrappers）等高级功能，能够轻松应对各种复杂的定时任务场景。

本文就来带大家见识下 cron 的用法

## 1. quickstart

首先就是在go项目中安装**cron**

```sh
$ go get github.com/robfig/cron/v3@master
```

```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

func main() {
	// 创建一个新的 Cron 实例
	c := cron.New(cron.WithSeconds())

	// 添加一个每秒执行的任务
	_, err := c.AddFunc("* * * * * *", func() {
		fmt.Println("每秒执行的任务:", time.Now().Format("2006-01-02 15:04:05"))
	})
	if err != nil {
		log.Fatalf("添加任务失败: %v", err)
	}

	// 添加一个每 5 秒执行的任务
	_, err = c.AddFunc("*/5 * * * * *", func() {
		fmt.Println("每 5 秒执行的任务:", time.Now().Format("2006-01-02 15:04:05"))
	})
	if err != nil {
		log.Fatalf("添加任务失败: %v", err)
	}

	// 启动 Cron
	c.Start()
	defer c.Stop() // 确保程序退出时停止 Cron

	// 主程序等待 10 秒，以便观察任务执行
	time.Sleep(10 * time.Second)
	fmt.Println("主程序结束")

/*
输出如下：
每秒执行的任务: 2025-06-17 21:12:23
每秒执行的任务: 2025-06-17 21:12:24
每秒执行的任务: 2025-06-17 21:12:25
每 5 秒执行的任务: 2025-06-17 21:12:25
每秒执行的任务: 2025-06-17 21:12:26
每秒执行的任务: 2025-06-17 21:12:27
每秒执行的任务: 2025-06-17 21:12:28
每秒执行的任务: 2025-06-17 21:12:29
每 5 秒执行的任务: 2025-06-17 21:12:30
每秒执行的任务: 2025-06-17 21:12:30
每秒执行的任务: 2025-06-17 21:12:31
每秒执行的任务: 2025-06-17 21:12:32
主程序结束
*/
}
```

这个示例展示了 cron 的基础用法，注册了两个定时任务到 cron 中，并在程序启动 10 秒后退出。

使用 cron.New 方法可以创建一个 cron 对象，cron.WithSeconds() 参数可以扩展 crontab 表达式语法支持到秒级别，语法规则不变。

cron 对象的 AddFunc 方法可以添加一个任务到 cron 中，它接收两个参数，第一个字符串类型的参数用来定义 crontab 表达式，* * * * * * 表示每秒执行一次，第二个参数 func() 就是我们要添加的任务。这里为 cron 对象添加了两个任务。

cron 对象的 Start 方法内部会创建一个新的 goroutine 并启动 cron 调度器，调度器可以控制所有注册进来的任务的执行，在任务的执行计划到期时运行任务。

cron 对象的 Stop 方法可以停止调度器。

## 2. 组件剖析

为了深入的了解 cron ， 我们就按照之前的quick start的顺序来进行讲解。

### 2.1 cron.New

这是创建cron对象的 export 方法，用来暴露一个 Cron 对象。


```go
func New(opts ...Option) *Cron {
	c := &Cron{
		entries:   nil,
		chain:     NewChain(),
		add:       make(chan *Entry),
		stop:      make(chan struct{}),
		snapshot:  make(chan chan []Entry),
		remove:    make(chan EntryID),
		running:   false,
		runningMu: sync.Mutex{},
		logger:    DefaultLogger,
		location:  time.Local,
		parser:    standardParser,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
```

这是使用函数选项式的构造函数，而这些选项与Cron对象字段有着密不可分的关系，所以只需要看Cron即可。

```go
type Cron struct {
	entries   []*Entry
	chain     Chain
	stop      chan struct{}
	add       chan *Entry
	remove    chan EntryID
	snapshot  chan chan []Entry
	running   bool
	logger    Logger
	runningMu sync.Mutex
	location  *time.Location
	parser    ScheduleParser
	nextID    EntryID
	jobWaiter sync.WaitGroup
}
```

Cron:
- entries:存储所有的定时任务条目
- chain:任务包装器链条，用于任务执行前后添加自定义行为
- stop：停止信号，用于终止cron调度器
- add：添加新任务的通道，当cron运行时，通过此通道安全的添加任务
- remove：移除任务通道
- snapshot：获取当前所有任务快照的通道
- running：标记 cron 调度器是否正在运行
- logger：日志接口
- runningMu：保护running状态的互斥锁
- location：时区设置
- parser：cron表达式解析接口
- nextID：下一个任务Id的计数器，每次添加新任务时递增，生成唯一ID
- jobWaiter：等待所有正在运行的任务完成

本项目将用户提交的任务信息组装成了最小支配单位：**Entry**

```go
  type Entry struct {
	// 任务唯一Id
	ID EntryID

	// 调度策略
	Schedule Schedule

	// 任务下次运行的时间
	Next time.Time

	// 上次运行的时间
	Prev time.Time

	// 要运行的具体任务
	WrappedJob Job

	// 前台提交的任务
	Job Job
}

type Schedule interface {
	Next(time.Time) time.Time
}

type Job interface {
	Run()
}
```

可以看出要执行的任务是一个包含Run方法的接口，这就大大的增加了用户的自定义任务需求。

### 2.2 AddFunc

AddFunc 将一个函数添加到 Cron 中，以便按照指定的计划执行。规则将使用此 Cron 实例的时间区作为默认值进行解析。返回一个不可见的 ID，可用于稍后删除该函数。

```go
func (c *Cron) AddFunc(spec string, cmd func()) (EntryID, error) {
	return c.AddJob(spec, FuncJob(cmd))
}
```

其中FuncJob是一个函数类型适配器，用来转换成项目内部的job接口。

```go
type FuncJob func()

func (f FuncJob) Run() { f() }
```

```go
func (c *Cron) AddJob(spec string, cmd Job) (EntryID, error) {
	schedule, err := c.parser.Parse(spec)
	if err != nil {
		return 0, err
	}
	return c.Schedule(schedule, cmd), nil
}

// Schedule adds a Job to the Cron to be run on the given schedule.
// The job is wrapped with the configured Chain.
func (c *Cron) Schedule(schedule Schedule, cmd Job) EntryID {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()
	c.nextID++
	entry := &Entry{
		ID:         c.nextID,
		Schedule:   schedule,
		WrappedJob: c.chain.Then(cmd),
		Job:        cmd,
	}
	if !c.running {
		c.entries = append(c.entries, entry)
	} else {
		c.add <- entry
	}
	return entry.ID
}
```

核心逻辑就两步：

1. 加锁生成ID并包装：互斥锁保护并发安全，生成唯一ID，并将用户任务传入责任链（Chain）进行装饰增强（如添加日志、异常恢复）。

2. 根据运行状态分流：若调度器未启动，直接将任务追加到切片缓存；若已启动，则将任务发送到通道，避免阻塞主循环，实现运行时动态热更新。

### 2.3 Start

启动Cron调度器

```go
func (c *Cron) Start() {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()
	if c.running {
		return
	}
	c.running = true
	go c.run()
}
```

run() 是 cron 调度器的核心方法，它在一个独立的 goroutine 中运行，负责调度和执行所有定时任务。

```go
func (c *Cron) run() {
	c.logger.Info("start")

	// Figure out the next activation times for each entry.
	now := c.now()
	for _, entry := range c.entries {
		entry.Next = entry.Schedule.Next(now)
		c.logger.Info("schedule", "now", now, "entry", entry.ID, "next", entry.Next)
	}

	for {
		// Determine the next entry to run.
		sort.Sort(byTime(c.entries))

		var timer *time.Timer
		if len(c.entries) == 0 || c.entries[0].Next.IsZero() {
			// If there are no entries yet, just sleep - it still handles new entries
			// and stop requests.
			timer = time.NewTimer(100000 * time.Hour)
		} else {
			timer = time.NewTimer(c.entries[0].Next.Sub(now))
		}

		for {
			select {
			case now = <-timer.C:
				now = now.In(c.location)
				c.logger.Info("wake", "now", now)

				// Run every entry whose next time was less than now
				for _, e := range c.entries {
					if e.Next.After(now) || e.Next.IsZero() {
						break
					}
					c.startJob(e.WrappedJob)
					e.Prev = e.Next
					e.Next = e.Schedule.Next(now)
					c.logger.Info("run", "now", now, "entry", e.ID, "next", e.Next)
				}

			case newEntry := <-c.add:
				timer.Stop()
				now = c.now()
				newEntry.Next = newEntry.Schedule.Next(now)
				c.entries = append(c.entries, newEntry)
				c.logger.Info("added", "now", now, "entry", newEntry.ID, "next", newEntry.Next)

			case replyChan := <-c.snapshot:
				replyChan <- c.entrySnapshot()
				continue

			case <-c.stop:
				timer.Stop()
				c.logger.Info("stop")
				return

			case id := <-c.remove:
				timer.Stop()
				now = c.now()
				c.removeEntry(id)
				c.logger.Info("removed", "entry", id)
			}

			break
		}
	}
}

func (c *Cron) startJob(j Job) {
	c.jobWaiter.Add(1)
	go func() {
		defer c.jobWaiter.Done()
		j.Run()
	}()
}
```

详细流程解析：

1. 初始化：
   1. 记录启动日志
   2. 获取当前时间
   3. 遍历已添加的任务，计算第一次执行时间
   4. 记录每个任务的调度信息
2. 主调度循环：这是一个无限循环，直到停止信号
   1. 对任务进行排序，按 Next 时间 从小到大 排序
   2. 设置定时器
      1. 没有任务或第一个任务就已经是zero了，设置一个超长定时器
      2. 有任务，就设置为距离第一个任务的时间差
   3. 事件处理
      1. 任务执行：遍历所有任务（前面已经排序了），将已经到期可以执行的任务开始执行
      2. 添加任务：
         1. 停止当前定时器
         2. 获取当前时间
         3. 计算新任务的首次执行时间
         4. 将新任务添加到列表
         5. 跳出内层循环，重新进入外层循环排序和设置新定时器
      3. 获取任务快照：
      4. 停止事件：
      5. 移除任务


同时job在运行时，使用过waitgroup来进行跟踪的，在调度器stop时起到作用，可以让调用者等待所有的任务执行完成后再退出程序，避免程序被中断。

```go
c := cron.New()
c.AddFunc("* * * * *", func() {
    // 某个耗时较长的任务，比如：
    time.Sleep(30 * time.Second)
    fmt.Println("任务完成")
})
c.Start()

// ... 运行一段时间后 ...

ctx := c.Stop()  // 停止调度器
<-ctx.Done()     // 等待所有正在运行的任务完成
fmt.Println("所有任务都完成了，可以安全退出程序")
```


### 2.4 WrappedJob

```go
type JobWrapper func(Job) Job

// Chain is a sequence of JobWrappers that decorates submitted jobs with
// cross-cutting behaviors like logging or synchronization.
type Chain struct {
	wrappers []JobWrapper
}

// NewChain returns a Chain consisting of the given JobWrappers.
func NewChain(c ...JobWrapper) Chain {
	return Chain{c}
}

// Then decorates the given job with all JobWrappers in the chain.
//
// This:
//     NewChain(m1, m2, m3).Then(job)
// is equivalent to:
//     m1(m2(m3(job)))
func (c Chain) Then(j Job) Job {
	for i := range c.wrappers {
		j = c.wrappers[len(c.wrappers)-i-1](j)
	}
	return j
}
```

这是**装饰器模式**的经典实现，同时也融合了 **责任链模式**的思想。

✅ 灵活组合 ：可以随意搭配装饰器

✅ 开闭原则 ：新增装饰器无需修改现有代码

✅ 单一职责 ：每个装饰器只关注一个功能

✅ 可复用 ：装饰器可以在多个 job 间复用

本项目有两个自带的装饰器，主要作用是：

- 如果上一次执行还没完成，直接跳过本次执行 ，不会等待。

- 确保同一个 job 不会并发执行 ，如果上一次执行还没完成，后续触发的执行会 等待 ，直到上一次完成。

```go
func SkipIfStillRunning(logger Logger) JobWrapper {
    return func(j Job) Job {
        // 1. 创建一个缓冲大小为 1 的通道（作为"令牌"）
        var ch = make(chan struct{}, 1)
        
        // 2. 预先放入一个"令牌"
        ch <- struct{}{}
        
        return FuncJob(func() {
            // 3. 尝试获取令牌（非阻塞）
            select {
            // 情况 A：成功获取到令牌
            case v := <-ch:
                // 4. 任务执行完后，把令牌放回去
                defer func() { ch <- v }()
                // 5. 执行原始 job
                j.Run()
            
            // 情况 B：获取不到令牌（通道为空，说明有人在用）
            default:
                // 6. 记录跳过日志
                logger.Info("skip")
            }
        })
    }
}
```


```go
func DelayIfStillRunning(logger Logger) JobWrapper {
    return func(j Job) Job {
        // 1. 创建一个互斥锁（每个 job 实例都有自己的锁）
        var mu sync.Mutex
        
        return FuncJob(func() {
            // 2. 记录尝试获取锁的时间
            start := time.Now()
            
            // 3. 获取锁
            //    - 如果锁空闲：立即获取，继续执行
            //    - 如果锁被占用：阻塞等待，直到锁释放
            mu.Lock()
            
            // 4. 确保函数退出时释放锁
            defer mu.Unlock()
            
            // 5. 检查是否等待了超过 1 分钟
            if dur := time.Since(start); dur > time.Minute {
                logger.Info("delay", "duration", dur)
            }
            
            // 6. 执行原始 job
            j.Run()
        })
    }
}
```

