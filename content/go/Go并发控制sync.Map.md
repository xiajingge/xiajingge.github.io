+++
date = '2025-06-18T21:16:43+08:00'
title = "Go并发控制:sync.Map"
tags = ["go","并发安全"]
categories = ["go"]
+++

我们之前讨论过[如果实现一个并发安全的map](./Go并发编程:实现一个并发安全的map.md),其中主要讲了使用读写锁,分片map以及使用channel的并发map,但其实我们熟知的其实是Go本身提供的并发原语`sync.Map`.

## 1. 使用示例

```go
package main

import (
	"fmt"
	"sync"
)

func main() {
	var s sync.Map

	// 存储键值对
	s.Store("name", "xiaoming")
	s.Store("age", 20)
	s.Store("location", "Beijing")

	// 读取值
	if value, ok := s.Load("name"); ok {
		fmt.Println("name:", value)
	}

	// 删除一个键
	s.Delete("age")

	// 遍历 sync.Map
	s.Range(func(key, value interface{}) bool {
		fmt.Printf("%s: %s\n", key, value)
		return true // 继续遍历
	})
}
```

`sync.Map` 是一个结构体，和其他常见的并发原语一样零值可用。

- `Store 方法`用于存储一个键值对

- `Load 方法`根据给定的键读取对应的值

- `Delete 方法`则可以删除一个键值对

- `Range 方法`则用于遍历 sync.Map 中存储的所有键值对。

## 2.深入剖析

我们可以在 [sync.Map 文档](https://pkg.go.dev/sync@go1.23.0#Map)中看到其定义和实现的 exported 方法：

```go
type Map
    func (m *Map) Clear()
    func (m *Map) CompareAndDelete(key, old any) (deleted bool)
    func (m *Map) CompareAndSwap(key, old, new any) (swapped bool)
    func (m *Map) Delete(key any)
    func (m *Map) Load(key any) (value any, ok bool)
    func (m *Map) LoadAndDelete(key any) (value any, loaded bool)
    func (m *Map) LoadOrStore(key, value any) (actual any, loaded bool)
    func (m *Map) Range(f func(key, value any) bool)
    func (m *Map) Store(key, value any)
    func (m *Map) Swap(key, value any) (previous any, loaded bool)
```

### 2.1 核心结构体

```go
type Map struct {
    mu   sync.Mutex               // 保护 dirty map 的互斥锁
    read atomic.Pointer[readOnly] // 只读 map，原子操作无需加锁
    dirty map[any]*entry          // 可变 map，包含所有数据，操作时需要加锁
    misses int                    // 记录 read map 的 miss 计数
}

type readOnly struct {
    m       map[any]*entry // 存储数据的只读 map
    amended bool           // 是否有新的 key 只存在于 dirty map
}

type entry struct {
    p atomic.Pointer[any] // 存储值的指针，支持原子操作
}

var expunged = new(any) // 用于标记从 dirty map 中被删除的元素
```

我们可以粗略的将 sync.Map 的设计类比成`缓存系统`: 当我们要读取 map 中某个 key 对应的值时，sync.Map 会优先从缓存中读取，如果缓存未命中，则继续从 DB 中读取。sync.Map 还会在特定条件满足的情况下同步缓存和 DB 中的数据。

有了缓存系统的类比，再来看 sync.Map 的源码，就更容易理解了。

- `Map`
    - `mu 字段`是一个互斥锁，用于保护对 dirty 属性的操作

    - `read 字段`是一个**原子类型的指针**，指向 readOnly 结构体，可以将 read 类比为缓存
        - `m` 使用 map 来存储数据
        - `amended` 用于标记是否有新的 key 只存在于 dirty 中，而不在 read 中

    - `dirty 字段`是一个 map 类型，可以类比为缓存系统中的 DB，所以理论上 dirty 中的数据应该是全量的

    - `misses 字段`则用于计数，记录缓存未命中的次数

- entry
    - p 值为一个对象的指针时，表示正常状态，即这个键值对正常存在于 sync.Map 中
    - p 值为 nil 时，表示这个键值对已经被删除
    - p 值为 expunged 时，表示这个键值对已经被彻底删除

> 关于entry的p值的两个删除状态，简单的将 nil 类比为数据库中的逻辑删除，expunged 理解为物理删除

当我们要读取 map 中某个 key 对应的值时

- 优先从 `read` 读取，
    - `read.amended` 则决定了是否需要继续从 dirty 中读取，`read.amended` 为 false 时，表示 `read.m` 和 `dirty` 中的数据一致，所以无需继续从 dirty 中读取；
    - `read.amended` 为 true 时，表示 `read.m` 中的数据要落后于 `dirty`，`read.m` 存在数据缺失，所以可以尝试继续从 dirty 中读取

- 如果 `read` 中不存在且`read.amended` 为 true, 从 `dirty` 中读取，

- 不管在`dirty`中有没有找到, `misses` 值都加 1

- 当 `misses` 值达到某个阈值时，`sync.Map` 就会将 `dirty` 提升为 `read`。

### 2.2 核心方法

#### 2.2.1 Store

```go
func (m *Map) Store(key, value any) {
	_, _ = m.Swap(key, value)
}
```

`Store` 方法实际上会将操作代理到 `Swap` 方法。

`Swap` 方法能够交换给定 key 对应的值，并返回之前的旧值（如果存在的话），返回值 loaded 表示 key 是否存在。即 Swap 用于存储键值对，它可以设置一个新的键值对，也可以更新一个已存在的键值对。

```go
func (m *Map) Swap(key, value any) (previous any, loaded bool) {
	// 先尝试从 read map 获取 key 对应的 entry
	read := m.loadReadOnly()
	if e, ok := read.m[key]; ok { // 如果 key 存在
		// 尝试交换值
		if v, ok := e.trySwap(&value); ok { // 如果交换成功
			if v == nil { // 说明已被删除，但没有彻底删除（expunged）
				return nil, false // 新增键值对成功
			}
			return *v, true // 交换成功
		}
	}

	// 未找到或需要修改 dirty map，需要加锁处理
	m.mu.Lock()
	read = m.loadReadOnly()
	if e, ok := read.m[key]; ok { // 如果 key 在 read map 中
		if e.unexpungeLocked() {
			// 如果 entry 之前被 expunged，意味着 dirty map 不为 nil 且该 entry 不在 dirty map 中
			m.dirty[key] = e
		}
		// 进行值交换
		if v := e.swapLocked(&value); v != nil {
			loaded = true
			previous = *v
		}
	} else if e, ok := m.dirty[key]; ok { // 如果 key 在 dirty map 中
		// 进行值交换
		if v := e.swapLocked(&value); v != nil {
			loaded = true
			previous = *v
		}
	} else { // key 即不在 read map 中，也不在 dirty map 中
		if !read.amended {
			// 添加第一个新 key 到 dirty map，需要先标记 read map 为不完整
			m.dirtyLocked()
			m.read.Store(&readOnly{m: read.m, amended: true})
		}
		// 在 dirty map 中创建新 entry
		m.dirty[key] = newEntry(value)
	}
	m.mu.Unlock()
	return previous, loaded
}
```

**流程介绍:**

1. `Swap` 方法会先尝试从 `read` 中获取给定 `key` 对应的值 `entry`，此时无需加锁。

2. 如果`key 存在`，说明是 `更新操作`，那么调用 `e.trySwap(&value)` 尝试交换 key 对应的值。
    - 如果 `ok` 为 true, 说明交换成功，那么此时有两种情况:
        - 如果 `v` 等于 nil, 说明这个 key 以前存在过，但是已经被删除了，不过还没有被彻底删除（expunged），此时返回的 loaded 值为 false 标识 key 不存在，是新增键值对成功；
        - 如果 `v` 的值不为 nil，说明 key 存在，是更新操作，返回的 loaded 值设为 true。
    - 如果 `ok` 为 false，说明这个 key 以前存在过，但是已经被彻底删除了（expunged），交换失败，程序继续往下执行。

3. `key 不存在`,对 dirty 进行操作，所以需要加锁

4. 调用 `m.loadReadOnly()` 重新加载 `read`，加锁后再看一次 read，避免锁等待期间 dirty 已经被提升为 read

    - 调用 e.unexpungeLocked() 能确保 entry 未被标记为 expunged
    - 如果这次检查发现 key 在 read 中，那么调用 `e.swapLocked(&value)` 将 entry 的值更新成新的值

5. 如果 key 不在 read 中，而在 dirty 中存在，那么直接将其更新成新的值

6. 如果 key 即不在 read 中，也不在 dirty 中，则说明是`新插入的键值对`
    - 在 dirty 中创建并保存新的 entry
    - 这里有一个判断 `if !read.amended`
        - 如果成立，即 read.amended 值为 false，意味着现在的 read 中数据是完整的，此时 dirty 有可能为 nil,调用 `m.dirtyLocked()` 能够确保 dirty 不为 nil，如果为 nil 则将其初始化，并且这里会将 read.amended 值标记为 true，即标记 read.m 是不完整的（因为这里只将 newEntry(value) 赋值给了 dirty，并没有赋值给 read）

7. 键值对就一定被存储到了 sync.Map 中，调用 m.mu.Unlock() 解锁，并返回存储键值对的结果。

> 这里也可以发现，read 其实并不完全是“只读”，在更新的情况下，其实是可以写的（调用 e.trySwap(&value) 会直接修改 read.m 中的值，read 真正代表的其实是无锁操作，只有修改 dirty 的时候才需要加锁。

接下来我们再看下 Swap 方法中的几个依赖方法是如何实现的。

```go
func (m *Map) loadReadOnly() readOnly {
	if p := m.read.Load(); p != nil {
		return *p
	}
	return readOnly{}
}
```
- `read` 属性是 atomic.Pointer 类型，其 Load 方法用于加载指针对象，*p 则可以获取指针中的值。

```go
func (e *entry) trySwap(i *any) (*any, bool) {
	for {
		p := e.p.Load()
		if p == expunged {
			return nil, false
		}
		if e.p.CompareAndSwap(p, i) {
			return p, true
		}
	}
}
```
调用 `e.trySwap(&value)` 方法可以尝试将 entry 中的值更新

- 如果 entry 的值为 expunged，说明此 entry 已被彻底删除，那么无法交换其值

- 如果允许交换，调用 CompareAndSwap 方法可以原子的交换值，交换成功返回 true

```go
func (e *entry) unexpungeLocked() (wasExpunged bool) {
	return e.p.CompareAndSwap(expunged, nil)
}
```
- 这里仅有一行代码，如果 entry 的值为 expunged，则将其交换为 nil

```go
func (e *entry) swapLocked(i *any) *any {
	return e.p.Swap(i)
}
```
- 调用 e.swapLocked(&value) 方法可以更新 entry 对象中的值


```go
func (m *Map) dirtyLocked() {
	if m.dirty != nil {
		return
	}

	read := m.loadReadOnly()
	m.dirty = make(map[any]*entry, len(read.m))
	for k, e := range read.m {
		if !e.tryExpungeLocked() {
			m.dirty[k] = e
		}
	}
}

func (e *entry) tryExpungeLocked() (isExpunged bool) {
	p := e.p.Load()
	for p == nil {
		if e.p.CompareAndSwap(nil, expunged) {
			return true
		}
		p = e.p.Load()
	}
	return p == expunged
}
```

- 如果 `dirty` 不为 nil，则无需处理，直接返回。**至于 `dirty` 何时会为 nil 呢？这里留个疑问，后续阅读 Load 方法源码时，你就知道了**

- 否则，需要初始化一个新的 dirty，然后全量遍历 read.m,`*entry.tryExpungeLocked` 方法的作用是过滤掉值为 nil 或 expunged 的 entry，即已经标记为删除的键值对。

`sync.Map` 中一个值的`删除过程`是，如果在 `read` 中先置为了 `nil`，之后在某次从 `read` 复制全量数据到 `dirty` 时，会将其置为 `expunged` 状态，并且不会被拷贝到 `dirty`。所以此时 `read` 和 `dirty` 中就存在数据不一致了，在 `read` 中这个值被标记为彻底删除 `expunged`，在 `dirty` 没有此键值对。所以我们说：如果某个 `entry 对象`的值被标记为 `expunged`，那么它一定是在 `read` 中有，而在 `dirty` 中没有。

所以 `e.unexpungeLocked()` 方法和 `e.tryExpungeLocked()` 方法其实是有联系的，二者是相反操作，它们通过修改 `entry` 的值来标识一个键值对的状态。其实 `sync.Map` 之所以这么设计，也是为了**减少 entry 被彻底删除的可能性**，我们可以认为，某个键值对被加入进来，之后删除掉，那么很有可能下次还会被重新加入进来，所以才搞了个 nil 作为逻辑删除，而非彻底从 map 中删除，也许可以救一下。而 expunged 则用来处理某个被删除的对象在 read 中存在，而在 dirty 中不存在的情况，当读取 read 发现其已经被标记为彻底删除，那么就没比较加锁再去 dirty 中查找了。

并且，我们还可以发现，因为调用 `dirtyLocked` 可能全量遍历 `read.m`，所以就可能会导致 `sync.Map` 的性能退化。

```go
func newEntry(i any) *entry {
	e := &entry{}
	e.p.Store(&i)
	return e
}
```

- 调用 `newEntry(value)` 可以创建一个新的 entry 对象

至此，Store 方法涉及的所有源码，就都讲解清楚了。

需要特别强调的是，以上提到的方法中，带有 Locked 结尾的方法，都需要外部持有锁才可以调用，以保证并发安全。

#### 2.2.2 Load

`Load` 方法用于从 sync.Map 中读取给定 `key` 对应的值，如果 `key` 不存在，则返回值为 `nil`，且第二个返回值 `ok` 为 `false`

```go
func (m *Map) Load(key any) (value any, ok bool) {
	read := m.loadReadOnly() // 加载只读 map
	e, ok := read.m[key]     // 尝试从 read map 中查找 key

	// 如果 read map 中找不到 key，并且 dirty map 可能包含新键（amended 为 true），则进入慢路径
	if !ok && read.amended {
		m.mu.Lock() // 操作 dirty 需要加锁
		// 在获取锁的过程中，dirty 可能已经被提升为 read，因此需要重新加载 read map 进行 double-checking
		read = m.loadReadOnly()
		e, ok = read.m[key]
		if !ok && read.amended {
			// 如果在 read map 仍然找不到，则尝试从 dirty map 查找
			e, ok = m.dirty[key] // 从 dirty map 中查找 key
			// 无论 key 是否存在，记录一次 miss：
			// 在 dirty map 提升为 read map 之前，这个 key 的查询都会走慢路径
			m.missLocked() // miss 计数值 + 1
		}
		m.mu.Unlock()
	}

	// 如果最终 key 仍然不存在，则返回 nil
	if !ok {
		return nil, false
	}
	return e.load() // 返回找到的值
}
```

1. 同 Store 方法一样，Load 方法也会优先加载只读的 map 对象 read，并尝试从 read 中查找 key。
	- 如果 ok 说明找到了，那么可以直接调用 e.load() 加载并返回找到的值。这是一个快速路径，不需要加锁。
	- 如果 !ok 说明没找到，并且 read.amended 为 true 时，说明 dirty 中可能包含新的键值对，则需要加锁，进入到慢路径查找

2. 先进行 double-checking，在获取锁的过程中，dirty 可能已经被提升为 read，因此需要重新加载 read。如果这次检查 read 还不满足，则尝试从 dirty 中查找，并且 misses 计数值加 1

3. 查找结束后进行解锁操作，然后根据 ok 的值返回结果，如果 !ok 说明没找到，返回 nil。否则说明查找到了 key，调用 e.load() 返回找到的值。

```go
func (m *Map) missLocked() {
	m.misses++
	if m.misses < len(m.dirty) {
		return
	}
	m.read.Store(&readOnly{m: m.dirty})
	m.dirty = nil
	m.misses = 0
}
```

可以发现，当 misses 的次数达到 dirty 中全量数据的长度时,dirty 会被提升为 read.**所以 misses 的阈值就是 dirty 中全量数据的长度.**

这里直接使用 dirty 作为 readOnly 的 m 属性值，这一步相当于**刷新缓存**，更新成最新数据，并且 dirty 被置为了 nil，misses 计数值清零。

所以这里也解释了前文中的疑问，dirty 何时会为 nil 呢？就是在此时。那么这个目的又是什么呢？还记得前文中讲过调用 `m.dirtyLocked()` 方法时，如果 dirty 值为 nil，则会遍历 read.m 的数据，然后过滤掉被删除的值，将正常值存入 dirty 吗，这就是为了有一个机会，能够清理被标记删除的对象，以防止 sync.Map 无限增长。

这里有个细节，构造 readOnly 对象时 amended 没有赋值，默认值为 false，标识 read 和 dirty 现在数据是等价的，read 是全量数据,其实 dirty 中此时数据为 nil，所以此时以 read 数据为准。后续查找 key 时，如果 read 中没找到，就无需再加锁继续查找 dirty 了。

```go
func (e *entry) load() (value any, ok bool) {
	p := e.p.Load()
	if p == nil || p == expunged {
		return nil, false
	}
	return *p, true
}
```
- 调用 e.load() 能够加载 entry 对象中记录的具体值,其实就是从 atomic.Pointer 类型中获取包含的值。

#### 2.2.3 Delete

`Delete` 方法用于从 sync.Map 中删除指定的键值对。

```go
func (m *Map) Delete(key any) {
	m.LoadAndDelete(key) // 直接调用 LoadAndDelete 进行删除，忽略其返回值
}
```

在 Delete 方法内部，直接调用了 LoadAndDelete 方法。LoadAndDelete 方法会删除 key 对应的值，并返回删除前的值（如果存在），第二个返回值 loaded 标识该 key 是否存在

```go
func (m *Map) LoadAndDelete(key any) (value any, loaded bool) {
	// 先从 read map 读取，避免加锁
	read := m.loadReadOnly() // 加载只读 map
	e, ok := read.m[key]
	if !ok && read.amended {
		// 若 read map 没有 key，且 dirty map 可能包含该 key，则加锁检查 dirty map
		m.mu.Lock()
		read = m.loadReadOnly() // double-checking
		e, ok = read.m[key]
		if !ok && read.amended {
			// 在 dirty map 查找 key
			e, ok = m.dirty[key]
			delete(m.dirty, key) // 删除 dirty map 中的 key
			// 记录一次 miss，表示该 key 走了慢路径，直到 dirty map 提升为 read map
			m.missLocked()
		}
		m.mu.Unlock()
	}
	if ok {
		// 若 key 存在，调用 entry.delete() 删除值并返回
		return e.delete()
	}
	return nil, false // read map 和 dirty map 都无此 key
}

func (e *entry) delete() (value any, ok bool) {
	for {
		p := e.p.Load()
		// nil | expunged 说明该 key 已经被删除了
		if p == nil || p == expunged {
			return nil, false
		}
		// 通过CAS操作对其进行删除
		if e.p.CompareAndSwap(p, nil) {
			return *p, true
		}
	}
}
```

优先加载read, 如果read中没有,同时dirty中可能有,那就加锁检查dirty,同时进行了doublecheck,删除 dirty 中的 key，这也正是 Load 和 Delete 两个操作。此外，这里还会记录一次 misses

注意这里的删除只会将 entry 中存储的值置为 nil，并不会彻底删除 entry 对象。

#### 2.2.4 Range

`Range` 方法可以遍历 sync.Map 中所有的键值对，并依次调用给定的 f(key, value) 函数，如果 f 返回 false，则停止遍历

```go
func (m *Map) Range(f func(key, value any) bool) {
	// 读取当前的 read map
	read := m.loadReadOnly()

	// 如果 read map 是完整的（未修改），直接遍历
	if read.amended {
		// 若 read.amended == true，说明 dirty map 存在新增 key，需要提升为 read
		m.mu.Lock()
		read = m.loadReadOnly()
		if read.amended {
			// 提升 dirty map 为 read
			read = readOnly{m: m.dirty}
			copyRead := read
			m.read.Store(&copyRead)
			// 清空 dirty map 并重置 miss 计数
			m.dirty = nil
			m.misses = 0
		}
		m.mu.Unlock()
	}

	// 遍历 read map
	for k, e := range read.m {
		v, ok := e.load()
		if !ok {
			continue
		}
		// 调用用户提供的函数 f
		if !f(k, v) {
			break // f 返回 false 时停止遍历
		}
	}
}
```

对于 Range 操作来说，只需要遍历 map，即这是一个读取操作，所以会读取当前的 read 值，如果 read 是完整的（read.amended == false），那么可以直接遍历 read，并且每次 for 循环中都会调用用户提供的函数 f，如果 f 返回 false，则停止遍历。

如果 read.amended 值为 true，说明 dirty 中存在新增的 key，那么就将其提升为 read，然后再进行 read 遍历。因为需要操作 dirty，所以会加锁，并且进行了 double-checking,在提升 dirty 为 read 后，会清空 dirty 并重置 misses 计数。所以说调用 Range 方法也是一个可能导致后面 sync.Map 会出现性能退化的原因。


#### 2.2.5 Clear

`Clear` 方法会删除所有键值对，使 sync.Map 变为空

```go
func (m *Map) Clear() {
	read := m.loadReadOnly() // 加载只读 map
	// 如果 read map 为空且 dirty map 也未修改（amended 为 false），则无需执行清理
	if len(read.m) == 0 && !read.amended {
		// 如果 map 已经为空，则避免分配新的 readOnly。
		return
	}

	m.mu.Lock() // 加锁
	defer m.mu.Unlock()

	// double-checking
	// 重新加载 read map，避免在获取锁的过程中发生变化
	read = m.loadReadOnly()
	if len(read.m) > 0 || read.amended {
		// 清空 read map，存储一个新的空 readOnly 结构
		m.read.Store(&readOnly{})
	}

	// 清空 dirty map
	clear(m.dirty)
	// 复位 miss 计数，防止下一次操作时刚清空的 dirty map 立即被提升到 read map
	m.misses = 0
}
```

## 3. 总结

`sync.Map` 的设计思路，可以将其类比为缓存系统，主体逻辑是，**所有方法都优先尝试无锁操作 read，如果 read 条件不满足，再尝试加锁操作 dirty。**

而 sync.Map 之所以能够实现并发安全，其核心设计逻辑可以总结为以下几个关键点：

- **读写分离的双map结构**

	类似“缓存系统”的思路：

	- read（只读 map/缓存）： 指向一个 readOnly 结构体。对它的读取操作是完全无锁（Lock-Free）的，通过原子操作（atomic 指针）来保证并发安全。

	- dirty（可变 map/数据库）： 包含全量数据的传统 map。对它的任何读写、更新或新插入操作，都必须先获取互斥锁（mu sync.Mutex）

	这种结构使得大量的纯读取请求可以直接在无锁的 read 中命中，极大地减少了多线程竞争锁的开销。

- **原子操作和CAS**

	当需要更新一个已经存在的键值对时，即使这个键在 read 中，sync.Map 也不会直接加锁修改。

	- 它会调用 `trySwap` 方法，利用底层的 CAS（无锁原子比较并交换） 机制去尝试更新 entry 里的值。

	- 因为 read.m 和 dirty 中相同的 key 实际上指向的是同一个 entry 内存对象指针，所以通过原子操作直接修改 read 的同时，dirty 里的数据也会同步更新，整个过程无需触及互斥锁。

- **双重检查机制**

	在涉及必须加锁的“慢路径”（比如 read 中没找到某个 key，需要去 dirty 中查找、或者准备新插入键值对时），sync.Map 采用了经典的双重检查来确保并发安全：

	1. 第一次检查： 在无锁状态下读取 read，发现没有命中。

	2.加锁： 获取 m.mu.Lock()。

	3. 第二次检查： 重新加载（Reload） 一次 read。因为在多线程环境下，从你第一次检查到你成功拿到锁的这段间隙里，可能另外一个 Goroutine 已经把 dirty 提升为了 read，或者别人已经刚把这个 key 存进去了。

	4. 只有在第二次检查依然未命中的情况下，才会真正去操作 dirty。这有效地防止了重复写入或读到脏数据。

- **entry 的状态机标记（逻辑删除与物理删除）**

	为了在无锁和有锁之间安全地安全切换，sync.Map 里的每一个 value 都被包装成了一个 entry 结构体，利用其内部指针 p 的不同状态来充当状态机

	- 正常状态： p 指向具体的 Value 内存地址。

	- nil（逻辑删除）： 当调用 Delete 时，如果在 read 中找到了，它只会用 CAS 将其原子的置为 nil，而不会直接从 map 中抹去。这保证了其他并发读取的 Goroutine 不会因为找不到指针而崩溃。

	- expunged（彻底/物理删除）： 当 dirty 为 nil 需要重新从 read 复制数据初始化时，被标记为 nil 的垃圾数据会被转换为 expunged 状态，并且不会被复制到新的 dirty 中。这时候，read 有这个 key 而 dirty 没有，从而保持了边界的绝对清晰。
