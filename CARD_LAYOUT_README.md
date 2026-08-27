# Hugo 双列卡片布局实现说明

## 概述
为Hugo博客的分类页面(如k8s、Go等)实现了双列卡片布局,替代了原来的单列列表布局。

## 修改的文件

### 1. 自定义列表布局
**文件**: `layouts/_default/list.html`
- 覆盖了Congo主题的默认列表布局
- 将文章列表从单列改为双列网格布局
- 使用 `grid grid-cols-1 md:grid-cols-2 gap-6` 实现响应式双列布局
- 调用新的 `article-card.html` partial来渲染卡片

### 2. 文章卡片组件
**文件**: `layouts/partials/article-card.html`
- 创建了全新的卡片样式组件
- 包含以下元素:
  - 特色图片(如果有)
  - 发布日期和分类标签
  - 文章标题(限制2行)
  - 文章摘要(限制3行)
  - 阅读时间、字数统计
  - 标签列表
- 卡片特性:
  - 白色背景(深色模式下为深色)
  - 圆角边框和阴影效果
  - 悬停时阴影加深
  - 图片悬停时缩放效果

### 3. 自定义CSS样式
**文件**: `assets/css/custom.css`
- 添加了 `line-clamp-2` 和 `line-clamp-3` 类用于文本截断
- 卡片悬停效果
- 响应式布局支持

### 4. Hugo配置更新
**文件**: `hugo.toml`
- 添加了自定义CSS配置(Congo主题会自动加载 `assets/css/custom.css`)

## 布局特点

### 响应式设计
- **移动端**: 单列布局 (`grid-cols-1`)
- **桌面端**: 双列布局 (`md:grid-cols-2`)
- 卡片间距: 6个单位 (`gap-6`)

### 卡片样式
- 圆角: `rounded-lg`
- 阴影: `shadow-md` (悬停时 `shadow-xl`)
- 边框: `border border-neutral-200`
- 过渡动画: `transition-shadow duration-300`

### 内容展示
- 日期和分类标签在顶部
- 标题最多显示2行
- 摘要最多显示3行
- 底部显示阅读时间、字数和标签

## 使用方法

1. 访问任何分类页面,如:
   - `/k8s/` - Kubernetes分类
   - `/go/` - Go语言分类
   - `/ai/` - AI分类
   - `/categories/xxx/` - 任何其他分类

2. 文章会自动以双列卡片形式展示

## 兼容性

- ✅ 保持了Congo主题的所有配置选项
- ✅ 支持深色模式
- ✅ 支持按年份分组 (`groupByYear`)
- ✅ 支持分页
- ✅ 支持面包屑导航
- ✅ 支持目录显示

## 注意事项

1. 如果文章有特色图片(feature或thumbnail),会显示在卡片顶部
2. 如果没有图片,卡片会自动调整布局
3. 所有原有的Hugo配置参数仍然有效
4. 可以通过修改 `custom.css` 来调整卡片样式

## 效果预览

访问 http://localhost:1313/k8s/ 查看实际效果。
