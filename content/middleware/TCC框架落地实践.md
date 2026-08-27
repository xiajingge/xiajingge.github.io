+++
date = '2025-05-15T21:23:34+08:00'
title = "TCC框架落地实践"
tags = ["Transaction"]
categories = ["分布式"]
+++


本篇内容是为了延续[分布式事务实现原理](./分布式事务实现原理.md)的话题，带来的TCC框架的落地实践分享。

## 1. 整体架构

首先我们回顾一下有关于分布式事务以及 TCC 的概念.

所谓事务，对应的语义是“要么什么都不做，要么全都做到位”，需要针对多个执行动作，建立一套一气呵成、不可拆解的运行机制.

在事务中包括的一些执行动作，倘若涉及到跨数据库、跨组件、跨服务等分布式操作，那我们就称这样的事务是分布式事务。

TCC 本质上是一种 2PC（two phase commitment protocal 两阶段提交）的实现：

- 把分布式事务中，负责维护状态数据变更的模块，封装成一个 TCC 组件

- 把数据的变更状态拆分为对应 Try 操作的【冻结】、对应 Confirm 操作的【成功】以及对应 Cancel 操作的【失败回滚】

- 抽出一个统筹全局的事务协调者角色 TXManager. 在执行分布式事务时，分为两个阶段：

    - 阶段 I：先对所有组件执行 Try 操作

    - 阶段 II：根据上阶段 Try 操作的执行结果，决定本轮执行 Confirm 还是 Cancel 操作

![TCC事务架构](/middleware/images/tcc事务架构.png)

那为了实现这个架构，需要明确设计的思路：

- TCC Component：
    - 使用方自行实现ry、Confirm、Cancel 方法的执行逻辑

- Tx Manager:
    - tcc component注册接口
    - 开启事务接口
        - 作为和 Application 交互的唯一入口，并基于 Application 事务执行结果的反馈
        - try-confirm-cancel的2PC流程
    - 异步轮询接口

- 日志组件
    - Tx store组件接口，可供使用方自行接口自己的存储组件
    - 需要支持事务明细数据的CRUD能力，用作创建事务、查询事务、更新事务进度、提交事务结果

![tcc设计思路](/middleware/images/tcc设计思路.jpg)

### 1.2 TCC Component

下面是关于 TCC 组件的定位：

- 这部分内容需要由用户自行实现，并在 TXManager 启动时将其注册到注册中心 RegistryCenter 当中.

- 当使用方调用 TXManager 开启事务时，会通过 RegistryCenter 获取这些组件，并对其进行使用

TCC 组件需要具备的能力包括如下几项：

- ID：组件必须是唯一的

- 暴露出 Try、Confirm、Cancel 三个入口，对应于 TCC 的语义

- 针对数据记录，新增出一个对应于 Try 操作的中间状态枚举值

- 针对于同一笔事务的重复请求，需要执行幂等性校验

- 需要支持空回滚操作. 即针对于一笔新的 Transaction ID，在没收到 Try 的前提下，若提前收到了 Cancel 操作，也需要将这个信息记录下来，但不需要对真实的状态数据发生变更

下面针对最后一点提到的空回滚操作，进一步加以说明：

这个空回滚机制本质上是为了解决 TCC 流程中出现的悬挂问题，下面我们举个具体例子加以说明：

- TX Manager 在向 Component A 发起 Try 请求时，由于出现网络拥堵，导致请求超时

- TX Manager 发现存在 Try 请求超时，将其判定为失败，因此批量执行 Component 的 Cancel 操作

- Component A 率先收到了后发先至的 Cancel 请求

- 过了一会儿，之前阻塞在网络链路中的 Try 请求也到达了 Component A

从执行逻辑上，Try 应该先于 Cancel 到达和处理，然而在事实上，由于网络环境的不稳定性，请求到达的**先后次序可能颠倒**。 在这个场景中，Component A 需要保证的是，针对于同一笔事务，只要接受过对应的 Cancel 请求，之后到来的 Try 请求需要被忽略. 这就是 TCC Component 需要支持**空回滚操作**的原因所在.

![空回滚操作的原因](/middleware/images/空回滚.jpg)

## 2. Tx Manager核心源码讲解

### 2.1 类图

首先捋一下，在 tcc 核心 sdk 中，涉及到的几个核心类：

-  TXManager：事务协调器，class

- TXStore：事务日志存储模块，interface

- registryCenter：TCC 组件注册管理中心，class

- TCCComponent：TCC 组件，interface

![TXManager类图](/middleware/images/Txmanager类图.svg)

#### 2.1.1 Tx Manager

下面是关于事务协调器 TXManager 的几个核心字段：

- txStore：内置的事务日志存储模块，需要由使用方实现并完成注入

- registryCenter：TCC 组件的注册管理中心

- opts：内聚了一些 TXManager 的配置项，可以由使用方自定义，并通过 option 注入

- ctx：用于反映 TXManager 运行生命周期的的 context，当 ctx 终止时，异步轮询任务也会随之退出

- stop：用于停止 txManager 的控制器. 当 stop 被调用后，异步轮询任务会被终止

```go
type TXManager struct {
    ctx            context.Context
    stop           context.CancelFunc
    opts           *Options
    txStore        TXStore
    registryCenter *registryCenter
}


func NewTXManager(txStore TXStore, opts ...Option) *TXManager {
    ctx, cancel := context.WithCancel(context.Background())
    txManager := TXManager{
        opts:           &Options{},
        txStore:        txStore,
        registryCenter: newRegistryCenter(),
        ctx:            ctx,
        stop:           cancel,
    }


    for _, opt := range opts {
        opt(txManager.opts)
    }


    repair(txManager.opts)


    go txManager.run()
    return &txManager
}

func (t *TXManager) Register(component component.TCCComponent) error {
    return t.registryCenter.register(component)
}

```

#### 2.1.2 RegistryCenter
注册中心 registryCenter 中的内容很简单，通过 map 存储所有注册进来的 TCC 组件，要求各组件都有独立的组件 ID；通过一把读写锁 rwMutex 保护 map 的并发安全性
```go
type registryCenter struct {
    mux        sync.RWMutex
    components map[string]component.TCCComponent
}

func newRegistryCenter() *registryCenter {
    return &registryCenter{
        components: make(map[string]component.TCCComponent),
    }
}

func (r *registryCenter) register(component component.TCCComponent) error {
    r.mux.Lock()
    defer r.mux.Unlock()
    if _, ok := r.components[component.ID()]; ok {
        return errors.New("repeat component id")
    }
    r.components[component.ID()] = component
    return nil
}

func (r *registryCenter) getComponents(componentIDs ...string) ([]component.TCCComponent, error) {
    components := make([]component.TCCComponent, 0, len(componentIDs))


    r.mux.RLock()
    defer r.mux.RUnlock()


    for _, componentID := range componentIDs {
        component, ok := r.components[componentID]
        if !ok {
            return nil, fmt.Errorf("component id: %s not existed", componentID)
        }
        components = append(components, component)
    }


    return components, nil
}
```

#### 2.1.3 TXStore

事务日志存储模块传入的是一个接口，是由使用方自行实现并注入的，TXStore interface 的定义
```go
type TXStore interface {
    // 创建一条事务明细记录
    CreateTX(ctx context.Context, components ...component.TCCComponent) (txID string, err error)
    // 更新事务进度：
    // 规则为：倘若有一个 component try 操作执行失败，则整个事务失败；倘若所有 component try 操作执行成功，则事务成功
    TXUpdate(ctx context.Context, txID string, componentID string, accept bool) error
    // 提交事务的最终状态
    TXSubmit(ctx context.Context, txID string, success bool) error
    // 获取到所有处于中间态的事务
    GetHangingTXs(ctx context.Context) ([]*Transaction, error)
    // 获取指定的一笔事务
    GetTX(ctx context.Context, txID string) (*Transaction, error)
    // 锁住事务日志表
    Lock(ctx context.Context, expireDuration time.Duration) error
    // 解锁事务日志表
    Unlock(ctx context.Context) error
}
```


### 2.2 运行流程

![事务的整体运行流程](/middleware/images/运行流程.png)

txManager.Transaction 方法是用户启动分布式事务的入口，需要在入参中声明本次事务涉及到的组件以及需要在 Try 流程中传递给对应组件的请求参数：
```go
type RequestEntity struct {
    // 组件名称
    ComponentID string `json:"componentName"`
    // Try 请求时传递的参数
    Request map[string]interface{} `json:"request"`
}
```

txManager.Transaction 对应源码如下，核心步骤均给出了注释. 核心的 try-confirm/cancel 流程，会在后续的 txManager.twoPhaseCommit 方法中展开。

```go
// 启动事务
func (t *TXManager) Transaction(ctx context.Context, reqs ...*RequestEntity) (bool, error) {
    // 1 限制分布式事务执行时长
    tctx, cancel := context.WithTimeout(ctx, t.opts.Timeout)
    defer cancel()


    // 2 获得所有的涉及使用的 tcc 组件
    componentEntities, err := t.getComponents(tctx, reqs...)
    if err != nil {
        return false, err
    }


    // 3 调用 txStore 模块，创建新的事务明细记录，并取得全局唯一的事务 id
    txID, err := t.txStore.CreateTX(tctx, componentEntities.ToComponents()...)
    if err != nil {
        return false, err
    }


    // 4. 开启两阶段提交流程：try-confirm/cancel
    return t.twoPhaseCommit(ctx, txID, componentEntities)
}
```

try-confirm/cancel 流程的串联，可以说是整个 tcc 框架的精髓所在，对应流程图展示如下：

![try-confirm/cancel 流程的串联](/middleware/images/2pc.png)

```go
func (t *TXManager) twoPhaseCommit(ctx context.Context, txID string, componentEntities ComponentEntities) (bool, error) {
    // 1 创建子 context 用于管理子 goroutine 生命周期
    // 手握 cancel 终止器，能保证在需要的时候终止所有子 goroutine 生命周期
    cctx, cancel := context.WithCancel(ctx)
    defer cancel()


    // 2 创建一个 chan，用于接收子 goroutine 传递的错误
    errCh := make(chan error)
    // 3 并发启动，批量执行各 tcc 组件的 try 流程
    go func() {
        // 通过 waitGroup 进行多个子 goroutine 的汇总
        var wg sync.WaitGroup
        for _, componentEntity := range componentEntities {
            // shadow
            componentEntity := componentEntity
            wg.Add(1)
            // 并发执行各组件的 try 流程
            go func() {
                defer wg.Done()
                resp, err := componentEntity.Component.Try(cctx, &component.TCCReq{
                    ComponentID: componentEntity.Component.ID(),
                    TXID:        txID,
                    Data:        componentEntity.Request,
                })
                // 出现 tcc 组件执行 try 操作失败，则需要对事务明细记录进行更新，同时把错误通过 chan 抛给父 goroutine
                if err != nil || !resp.ACK {
                    // 对对应的事务进行更新
                    _ = t.txStore.TXUpdate(cctx, txID, componentEntity.Component.ID(), false)
                    errCh <- fmt.Errorf("component: %s try failed", componentEntity.Component.ID())
                    return
                }
                // try 请求成功，则对事务明细记录进行更新. 倘若更新失败，也要视为错误，抛给父 goroutine
                if err = t.txStore.TXUpdate(cctx, txID, componentEntity.Component.ID(), true); err != nil {
                    errCh <- err
                }
            }()
        }


        // 等待所有子 goroutine 运行完成
        wg.Wait()
        // 关闭 errCh，告知父 goroutine 所有任务已运行完成的信息
        close(errCh)
    }()




    successful := true
    // 4 通过 chan，阻塞子 goroutine 执行完成
    // 4.1 但凡出现一个子 goroutine 遇到了错误，则会提前接收到错误，并调用 cancel 方法熔断其他所有子 goroutine 流程
    // 4.2 倘若所有子 goroutine 都执行成功，则会通过 chan 的关闭事件推进流程，对应 err 为 nil
    if err := <-errCh; err != nil {
        // 只要有一笔 try 请求出现问题，其他的都进行终止
        cancel()
        successful = false
    }


    // 5 异步执行第二阶段的 confirm/cancel 流程
    // 之所以是异步，是因为实际上在第一阶段 try 的响应结果尘埃落定时，对应事务的成败已经有了定论
    // 第二阶段能够容忍异步执行的原因在于，执行失败时，还有轮询任务进行兜底
    go t.advanceProgressByTXID(txID)
    
    // 6 响应结果
    // 6.1 倘若所有 try 请求都成功，则 successful 为 try，事务成功
    // 6.2 但凡有一个 try 请求处理出现问题，successful 为 false，事务失败
    return successful, nil
}
```

上述过程中提到的`advanceProgressByTXID`的作用是当一笔事务在第一阶段中所有的 Try 请求都有了响应后，就需要根据第一阶段的结果，执行第二阶段的 Confirm 或者 Cancel 操作，并且将事务状态推进为成功或失败的终态：

- 倘若所有组件的 Try 响应都是成功，则需要批量调用组件的 Confirm 接口，并在这之后将事务状态更新为成功

- 倘若存在某个组件 Try 响应为失败，则需要批量调用组件的 Cancel 接口，并在这之后将事务状态更新为失败

- 倘若当前事务已执行超时，同样需要批量调用组件的 Cancel 接口，并在这之后将事务状态更新为失败

```go
// 传入一个事务 id 推进其进度
func (t *TXManager) advanceProgressByTXID(txID string) error {
    // 获取事务日志明细
    tx, err := t.txStore.GetTX(t.ctx, txID)
    if err != nil {
        return err
    }
    // 推进进度
    return t.advanceProgress(tx)
}

// 传入一个事务 id 推进其进度
func (t *TXManager) advanceProgress(tx *Transaction) error {
    // 1 推断出事务当前的状态
    // 1.1 倘若所有组件 try 都成功，则为 successful
    // 1.2 倘若存在组件 try 失败，则为 failure
    // 1.3 倘若事务超时了，则为 failure
    // 1.4 否则事务状态为 hanging
    txStatus := tx.getStatus(time.Now().Add(-t.opts.Timeout))
    // hanging 状态的事务暂时不处理
    if txStatus == TXHanging {
        return nil
    }


    // 2 根据事务是否成功，定制不同的处理函数
    success := txStatus == TXSuccessful
    var confirmOrCancel func(ctx context.Context, component component.TCCComponent) (*component.TCCResp, error)
    var txAdvanceProgress func(ctx context.Context) error
    if success {
        // 如果事务成功，则需要对组件进行 confirm
        confirmOrCancel = func(ctx context.Context, component component.TCCComponent) (*component.TCCResp, error) {
            return component.Confirm(ctx, tx.TXID)
        }
        // 如果事务成功，则需要在最后更新事务日志记录的状态为成功
        txAdvanceProgress = func(ctx context.Context) error {
            return t.txStore.TXSubmit(ctx, tx.TXID, true)
        }




    } else {
        // 如果事务失败，则需要对组件进行 cancel
        confirmOrCancel = func(ctx context.Context, component component.TCCComponent) (*component.TCCResp, error) {           
            return component.Cancel(ctx, tx.TXID)
        }


        // 如果事务失败，则需要在最后更新事务日志记录的状态为失败
        txAdvanceProgress = func(ctx context.Context) error {           
            return t.txStore.TXSubmit(ctx, tx.TXID, false)
        }
    }


    // 3 批量调用组件，执行第二阶段的 confirm/cancel 操作
    for _, component := range tx.Components {
        // 获取对应的 tcc component
        components, err := t.registryCenter.getComponents(component.ComponentID)
        if err != nil || len(components) == 0 {
            return errors.New("get tcc component failed")
        }      
        resp, err := confirmOrCancel(t.ctx, components[0])
        if err != nil {
            return err
        }
        if !resp.ACK {
            return fmt.Errorf("component: %s ack failed", component.ComponentID)
        }
    }


    // 4 二阶段 confirm/cancel 操作都执行完成后，对事务状态进行提交
    return txAdvanceProgress(t.ctx)
}
```

### 2.3 异步轮询流程

**异步轮询流程**这个流程同样非常重要，是支撑 txManager 鲁棒性的重要机制。倘若存在事务已经完成第一阶段 Try 操作的执行，但是第二阶段没执行成功，则需要由异步轮询流程进行兜底处理，为事务补齐第二阶段的操作，并将事务状态更新为终态。

#### 2.3.1 启动时机

异步轮询任务是在 txManager 的初始化流程中启动的，通过异步 goroutine 持久运行：
```go
func NewTXManager(txStore TXStore, opts ...Option) *TXManager {
    ctx, cancel := context.WithCancel(context.Background())
    txManager := TXManager{
        opts:           &Options{},
        txStore:        txStore,
        registryCenter: NewRegistryCenter(),
        ctx:            ctx,
        stop:           cancel,
    }


    for _, opt := range opts {
        opt(txManager.opts)
    }


    repair(txManager.opts)


    go txManager.run()
    return &txManager
}
```

#### 2.3.2 轮询流程

异步轮询任务运行时，基于 for 循环 + select 多路复用的方式，实现定时任务的执行。

轮询的时间间隔会根据一轮任务处理过程中是否出现错误，而进行动态调整. 这里调整规则指的是：当一次处理流程中发生了错误，就需要调大当前节点轮询的时间间隔，让其他节点的异步轮询任务得到更大的执行机会。

![异步轮询任务流程](/middleware/images/异步轮询任务.svg)

```go
func (t *TXManager) run() {
    var tick time.Duration
    var err error
    // 1 for 循环自旋式运行任务
    for {
        // 如果处理过程中出现了错误，需要增长轮询时间间隔
        if err == nil {
            tick = t.opts.MonitorTick
        } else {
            tick = t.backOffTick(tick)
        }
        
        // select 多路复用
        select {
        // 倘若 txManager.ctx 被终止，则异步轮询任务退出
        case <-t.ctx.Done():
            return


        // 2 等待 tick 对应时长后，开始执行任务
        case <-time.After(tick):
            // 对 txStore 加分布式锁，避免分布式服务下多个服务节点的轮询任务重复执行
            if err = t.txStore.Lock(t.ctx, t.opts.MonitorTick); err != nil {
                // 取锁失败时（大概率被其他节点占有），不需要增加 tick 时长
                err = nil
                continue
            }


            // 3 获取处于 hanging 状态的事务
            var txs []*Transaction
            if txs, err = t.txStore.GetHangingTXs(t.ctx); err != nil {
                _ = t.txStore.Unlock(t.ctx)
                continue
            }
   
            // 4 批量推进事务进度
            err = t.batchAdvanceProgress(txs)
            _ = t.txStore.Unlock(t.ctx)
        }
    }
}
```

有关于轮询时间间隔的退避谦让策略为：每次对时间间隔进行翻倍，封顶为初始时长的 8 倍：
```go
func (t *TXManager) backOffTick(tick time.Duration) time.Duration {
    tick <<= 1
    if threshold := t.opts.MonitorTick << 3; tick > threshold {
        return threshold
    }
    return tick
}
```

批量推进事务进度核心是开启多个 goroutine 并发对多项事务进行处理：

```go
func (t *TXManager) batchAdvanceProgress(txs []*Transaction) error {
    // 1 创建一个 chan，用于接收子 goroutine 传输的 err
    errCh := make(chan error)
    go func() {
        // 2 通过 waitGroup 聚合多个子 groutine
        var wg sync.WaitGroup
        for _, tx := range txs {
            // shadow
            tx := tx
            wg.Add(1)
            go func() {
                defer wg.Done()
                // 3 推进每笔事务的进度
                if err := t.advanceProgress(tx); err != nil {
                    // 遇到错误则投递到 errCh
                    errCh <- err
                }
            }()
        }
        
        // 4 收口等待所有子 goroutine 执行完成
        wg.Wait()
        // 5 所有子 goroutine 执行完成后关闭 chan，唤醒阻塞等待的父 goroutine
        close(errCh)
    }()


    // 记录遇到的第一个错误
    var firstErr error
    // 6 父 goroutine 通过 chan 阻塞在这里，直到所有 goroutine 执行完成，chan 被 close 才能往下
    for err := range errCh {
        // 记录遇到的第一个错误
        if firstErr != nil {
            continue
        }
        firstErr = err
    }


    // 7 返回错误，核心是标识执行过程中，是否发生过错误
    return firstErr
}
```

## 3. 使用案例讲解

这一节，我们从实际应用的使用方视角出发，对所需要实现的模块进行定义，然后给出应用代码示例。

### 3.1 tcc component

首先，我们提到了tcc component组件需要自己定义：

定义一个 MockComponent 类，其中内置了 redis 客户端，用于完成一些状态数据的存取。

```go
// 实现的 tcc 组件
type MockComponent struct {
    // tcc 组件唯一标识 id，构造时由使用方传入
    id     string
    // redis 客户端
    client *redis_lock.Client
}


func NewMockComponent(id string, client *redis_lock.Client) *MockComponent {
    return &MockComponent{
        id:     id,
        client: client,
    }
}


// 返回 tcc 组件的唯一标识 id
func (m *MockComponent) ID() string {
    return m.id
}
```

下面实现一下 TCC 组件的 **Try 方法:**

![tcc component的try实现](/middleware/images/redis-try.svg)

```go
func (m *MockComponent) Try(ctx context.Context, req *component.TCCReq) (*component.TCCResp, error) {
    // 1 基于 txID 维度加 redis 分布式锁
    lock := redis_lock.NewRedisLock(pkg.BuildTXLockKey(m.id, req.TXID), m.client)
    if err := lock.Lock(ctx); err != nil {
        return nil, err
    }
    defer func() {
        _ = lock.Unlock(ctx)
    }()


    // 2 基于 txID 幂等性去, 需要对事务的状态进行检查
    txStatus, err := m.client.Get(ctx, pkg.BuildTXKey(m.id, req.TXID))
    if err != nil && !errors.Is(err, redis_lock.ErrNil) {
        return nil, err
    }


    res := component.TCCResp{
        ComponentID: m.id,
        TXID:        req.TXID,
    }
    switch txStatus {
    case TXTried.String(), TXConfirmed.String(): // 重复的 try 请求，给予成功的响应
        res.ACK = true
        return &res, nil
    case TXCanceled.String(): // 此前该事务已 cancel，则拒绝本次 try 请求
        return &res, nil
    default:
    }


    // 3 建立 txID 与 bizID 的关联
    bizID := gocast.ToString(req.Data["biz_id"])
    if _, err = m.client.Set(ctx, pkg.BuildTXDetailKey(m.id, req.TXID), bizID); err != nil {
        return nil, err
    }


    // 4 把 bizID 对应的业务数据置为冻结态
    reply, err := m.client.SetNX(ctx, pkg.BuildDataKey(m.id, req.TXID, bizID), DataFrozen.String())
    if err != nil {
        return nil, err
    }
    // 倘若数据此前已冻结或已使用，则拒绝本次 try 请求
    if reply != 1 {
        return &res, nil
    }


    // 5 更新当前组件下的事务状态为 tried
    _, err = m.client.Set(ctx, pkg.BuildTXKey(m.id, req.TXID), TXTried.String())
    if err != nil {
        return nil, err
    }


    // 6 给予接收 try 请求的响应
    res.ACK = true
    return &res, nil
}
```

下面实现一下 TCC 组件的 **Confirm 方法:**

![tcc component的confirm实现](/middleware/images/redis-confirm.svg)

```go
func (m *MockComponent) Confirm(ctx context.Context, txID string) (*component.TCCResp, error) {
    // 1 基于 txID 维度加锁
    lock := redis_lock.NewRedisLock(pkg.BuildTXLockKey(m.id, txID), m.client)
    if err := lock.Lock(ctx); err != nil {
        return nil, err
    }
    defer func() {
        _ = lock.Unlock(ctx)
    }()




    // 2. 校验事务状态，要求对应组件下，事务此前的状态为 tried
    txStatus, err := m.client.Get(ctx, pkg.BuildTXKey(m.id, txID))
    if err != nil {
        return nil, err
    }


    res := component.TCCResp{
        ComponentID: m.id,
        TXID:        txID,
    }
    switch txStatus {
    case TXConfirmed.String(): // 事务状态已 confirm，直接幂等响应为成功
        res.ACK = true
        return &res, nil
    case TXTried.String(): // 只有事务状态为 try 才是合法的，会对程序放行
    default: // 其他情况直接拒绝，ack 为 false
        return &res, nil
    }


    // 3 获取事务对应的 bizID
    bizID, err := m.client.Get(ctx, pkg.BuildTXDetailKey(m.id, txID))
    if err != nil {
        return nil, err
    }


    // 4. 校验业务数据此前状态是否为冻结
    dataStatus, err := m.client.Get(ctx, pkg.BuildDataKey(m.id, txID, bizID))
    if err != nil {
        return nil, err
    }
    
    // 如果此前非冻结态，则拒绝本次请求
    if dataStatus != DataFrozen.String() {
        return &res, nil
    }




    // 5 把业务数据的更新操作置为 successful
    if _, err = m.client.Set(ctx, pkg.BuildDataKey(m.id, txID, bizID), DataSuccessful.String()); err != nil {
        return nil, err
    }


    // 6 把对应组件下的事务状态更新为成功，这一步哪怕失败了也不阻塞主流程
    _, _ = m.client.Set(ctx, pkg.BuildTXKey(m.id, txID), TXConfirmed.String())


    // 7 处理成功，给予成功的响应
    res.ACK = true
    return &res, nil
}
```

下面实现一下 TCC 组件的 **Cancel 方法:**

![tcc component的cancel实现](/middleware/images/redis-cancel.svg)

```go
func (m *MockComponent) Cancel(ctx context.Context, txID string) (*component.TCCResp, error) {
    // 1 基于 txID 维度加锁
    lock := redis_lock.NewRedisLock(pkg.BuildTXLockKey(m.id, txID), m.client)
    if err := lock.Lock(ctx); err != nil {
        return nil, err
    }
    defer func() {
        _ = lock.Unlock(ctx)
    }()


    // 2 校验事务状态，只要不是 confirmed，都允许被置为 canceld
    txStatus, err := m.client.Get(ctx, pkg.BuildTXKey(m.id, txID))
    if err != nil && !errors.Is(err, redis_lock.ErrNil) {
        return nil, err
    }
    // 倘若组件内事务此前的状态为 confirmed，则说明流程有异常.
    if txStatus == TXConfirmed.String() {
        return nil, fmt.Errorf("invalid tx status: %s, txid: %s", txStatus, txID)
    }


    // 3 根据事务获取对应的 bizID
    bizID, err := m.client.Get(ctx, pkg.BuildTXDetailKey(m.id, txID))
    if err != nil {
        return nil, err
    }


    // 4 删除对应的 frozen 冻结记录，代表对数据执行了回滚操作
    if err = m.client.Del(ctx, pkg.BuildDataKey(m.id, txID, bizID)); err != nil {
        return nil, err
    }


    // 5 把事务状态更新为 canceled
    _, err = m.client.Set(ctx, pkg.BuildTXKey(m.id, txID), TXCanceled.String())
    if err != nil {
        return nil, err
    }


    // 6 给予处理成功的 ack 
    return &component.TCCResp{
        ACK:         true,
        ComponentID: m.id,
        TXID:        txID,
    }, nil
}
```

### 3.2 Tx Store
声明了一个 MockTXStore 类，里面通过 mysql 存储事务日志明细数据，通过 redis 实现 TXStore 模块的分布式锁.

其中和事务日志明细数据库直接交互的操作被封装在 TXRecordDAO 当中。

![tx store的实现](/middleware/images/txstore.png)

```go

// TXStore 模块具体实现
type MockTXStore struct {
    // redis 客户端，用于实现分布式锁
    client *redis_lock.Client
    // 事务日志存储 DAO 层
    dao    *expdao.TXRecordDAO
}


func NewMockTXStore(dao *expdao.TXRecordDAO, client *redis_lock.Client) *MockTXStore {
    return &MockTXStore{
        dao:    dao,
        client: client,
    }
}
```

dao层的实现是基于`gorm`来实现的对象的CRUD操作，对象的定义如下：

```go
type TXRecordDAO struct {
    db *gorm.DB
}


func NewTXRecordDAO(db *gorm.DB) *TXRecordDAO {
    return &TXRecordDAO{
        db: db,
    }
}


// 事务日志明细记录的持久化对象（PO，Persistent Object）模型定义
type TXRecordPO struct {
    gorm.Model
    // 事务状态 hanging / successful / failure
    Status               string `gorm:"status"`
    // 各组件Try的响应记录
    ComponentTryStatuses string `gorm:"component_try_statuses"`
}


func (t TXRecordPO) TableName() string {
    return "tx_record"
}


type ComponentTryStatus struct {
    // 组件的唯一ID
    ComponentID string `json:"componentID"`
    // 组件状态 hanging / successful / failure
    TryStatus   string `json:"tryStatus"`
}
```

下面是事务日志明细表的建表语句：
```sql
CREATE TABLE IF NOT EXISTS `tx_record`
(
    `id`                       bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
    `status`                   varchar(16) NOT NULL COMMENT '事务状态 hanging/successful/failure',
    `component_try_statuses`   json DEFAULT NULL COMMENT '各组件 try 接口请求状态 hanging/successful/failure',
    `deleted_at`        datetime     DEFAULT NULL COMMENT '删除时间',
    `created_at`        datetime     NOT NULL COMMENT '创建时间',
    `updated_at`        datetime     DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`) USING BTREE COMMENT '主键索引',
    KEY `idx_status` (`status`) COMMENT '事务状态索引'
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COMMENT '事务日志记录';
```

### 3.3 使用代码示例

```go
const (
    dsn      = "请输入你的 mysql dsn"
    network  = "tcp"
    address  = "请输入你的 redis ip"
    password = "请输入你的 redis 密码"
)


// 使用 tcc 单测代码
func Test_TCC(t *testing.T) {
    // 创建 redis 客户端
    redisClient := pkg.NewRedisClient(network, address, password)
    // 创建 mysql 客户端
    mysqlDB, err := pkg.NewDB(dsn)
    if err != nil {
        t.Error(err)
        return
    }


    // 构造三个 tcc 组件
    componentAID := "componentA"
    componentBID := "componentB"
    componentCID := "componentC"
    componentA := NewMockComponent(componentAID, redisClient)
    componentB := NewMockComponent(componentBID, redisClient)
    componentC := NewMockComponent(componentCID, redisClient)


    // 构造出事务日志存储模块
    txRecordDAO := dao.NewTXRecordDAO(mysqlDB)
    txStore := NewMockTXStore(txRecordDAO, redisClient)


    // 构造出 txManager 模块
    txManager := txmanager.NewTXManager(txStore, txmanager.WithMonitorTick(time.Second))
    defer txManager.Stop()


    // 完成三个组件的注册
    if err := txManager.Register(componentA); err != nil {
        t.Error(err)
        return
    }


    if err := txManager.Register(componentB); err != nil {
        t.Error(err)
        return
    }


    if err := txManager.Register(componentC); err != nil {
        t.Error(err)
        return
    }


    
    ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
    defer cancel()
    // 启动分布式事务
    success, err := txManager.Transaction(ctx, []*txmanager.RequestEntity{
        {ComponentID: componentAID,
            Request: map[string]interface{}{
                "biz_id": componentAID + "_biz",
            },
        },
        {ComponentID: componentBID,
            Request: map[string]interface{}{
                "biz_id": componentBID + "_biz",
            },
        },
        {ComponentID: componentCID,
            Request: map[string]interface{}{
                "biz_id": componentCID + "_biz",
            },
        },
    }...)
    if err != nil {
        t.Errorf("tx failed, err: %v", err)
        return
    }
    if !success {
        t.Error("tx failed")
        return
    }
    
    // 分布式事务处理成功
    t.Log("success")
}
```

本文落地实践项目的开源地址为 [github](https://github.com/zenthys/tcc) 。 

至此，有关 TCC 分布式事务框架实战篇内容全部讲解完成。 
