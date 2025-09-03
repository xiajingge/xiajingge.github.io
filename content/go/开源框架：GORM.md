+++
date = '2025-09-03T16:35:13+08:00'
title = "开源系列--GORM"
tags = ["go"]
categories = ["opensource"]
+++

GORM 应该是 Go 生态里用得最多的 ORM 了。它的核心思路是用结构体对象去间接操作数据库：你只管操作 Go 对象，SQL 交给它来生成。用法本身没什么好讲的，官方文档很全，没上手过的直接看[gorm官方指南](https://gorm.io/zh_CN/)就行。

这篇文章关心的是另一个问题：那些 `db.Where().Order().Find()` 的链式调用，内部是怎么一步步变成 SQL 的？所以下面会直接翻源码。只讲主线，不抠细节，属于扫盲性质。

## 1. 整体架构

先把 GORM 的分层画出来，后面所有内容都能对号入座：

```
┌──────────────────────────────────────────────┐
│                使用层 / API 层                 │
│  db.Model().Where().Order().Find()            │
└──────────────────────────────────────────────┘
                      ↓
┌──────────────────────────────────────────────┐
│                Statement 层                   │
│  保存 Model/Dest/Table/Clauses/Vars/SQL       │
└──────────────────────────────────────────────┘
                      ↓
┌──────────────────────────────────────────────┐
│                Callbacks 层                   │
│  create/query/update/delete/raw/row           │
└──────────────────────────────────────────────┘
                      ↓
┌──────────────────────────────────────────────┐
│                Clause / Builder 层            │
│  SELECT / FROM / WHERE / INSERT / VALUES      │
└──────────────────────────────────────────────┘
                      ↓
┌──────────────────────────────────────────────┐
│                Dialector / ConnPool 层        │
│  MySQL / PostgreSQL / SQLite / database/sql   │
└──────────────────────────────────────────────┘
```

落到源码文件上，各处的分工大致是这样：

| 文件                  | 作用                              |
| --------------------- | --------------------------------- |
| `gorm.go`             | `Open`、`DB`、`Session`、事务入口 |
| `chainable_api.go`    | 链式条件方法                      |
| `finisher_methods.go` | 执行型方法                        |
| `callbacks/`          | 真正的执行器                      |
| `statement.go`        | SQL 构建上下文                    |
| `clause/`             | SQL 子句抽象                      |
| `schema/`             | ORM 元信息解析                    |
| `scan.go`             | 查询结果映射                      |

整个源码拆下来其实就是三件事：SQL 的生成被拆成了一个个很小的 `clause`，执行流程被拆成了可插拔的 `callbacks`，模型到表的映射交给 `schema` 去做。后面几节基本也是围着这三块展开。

## 2. 核心对象

### 2.1 *gorm.DB

先从最常打交道的 `*gorm.DB` 说起。要注意它并不是数据库连接本身，而是一个"GORM 会话句柄"，结构如下：

```go
type DB struct {
	*Config
	Error        error
	RowsAffected int64
	Statement    *Statement
	clone        int
}
```

| 字段           | 含义                                                         |
| -------------- | ------------------------------------------------------------ |
| `Config`       | 全局配置：Dialector、ConnPool、Logger、NamingStrategy、插件、回调等 |
| `Error`        | 当前链路的错误                                               |
| `RowsAffected` | 受影响行数                                                   |
| `Statement`    | 当前 SQL 上下文                                              |
| `clone`        | 是否是模板 DB，用于链式复制                                  |

它有几个容易误解的地方：链式调用返回的往往不是原来那个 `db`，而是一个复制出来的新实例，靠的就是 `clone` 字段；真正的连接池藏在 `Statement.ConnPool` 里，所以它和 `database/sql` 的 `*sql.DB` 不是一回事。

#### 2.1.1 Config

`Config` 保存 GORM 的全局配置，`Open` 时确定，之后所有会话共享：

```go
type Config struct {
	Dialector      Dialector // 数据库驱动抽象
	ConnPool       ConnPool // 真正执行SQL的连接池
	Logger         logger.Interface
	NowFunc        func() time.Time
	DryRun         bool // 只生成SQL不执行
	PrepareStmt    bool // 预编译SQL
	SkipDefaultTransaction bool // 跳过默认事务
	NamingStrategy schema.Namer // 命名策略：结构体名 -> 表名
	ClauseBuilders map[string]clause.ClauseBuilder // 允许驱动覆盖某些 SQL 子句的构建方式
	plugins        map[string]Plugin
	cacheStore     *sync.Map // 缓存 schema 解析结果
	callbacks      *callbacks // 回调系统
}
```

#### 2.1.2 Statement

`Statement` 保存一次 SQL 执行所需要的一切，可以理解为"这次查询的草稿纸"：

```go
type Statement struct {
	// 运行时环境
    DB             *DB // 用于后续的链式调用
	ConnPool       ConnPool // 数据库连接池的接口，负责执行sql语句的地方
	Context        context.Context // context相关功能：超时控制、取消信号等
	// 数据模型映射
    TableExpr      *clause.Expr // 表名的表达式，比如users AS u
	Table          string // 指定的表名
	Model          interface{} // 模型对象，GORM会通过反射获得表名、schema和关联关系
    Dest           interface{} // 目标对象，即查询结果要填充的对象，比如&User{}
	ReflectValue   reflect.Value // Model和Dest的反射值缓存，避免重复反射
	Schema         *schema.Schema // 解析后的模型元数据（字段映射、关联关系、表名等）
	// sql 构建，核心组装部分
    Clauses        map[string]clause.Clause // sql子句集合，比如where:id=? 
	BuildClauses   []string // 指定构建Clauses成sql的顺序，比如[select,from,where,order by]
	SQL            strings.Builder // 最终sql语句
	Vars           []interface{} // sql中的占位符参数，对应？的值
	// 高级特性
    SkipHooks      bool // 是否跳过钩子函数
	Unscoped       bool // 是否忽略软删除，为true时查询会包含已软删除的记录
	Preloads       map[string][]interface{} // 预加载关联关系的配置
	Joins          []join // Join操作的配置
	Settings       sync.Map // 线程安全的键值对存储
}
```

在 `Create`、`Find`、`Update` 的调用链中，每次执行都会新建一个 `Statement`，链式方法（`Where`、`Order`）不断往它的 `Clauses` 里塞子句，最后由 callbacks 按 `BuildClauses` 的顺序把这些子句拼成 SQL 并执行。记住这条主线，剩下的都是细节。

### 2.2 schema.Schema

`schema.Schema` 描述一个 Go 结构体在数据库中的元信息，是"结构体 -> 表"映射的载体：

```go
type Schema struct {
    // 基础信息
	Name                      string // 结构体名称，不是表名称
	ModelType                 reflect.Type // 结构体类型
	Table                     string // 表名
    // 主键相关
	PrioritizedPrimaryField   *Field // 优先级最高的主键字段
    PrimaryFields             []*Field // 所有主键字段的切片
	PrimaryFieldDBNames       []string // 所有主键对应的数据库列名字段
    
    // 字段
    Fields                    []*Field // 所有字段的切片（包括主键、普通字段、关联字段）
	FieldsByName              map[string]*Field // 通过结构体字段名快速查找字段元数据
	FieldsByDBName            map[string]*Field // 通过数据库列名快速查找字段元数据
	
    // 关联关系
    Relationships             Relationships  // 存储模型间的所有关联关系（HasOne、HasMany、BelongsTo、ManyToMany）
	
    // sql子句模板，用于CRUD
    CreateClauses             []clause.Interface // 构建 INSERT 语句时默认使用
	QueryClauses              []clause.Interface // 构建 SELECT 语句时默认使用
	UpdateClauses             []clause.Interface // 构建 UPDATE 语句时默认使用
	DeleteClauses             []clause.Interface // 构建 DELETE 语句时默认使用
}
```

举个例子。有这样一张表：

```go
// User 用户表
type User struct {
    ID        uint           `gorm:"primaryKey;column:user_id"`
    Username  string         `gorm:"column:user_name;type:varchar(50);uniqueIndex:idx_username"`
    Email     string         `gorm:"column:email;type:varchar(100);uniqueIndex"`
    Age       int            `gorm:"column:age;default:0"`
    Active    bool           `gorm:"column:active;default:true"`
    CreatedAt time.Time      `gorm:"column:created_at"`
    UpdatedAt time.Time      `gorm:"column:updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index:idx_deleted"`

    // 关联关系
    Orders    []Order        `gorm:"foreignKey:UserID"`
    Profile   Profile        `gorm:"foreignKey:UserID"`
}
```

当程序启动执行 `db.AutoMigrate(&User{})`，或者第一次用到这个模型时，GORM 会通过反射解析 `User` 结构体，生成对应的 `Schema` 对象。大概长这样：

```go
schema := &Schema{
    Name:  "User",
    Table: "users", // 默认蛇形复数表名，也可通过 TableName() 自定义

    PrioritizedPrimaryField: field_ID,   // 指向 ID 字段（指针）
    PrimaryFields:           []*Field{field_ID},

    Fields: []*Field{
        field_ID, field_Username, field_Email, field_Age, field_Active,
        field_CreatedAt, field_UpdatedAt, field_DeletedAt,
        field_Orders,   // 关联字段（HasMany）
        field_Profile,  // 关联字段（HasOne）
    },
    FieldsByDBName: map[string]*Field{
        "user_id":    field_ID,
        "user_name":  field_Username,
        "email":      field_Email,
        // ...
        // 注意：关联字段没有对应的数据库列名，不在此 map 中
    },

    Relationships: Relationships{
        // HasMany: User 有多个 Order
        "Orders": {Type: HasMany, References: []Reference{{PrimaryKey: "ID", ForeignKey: "UserID"}}},
        // HasOne: User 有一个 Profile
        "Profile": {Type: HasOne, References: []Reference{{PrimaryKey: "ID", ForeignKey: "UserID"}}},
    },
}
```

除了字段映射和关联关系，`Schema` 上还挂着一组默认的 CRUD 子句模板（`CreateClauses`、`QueryClauses` 等）和索引信息，构建对应 SQL 时会直接拿去用，这里就不展开了。解析结果会缓存在 `Config` 的 `cacheStore` 里，同一个模型只会解析一次。

### 2.3 clause.Clause

GORM 内部不会把 SQL 当成一个整字符串去拼，而是先拆成子句。比如：

```sql
SELECT * FROM users WHERE age > 18 ORDER BY id LIMIT 10
```

会被拆成 `SELECT`、`FROM`、`WHERE`、`ORDER BY`、`LIMIT` 五个部分，每部分都是一个 `clause`。每个子句只要实现三个方法：

```go
type Interface interface {
	Name() string
	Build(Builder) // 如何把自己组装成sql
	MergeClause(*Clause) // 如何和已有子句合并
}
```

常见的子句有：

| Clause           | SQL 片段               |
| ---------------- | ---------------------- |
| `clause.Select`  | `SELECT ...`           |
| `clause.From`    | `FROM ...`             |
| `clause.Where`   | `WHERE ...`            |
| `clause.GroupBy` | `GROUP BY ...`         |
| `clause.OrderBy` | `ORDER BY ...`         |
| `clause.Limit`   | `LIMIT ... OFFSET ...` |
| `clause.Join`    | `JOIN ...`             |
| `clause.Insert`  | `INSERT INTO ...`      |
| `clause.Values`  | `VALUES ...`           |

## 3. gorm.Open()

通常这样初始化：

```go
dsn := "user:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True"
db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
```

这里有两个对象：`mysql.Open(dsn)` 返回一个 `Dialector`，负责 MySQL 相关的差异；`gorm.Open()` 负责创建 `*gorm.DB`。简化后的 `Open` 逻辑如下：

```go
func Open(dialector Dialector, opts ...Option) (*DB, error) {
	config := &Config{}

	for _, opt := range opts {
		if opt != nil {
			opt.Apply(config)
		}
	}

	if config.NamingStrategy == nil {
		config.NamingStrategy = schema.NamingStrategy{}
	}

	if config.Logger == nil {
		config.Logger = logger.Default
	}

	if dialector != nil {
		config.Dialector = dialector
	}

	db := &DB{
		Config: config,
		clone:  1,
	}

	db.callbacks = initializeCallbacks(db)

	if config.Dialector != nil {
		err := config.Dialector.Initialize(db)
		if err != nil {
			return nil, err
		}
	}

	return db, nil
}
```

有两个地方值得说：

- `clone`

  `Open` 返回的 `*gorm.DB` 是一个根对象，也叫模板对象，它内部的 `clone = 1` 表示：

  > 后续第一次链式调用时，需要复制出一个新的 `*gorm.DB`，不能污染这个根对象。

  我们平时全局共享一个 `db` 变量却不会互相串查询，靠的就是这个机制。

- `Dialector`

  GORM 能支持 MySQL、PostgreSQL、SQLite、SQLServer，核心就是 `Dialector`。它把各家数据库的差异抽象成了接口：

```go
type Dialector interface {
	Name() string
	Initialize(db *DB) error // 初始化连接、回调、子句构建器
	Migrator(db *DB) Migrator // DDL迁移能力
	DataTypeOf(field *schema.Field) string // 字段类型映射
	DefaultValueOf(field *schema.Field) clause.Expression // 默认值表达式
	BindVarTo(writer clause.Writer, stmt *Statement, v interface{}) // 占位符风格，例如MySQL的 ?、PostgreSQL的 $1
	QuoteTo(writer clause.Writer, str string) // 标识符转义，例如 MySQL 的反引号、PostgreSQL 的双引号
	Explain(sql string, vars ...interface{}) string // SQL日志解释
}
```

## 4. 一次完整查询

最后把前面的东西串起来。假设代码是这样写的：

```go
type User struct {
	ID   uint
	Name string
	Age  int
}

var users []User

db.WithContext(ctx).
	Where("age > ?", 18).
	Order("id desc").
	Limit(10).
	Find(&users)
```

### 4.1 db.WithContext

```go
func (db *DB) WithContext(ctx context.Context) *DB {
	tx := db.getInstance()
	tx.Statement.Context = ctx
	return tx
}
```

#### 4.1.1 db.getInstance

关键在 `getInstance`。GORM 的链式调用不是简单地返回 `db` 自身，而是在需要时复制出一个新的 `*gorm.DB`：

```go
func (db *DB) getInstance() *DB {
	if db.clone > 0 {
		tx := &DB{
			Config: db.Config,
			Error:  db.Error,
		}

		if db.clone == 1 {
			// 第一次从模板分裂出来：创建全新的 Statement
			tx.Statement = &Statement{
				DB:       tx,
				ConnPool: db.Statement.ConnPool,
				Context:  db.Statement.Context,
				Clauses:  map[string]clause.Clause{},
				Vars:     make([]interface{}, 0, 8),
			}
		} else {
			// 链已经开始了：克隆现有 Statement 再接着改
			tx.Statement = db.Statement.clone()
			tx.Statement.DB = tx
		}

		return tx
	}

	return db
}
```

每一步链式调用拿到的都是自己的副本，所以多次查询之间互不干扰，全局的根 `db` 也始终是干净的。

不过别误会：复制出来的实例本身并不是并发安全的。同一个 `tx` 拿到多个 goroutine 里同时改，还是会有数据竞争，正确的做法是每个 goroutine 都从根 `db` 出发重新起链。

### 4.2 db.Where

```go
func (db *DB) Where(query interface{}, args ...interface{}) *DB {
	tx := db.getInstance()
	tx.Statement.AddClause(buildCondition(query, args...))
	return tx
}
```

拿到 `tx` 之后，会把条件包装成：

```go
clause.Where{
	Exprs: []clause.Expression{...},
}
```

如果多次调用：

```go
db.Where("age > ?", 18).Where("name = ?", "alice")
```

最终会合并成 `WHERE age > ? AND name = ?`。合并这个动作就是 `MergeClause` 干的，每种子句自己决定怎么合。

### 4.3 Order 和 Limit

这两个方法就是"往 Statement 里塞一个子句"，没什么花样：

```go
func (db *DB) Order(value interface{}) *DB {
	tx := db.getInstance()

	switch v := value.(type) {
	case clause.OrderByColumn:
		tx.Statement.AddClause(clause.OrderBy{
			Columns: []clause.OrderByColumn{v},
		})
	case string:
		tx.Statement.AddClause(clause.OrderBy{
			Columns: []clause.OrderByColumn{{
				Column: clause.Column{Name: v, Raw: true}, // 字符串原样输出，不做转义
			}},
		})
	}

	return tx
}

func (db *DB) Limit(limit int) *DB {
	tx := db.getInstance()
	tx.Statement.AddClause(clause.Limit{Limit: &limit})
	return tx
}
```

### 4.4 db.Find

前面的链式方法都只是攒条件，真正动手的是 `Find` 这类 finisher 方法：

```go
func (db *DB) Find(dest interface{}, conds ...interface{}) *DB {
	tx := db.getInstance()
	tx.Statement.Dest = dest

	if len(conds) > 0 {
		tx.Statement.AddClause(clause.Where{
			Exprs: []clause.Expression{buildCondition(conds, nil)},
		})
	}

	tx.callbacks.Query().Execute(tx)
	return tx
}
```

`Find` 自己只做两件事：把结果写入目标 `Dest` 存好，然后把活儿交给 `callbacks.Query().Execute(tx)`。另外注意，`Find` 查不到数据不会报错，想要 `ErrRecordNotFound` 得用 First 系方法。

#### 4.4.1 db.First、db.Take、db.Last

这三个和 Find 类似，都是查一条，行为略有差别：

- `first` 相当于 `SELECT * FROM users ORDER BY id LIMIT 1`，会自动按主键排序并加 limit，查不到就返回 `ErrRecordNotFound`
- `take` 相当于 `SELECT * FROM users LIMIT 1`
- `last` 相当于 `SELECT * FROM users ORDER BY id DESC LIMIT 1`

### 4.5 db.Callbacks

`Execute` 背后是 GORM 的回调系统。每种操作对应一条回调链，核心的几条：

| 回调名称      | 作用         |
| ------------- | ------------ |
| `gorm:create` | 执行插入     |
| `gorm:query`  | 执行查询     |
| `gorm:update` | 执行更新     |
| `gorm:delete` | 执行删除     |
| `gorm:row`    | 执行 `Row()` |
| `gorm:raw`    | 执行 `Raw()` |

回调处理器的概念模型如下，其实有点中间件的意思：

```go
type processor struct {
	name      string
	callbacks []*callback // 排好序的回调链
	fns       []func(*gorm.DB) // 编译后的执行函数链
}

func (p *processor) Execute(db *gorm.DB) {
	for _, fn := range p.fns {
		fn(db)
	}
}
```

比如 `Create` 执行时，回调顺序大致是：

```
BeforeSave
BeforeCreate
gorm:create
AfterCreate
AfterSave
```

这条链是开放给我们改的。可以注册自己的回调：

```go
db.Callback().Query().After("gorm:query").Register("my:metric", func(db *gorm.DB) {
	log.Println("query executed:", db.Statement.SQL.String())
})
```

也可以替换某一步：

```go
db.Callback().Query().Replace("gorm:query", func(db *gorm.DB) {
	// custom query logic
})
```

或者删掉：

```go
db.Callback().Create().Remove("gorm:create")
```

GORM 很多扩展能力（读写分离、多租户插件之类）就是靠这套机制实现的。

### 4.6 clause系统

#### 4.6.1 clause.Expression

很多子句内部不是简单字符串，而是表达式。`clause.Interface` 在 2.3 已经见过了，这里看它的内部构件 `Expression`：

```go
type Expression interface {
	Build(Builder)
}
```

例如：

```go
clause.Eq{Column: "age", Value: 18}
clause.Gt{Column: "age", Value: 18}
clause.IN{Column: "id", Values: []int{1, 2, 3}}
```

会被组装成：

```
age = ?
age > ?
id IN (?,?,?)
```

#### 4.6.2 clause.Where

`Where` 是最典型的合并子句，前面 4.2 说的合并逻辑就在这：

```go
type Where struct {
	Exprs []Expression
}

func (where Where) MergeClause(c *Clause) {
	if w, ok := c.Expression.(Where); ok {
		exprs := make([]Expression, 0, len(w.Exprs)+len(where.Exprs))
		exprs = append(exprs, w.Exprs...)
		exprs = append(exprs, where.Exprs...)
		c.Expression = Where{Exprs: exprs}
		return
	}

	c.Expression = where
}
```

每次 `Where` 都把已有条件和新条件追加到一起，天然就是 AND 语义。

#### 4.6.3 Builder

`clause.Build` 需要一个 `Builder`，GORM 中由 `Statement` 实现：

```go
type Builder interface {
	Writer
	WriteQuoted(field interface{})
	AddVar(writer Writer, vars ...interface{})
	AddError(error) error
}
```

两个关键方法：

- `WriteQuoted` 负责写标识符（列名、表名），转义风格由 Dialector 决定。比如 `stmt.WriteQuoted(clause.Column{Name: "user_name"})`，MySQL 输出 `` `user_name` ``，PostgreSQL 输出 `"user_name"`
- `AddVar` 负责写占位符。比如 `stmt.AddVar(stmt, 18)`，MySQL 输出 `?`，PostgreSQL 输出 `$1`

到这里，一次 `Find` 从链式调用到 SQL 落地的路径就通了：链式方法往 `Statement` 里塞 clause，finisher 方法触发 callbacks，callbacks 按顺序把 clause 拼成 SQL，最后交给 ConnPool 执行。事务、Preload、软删除这些主题，都是在这一条主线上加的东西，有兴趣可以顺着这条线自己去翻源码。
