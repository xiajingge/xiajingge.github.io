# Hugo 简洁双列卡片布局

## 概述
为Hugo博客的分类页面实现了简洁的双列卡片布局,只显示核心信息:日期、分类、标题和标签。

## 样式特点

### 卡片内容
- **日期** - 格式: `YYYY-MM-DD` (如 2024-11-22)
- **主分类** - 显示第一个分类,蓝色文字
- **标题** - 蓝色链接,悬停时颜色加深
- **标签组** - 
  - Categories: 绿色背景标签
  - Tags: 紫色背景标签

### 布局特性
- **双列布局** - 桌面端显示2列,移动端显示1列
- **响应式** - 自动适配不同屏幕尺寸
- **按时间排序** - 最新文章在前
- **按年份分组** - 自动按年份分组显示

### 视觉效果
- 白色卡片背景(深色模式下为深色)
- 轻微阴影,悬停时加深
- 圆角边框
- 悬停时轻微上移动画

## 文件结构

```
layouts/
├── _default/
│   └── list.html          # 列表页面布局(双列网格)
└── partials/
    └── article-card.html  # 简洁卡片组件

assets/
└── css/
    └── custom.css         # 自定义样式(标签颜色等)
```

## 使用示例

访问任何分类页面即可看到效果:
- http://localhost:1313/k8s/
- http://localhost:1313/go/
- http://localhost:1313/ai/
- http://localhost:1313/categories/xxx/

## 卡片HTML结构

```html
<article class="...">
  <!-- 日期和主分类 -->
  <div>
    <time>2024-11-22</time>
    <span>kubernetes</span>
  </div>
  
  <!-- 标题 -->
  <h3>
    <a href="...">文章标题</a>
  </h3>
  
  <!-- 标签组 -->
  <div>
    <span class="bg-green-500">category1</span>
    <span class="bg-green-500">category2</span>
    <span class="bg-purple-600">tag1</span>
    <span class="bg-purple-600">tag2</span>
  </div>
</article>
```

## 自定义配置

### 修改标签颜色
编辑 `assets/css/custom.css`:

```css
/* Categories标签 - 当前为绿色 */
.bg-green-500 {
  background-color: #22c55e;
}

/* Tags标签 - 当前为紫色 */
.bg-purple-600 {
  background-color: #9333ea;
}
```

### 修改日期格式
编辑 `layouts/partials/article-card.html`:

```html
<!-- 当前格式: 2024-11-22 -->
{{ .Date.Format "2006-01-02" }}

<!-- 改为中文格式: 2024年11月22日 -->
{{ .Date.Format "2006年01月02日" }}
```

### 调整卡片间距
编辑 `layouts/_default/list.html`:

```html
<!-- 当前间距: gap-6 -->
<div class="grid grid-cols-1 md:grid-cols-2 gap-6 mt-6">

<!-- 改为更大间距: gap-8 -->
<div class="grid grid-cols-1 md:grid-cols-2 gap-8 mt-6">
```

## 兼容性

✅ 保持Congo主题所有功能
✅ 支持深色模式
✅ 支持按年份分组
✅ 支持分页
✅ 响应式设计

## 注意事项

1. 文章需要在front matter中定义categories和tags
2. 日期按降序排列(最新在前)
3. 只显示第一个category作为主分类
4. 所有categories和tags都会显示为标签

## 示例Front Matter

```yaml
---
title: "部署：使用kind安装k8s"
date: 2024-11-22
categories:
  - kubernetes
  - k8s
tags:
  - kubernetes
  - k8s
  - docker
---
```
