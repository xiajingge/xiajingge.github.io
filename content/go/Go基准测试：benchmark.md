+++
date = '2025-02-26T15:48:32+08:00'
title = "Go基准测试：benchmark"
tags = ["go", "基准测试"]
categories = ["go","后端开发"]

+++

# Go基准测试：benchmark

​	Go 语言的 **benchmark** 功能用于性能测试，它可以帮助测量和比较不同代码段的执行效率。Benchmark 测试不仅是为了测试程序的执行速度，还能帮助开发者优化代码。Go 提供了一个非常方便的内置工具来进行基准测试，它主要依赖于 `testing` 包中的 `Benchmark` 函数。

​	其实用简单的话将它的测试原理就是：测量一段代码执行 N 次所花的时间，从而得出单次操作的平均耗时。而这个次数N不需要自己设置，Go 会从一个小数字开始，逐步增大，直到运行时间足够长（默认至少 1 秒）以得到可靠结果。

## 1. 基础使用

### 1.1 编写Benchmark函数

一个基准测试函数的格式为：

```go
func BenchmarkXxx(b *testing.B) {
    for i := 0; i < b.N; i++ {
        // 被测代码
    }
}
```

Benchmark 函数必须遵循以下规则：

- 文件名以 `_test.go` 结尾
- 函数名以 `Benchmark` 开头
- 参数是 `*testing.B`（不是 `*testing.T`）
- 核心代码放在 `for i := 0; i < b.N; i++` 循环中

来举一个简单的例子来进行后续的说明。

```go
// math_test.go
package math

import "testing"

func Add(i,j int)int{
    return i + j
}

// 用包级变量接收结果
var result int
func BenchmarkAdd(b *testing.B) {
    var r int
    for i := 0; i < b.N; i++ {
        r = Add(1, 2) // 你要测试的函数
    }
    result = r // 赋值给包级变量，防止优化
}
```

> 避免编译器优化的陷阱
>
> Go 编译器可能会把"结果没被使用"的函数调用直接优化掉，导致 benchmark 结果不准确：
>
> ```go
> // 错误写法：结果可能被优化掉
> func BenchmarkBad(b *testing.B) {
>     for i := 0; i < b.N; i++ {
>         Add(1,2) // 返回值没用到，可能被优化
>     }
> }
> ```
>
> goos: windows
> goarch: amd64
> pkg: benchmark-dmeo
> cpu: 13th Gen Intel(R) Core(TM) i5-13400
> BenchmarkBad-8          1000000000               0.1262 ns/op          0 B/op          0 allocs/op
> BenchmarkAdd-8          1000000000               0.1735 ns/op          0 B/op          0 allocs/op

### 1.2 运行Benchmark

可以通过以下命令来运行基准测试：

```sh
go test -bench .
```

这会运行所有的基准测试并打印出结果。

也可以指定特定的基准测试函数，例如：

```sh
go test -bench BenchmarkAdd
```

### 1.3 结果分析

Go 会输出基准测试的执行时间和每次执行的平均时间，通常包含以下信息：

- **Ns/op**：每次操作的平均纳秒数。
- **B/op**：每次操作分配的字节数。
- **allocs/op**：每次操作的内存分配次数。

例如，运行基准测试后可能得到类似以下的输出：

```sh
goos: windows
goarch: amd64
pkg: benchmark-dmeo
cpu: 13th Gen Intel(R) Core(TM) i5-13400
=== RUN   BenchmarkAdd
BenchmarkAdd
BenchmarkAdd-16
1000000000               0.1292 ns/op          0 B/op          0 allocs/op
PASS
ok      benchmark-dmeo  0.542s
```

含义：

- `BenchmarkAdd-16：函数名，`16` 是使用的 CPU 核心数（GOMAXPROCS）
- `1000000000`：总共执行了 10 亿次
- `0.1292 ns/op`：每次操作耗时 0.1292 纳秒

- `0 B/op`：每次操作分配了 0 字节内存
- `0 allocs/op`：每次操作的内存分配次数

### 1.4 常用的参数

```sh
# 运行所有基准测试
go test -bench=.

# 指定运行时间（默认1秒），时间越长结果越稳定
go test -bench=. -benchtime=5s

# 指定固定迭代次数
go test -bench=. -benchtime=1000x

# 运行多轮取平均，减少波动
go test -bench=. -count=5

# 显示内存分配统计
go test -bench=. -benchmem

# 指定 CPU 核心数
go test -bench=. -cpu=1,2,4,8

# 运行名称中包含 "Map" 的基准测试
go test -bench=Map

# 运行特定的基准测试函数
go test -bench=BenchmarkMyFunc

# 精确匹配函数名（使用锚点）
go test -bench='^BenchmarkMyFunc$'
```

有时候 benchmark 前需要做一些准备工作（比如初始化数据），这些不应该被计入耗时

```go
func BenchmarkBigProcess(b *testing.B) {
    // 准备阶段
    data := makeHugeSlice()

    b.ResetTimer() // 重置计时器，忽略上面的准备时间

    for i := 0; i < b.N; i++ {
        process(data)
    }
}
```

还有更细粒度的控制：

```go
func BenchmarkWithSetup(b *testing.B) {
    for i := 0; i < b.N; i++ {
        b.StopTimer()  // 暂停计时
        data := setup() // 每次迭代的准备工作
        b.StartTimer() // 恢复计时

        process(data)
    }
}
```

不过 `StopTimer/StartTimer` 在循环内频繁调用会有额外开销，尽量用 `ResetTimer` 代替。



## 3. 高级用法

### 3.1 多基准测试

**多基准测试** 允许对同一功能的不同实现进行性能比较。这样，可以测试不同算法或不同代码路径的执行效率，从而选择最佳的实现。举个例子：

​	假设计算一个数组的总和。有两种实现方法，一个使用 `for` 循环，另一个使用 `range` 关键字。那就可以为这两种实现分别编写基准测试函数，这样可以直观地比较两者的性能。

```go
package main

import (
	"testing"
)

// 使用 for 循环的实现
func sumForLoop(arr []int) int {
	sum := 0
	for i := 0; i < len(arr); i++ {
		sum += arr[i]
	}
	return sum
}

// 使用 range 的实现
func sumRange(arr []int) int {
	sum := 0
	for _, v := range arr {
		sum += v
	}
	return sum
}

// 基准测试 for 循环的实现
func BenchmarkSumForLoop(b *testing.B) {
	arr := make([]int, 1000)
	for i := 0; i < len(arr); i++ {
		arr[i] = i
	}

	b.ResetTimer() // 只测试函数体的时间，不包括初始化
	for i := 0; i < b.N; i++ {
		sumForLoop(arr)
	}
}

// 基准测试 range 的实现
func BenchmarkSumRange(b *testing.B) {
	arr := make([]int, 1000)
	for i := 0; i < len(arr); i++ {
		arr[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sumRange(arr)
	}
}
```

你可以通过 `go test -bench` 命令运行这两个基准测试，并得到每个实现的性能对比：

```go
PS D:\test\benchmark-demo> go test -bench=^BenchmarkSum -benchmem                        
goos: windows
goarch: amd64
pkg: benchmark-dmeo
cpu: 13th Gen Intel(R) Core(TM) i5-13400
BenchmarkSumForLoop-16           8944896               130.8 ns/op             0 B/op          0 allocs/op
BenchmarkSumRange-16             8989322               131.4 ns/op             0 B/op          0 allocs/op
PASS
ok      benchmark-dmeo  3.005s
```

### 3.2 性能基准和负载测试

**性能基准和负载测试** 主要是用于测量系统或函数在高负载情况下的表现。Go 的基准测试也可以通过模拟压力或负载测试，来帮助你评估代码在处理大量数据时的表现，或者在多次调用时是否会出现性能瓶颈。

Go 的基准测试会自动调整每次测试的次数（`b.N`），使得测试能够运行足够多的次数以获得可靠的结果。而你也可以通过增加 `b.N` 的规模，来模拟负载的增加，从而评估在高负载下的性能。

举个例子：

假设你有一个函数用于计算斐波那契数列，你想要测试它在多次调用时的性能（例如，负载测试），你可以使用以下基准测试。

```go
package main

import (
	"testing"
)

// 计算斐波那契数列的实现
func fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	return fibonacci(n-1) + fibonacci(n-2)
}

// 基准测试斐波那契数列
func BenchmarkFibonacci(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// 每次测试斐波那契数列第 30 项
		fibonacci(30)
	}
}
```

**模拟负载测试：**可以通过调节 `b.N` 的大小来模拟负载。比如，当需要执行多次 Fibonacci 计算时，你可以让 `b.N` 增加：

```sh
go test -bench . -benchtime=10s
```

这样，Go 会强制运行基准测试 10 秒钟，直到时间结束，测试的次数可能会显著增加。你也可以使用 `-benchmem` 标志来查看内存分配和操作数。

**运行结果：**

```sh
PS D:\test\benchmark-demo> go test -bench=BenchmarkFib -benchmem      
goos: windows
goarch: amd64
pkg: benchmark-dmeo
cpu: 13th Gen Intel(R) Core(TM) i5-13400
BenchmarkFibonacci-16                339           3309542 ns/op               0 B/op          0 allocs/op
PASS
ok      benchmark-dmeo  1.857s
```

这表示，在每次斐波那契计算中，程序花费了大约 3309542 纳秒的时间，测试运行了 339 次。

如果你希望测试系统在更高负载下的表现，你可以让基准测试的时间更长，或者增加需要计算的次数。例如：

```sh
go test -bench . -benchtime=30s
```

这样，你就可以更好地模拟实际生产环境中的高负载情况，并观察性能是否存在瓶颈。

### 3.3 sub-Benchmark 子基准测试

​	类似于子测试 `t.Run`，子基准测试允许在一个基准测试函数内部，通过 `b.Run` 方法运行多个独立的基准测试场景，从而更灵活地组织代码并对不同输入或配置进行性能测量。

基本格式就是：

```go
func (b *B) Run(name string, f func(b *B)) bool
```

- `name`：子基准测试的名称，可以与父级名称组合形成层次化名称。
- `f`：子基准测试的函数体，在其中需要像普通基准测试一样使用 `b.N` 循环。
- 返回值：如果子基准测试没有失败且没有跳过，则返回 `true`。



假设我们要测试一个字符串拼接函数 `Concat` 在不同字符串长度下的性能：

```go
package main

import (
    "strings"
    "testing"
)

// Concat 将字符串切片拼接成一个字符串
func Concat(parts []string, sep string) string {
    return strings.Join(parts, sep)
}

// BenchmarkConcat 是包含子基准测试的父基准测试
func BenchmarkConcat(b *testing.B) {
    // 定义测试数据：不同长度的字符串切片
    testCases := []struct {
        name string
        size int
    }{
        {"Small", 10},
        {"Medium", 100},
        {"Large", 1000},
    }

    for _, tc := range testCases {
        b.Run(tc.name, func(b *testing.B) {
            // 准备测试数据（每次子基准测试运行前执行）
            parts := make([]string, tc.size)
            for i := 0; i < tc.size; i++ {
                parts[i] = "a"
            }
            sep := ","

            // 重置计时器，排除准备数据的时间
            b.ResetTimer()

            // 实际基准测试循环
            for i := 0; i < b.N; i++ {
                Concat(parts, sep)
            }
        })
    }
}
```

运行`go test -bench=.`会输出：

```sh
PS D:\test\benchmark-demo> go test -bench BenchmarkConcat    
goos: windows
goarch: amd64
pkg: benchmark-dmeo
cpu: 13th Gen Intel(R) Core(TM) i5-13400
BenchmarkConcat/Small-16                18009932                61.33 ns/op
BenchmarkConcat/Medium-16                2584550               449.5 ns/op
BenchmarkConcat/Large-16                  252168              4382 ns/op
PASS
ok      benchmark-dmeo  4.038s


PS D:\test\benchmark-demo> go test -bench=Concat -benchmem 
goos: windows
goarch: amd64
pkg: benchmark-dmeo
cpu: 13th Gen Intel(R) Core(TM) i5-13400
BenchmarkConcat/Small-16                18192705                61.51 ns/op             24 B/op          1 allocs/op
BenchmarkConcat/Medium-16                2624902               450.1 ns/op     208 B/op          1 allocs/op
BenchmarkConcat/Large-16                  261684              4358 ns/op     2048 B/op           1 allocs/op
PASS
ok      benchmark-dmeo  4.085s
```

可以看到子基准测试的名称是 `父名/子名` 的形式。

## 4. 并行Benchmark

### 4.1 基本使用

测试代码在并行场景下的性能，适合测试锁竞争、并发安全等场景。通过 `b.RunParallel` 方法，可以模拟多个 Goroutine 同时执行被测代码。



普通的基准测试（串行）在一个 Goroutine 中循环执行 `b.N` 次，测量的是单线程性能。但在实际生产环境中，代码往往会被多个 Goroutine 并发调用，例如 HTTP 处理函数、数据库连接池、并发数据结构等。并行基准测试能够：

- 评估代码在并发压力下的吞吐量和延迟。
- 检测并发安全问题和锁竞争导致的性能下降。
- 比较不同并发级别下的性能表现，帮助确定最佳工作线程数。

**基本语法：**

在基准测试函数中调用 `b.RunParallel`，参数是一个函数类型 `func(pb *testing.PB)`，其中 `pb` 是一个 `*testing.PB` 类型，用于控制循环。

```go
func BenchmarkParallel(b *testing.B) {
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            // 被测代码
        }
    })
}
```

`pb.Next()` 返回一个布尔值，当返回 `false` 时表示当前 Goroutine 应该停止。框架会创建多个 Goroutine 并发执行该函数，每个 Goroutine 在自己的循环中调用 `pb.Next()` 直到所有工作完成。

**完整示例：**

假设我们要测试一个并发安全的计数器 `SafeCounter` 的 `Inc` 方法：

```go
package main

import (
	"sync"
	"testing"
)

type SafeCounter struct {
	mu  sync.Mutex
	val int
}

func (c *SafeCounter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.val++
}

func BenchmarkSafeCounterInc(b *testing.B) {
	counter := &SafeCounter{}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.Inc()
		}
	})
}
```

运行命令 `go test -bench=BenchmarkSafeCounterInc -cpu=1,2,4` 会分别使用 1、2、4 个 CPU 核心（实际上是 P 的数量）来执行基准测试，输出：

```sh
PS D:\test\benchmark-demo> go test -bench=BenchmarkSafeCounterInc -benchmem -cpu="1,2,4"
goos: windows
goarch: amd64
pkg: benchmark-dmeo
cpu: 13th Gen Intel(R) Core(TM) i5-13400
BenchmarkSafeCounterInc         91427178                12.27 ns/op            0 B/op          0 allocs/op
BenchmarkSafeCounterInc-2       83011662                13.75 ns/op            0 B/op          0 allocs/op
BenchmarkSafeCounterInc-4       55810059                19.29 ns/op            0 B/op          0 allocs/op
PASS
ok      benchmark-dmeo  3.777s
```

注意：随着 CPU 数增加，每次操作耗时可能增加（因为锁竞争加剧），这正是并行基准测试希望揭示的现象。

### 4.2 工作原理

理解 `RunParallel` 的内部机制有助于正确使用它。



1. 根据 `GOMAXPROCS` 和 `b.parallelism` 计算出要启动的 Goroutine 数量 `n`。
2. 创建一个计数器，初始值为 `b.N`。
3. 启动 `n` 个 Goroutine，每个 Goroutine 执行传入的函数。
4. 每个 Goroutine 在循环中原子地递减计数器，直到计数器为 0，然后退出。
5. 主 Goroutine 等待所有工作 Goroutine 结束，期间计时器持续计时。

因此，`RunParallel` 实际上是一个工作共享（work-sharing）的并发模型，所有 Goroutine 共同消费一个任务池。

> #### b.SetParallelism
>
> 可以在调用 `RunParallel` 之前使用 `b.SetParallelism(p int)` 来调整并行度。实际启动的 Goroutine 数量为 `GOMAXPROCS * p`。例如：
>
> ```go
> b.SetParallelism(2) // 如果 GOMAXPROCS=4，则启动 8 个 Goroutine
> b.RunParallel(...)
> ```
>
> 注意：`p` 是倍乘因子，而不是绝对数量。



根据运行的过程那么需要清楚：

- **`b.N` 的分配**：`b.N` 是基准测试框架决定的总迭代次数，所有并行 Goroutine 共同完成这 `b.N` 次迭代。每个 Goroutine 通过 `pb.Next()` 获取下一个迭代任务，直到所有迭代完成。
- **Goroutine 数量**：默认情况下，`RunParallel` 会启动 `GOMAXPROCS` 个 Goroutine（即逻辑 CPU 核心数）。可以通过 `-cpu` 标志改变，例如 `-cpu=4,8` 会分别以 4 和 8 个 Goroutine 运行。
- **计时器行为**：`b.N` 次迭代的总耗时是从调用 `RunParallel` 开始到所有 Goroutine 完成的时间。框架会自动处理计时器的暂停/恢复，但需要注意：如果在 `RunParallel` 之前有耗时的设置代码，应调用 `b.ResetTimer()`。
- **停止/启动计时器**：在 `RunParallel` 内部，`b.StopTimer()` 和 `b.StartTimer()` 不会直接影响并行 Goroutine 的计时，因为这些方法作用于全局计时器。如果需要排除某些操作的时间，应在 `RunParallel` 之前或之后进行。

### 4.3 结合子基准测试

```go
func BenchmarkCounter(b *testing.B) {
    benchmarks := []struct{
        name string
        parallel bool
    }{
        {"Serial", false},
        {"Parallel", true},
    }
    for _, bm := range benchmarks {
        b.Run(bm.name, func(b *testing.B) {
            counter := &SafeCounter{}
            if bm.parallel {
                b.RunParallel(func(pb *testing.PB) {
                    for pb.Next() {
                        counter.Inc()
                    }
                })
            } else {
                for i := 0; i < b.N; i++ {
                    counter.Inc()
                }
            }
        })
    }
}
```

```sh
PS D:\test\benchmark-demo> go test -bench=BenchmarkCounter -benchmem                    
goos: windows
goarch: amd64
pkg: benchmark-dmeo
cpu: 13th Gen Intel(R) Core(TM) i5-13400
BenchmarkCounter/Serial-16              93185788                12.16 ns/op            0 B/op          0 allocs/op
BenchmarkCounter/Parallel-16            21338150                58.82 ns/op            0 B/op          0 allocs/op
PASS
ok      benchmark-dmeo  2.850s
```

这样可以直接比较串行和并行的性能差异。

### 4.4 最佳实践

- **总是使用 `-race` 检测并发安全性**。
- **合理设置 `-cpu` 值**：从 1 开始，逐步增加，观察性能变化曲线。
- **将初始化代码放在 `RunParallel` 之外或使用 `b.ResetTimer`**。
- **避免在并行体中使用非并发安全的 `b` 方法**。
- **结合 `-benchmem` 观察内存分配**，并发可能导致更多内存竞争。
- **对于锁竞争严重的代码，并行基准测试可能会显示平均延迟上升，这是正常现象**。
- **使用 `b.SetParallelism` 微调并行 Goroutine 数量**，模拟真实负载。

## 5. benchstat

`benchstat` 是 Go 官方提供的基准测试结果统计分析工具，用于比较不同基准测试运行的结果，计算统计显著性，并以友好的表格形式展示。

### 5.1 安装

```sh
go install golang.org/x/perf/cmd/benchstat@latest
```



### 5.2 使用示例

#### 5.2.1 准备示例

创建一个包含多种字符串拼接方式的包：

```go
// string_concat.go
package concat

import "strings"

// 使用 + 拼接
func PlusConcat(strs []string) string {
    result := ""
    for _, s := range strs {
        result += s
    }
    return result
}

// 使用 strings.Builder 拼接
func BuilderConcat(strs []string) string {
    var builder strings.Builder
    for _, s := range strs {
        builder.WriteString(s)
    }
    return builder.String()
}

// 使用 strings.Join 拼接
func JoinConcat(strs []string) string {
    return strings.Join(strs, "")
}
```

#### 5.2.2 基准测试代码

```go
// string_concat_test.go
package concat

import (
    "testing"
)

var result string // 包级变量防止优化

func generateInput(n int) []string {
    strs := make([]string, n)
    for i := 0; i < n; i++ {
        strs[i] = "a"
    }
    return strs
}

func BenchmarkPlusConcat(b *testing.B) {
    strs := generateInput(100) // 100个字符串
    b.ResetTimer()
    
    var r string
    for i := 0; i < b.N; i++ {
        r = PlusConcat(strs)
    }
    result = r
}

func BenchmarkBuilderConcat(b *testing.B) {
    strs := generateInput(100)
    b.ResetTimer()
    
    var r string
    for i := 0; i < b.N; i++ {
        r = BuilderConcat(strs)
    }
    result = r
}

func BenchmarkJoinConcat(b *testing.B) {
    strs := generateInput(100)
    b.ResetTimer()
    
    var r string
    for i := 0; i < b.N; i++ {
        r = JoinConcat(strs)
    }
    result = r
}
```

#### 5.2.3 运行并保存结果

```sh
# 运行基准测试并保存到文件
go test -bench=. -count=10 -benchmem > old.txt
```

#### 5.2.4 benchstate-查看单个结果文件

```sh
benchstat old.txt
```

输出：

```sh
goos: windows
goarch: amd64
pkg: benchmark-dmeo
cpu: 13th Gen Intel(R) Core(TM) i5-13400
                 │   old.txt   │
                 │   sec/op    │
PlusConcat-16      2.527µ ± 4%
BuilderConcat-16   340.4n ± 6%
JoinConcat-16      428.7n ± 3%
geomean            717.2n

                 │   old.txt    │
                 │     B/op     │
PlusConcat-16      5.531Ki ± 0%
BuilderConcat-16     248.0 ± 0%
JoinConcat-16        112.0 ± 0%
geomean              539.8

                 │  old.txt   │
                 │ allocs/op  │
PlusConcat-16      99.00 ± 0%
BuilderConcat-16   5.000 ± 0%
JoinConcat-16      1.000 ± 0%
geomean            7.910
```

解释：

- `± 2%` 表示运行间的变异系数（标准差/平均值）
- 显示每次操作的时间、内存分配量和分配次数

#### 5.2.5 benchstat-比较两个版本

假设我们对代码进行了优化，再次运行基准测试：

```sh
# 优化后再次运行
go test -bench=. -count=10 -benchmem > new.txt

# 比较两个版本
benchstat old.txt new.txt
```

输出：

```sh
goos: windows
goarch: amd64
pkg: benchmark-dmeo
cpu: 13th Gen Intel(R) Core(TM) i5-13400
                 │   old.txt   │              new.txt               │
                 │   sec/op    │   sec/op     vs base               │
PlusConcat-16      2.527µ ± 4%   2.551µ ± 2%       ~ (p=0.724 n=10)
BuilderConcat-16   340.4n ± 6%   345.7n ± 3%       ~ (p=0.684 n=10)
JoinConcat-16      428.7n ± 3%   454.4n ± 7%  +5.98% (p=0.004 n=10)
geomean            717.2n        737.1n       +2.78%

                 │   old.txt    │                new.txt                │
                 │     B/op     │     B/op      vs base                 │
PlusConcat-16      5.531Ki ± 0%   5.531Ki ± 0%       ~ (p=1.000 n=10) ¹
BuilderConcat-16     248.0 ± 0%     248.0 ± 0%       ~ (p=1.000 n=10) ¹
JoinConcat-16        112.0 ± 0%     112.0 ± 0%       ~ (p=1.000 n=10) ¹
geomean              539.8          539.8       +0.00%
¹ all samples are equal

                 │  old.txt   │               new.txt               │
                 │ allocs/op  │ allocs/op   vs base                 │
PlusConcat-16      99.00 ± 0%   99.00 ± 0%       ~ (p=1.000 n=10) ¹
BuilderConcat-16   5.000 ± 0%   5.000 ± 0%       ~ (p=1.000 n=10) ¹
JoinConcat-16      1.000 ± 0%   1.000 ± 0%       ~ (p=1.000 n=10) ¹
geomean            7.910        7.910       +0.00%
¹ all samples are equal
```



**解读第一部分：**

- **PlusConcat**: 从 2.527µs → 2.551µs，变化很小，`p=0.724 > 0.05` → **无显著差异**
- **BuilderConcat**: 从 340.4ns → 345.7ns，`p=0.684 > 0.05` → **无显著差异**
- **JoinConcat**: 从 428.7ns → 454.4ns，**变慢 5.98%**，`p=0.004 < 0.05` → **有显著差异**
- **整体 (geomean)**: 平均变慢 2.78%

**统计术语解释：**

| 符号/术语 | 含义       | 说明                      |
| :-------- | :--------- | :------------------------ |
| `~`       | 无显著变化 | 统计上无显著差异          |
| `+5.98%`  | 变慢5.98%  | 新版本性能下降            |
| `p=0.004` | p值        | <0.05表示有统计显著性差异 |
| `n=10`    | 样本数     | 每组测试运行了10次        |
| `± 4%`    | 变异系数   | 测试结果的波动范围        |
