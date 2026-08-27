+++
date = '2025-06-18T19:23:48+08:00'
title = "Go并发编程:实现一个并发安全的map"
tags = ["go","并发安全"]
categories = ["go"]
+++

## 1 内置map
在实现一个并发安全的map之前,我们需要了解一下内置map为什么不是并发安全的.
```go
package main

import (
	"context"
	"fmt"
	"time"
)

// NOTE: Go 内置 map 不支持并发读写

func main() {
	m := make(map[string]int)

    ctx, cancel := context.WithCancel(context.Background())
	// 并发读写 map
	go func() {
		for {
            select {
            case <- ctx.Done():
                fmt.Println("writer exiting")
                return
            default:
                m["k"] = 1
			    fmt.Println("set k:", 1)
            }
		}
	}()

	go func() {
		for {
            select {
            case <- ctx.Done():
                fmt.Println("reader exiting")
                return
            default:
                v, _ := m["k"]
			    fmt.Println("read k:", v)
            }
		}
	}()

    // 让程序运行 10 秒后终止所有 goroutine
    time.Sleep(10 * time.Second)
    cancel()      // 发送取消信号
}
```

这里开了两个 goroutine 分别用来读写 map 对象变量 m，并且每个 goroutine 中都采用无限循环来不停的读写 m，这样就会造成大量并发操作。主要报错信息为 `fatal error: concurrent map read and map write`。这提示我们存在并发读写 map 的操作，所以说 Go 内置 map 不是并发安全的。

## 2. 实现并发安全map

要实现 map 的并发安全，我们首先想到的就是互斥锁 sync.Mutex。不过 Go 语言还在 sync.Mutex 的基础上提供了读写锁 sync.RWMutex，能够更大的程度的提升读多写少场景的并发性能，所以我们选用读写锁来实现并发安全的 map。

```go
type RWMutexMap struct {
	rw sync.RWMutex
	m  map[string]int
}

func NewRWMutexMap() *RWMutexMap {
	return &RWMutexMap{
		m: make(map[string]int),
	}
}

func (m *RWMutexMap) Set(key string, v int) {
	m.rw.Lock()
	defer m.rw.Unlock()
	m.m[key] = v
}

func (m *RWMutexMap) Get(key string) (int, bool) {
	m.rw.RLock()
	defer m.rw.RUnlock()
	v, ok := m.m[key]
	return v, ok
}

func (m *RWMutexMap) Del(key string) {
	m.rw.Lock()
	defer m.rw.Unlock()
	delete(m.m, key)
}
```

这里有必要解释一下读锁和写锁的区别：

- `写锁`就是`互斥锁`，不可以被多个 goroutine 同时持有，比如在调用写锁 m.rw.Lock() 时，如果有读锁或写锁已经被其他 goroutine 持有，则当前 goroutine 会被阻塞。

- `读锁`类似`可重入锁`，多个 goroutine 可以同时持有读锁，比如在调用读锁 m.rw.RLock() 时，如果有其他 goroutine 也持有读锁，则当前 goroutine 还可以继续获取这把读锁，但此时如果有 goroutine 尝试获取写锁则会被阻塞。

所以说`读写锁`对`读多写少`的场景更加适用，读写锁允许多个 goroutine 同时持有读锁，但只允许一个 goroutine 持有写锁。

我们可以写出同样测试代码，来测试这个使用读写锁实现的并发安全 map 的效果：
```go
func TestRWMutexMap(t *testing.T) {
	m := NewRWMutexMap()

	// 并发读写 map
	go func() {
		for {
			m.Set("k", 1)
			fmt.Println("set k:", 1)
		}
	}()

	go func() {
		for {
			v, _ := m.Get("k")
			fmt.Println("read k:", v)
		}
	}()

	select {}
}
```
这个示例不会终止，会交替打印 read 和 set 的日志。

没错，实现一个并发安全的 map 就是如此的简单，这么一对比，sync.Map 的实现确实很复杂。

## 3. 分片map

我们知道加锁会使程序的性能急剧下降，读写锁能够在读多写少的场景中降低互斥锁对性能的影响。那么我们实现的并发安全 map 是否还有可优化空间呢？答案是肯定的。

我们可以采用`分片`的思想，将一个大的 map 拆成多个更小的 map，然后让每个小 map 持有各自的锁，以此来`减小锁的粒度`，减少并发情况下加锁的次数。

```go
package main

import (
	"hash/maphash"
	"sync"
)

var seed = maphash.MakeSeed()

func hashKey(key string) uint64 {
	return maphash.String(seed, key)
}

type ShardingMap struct {
	locks  []sync.RWMutex
	shards []map[string]int
}

func NewShardingMap(size int) *ShardingMap {
	sm := &ShardingMap{
		locks:  make([]sync.RWMutex, size),
		shards: make([]map[string]int, size),
	}
	for i := 0; i < size; i++ {
		sm.shards[i] = make(map[string]int)
	}
	return sm
}

func (m *ShardingMap) getShardIdx(key string) uint64 {
	hash := hashKey(key)
	return hash % uint64(len(m.shards))
}

func (m *ShardingMap) Set(key string, value int) {
	idx := m.getShardIdx(key)
	m.locks[idx].Lock()
	defer m.locks[idx].Unlock()
	m.shards[idx][key] = value
}

func (m *ShardingMap) Get(key string) (int, bool) {
	idx := m.getShardIdx(key)
	m.locks[idx].RLock()
	defer m.locks[idx].RUnlock()
	value, ok := m.shards[idx][key]
	return value, ok
}

func (m *ShardingMap) Del(key string) {
	idx := m.getShardIdx(key)
	m.locks[idx].Lock()
	defer m.locks[idx].Unlock()
	delete(m.shards[idx], key)
}
```

`ShardingMap` 表示一个采用分片机制的并发安全 map，它有两个属性 `locks` 和 `shards`，它们都是切片类型，`locks` 用来保存每个分片的读写锁，`shards` 用来保存每个分片的数据。

`getShardIdx` 方法非常重要，会被所有方法使用，它可以计算给定 `key` 所在的分片索引。首先它调用辅助函数 `hashKey` 对给定 `key`计算出一个 `hash` 值，然后用这个 `hash` 值对分片长度做取模运算，这样就能得到一个值必然会落到某个分片的索引。这也是分片 map 的精髓所在，给定一个 key 为其计算一个分片索引值，然后将其存入对应的分片，这个过程不用加锁。如果 `hashKey` 函数实现的足够好，那么每个分片中分布的数据量也是比较均匀的。

接下来就是 map 的基本操作方法 `Set`、`Get` 和 `Del` 的实现了，它们内部都会先调用 `m.getShardIdx(key)` 获取分片索引，然后对指定的分片进行加锁，最后再操作对应分片中的数据。这样，当多个 goroutine 并发读写 map 时，只要对应的 key 不在同一个分片中，那么就能降低加锁次数，从而提升程序性能。只有对应的 key 计算后在同一个分片中，多个 goroutine 之间才需要抢锁。

## 4. 利用channel实现的并发安全

关于Go 并发,大家可是更推荐使用 channel 而非互斥锁，那么我们何不用 channel 来实现一个并发安全 map 呢？

```go
type ChannelMap struct {
	cmd chan command
	m   map[string]int
}

type command struct {
	action string // "get", "set", "delete"
	key    string
	value  int
	result chan<- result
}

type result struct {
	value int
	ok    bool
}

func NewChannelMap() *ChannelMap {
	sm := &ChannelMap{
		cmd: make(chan command),
		m:   make(map[string]int),
	}
	go sm.run()
	return sm
}

func (m *ChannelMap) run() {
	for cmd := range m.cmd {
		switch cmd.action {
		case "get":
			value, ok := m.m[cmd.key]
			cmd.result <- result{value, ok}
		case "set":
			m.m[cmd.key] = cmd.value
		case "delete":
			delete(m.m, cmd.key)
		}
	}
}

func (m *ChannelMap) Set(key string, value int) {
	m.cmd <- command{action: "set", key: key, value: value}
}

func (m *ChannelMap) Get(key string) (int, bool) {
	res := make(chan result)
	m.cmd <- command{action: "get", key: key, result: res}
	r := <-res
	return r.value, r.ok
}

func (m *ChannelMap) Del(key string) {
	m.cmd <- command{action: "delete", key: key}
}
```

`ChannelMap` 结构体表示我们用 `channel` 实现的`并发安全 map`，它有两个属性:

- `cmd 字段`是一个 chan command 类型用来传递操作指令和操作结果
    - `action` 表示操作指令，比如 get、set 和 delete 操作
    - `key` 用来记录我们在调用 Set、Get 和 Del 方法时要操作的 map 中的 key
    - `value` 用来记录调用 Set 方法时指定的 value 值
    - `result` 用来记录调用 Set、Get 和 Del 方法时返回的结果。

- `m 字段`用来存储 map 数据。

核心就是通过 m.cmd 这个`无缓冲的 channel` 实现`并发控制`，来保证多个 goroutine 同时操作 map 时的并发安全。
