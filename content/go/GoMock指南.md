+++

date = '2025-03-25T16:06:32+08:00'
title = "GoMock指南"
tags = ["go", "单元测试"]
categories = ["go"]

+++


## 1. 简介

在 Go 语言的单元测试中，我们常常面临一个困境：待测试的代码依赖了数据库、HTTP 客户端、消息队列等外部组件。直接调用真实依赖会让测试变得不稳定、速度慢，且难以覆盖各种边界条件。

而本文就要讲一个经典的mock框架：`go.uber.org/mock`，它主要做三件事情：

- 根据接口自动生成 mock 实现；
- 允许你在测试中设置“期望”：某个方法会被调用几次、参数是什么、返回什么；
- 在测试结束时验证这些期望是否被满足。



## 2. 快速开始

### 2.1 初始化项目

```sh
mkdir gomock-demo
cd gomock-demo
go mod init example.com/gomock-demo
```

### 2.2 安装依赖和工具

```sh
// 安装依赖
go get go.uber.org/mock/gomock@latest
// 安装工具，安装后，可执行文件一般在$(go env GOPATH)/bin/mockgen
go install go.uber.org/mock/mockgen@latest
```

### 2.3 定义接口和服务

我们写一个非常小的通知服务。

目录：

```
gomock-demo/
  go.mod
  notifier.go
  notifier_test.go
```

#### 2.3.1 notifier.go

```go
package notify

import "context"

//go:generate mockgen -source=notifier.go -destination=mock_notifier.go -package=notify

type Notifier interface {
	Send(ctx context.Context, userID, message string) error
}

type Service struct {
	notifier Notifier
}

func NewService(n Notifier) *Service {
	return &Service{notifier: n}
}

func (s *Service) SendWelcome(ctx context.Context, userID string) error {
	return s.notifier.Send(ctx, userID, "welcome")
}
```

可以发现有一行代码是：`//go:generate mockgen -source=notifier.go -destination=mock_notifier.go -package=notify`

它的意思是：

1. 读取 notifier.go
2. 找到其中的接口
3. 生成文件mock_notifier.go
4. 生成代码的包名是notify

#### 2.3.2 生成mock

执行：

```sh
go generate ./...
```

会生成`mock_notifier.go`文件。

会看到类似的内容：

```go
type MockNotifier struct {
	ctrl     *gomock.Controller
	recorder *MockNotifierMockRecorder
}

func NewMockNotifier(ctrl *gomock.Controller) *MockNotifier {
	return &MockNotifier{ctrl: ctrl}
}

func (m *MockNotifier) Send(ctx context.Context, userID, message string) error {
	// ...
}
```

> 生成的文件是不需要更改的。放在那里即可。

#### 2.3.3 notifier_test.go

```go
package notify

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestService_SendWelcome(t *testing.T) {
	ctrl := gomock.NewController(t)

	n := NewMockNotifier(ctrl)
	svc := NewService(n)

	n.EXPECT().
		Send(gomock.Any(), "user-1", "welcome").
		Return(nil)

	err := svc.SendWelcome(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("SendWelcome failed: %v", err)
	}
}
```

其中`ctrl := gomock.NewController(t)`创建控制器。它会负责管理所有mock的期望和验证。`n := NewMockNotifier(ctrl)`创建mock对象。



`n.EXPECT().Send(gomock.Any(), "user-1", "welcome").Return(nil)`



这句代码是在设置期望：

- `Send` 会被调用；
- 第一个参数 `ctx` 任意；
- 第二个参数必须是 `"user-1"`；
- 第三个参数必须是 `"welcome"`；
- 返回 `nil`。

## 3. 核心概念

### 3.1 Controller 总控制器

`gomock.Controller` 是整个 mock 体系的核心。

它负责：

1. 记录所有期望；
2. 接收 mock 方法的调用；
3. 查找是否有匹配的期望；
4. 返回预设值；
5. 记录实际调用；
6. 在测试结束时验证期望是否满足。

创建方式：

```go
ctrl := gomock.NewController(t)
```

在较新的 `go.uber.org/mock` 版本中，`NewController` 通常会自动注册 `t.Cleanup`，测试结束时自动验证。

> 每个测试使用独立的Controller和Mock
>
> ```go
> func TestA(t *testing.T) {
> 	ctrl := gomock.NewController(t)
> 	repo := NewMockRepository(ctrl)
> 	// ...
> }
> 
> func TestB(t *testing.T) {
> 	ctrl := gomock.NewController(t)
> 	repo := NewMockRepository(ctrl)
> 	// ...
> }
> ```
>
> 

### 3.2 Expect 设置期望

每个生成的 mock 对象都有 `EXPECT()` 方法

例如：`n.EXPECT().Send(gomock.Any(), "user-1", "welcome").Return(nil)`

等价于：我期望 mock 的 Send 方法被调用一次，参数必须满足这些条件，返回值是 nil。

默认情况下，每个 `EXPECT` 期望调用 **一次**。

如果你期望调用两次：`n.EXPECT().Send(gomock.Any(), "user-1", "welcome").Return(nil).Times(2)`



> 如果你期望调用 0 次，通常不要写这个期望。因为如果它被调用了，`gomock` 会报 unexpected call。

### 3.3 Matcher 参数匹配器

`gomock` 的参数不一定是精确值，也可以是匹配规则。

常见 Matcher：

| Matcher                        | 含义                          |
| ------------------------------ | ----------------------------- |
| `gomock.Any()`                 | 匹配任意值                    |
| `gomock.Eq(x)`                 | 等于 x，通常使用深度比较      |
| `gomock.Nil()`                 | 匹配 nil                      |
| `gomock.Not(m)`                | 不匹配 m                      |
| `gomock.Len(i)`                | 长度等于 i                    |
| `gomock.AssignableToTypeOf(x)` | 类型可赋值给 x 的类型         |
| `gomock.InAnyOrder(x)`         | 匹配切片/集合元素，不关心顺序 |

比如：

```go
n.EXPECT().
	Send(gomock.Any(), gomock.Not(""), gomock.Any()).
	Return(nil)
```

表示：

```
ctx 任意，
userID 不能是空字符串，
message 任意。
```

**自定义matcher**



如果内置 Matcher 不够，可以自己实现。

假设我们要求金额必须是正数：

```go
type positiveAmount struct{}

func (positiveAmount) Matches(x any) bool {
	amount, ok := x.(int64)
	return ok && amount > 0
}

func (positiveAmount) String() string {
	return "is a positive int64 amount"
}
```

使用：

```go
pay.EXPECT().
	Charge(gomock.Any(), gomock.Any(), positiveAmount{}).
	Return("tx-1", nil)
```

这在金额、时间、ID、分页参数等场景非常有用。



### 3.4 Return 固定返回值

比如接口是：

```go
Charge(ctx context.Context, orderID string, amount int64) (string, error)
```

那么return必须返回<string,error>

```go
pay.EXPECT().
	Charge(gomock.Any(), "order-1", int64(1000)).
	Return("tx-001", nil)
```

Return 的参数个数和类型必须和接口方法返回值一致。

### 3.5 Do 执行副作用

有时候你不想只返回固定值，而是想在被调用时做一些事情。

例如记录调用：

```go
var called bool

n.EXPECT().
	Send(gomock.Any(), "user-1", "welcome").
	Do(func(ctx context.Context, userID, message string) {
		called = true
	}).
	Return(nil)
```

Do 只执行副作用，返回值仍然来自 Return。

> - **参数**：与被 mock 的接口方法参数**完全一致**
> - **返回值**: 不能有任何返回值

### 3.6 DoAndReturn 动态返回值

如果返回值需要根据输入动态计算，使用 `DoAndReturn`。

```go
pay.EXPECT().
	Charge(gomock.Any(), gomock.Any(), gomock.Any()).
	DoAndReturn(func(ctx context.Context, orderID string, amount int64) (string, error) {
		return "tx-" + orderID, nil
	})
```

> DoAndReturn 的函数签名必须和原方法一致。

### 3.7 Times 调用次数限制

默认调用一次。



但是可以显示指定：`Times(n)`



比如调用1次就是`Times(1)`调用2次就是`Times(2)`。。。



调用任意次数：`AnyTimes()`，0次、1次、多少次都行。



| 方法           | 含义                                 | 示例           |
| -------------- | ------------------------------------ | -------------- |
| `.Times(n)`    | 必须**恰好**调用 n 次                | `.TImes(3)`    |
| `.AnyTimes()`  | **任意次数**（0 次、1 次、多次都行） | `.AnyTimes()`  |
| `.MinTimes(n)` | 至少调用 n 次                        | `.MinTimes(2)` |
| `.MaxTimes(n)` | 最多调用 n 次                        | `.MaxTimes(5)` |

### 3.8 顺序控制 Inorder 和 After

有时候调用顺序很重要。

例如：扣库存 -> 支付 -> 保存订单

```go
gomock.InOrder(
	inv.EXPECT().Reserve(gomock.Any(), "order-1", items).Return(nil),
	pay.EXPECT().Charge(gomock.Any(), "order-1", int64(1000)).Return("tx-1", nil),
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
)
```

也可以用 `After`：

```go
reserve := inv.EXPECT().Reserve(gomock.Any(), "order-1", items).Return(nil)

charge := pay.EXPECT().
	Charge(gomock.Any(), "order-1", int64(1000)).
	Return("tx-1", nil).
	After(reserve)

repo.EXPECT().
	Save(gomock.Any(), gomock.Any()).
	Return(nil).
	After(charge)
```

### 3.9 Finish 测试结束时验证

在旧版本中常见：

```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()
```

`Finish` 会检查：

1. 期望调用但没有调用的；
2. 调用次数不够的；
3. 调用了但没设置期望的；
4. 参数不匹配的。

在较新的 `go.uber.org/mock` 中，通常可以只写`ctrl := gomock.NewController(t)`它会自动注册清理逻辑。



## 4. mockgen

`gomock` 不是运行时动态代理，而是通过代码生成工具`mockgen`来生成代码。



### 4.1 source 模式

从一个 Go 源文件生成：

```
mockgen -source=notifier.go -destination=mock_notifier.go -package=notify
```

参数含义：

| 参数           | 含义       |
| -------------- | ---------- |
| `-source`      | 输入源文件 |
| `-destination` | 输出文件   |
| `-package`     | 输出包名   |

适合本地接口，尤其是接口和使用方在同一个包里。

### 4.2 reflect模式

通过反射生成：

```sh
mockgen -destination=mocks/mocks.go -package=mocks example.com/order Repository,PaymentGateway
```

意思是：从 example.com/order 包里，为 Repository 和 PaymentGateway 生成 mock，输出到 mocks/mocks.go，包名是 mocks。

### 4.3 source vs reflect

| 模式    | 输入            | 适用场景                       |
| ------- | --------------- | ------------------------------ |
| source  | 本地 `.go` 文件 | 本地接口，尤其同包测试         |
| reflect | 包路径 + 接口名 | 第三方包、标准库、**多个接口** |

## 5. 工程案例

### 5.1 项目代码

本节复用[testify测试套件](/content/go/testify测试套件/#5-工程案例)的工程案例。

项目的目录结构是：

```
.
├── go.mod
└── order
    ├── order.go
    ├── generate.go
    ├── mocks
    │   └── order_mocks.go
    └── order_gomock_test.go
```

`order/order.go`还是以前的业务代码。

`order/generate.go`用于防止`go:generate`指令。

```go
package order

//go:generate mockgen -destination=mocks/order_mocks.go -package=mocks example.com/order Repository,Inventory,Payment,Clock
```

`order/mocks/order_mocks.go`由mockgen自动生成。

`order_gomock_test`是重写后的测试代码:

```go
package order_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"example.com/order"
	"example.com/order/mocks"
)

// validOrderRequest 构造一个有效请求。
//
// 注意：这里使用 validOrderRequest，
// 避免和之前 testify 版本测试文件中的 validRequest 冲突。
func validOrderRequest() order.CreateOrderRequest {
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

// newOrderServiceWithMocks 构造带 gomock 依赖的 Service。
func newOrderServiceWithMocks(t *testing.T, ctrl *gomock.Controller) (
	*order.Service,
	*mocks.MockRepository,
	*mocks.MockInventory,
	*mocks.MockPayment,
	*mocks.MockClock,
) {
	t.Helper()

	repo := mocks.NewMockRepository(ctrl)
	inventory := mocks.NewMockInventory(ctrl)
	payment := mocks.NewMockPayment(ctrl)
	clock := mocks.NewMockClock(ctrl)

	service := order.NewService(
		repo,
		inventory,
		payment,
		clock,
	)

	return service, repo, inventory, payment, clock
}

func TestCreateOrder_Success_Gomock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, repo, inventory, payment, clock := newOrderServiceWithMocks(t, ctrl)

	req := validOrderRequest()
	ctx := context.Background()

	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	expectedID := fmt.Sprintf("ord_%d", fixedNow.UnixNano())

	inventory.EXPECT().
		Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
		Return("reservation-1", nil).
		Times(1)

	payment.EXPECT().
		Authorize(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(2000)).
		Return("payment-auth-1", nil).
		Times(1)

	var saved order.Order

	repo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Do(func(ctx context.Context, o order.Order) {
			saved = o
		}).
		Return(nil).
		Times(1)

	clock.EXPECT().
		Now().
		Return(fixedNow).
		Times(1)

	got, err := service.CreateOrder(ctx, req)

	require.NoError(t, err)

	assert.Equal(t, expectedID, got.ID)
	assert.Equal(t, order.StatusAuthorized, got.Status)
	assert.Equal(t, 2000, got.Total)
	assert.Equal(t, "reservation-1", got.ReservationID)
	assert.Equal(t, "payment-auth-1", got.PaymentAuthID)
	assert.Equal(t, fixedNow, got.CreatedAt)
	assert.Equal(t, req.Items, got.Items)

	assert.Equal(t, req.CustomerID, saved.CustomerID)
	assert.Equal(t, req.Items, saved.Items)
	assert.Equal(t, 2000, saved.Total)
	assert.Equal(t, order.StatusAuthorized, saved.Status)
}

func TestCreateOrder_InventoryFailure_Gomock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, _, inventory, _, _ := newOrderServiceWithMocks(t, ctrl)

	req := validOrderRequest()
	ctx := context.Background()

	inventory.EXPECT().
		Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
		Return("", errors.New("out of stock")).
		Times(1)

	_, err := service.CreateOrder(ctx, req)

	require.Error(t, err)
	assert.ErrorIs(t, err, order.ErrInventory)
}

func TestCreateOrder_PaymentFailure_ReleasesInventory_Gomock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, _, inventory, payment, _ := newOrderServiceWithMocks(t, ctrl)

	req := validOrderRequest()
	ctx := context.Background()

	inventory.EXPECT().
		Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
		Return("reservation-1", nil).
		Times(1)

	payment.EXPECT().
		Authorize(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(2000)).
		Return("", errors.New("card declined")).
		Times(1)

	inventory.EXPECT().
		Release(gomock.Any(), gomock.Eq("reservation-1")).
		Return(nil).
		Times(1)

	_, err := service.CreateOrder(ctx, req)

	require.Error(t, err)
	assert.ErrorIs(t, err, order.ErrPayment)
}

func TestCreateOrder_RepositoryFailure_ReleasesInventory_Gomock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service, repo, inventory, payment, clock := newOrderServiceWithMocks(t, ctrl)

	req := validOrderRequest()
	ctx := context.Background()

	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	inventory.EXPECT().
		Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
		Return("reservation-1", nil).
		Times(1)

	payment.EXPECT().
		Authorize(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(2000)).
		Return("payment-auth-1", nil).
		Times(1)

	repo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(errors.New("database down")).
		Times(1)

	inventory.EXPECT().
		Release(gomock.Any(), gomock.Eq("reservation-1")).
		Return(nil).
		Times(1)

	clock.EXPECT().
		Now().
		Return(fixedNow).
		Times(1)

	_, err := service.CreateOrder(ctx, req)

	require.Error(t, err)
	assert.ErrorIs(t, err, order.ErrPersistence)
}

func TestCreateOrder_InvalidRequests_Gomock(t *testing.T) {
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
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			service, _, _, _, _ := newOrderServiceWithMocks(t, ctrl)

			req := validOrderRequest()
			tc.mutate(&req)

			ctx := context.Background()

			_, err := service.CreateOrder(ctx, req)

			require.Error(t, err)
			assert.ErrorIs(t, err, order.ErrInvalidRequest)
		})
	}
}
```

这个版本是使用原生`testing`来进行的。

如果想使用`testify/suite`也是可以的。

核心思路就是：

1. 每个测试方法前创建新的`gomock.Controller`
2. 每个测试方法前重建`Mock`和`Service`
3. 每个测试方法后调用`ctrl.Finish()`

```go
package order_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"example.com/order"
	"example.com/order/mocks"
)

type OrderServiceGomockSuite struct {
	suite.Suite

	ctrl      *gomock.Controller
	repo      *mocks.MockRepository
	inventory *mocks.MockInventory
	payment   *mocks.MockPayment
	clock     *mocks.MockClock

	service *order.Service
	ctx     context.Context

	fixedNow time.Time
}

func TestOrderServiceGomockSuite(t *testing.T) {
	suite.Run(t, new(OrderServiceGomockSuite))
}

func (s *OrderServiceGomockSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())

	s.repo = mocks.NewMockRepository(s.ctrl)
	s.inventory = mocks.NewMockInventory(s.ctrl)
	s.payment = mocks.NewMockPayment(s.ctrl)
	s.clock = mocks.NewMockClock(s.ctrl)

	s.service = order.NewService(
		s.repo,
		s.inventory,
		s.payment,
		s.clock,
	)

	s.ctx = context.Background()

	s.fixedNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
}

func (s *OrderServiceGomockSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *OrderServiceGomockSuite) newValidRequest() order.CreateOrderRequest {
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

func (s *OrderServiceGomockSuite) TestCreateOrder_Success() {
	require := s.Require()
	assert := s.Assert()

	req := s.newValidRequest()
	expectedID := fmt.Sprintf("ord_%d", s.fixedNow.UnixNano())

	s.inventory.EXPECT().
		Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
		Return("reservation-1", nil).
		Times(1)

	s.payment.EXPECT().
		Authorize(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(2000)).
		Return("payment-auth-1", nil).
		Times(1)

	var saved order.Order

	s.repo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Do(func(ctx context.Context, o order.Order) {
			saved = o
		}).
		Return(nil).
		Times(1)

	s.clock.EXPECT().
		Now().
		Return(s.fixedNow).
		Times(1)

	got, err := s.service.CreateOrder(s.ctx, req)

	require.NoError(err)

	assert.Equal(expectedID, got.ID)
	assert.Equal(order.StatusAuthorized, got.Status)
	assert.Equal(2000, got.Total)
	assert.Equal("reservation-1", got.ReservationID)
	assert.Equal("payment-auth-1", got.PaymentAuthID)
	assert.Equal(s.fixedNow, got.CreatedAt)
	assert.Equal(req.Items, got.Items)

	assert.Equal(req.CustomerID, saved.CustomerID)
	assert.Equal(req.Items, saved.Items)
	assert.Equal(2000, saved.Total)
	assert.Equal(order.StatusAuthorized, saved.Status)
}

func (s *OrderServiceGomockSuite) TestCreateOrder_InventoryFailure() {
	require := s.Require()
	assert := s.Assert()

	req := s.newValidRequest()

	s.inventory.EXPECT().
		Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
		Return("", errors.New("out of stock")).
		Times(1)

	_, err := s.service.CreateOrder(s.ctx, req)

	require.Error(err)
	assert.ErrorIs(err, order.ErrInventory)
}

func (s *OrderServiceGomockSuite) TestCreateOrder_PaymentFailure_ReleasesInventory() {
	require := s.Require()
	assert := s.Assert()

	req := s.newValidRequest()

	s.inventory.EXPECT().
		Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
		Return("reservation-1", nil).
		Times(1)

	s.payment.EXPECT().
		Authorize(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(2000)).
		Return("", errors.New("card declined")).
		Times(1)

	s.inventory.EXPECT().
		Release(gomock.Any(), gomock.Eq("reservation-1")).
		Return(nil).
		Times(1)

	_, err := s.service.CreateOrder(s.ctx, req)

	require.Error(err)
	assert.ErrorIs(err, order.ErrPayment)
}

func (s *OrderServiceGomockSuite) TestCreateOrder_RepositoryFailure_ReleasesInventory() {
	require := s.Require()
	assert := s.Assert()

	req := s.newValidRequest()

	s.inventory.EXPECT().
		Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
		Return("reservation-1", nil).
		Times(1)

	s.payment.EXPECT().
		Authorize(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(2000)).
		Return("payment-auth-1", nil).
		Times(1)

	s.repo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		Return(errors.New("database down")).
		Times(1)

	s.inventory.EXPECT().
		Release(gomock.Any(), gomock.Eq("reservation-1")).
		Return(nil).
		Times(1)

	s.clock.EXPECT().
		Now().
		Return(s.fixedNow).
		Times(1)

	_, err := s.service.CreateOrder(s.ctx, req)

	require.Error(err)
	assert.ErrorIs(err, order.ErrPersistence)
}

func (s *OrderServiceGomockSuite) TestCreateOrder_InvalidRequests() {
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
		tc := tc

		s.Run(tc.name, func() {
			require := s.Require()
			assert := s.Assert()

			req := s.newValidRequest()
			tc.mutate(&req)

			_, err := s.service.CreateOrder(s.ctx, req)

			require.Error(err)
			assert.ErrorIs(err, order.ErrInvalidRequest)
		})
	}
}
```

如果表驱动测试中每个子测试也需要独立gomock Controller，那推荐这种写法：

```
package order_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"example.com/order"
	"example.com/order/mocks"
)

// mockDeps 聚合当前子测试使用的所有 mock。
type mockDeps struct {
	repo      *mocks.MockRepository
	inventory *mocks.MockInventory
	payment   *mocks.MockPayment
	clock     *mocks.MockClock

	// savedOrder 用于捕获 Save 方法保存的订单。
	savedOrder *order.Order
}

func newMockDeps(ctrl *gomock.Controller) mockDeps {
	return mockDeps{
		repo:       mocks.NewMockRepository(ctrl),
		inventory:  mocks.NewMockInventory(ctrl),
		payment:    mocks.NewMockPayment(ctrl),
		clock:      mocks.NewMockClock(ctrl),
		savedOrder: &order.Order{},
	}
}

// createOrderCase 表示一个创建订单测试用例。
type createOrderCase struct {
	name       string
	buildReq   func() order.CreateOrderRequest
	setupMocks func(d mockDeps, req order.CreateOrderRequest)
	wantErrIs  error
	verify     func(t *testing.T, d mockDeps, req order.CreateOrderRequest, got order.Order)
}

// newValidRequestForTable 每次返回一个新的有效请求。
func newValidRequestForTable() order.CreateOrderRequest {
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

func TestCreateOrder_TableDriven_Isolated_Gomock(t *testing.T) {
	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	testCases := []createOrderCase{
		{
			name:     "success",
			buildReq: newValidRequestForTable,
			setupMocks: func(d mockDeps, req order.CreateOrderRequest) {
				d.inventory.EXPECT().
					Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
					Return("reservation-1", nil).
					Times(1)

				d.payment.EXPECT().
					Authorize(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(2000)).
					Return("payment-auth-1", nil).
					Times(1)

				d.repo.EXPECT().
					Save(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, o order.Order) {
						*d.savedOrder = o
					}).
					Return(nil).
					Times(1)

				d.clock.EXPECT().
					Now().
					Return(fixedNow).
					Times(1)
			},
			verify: func(t *testing.T, d mockDeps, req order.CreateOrderRequest, got order.Order) {
				expectedID := fmt.Sprintf("ord_%d", fixedNow.UnixNano())

				assert.Equal(t, expectedID, got.ID)
				assert.Equal(t, order.StatusAuthorized, got.Status)
				assert.Equal(t, 2000, got.Total)
				assert.Equal(t, "reservation-1", got.ReservationID)
				assert.Equal(t, "payment-auth-1", got.PaymentAuthID)
				assert.Equal(t, fixedNow, got.CreatedAt)
				assert.Equal(t, req.Items, got.Items)

				assert.Equal(t, req.CustomerID, d.savedOrder.CustomerID)
				assert.Equal(t, req.Items, d.savedOrder.Items)
				assert.Equal(t, 2000, d.savedOrder.Total)
				assert.Equal(t, order.StatusAuthorized, d.savedOrder.Status)
			},
		},

		{
			name:     "inventory failure",
			buildReq: newValidRequestForTable,
			setupMocks: func(d mockDeps, req order.CreateOrderRequest) {
				d.inventory.EXPECT().
					Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
					Return("", errors.New("out of stock")).
					Times(1)
			},
			wantErrIs: order.ErrInventory,
		},

		{
			name:     "payment failure releases inventory",
			buildReq: newValidRequestForTable,
			setupMocks: func(d mockDeps, req order.CreateOrderRequest) {
				d.inventory.EXPECT().
					Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
					Return("reservation-1", nil).
					Times(1)

				d.payment.EXPECT().
					Authorize(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(2000)).
					Return("", errors.New("card declined")).
					Times(1)

				d.inventory.EXPECT().
					Release(gomock.Any(), gomock.Eq("reservation-1")).
					Return(nil).
					Times(1)
			},
			wantErrIs: order.ErrPayment,
		},

		{
			name:     "repository failure releases inventory",
			buildReq: newValidRequestForTable,
			setupMocks: func(d mockDeps, req order.CreateOrderRequest) {
				d.inventory.EXPECT().
					Reserve(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(req.Items)).
					Return("reservation-1", nil).
					Times(1)

				d.payment.EXPECT().
					Authorize(gomock.Any(), gomock.Eq(req.CustomerID), gomock.Eq(2000)).
					Return("payment-auth-1", nil).
					Times(1)

				d.repo.EXPECT().
					Save(gomock.Any(), gomock.Any()).
					Return(errors.New("database down")).
					Times(1)

				d.inventory.EXPECT().
					Release(gomock.Any(), gomock.Eq("reservation-1")).
					Return(nil).
					Times(1)

				d.clock.EXPECT().
					Now().
					Return(fixedNow).
					Times(1)
			},
			wantErrIs: order.ErrPersistence,
		},

		{
			name: "empty customer id",
			buildReq: func() order.CreateOrderRequest {
				req := newValidRequestForTable()
				req.CustomerID = ""
				return req
			},
			wantErrIs: order.ErrInvalidRequest,
		},

		{
			name: "empty items",
			buildReq: func() order.CreateOrderRequest {
				req := newValidRequestForTable()
				req.Items = nil
				return req
			},
			wantErrIs: order.ErrInvalidRequest,
		},

		{
			name: "empty sku",
			buildReq: func() order.CreateOrderRequest {
				req := newValidRequestForTable()
				req.Items[0].SKU = ""
				return req
			},
			wantErrIs: order.ErrInvalidRequest,
		},

		{
			name: "zero quantity",
			buildReq: func() order.CreateOrderRequest {
				req := newValidRequestForTable()
				req.Items[0].Quantity = 0
				return req
			},
			wantErrIs: order.ErrInvalidRequest,
		},

		{
			name: "negative unit price",
			buildReq: func() order.CreateOrderRequest {
				req := newValidRequestForTable()
				req.Items[0].UnitPrice = -1
				return req
			},
			wantErrIs: order.ErrInvalidRequest,
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			// 每个子测试独立创建 Controller。
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// 每个子测试独立创建 mocks。
			deps := newMockDeps(ctrl)

			service := order.NewService(
				deps.repo,
				deps.inventory,
				deps.payment,
				deps.clock,
			)

			req := tc.buildReq()

			// 当前用例设置自己的期望。
			if tc.setupMocks != nil {
				tc.setupMocks(deps, req)
			}

			got, err := service.CreateOrder(context.Background(), req)

			if tc.wantErrIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErrIs)
			} else {
				require.NoError(t, err)
			}

			if tc.verify != nil {
				tc.verify(t, deps, req, got)
			}
		})
	}
}
```

就是在testCases中以方法类型参数加进去。

### 5.2 运行

初始化依赖：

```sh
go get go.uber.org/mock/gomock
go get github.com/stretchr/testify
go install go.uber.org/mock/mockgen@latest
```

生成Mock：

```sh
go generate ./...
```

运行测试：

```sh
go test ./... -v -race -count=1
```

