+++

date = '2025-03-25T13:53:19+08:00'
title = "testify测试套件"
tags = ["go", "单元测试"]
categories = ["go"]

+++


## 1. 简介

`testify` 是 Go 生态中最常用的测试增强库之一。它不是替代 Go 原生的 `testing` 包，而是在 `testing` 之上提供更强的断言、Mock、测试套件能力。

Go 语言自带的 `testing` 虽然简洁，但当测试用例逐渐增多，尤其是多个测试需要共享初始化逻辑（比如共用一个数据库连接、Mock HTTP 服务器或预加载配置）时，就会出现大量重复的 setup 和 teardown 代码。

`testify/suite` 就是为了解决这个问题而生的。它允许你将一组相关的测试组织到一个结构体中，并通过生命周期钩子统一管理初始化和清理逻辑，让测试代码更清晰、更易维护

### 1.1 testing自带测试框架

```go
package mypackage

import "testing"

func TestAdd(t *testing.T) {
    got := Add(1, 2)
    if got != 3 {
        t.Errorf("Add(1, 2) = %d; want 3", got)
    }
}
```
标准库非常稳定，但写多了你会发现几个痛点：

1. 断言代码繁琐：每次需要判读都需要`if else`
2. 失败信息不友好：标准库需要自己拼错误信息，否则测试失败时你只看到：`--- FAIL: TestSomething`
3. mock需要自己写
4. 复杂测试需要生命周期管理：
   1. 启动数据库容器
   2. 初始化测试数据
   3. 每个测试前重置数据
   4. 每个测试后清理资源
   5. 所有测试结束后关闭连接

而`testify/suite` 提供了更结构化化的方式。

`testify`不是一个单一功能库，它包含几个核心包：



| 包      | 作用                       |
| ------- | -------------------------- |
| assert  | 断言失败但测试继续         |
| require | 断言失败立刻终止当前测试   |
| mock    | 提供Mock对象能力           |
| suite   | 提供测试套件和生命周期管理 |

它依然运行在标准`go test`之上



```go
go test ./...
```

`testify` 只是让测试更好写、更好读、更容易维护。

## 2. 快速上手

### 2.1 安装

首先，项目需要安装Testify:

```sh
go get github.com/stretchr/testify
```

## 2.2 assert 断言

假设现在有一个需要测试的对象。

```go
package calculator

func Add(a, b int) int {
    return a + b
}
```

- **使用标准库进行测试**：

  ```go
  func TestAdd(t *testing.T) {
      got := calculator.Add(1, 2)
      if got != 3 {
          t.Errorf("Add(1, 2) = %d; want 3", got)
      }
  }
  ```

  

- **使用testify进行测试**：

  ```go
  package calculator_test
  
  import (
      "testing"
  
      "github.com/stretchr/testify/assert"
  
      "example.com/calculator"
  )
  
  func TestAdd(t *testing.T) {
      got := calculator.Add(1, 2)
  
      assert.Equal(t, 3, got)
  }
  ```

  测试失败时，testify 会输出类似：

  ```sh
  Error: Not equal: 
      expected: 3
      actual  : 4
  ```

  比手写`t.Errorf`更直观

常见的断言函数：

| 断言函数                                  | 作用                     |
| :---------------------------------------- | ------------------------ |
| assert.Equal(t, expected, actual)         | 相等                     |
| assert.NotEqual(t, unexpected, actual)    | 不相等                   |
| assert.True(t, ok)                        | true                     |
| assert.False(t, failed)                   | false                    |
| assert.Nil(t, err)                        | nil                      |
| assert.NotNil(t, user)                    | non-nil                  |
| assert.Error(t, err)                      | 错误断言                 |
| assert.NoError(t, err)                    |                          |
| assert.ErrorIs(t, err, sql.ErrNoRows)     | 判读错误链               |
| assert.ErrorAs(t, err, &pathErr)          | 判断错误类型             |
| assert.ErrorContains(t, err, "not found") | 判断错误是否包含特定文本 |

所有断言几乎都支持最后的 `msgAndArgs`：

```go
assert.Equal(t, want, got, "order total should match")
```

也可以格式化：

```go
assert.Equal(
    t,
    want,
    got,
    "order id %s total mismatch",
    order.ID,
)
```

工程建议：

不要每个断言都写很长的消息，但在关键业务断言上加消息，能显著提高排错效率。

例如：

```go
require.NoError(t, err, "create order should succeed, req=%+v", req)
```

## 2.3 assert和require

### 2.3.1 assert： 失败后继续执行

```go
func TestUser(t *testing.T) {
    user := GetUser()

    assert.Equal(t, "alice", user.Name)
    assert.Equal(t, 18, user.Age)
    assert.Equal(t, "alice@example.com", user.Email)
}
```

如果第一个断言失败，后面的断言仍会继续执行。

这样做的优点就是：

- 一次性看到多个失败点
- 适合验证多个字段

但是也就会存在缺点：

- 如果前置条件失败，那后续的断言即便失败后也是噪音

### 2.3.2 require：失败后立即停止当前测试

```go
func TestUser(t *testing.T) {
    user, err := GetUser()

    require.NoError(t, err)

    assert.Equal(t, "alice", user.Name)
    assert.Equal(t, 18, user.Age)
}
```

如果`require.NoError(t, err)`失败，后续测试立刻被终止。

这就非常适合前置条件的判断。

### 2.3.3 注意点

`require` 失败时依赖 `t.FailNow()`。

但是 `FailNow` 只能停止当前测试函数所在的 goroutine，不能停止整个测试进程。

所以不要在子 goroutine 里直接用：

```go
go func() {
    require.NoError(t, err) // 不推荐
}()
```

更安全的做法：

```go
errCh := make(chan error, 1)

go func() {
    errCh <- doWork()
}()

select {
case err := <-errCh:
    require.NoError(t, err)
case <-time.After(time.Second):
    t.Fatal("timeout")
}
```

## 2.4 Suite测试套件

一个标准的 Suite 包含三个核心要素：

1. 定义结构体：内嵌 suite.Suite，并包含测试共享的状态。

2. 定义测试方法：方法名必须以 `Test` 开头，签名必须是 func (s *MySuite) TestXxx()，**没有参数**。

3. 创建入口函数：在 TestXxx 函数中调用 suite.Run() 来启动整个套件

```go
package mypackage

import (
    "testing"
    "github.com/stretchr/testify/suite"
)

// 1. 定义测试套件
type MathSuite struct {
    suite.Suite
    data []int // 共享状态
}

// 2. 每个测试前执行
func (s *MathSuite) SetupTest() {
    s.data = []int{1, 2, 3}
}

// 2. 测试方法（必须以 Test 开头）
func (s *MathSuite) TestSum() {
    sum := 0
    for _, v := range s.data {
        sum += v
    }
    s.Equal(6, sum) // 使用套件内置的断言
}

func (s *MathSuite) TestLength() {
    s.Equal(3, len(s.data))
}

// 3. 入口函数：运行套件
func TestMathSuite(t *testing.T) {
    suite.Run(t, new(MathSuite))
}
```

**关键点：** `suite.Run(t, new(MathSuite))` 这一行是必不可少的，它负责解析结构体中的所有 `TestXxx` 方法并按顺序执行

### 2.4.1 生命周期钩子

`testify/suite` 提供了一套完整的生命周期钩子



| 钩子                             | 触发时机               | 适合场景                             |
| -------------------------------- | ---------------------- | ------------------------------------ |
| `SetupSuite`                     | 整个套件开始前执行一次 | 初始化数据库连接、加载mock服务       |
| `TearDownSuite`                  | 整个套件结束后执行一次 | 关闭数据库连接、清理全局资源         |
| `SetupTest`                      | 每个测试方法执行前     | 清空临时表、初始化mock、构造service  |
| `TearDownTest`                   | 每个测试方法执行后     | 验证mock期望、回滚事务、清理临时文件 |
| `BeforeTest(suiteName,testName)` | 每个测试方法执行前     | 执行更细粒度的前置操作，如日志、记时 |
| `AfterTest(suiteName,testName)`  | 每个测试方法执行后     | 执行更细粒度的后置操作，如日志、记时 |

一个测试方法的完整生命周期顺序如下：

```
SetupSuite()   [整个套件开始前，执行一次]
  ↓
SetupTest()    [每个测试方法前，执行一次]
  ↓
BeforeTest(suiteName, testName)   [每个测试方法前，在 SetupTest 之后执行]
  ↓
【实际测试函数】
  ↓
AfterTest(suiteName, testName)    [每个测试方法后，在 TearDownTest 之前执行]
  ↓
TearDownTest() [每个测试方法后，执行一次]
  ↓
TearDownSuite() [整个套件结束后，执行一次]
```

## 3. 表驱动测试

### 3.1 `testing` 

```go
func TestAdd(t *testing.T) {
    testCases := []struct {
        name   string
        a      int
        b      int
        expect int
    }{
        {
            name:   "positive numbers",
            a:      1,
            b:      2,
            expect: 3,
        },
        {
            name:   "negative numbers",
            a:      -1,
            b:      -2,
            expect: -3,
        },
        {
            name:   "zero",
            a:      0,
            b:      0,
            expect: 0,
        },
    }

    for _, tc := range testCases {
        tc := tc

        t.Run(tc.name, func(t *testing.T) {
            
            // t.Parallel()
            
            got := Add(tc.a, tc.b)

            assert.Equal(t, tc.expect, got)
        })
    }
}
```

### 3.2 `testify`

```go
func (s *MySuite)TestAdd() {
    testCases := []struct {
        name   string
        a      int
        b      int
        expect int
    }{
        {
            name:   "positive numbers",
            a:      1,
            b:      2,
            expect: 3,
        },
        {
            name:   "negative numbers",
            a:      -1,
            b:      -2,
            expect: -3,
        },
        {
            name:   "zero",
            a:      0,
            b:      0,
            expect: 0,
        },
    }

    for _, tc := range testCases {
        tc := tc

        s.Run(tc.name, func() {
            
            // s.T().Parallel()
            
            got := Add(tc.a, tc.b)

            s.Assert().Equal(t, tc.expect, got)
        })
    }
}
```

其中的` t.Parallel()`或者`s.T().Parallel()`如果被开启，那就是将子测试并发执行，但是注意，此时测试中就不应该共享可变状态，如果依赖了全局变量、数据库单连接等就要谨慎，可以使用`-race`检测数据竞争。



## 4. Mock

mock是用来隔离外部依赖的，用在单元测试中，也就是只测当前单元，不测外部世界。



使用场景：

- 数据库
- HTTP客户端
- 第三方服务
- 消息队列
- 缓存
- 分布式锁

### 4.1 testify Mock的基本结构

假设接口：

```go
type UserRepository interface {
    FindByID(ctx context.Context, id string) (*User, error)
}
```

手写mock：

```go
type mockUserRepository struct {
    mock.Mock
}

func (m *mockUserRepository) FindByID(ctx context.Context, id string) (*User, error) {
    args := m.Called(ctx, id)

    if args.Get(0) == nil {
        return nil, args.Error(1)
    }

    return args.Get(0).(*User), args.Error(1)
}
```

解释：

- `mock.Mock`是嵌入结构体，提供记录调用、设置期望等能力
- `m.Called`记录本此调用，并根据`on(...)`设置返回结果
- `args.Get(0)`获取第一个返回值
- `args.Error(1)`获取第二个返回值，并要求它是`error`类型

### 4.2 设置期望

```go
repo := &mockUserRepository{}

repo.On("FindByID", mock.Anything, "user-1").
    Return(&User{ID: "user-1", Name: "alice"}, nil).
    Once()
```

含义：

- 当 `FindByID` 被调用；
- 第一个参数任意；
- 第二个参数是 `"user-1"`；
- 返回指定 User 和 nil；
- 只允许调用一次。

使用：

```go
user, err := repo.FindByID(ctx, "user-1")

require.NoError(t, err)
assert.Equal(t, "alice", user.Name)

repo.AssertExpectations(t)
```

### 4.3 参数匹配器

- **精确匹配**

  ```go
  m.On("Method", "alice", 18).Return(nil)
  ```

- **任意参数**

  ```go
  m.On("Method", mock.Anything, mock.Anything).Return(nil)
  ```

- **类型匹配**

  ```go
  m.On("Method", mock.AnythingOfType("string")).Return(nil)
  ```

- **自定义匹配函数**

  ```go
  m.On("Save", mock.MatchedBy(func(order Order) bool {
      return order.Status == StatusAuthorized && order.Total > 0
  })).Return(nil)
  ```

### 4.4 调用次数控制

```go
m.On("Send", mock.Anything).Return(nil).Once() // 期望调用一次
m.On("Send", mock.Anything).Return(nil).Twice() // 期望调用两次
m.On("Send", mock.Anything).Return(nil).Times(n) // 期望调用n次
m.On("Send", mock.Anything).Return(nil).Maybe() // 可以调用，也可以不调用
```

工程上对于副作用操作，比如：

- 支付；
- 扣库存；
- 发消息；
- 写数据库；

建议尽量明确调用次数。否则测试可能漏掉重复调用这种严重 bug。

### 4. 5 AssertExpectations

testify mock不会自动验证期望是否全部完成。必须显示调用`m.AssertExpectations(t)`



也可以在测试结束时统一调用：

```go
// 普通测试
t.Cleanup(func() {
    repo.AssertExpectations(t)
    payment.AssertExpectations(t)
})

// suite测试
func (s *OrderSuite) TearDownTest() {
    s.repo.AssertExpectations(s.T())
    s.payment.AssertExpectations(s.T())
}
```

如果使用了 `suite`，可以放在 `TearDownTest`。

否则可能出现这样的情况：

```go
repo.On("Save", mock.Anything, mock.Anything).Return(nil).Once()
```

`Save`方法根本没被调用，测试却通过了。



## 5. 工程案例

下面给出一个较完整的工程例子。



业务场景：

```
创建订单。
创建订单时需要：校验请求、预留库存、支付授权、保存订单。
如果支付失败或保存失败，需要释放库存。
```

这个例子覆盖了：

- `assert`
- `require`
- `mock`
- `suite`
- 表驱动测试
- 错误包装
- 时间注入
- Mock 期望验证
- 补偿逻辑验证

### 5.1 初始化项目

```bash
mkdir -p order
go mod init example.com
go get github.com/stretchr/testify
```

目录结构：

```
.
├── go.mod
└── order
    ├── order.go
    └── order_test.go
```



### 5.2 业务代码：order/order.go

```go
package order

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidRequest = errors.New("invalid order request")
	ErrInventory      = errors.New("inventory failure")
	ErrPayment        = errors.New("payment failure")
	ErrPersistence    = errors.New("order persistence failure")
)
// Status 订单状态
type Status string

const (
    // StatusAuthorized 订单已授权（支付成功，库存已锁定）
	StatusAuthorized Status = "AUTHORIZED"
)

// Item 订单项，代表一个商品及其购买信息
type Item struct {
	SKU       string // 商品库存单位（唯一标识）
	Quantity  int    // 购买数量
	UnitPrice int    // 单价（单位：分，避免浮点数精度问题）
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	CustomerID string
	Items      []Item
}

// Order 订单实体，代表一个完整的订单
type Order struct {
	ID            string
	CustomerID    string
	Items         []Item
	Total         int
	Status        Status
	ReservationID string
	PaymentAuthID string
	CreatedAt     time.Time
}

// Repository 订单仓储接口，负责订单的持久化
type Repository interface {
	Save(ctx context.Context, o Order) error
}

// Inventory 库存服务接口，负责库存管理
type Inventory interface {
	Reserve(ctx context.Context, customerID string, items []Item) (string, error)
	Release(ctx context.Context, reservationID string) error
}

// Payment 支付服务接口，负责支付处理
type Payment interface {
	Authorize(ctx context.Context, customerID string, amount int) (string, error)
}

// Clock 时钟接口，用于获取当前时间（
type Clock interface {
	Now() time.Time
}

// Service 订单服务
type Service struct {
	repo      Repository // 订单仓储
	inventory Inventory  // 库存服务
	payment   Payment    // 支付服务
	clock     Clock      // 时钟
}

func NewService(
	repo Repository,
	inventory Inventory,
	payment Payment,
	clock Clock,
) *Service {
	return &Service{
		repo:      repo,
		inventory: inventory,
		payment:   payment,
		clock:     clock,
	}
}

func (s *Service) CreateOrder(ctx context.Context, req CreateOrderRequest) (Order, error) {
	// ============ 1. 参数校验 ============
	
	// 校验客户ID不为空
	if req.CustomerID == "" {
		return Order{}, fmt.Errorf("customer id is empty: %w", ErrInvalidRequest)
	}

	// 校验订单项不为空
	if len(req.Items) == 0 {
		return Order{}, fmt.Errorf("items are empty: %w", ErrInvalidRequest)
	}

	// 计算订单总金额，同时校验每个订单项的合法性
	total := 0
	for _, item := range req.Items {
		// 校验商品SKU不为空
		if item.SKU == "" {
			return Order{}, fmt.Errorf("item sku is empty: %w", ErrInvalidRequest)
		}

		// 校验购买数量必须大于0
		if item.Quantity <= 0 {
			return Order{}, fmt.Errorf(
				"item %s has invalid quantity %d: %w",
				item.SKU,
				item.Quantity,
				ErrInvalidRequest,
			)
		}

		// 校验单价不能为负数
		if item.UnitPrice < 0 {
			return Order{}, fmt.Errorf(
				"item %s has invalid unit price %d: %w",
				item.SKU,
				item.UnitPrice,
				ErrInvalidRequest,
			)
		}

		// 累加总金额：数量 × 单价
		total += item.Quantity * item.UnitPrice
	}

	// ============ 2. 预留库存 ============
	// 调用库存服务锁定商品库存，防止超卖
	reservationID, err := s.inventory.Reserve(ctx, req.CustomerID, req.Items)
	if err != nil {
		return Order{}, fmt.Errorf("reserve inventory: %w: %v", ErrInventory, err)
	}

	// ============ 3. 授权支付 ============
	// 调用支付服务进行扣款授权
	paymentAuthID, err := s.payment.Authorize(ctx, req.CustomerID, total)
	if err != nil {
		// 支付失败：释放已预留的库存（补偿操作）
		_ = s.inventory.Release(ctx, reservationID)
		return Order{}, fmt.Errorf("authorize payment: %w: %v", ErrPayment, err)
	}

	// ============ 4. 构建订单对象 ============
	now := s.clock.Now() // 使用注入的时钟获取当前时间（便于测试）

	o := Order{
		// 订单ID生成规则：ord_ + 纳秒时间戳（简化示例，生产环境建议使用分布式ID）
		ID:            fmt.Sprintf("ord_%d", now.UnixNano()),
		CustomerID:    req.CustomerID,
		Items:         req.Items,
		Total:         total,
		Status:        StatusAuthorized, // 初始状态为已授权
		ReservationID: reservationID,
		PaymentAuthID: paymentAuthID,
		CreatedAt:     now,
	}

	// ============ 5. 持久化订单 ============
	// 将订单保存到数据库
	if err := s.repo.Save(ctx, o); err != nil {
		// 保存失败：释放已预留的库存（补偿操作）
		_ = s.inventory.Release(ctx, reservationID)
		return Order{}, fmt.Errorf("save order: %w: %v", ErrPersistence, err)
	}

	// ============ 6. 返回创建成功的订单 ============
	return o, nil
}
```

### 5.3 测试代码：order/order_test.go

```go
package order_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"example.com/order"
)

// fixedClock 用于让时间可预测。
type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

var _ order.Clock = fixedClock{}

// mockRepository 是 order.Repository 的 mock 实现。
type mockRepository struct {
	mock.Mock
}

func (m *mockRepository) Save(ctx context.Context, o order.Order) error {
	args := m.Called(ctx, o)
	return args.Error(0)
}

var _ order.Repository = (*mockRepository)(nil)

// mockInventory 是 order.Inventory 的 mock 实现。
type mockInventory struct {
	mock.Mock
}

func (m *mockInventory) Reserve(
	ctx context.Context,
	customerID string,
	items []order.Item,
) (string, error) {
	args := m.Called(ctx, customerID, items)
	return args.String(0), args.Error(1)
}

func (m *mockInventory) Release(ctx context.Context, reservationID string) error {
	args := m.Called(ctx, reservationID)
	return args.Error(0)
}

var _ order.Inventory = (*mockInventory)(nil)

// mockPayment 是 order.Payment 的 mock 实现。
type mockPayment struct {
	mock.Mock
}

func (m *mockPayment) Authorize(
	ctx context.Context,
	customerID string,
	amount int,
) (string, error) {
	args := m.Called(ctx, customerID, amount)
	return args.String(0), args.Error(1)
}

var _ order.Payment = (*mockPayment)(nil)

// validRequest 构造一个有效请求。
func validRequest() order.CreateOrderRequest {
	return order.CreateOrderRequest{
		CustomerID: "customer-1",
		Items: []order.Item{
			{
				SKU:       "sku-1",
				Quantity:  2,
				UnitPrice: 500,
			},
			{
				SKU:       "sku-2",
				Quantity:  1,
				UnitPrice: 1000,
			},
		},
	}
}

type OrderServiceSuite struct {
	suite.Suite

	repo      *mockRepository
	inventory *mockInventory
	payment   *mockPayment
	clock     fixedClock
	service   *order.Service
	ctx       context.Context
}

func TestOrderServiceSuite(t *testing.T) {
	suite.Run(t, new(OrderServiceSuite))
}

func (s *OrderServiceSuite) SetupTest() {
	s.repo = &mockRepository{}
	s.inventory = &mockInventory{}
	s.payment = &mockPayment{}

	s.clock = fixedClock{
		now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	s.service = order.NewService(
		s.repo,
		s.inventory,
		s.payment,
		s.clock,
	)

	s.ctx = context.Background()
}

func (s *OrderServiceSuite) TearDownTest() {
	s.repo.AssertExpectations(s.T())
	s.inventory.AssertExpectations(s.T())
	s.payment.AssertExpectations(s.T())
}

func (s *OrderServiceSuite) TestCreateOrder_Success() {
	req := validRequest()

	expectedID := fmt.Sprintf("ord_%d", s.clock.Now().UnixNano())

	s.inventory.
		On("Reserve", mock.Anything, req.CustomerID, req.Items).
		Return("reservation-1", nil).
		Once()

	s.payment.
		On("Authorize", mock.Anything, req.CustomerID, 2000).
		Return("payment-auth-1", nil).
		Once()

	var savedOrder order.Order

	s.repo.
		On("Save", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			savedOrder = args.Get(1).(order.Order)
		}).
		Return(nil).
		Once()

	got, err := s.service.CreateOrder(s.ctx, req)

	require.NoError(s.T(), err)

	assert.Equal(s.T(), expectedID, got.ID)
	assert.Equal(s.T(), order.StatusAuthorized, got.Status)
	assert.Equal(s.T(), 2000, got.Total)
	assert.Equal(s.T(), "reservation-1", got.ReservationID)
	assert.Equal(s.T(), "payment-auth-1", got.PaymentAuthID)
	assert.Equal(s.T(), s.clock.Now(), got.CreatedAt)
	assert.Equal(s.T(), req.Items, got.Items)

	assert.Equal(s.T(), req.CustomerID, savedOrder.CustomerID)
	assert.Equal(s.T(), req.Items, savedOrder.Items)
	assert.Equal(s.T(), 2000, savedOrder.Total)
	assert.Equal(s.T(), order.StatusAuthorized, savedOrder.Status)
}

func (s *OrderServiceSuite) TestCreateOrder_InventoryFailure() {
	req := validRequest()

	s.inventory.
		On("Reserve", mock.Anything, req.CustomerID, req.Items).
		Return("", errors.New("out of stock")).
		Once()

	_, err := s.service.CreateOrder(s.ctx, req)

	require.Error(s.T(), err)
	assert.ErrorIs(s.T(), err, order.ErrInventory)

	s.payment.AssertNumberOfCalls(s.T(), "Authorize", 0)
	s.repo.AssertNumberOfCalls(s.T(), "Save", 0)
	s.inventory.AssertNumberOfCalls(s.T(), "Release", 0)
}

func (s *OrderServiceSuite) TestCreateOrder_PaymentFailure_ReleasesInventory() {
	req := validRequest()

	s.inventory.
		On("Reserve", mock.Anything, req.CustomerID, req.Items).
		Return("reservation-1", nil).
		Once()

	s.payment.
		On("Authorize", mock.Anything, req.CustomerID, 2000).
		Return("", errors.New("card declined")).
		Once()

	s.inventory.
		On("Release", mock.Anything, "reservation-1").
		Return(nil).
		Once()

	_, err := s.service.CreateOrder(s.ctx, req)

	require.Error(s.T(), err)
	assert.ErrorIs(s.T(), err, order.ErrPayment)

	s.repo.AssertNumberOfCalls(s.T(), "Save", 0)
}

func (s *OrderServiceSuite) TestCreateOrder_RepositoryFailure_ReleasesInventory() {
	req := validRequest()

	s.inventory.
		On("Reserve", mock.Anything, req.CustomerID, req.Items).
		Return("reservation-1", nil).
		Once()

	s.payment.
		On("Authorize", mock.Anything, req.CustomerID, 2000).
		Return("payment-auth-1", nil).
		Once()

	s.repo.
		On("Save", mock.Anything, mock.Anything).
		Return(errors.New("database down")).
		Once()

	s.inventory.
		On("Release", mock.Anything, "reservation-1").
		Return(nil).
		Once()

	_, err := s.service.CreateOrder(s.ctx, req)

	require.Error(s.T(), err)
	assert.ErrorIs(s.T(), err, order.ErrPersistence)

	s.payment.AssertNumberOfCalls(s.T(), "Authorize", 1)
	s.repo.AssertNumberOfCalls(s.T(), "Save", 1)
	s.inventory.AssertNumberOfCalls(s.T(), "Release", 1)
}

func (s *OrderServiceSuite) TestCreateOrder_InvalidRequests() {
	testCases := []struct {
		name   string
		mutate func(req *order.CreateOrderRequest)
	}{
		{
			name: "empty customer id",
			mutate: func(req *order.CreateOrderRequest) {
				req.CustomerID = ""
			},
		},
		{
			name: "empty items",
			mutate: func(req *order.CreateOrderRequest) {
				req.Items = nil
			},
		},
		{
			name: "empty sku",
			mutate: func(req *order.CreateOrderRequest) {
				req.Items[0].SKU = ""
			},
		},
		{
			name: "zero quantity",
			mutate: func(req *order.CreateOrderRequest) {
				req.Items[0].Quantity = 0
			},
		},
		{
			name: "negative unit price",
			mutate: func(req *order.CreateOrderRequest) {
				req.Items[0].UnitPrice = -1
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			req := validRequest()
			tc.mutate(&req)

			_, err := s.service.CreateOrder(s.ctx, req)

			require.Error(s.T(), err)
			assert.ErrorIs(s.T(), err, order.ErrInvalidRequest)

			s.inventory.AssertNumberOfCalls(s.T(), "Reserve", 0)
			s.payment.AssertNumberOfCalls(s.T(), "Authorize", 0)
			s.repo.AssertNumberOfCalls(s.T(), "Save", 0)
		})
	}
}
```

### 5.4 运行测试

```bash
go test ./... -v -race -count=1
```

如果一切正常，会看到类似输出：

```
=== RUN   TestOrderServiceSuite
=== RUN   TestOrderServiceSuite/TestCreateOrder_Success
=== RUN   TestOrderServiceSuite/TestCreateOrder_InventoryFailure
=== RUN   TestOrderServiceSuite/TestCreateOrder_PaymentFailure_ReleasesInventory
=== RUN   TestOrderServiceSuite/TestCreateOrder_RepositoryFailure_ReleasesInventory
=== RUN   TestOrderServiceSuite/TestCreateOrder_InvalidRequests
--- PASS: TestOrderServiceSuite
PASS
```

