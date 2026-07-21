# 02 · 设计 Token

> 所有 token 以 **CSS 变量** 形式定义，集中在 3 个 CSS 文件：
>
> - `web/default/src/styles/index.css` — 全局基础、字体加载、CSS 动画
> - `web/default/src/styles/theme.css` — `@theme inline` 桥接 + `:root`/`.dark` 默认配色
> - `web/default/src/styles/theme-presets.css` — 10 套预设 + 字体/圆角/密度/布局轴
>
> **没有 `tailwind.config.{js,ts}`**。Tailwind v4 全靠 CSS 内的 `@theme inline` 块驱动工具类生成。

## 2.1 色彩空间约定

- 全部使用 **OKLCH**（`oklch(L C H)` 或 `oklch(L C H / A)`）
- 表面派生用 `color-mix(in oklch, var(--primary) N%, var(--background))`
- **禁止**在新 token 中使用 HEX/HSL（除非是外部库要求，如 meta theme-color）
- OKLCH 优势：感知均匀、跨设备一致、暗色模式亮度可控

## 2.2 颜色 Token 命名约定

### 层级化命名：`<角色>[-<变体>][-foreground]`

| 角色 | 说明 |
|------|------|
| `background` / `foreground` | 全局底色 / 全局前景文字 |
| `card` / `card-foreground` | 卡片表面 / 卡片文字 |
| `popover` / `popover-foreground` | 浮层（下拉、tooltip）/ 浮层文字 |
| `primary` / `primary-foreground` | 主色（按钮底色）/ 主色上的文字 |
| `secondary` / `secondary-foreground` | 次要按钮 / 文字 |
| `muted` / `muted-foreground` | 静音表面 / 次要文字 |
| `accent` / `accent-foreground` | 强调色（hover/active/选中态） |
| `destructive(-foreground)` | 危险/错误 |
| `success(-foreground)` | 成功 |
| `warning(-foreground)` | 警告 |
| `info(-foreground)` | 信息 |
| `neutral(-foreground)` | 中性按钮 |
| `border` / `input` / `ring` | 描边 / 输入框 / 焦点环 |
| `chart-1` ~ `chart-5` | 图表色板（5 色） |
| `overview-accent-1/2/3` | Dashboard 概览强调色 |
| `sidebar(-foreground/-primary/-accent/-border/-ring)` | **侧边栏独立子主题** |
| `skeleton-base` / `skeleton-highlight` | 骨架屏 |
| `table-row/header/header-hover/disabled(-hover/-border)` | 表格行 |

### Tailwind 工具类映射

`@theme inline` 块（theme.css:21-94）把每个 CSS 变量桥接到 Tailwind 工具类，可直接使用：

```
bg-background, text-foreground, bg-card, text-card-foreground,
bg-primary, text-primary-foreground, bg-secondary, ...,
border-border, ring-ring, bg-chart-1, bg-sidebar, text-sidebar-accent-foreground, ...
```

## 2.3 默认配色（亮色 / 暗色）

来源：`theme.css:96-239`。这是 `default` 预设的色值；其他预设会覆盖部分 token。

### 表面层

| Token | 亮色 `:root` | 暗色 `.dark` |
|-------|------|------|
| `--background` | `oklch(1 0 0)` | `oklch(0.235 0 0)` |
| `--foreground` | `oklch(0.145 0 0)` | `oklch(0.965 0 0)` |
| `--card` | `oklch(1 0 0)` | `oklch(0.285 0 0)` |
| `--card-foreground` | `oklch(0.145 0 0)` | `oklch(0.965 0 0)` |
| `--popover` | `oklch(1 0 0)` | `oklch(0.305 0 0)` |
| `--popover-foreground` | `oklch(0.145 0 0)` | `oklch(0.965 0 0)` |
| `--muted` | `oklch(0.97 0 0)` | `oklch(0.305 0 0)` |
| `--muted-foreground` | `oklch(0.49 0 0)` | `oklch(0.78 0 0)` |
| `--border` | `oklch(0.93 0 0)` | `oklch(1 0 0 / 10%)` |
| `--input` | `oklch(0.93 0 0)` | `oklch(1 0 0 / 17%)` |

### 品牌层

| Token | 亮色 | 暗色 |
|-------|------|------|
| `--primary` | `oklch(0.692 0.141 243.716)` ≈ #4f7eff | `oklch(0.54 0.142 248.516)` ≈ #3b6fd6 |
| `--primary-foreground` | `oklch(1 0 0)` | `oklch(1 0 0)` |
| `--secondary` | `oklch(0.95 0 0)` | `oklch(0.335 0 0)` |
| `--secondary-foreground` | `oklch(0.42 0.16 250)` | `oklch(0.9 0.05 250)` |
| `--accent` | `color-mix(in oklch, var(--primary) 12%, var(--background))` | `... 20% ...` |
| `--accent-foreground` | `oklch(0.145 0 0)` | `oklch(0.985 0 0)` |
| `--ring` | `oklch(0.708 0.16 249.003)` | `oklch(0.554 0.148 250.726)` |

### 语义层

| Token | 亮色 | 暗色 | 大致 HEX |
|-------|------|------|---------|
| `--destructive` | `oklch(0.577 0.245 27.325)` | `oklch(0.704 0.191 22.216)` | #dc2626 系红 |
| `--destructive-foreground` | `oklch(0.985 0 0)` | `oklch(0.985 0 0)` | |
| `--success` | `oklch(0.596 0.145 163.225)` | `oklch(0.696 0.17 162.48)` | #16a34a 系绿 |
| `--success-foreground` | `oklch(0.985 0 0)` | `oklch(0.145 0 0)` | |
| `--warning` | `oklch(0.681 0.162 75.834)` | `oklch(0.769 0.188 70.08)` | #eab308 系黄 |
| `--warning-foreground` | `oklch(0.145 0 0)` | `oklch(0.145 0 0)` | |
| `--info` | `oklch(0.588 0.158 241.966)` | `oklch(0.613 0.14 239.919)` | #3b82f6 系蓝 |
| `--info-foreground` | `oklch(0.985 0 0)` | `oklch(0.145 0 0)` | |
| `--neutral` | `oklch(0.708 0 0)` | `oklch(0.76 0 0)` | 中性灰 |
| `--neutral-foreground` | `oklch(0.145 0 0)` | `oklch(0.155 0 0)` | |

### 图表色板（chart-1 ~ chart-5）

亮色 / 暗色分别独立定义，暗色略调亮以保证对比度。色相分布：

| | 亮色色相 | 暗色色相 | 含义 |
|---|---|---|---|
| `--chart-1` | 250（蓝） | 250 | 主序列 |
| `--chart-2` | 200（青） | 200 | 次序列 |
| `--chart-3` | 280（紫） | 285 | 第三序列 |
| `--chart-4` | 325（品红） | 325 | 第四序列 |
| `--chart-5` | 155（绿） | 155 | 第五序列 |

`--overview-accent-1/2/3` 默认复用 chart-1/2/3。

### 侧边栏子主题

侧边栏用独立 token，可与主色不同（方便做深色侧边栏）：

| Token | 亮色 | 暗色 |
|-------|------|------|
| `--sidebar` | `color-mix(in oklch, var(--foreground) 0.5%, var(--background))` | `oklch(0.225 0 0)` |
| `--sidebar-foreground` | `oklch(0.145 0 0)` | `oklch(0.95 0 0)` |
| `--sidebar-primary` | `oklch(0.64 0.197 253.892)` | `oklch(0.588 0.123 246.488)` |
| `--sidebar-accent` | `color-mix(in oklch, var(--primary) 12%, var(--background))` | `color-mix(in oklch, var(--primary) 20%, var(--background))` |
| `--sidebar-border` | `oklch(0.93 0 0)` | `oklch(1 0 0 / 11%)` |
| `--sidebar-ring` | `oklch(0.58 0.2 250)` | `oklch(0.7 0.17 250)` |

### 骨架屏与表格

| Token | 亮色 | 暗色 |
|-------|------|------|
| `--skeleton-base` | `oklch(0.97 0 0)` | `oklch(0.335 0 0)` |
| `--skeleton-highlight` | `oklch(0.985 0 0)` | `oklch(0.44 0 0)` |
| `--table-row` | `var(--background)` | `var(--background)` |
| `--table-header` | `color-mix(... foreground 1.5% ...)` | `color-mix(... foreground 6% ...)` |
| `--table-header-hover` | `color-mix(... foreground 3% ...)` | `color-mix(... foreground 9% ...)` |
| `--table-disabled` | `color-mix(... foreground 3% ...)` | `color-mix(... foreground 7% ...)` |
| `--table-disabled-hover` | `color-mix(... foreground 4.5% ...)` | `color-mix(... foreground 9.5% ...)` |
| `--table-disabled-border` | `color-mix(... foreground 16% ...)` | `color-mix(... foreground 24% ...)` |

## 2.4 字体 Token（theme.css:22-41）

```css
--font-sans: 'Public Sans', sans-serif;

--font-serif: 'Lora Variable', 'Lora', 'Source Serif Pro', 'Source Serif 4',
  'Noto Serif SC', 'Noto Serif TC', 'Noto Serif JP', 'Noto Serif KR',
  'Source Han Serif SC', 'Source Han Serif TC', 'Source Han Serif',
  'Songti SC', 'STSong', 'STSongti-SC-Regular', 'PingFang SC', 'SimSun',
  'NSimSun', '宋体', 'FangSong', '仿宋', 'KaiTi', '楷体', Georgia,
  'Times New Roman', Cambria, 'Liberation Serif', serif;
```

- `--font-inter` / `--font-manrope`：当前未启用为 body 默认，预留
- body 实际使用的是 `--font-body`，由字体轴切换（默认 = `--font-sans`）
- CJK serif fallback 链特别处理 Windows 系统字体不确定问题（注释见 theme.css:23-28）

加载方式（index.css:22-28）：CSS `@import` + npm 安装本地字体，**无 Google Fonts CDN**：

```css
@import '@fontsource-variable/public-sans';
@import '@fontsource-variable/lora';
```

## 2.5 圆角 Token（theme.css:87-93）

全部基于 `--radius` 倍数派生：

```css
--radius-sm:  calc(var(--radius) * 0.6);
--radius-md:  calc(var(--radius) * 0.8);
--radius-lg:  var(--radius);            /* = --radius 本身 */
--radius-xl:  calc(var(--radius) * 1.4);
--radius-2xl: calc(var(--radius) * 1.8);
--radius-3xl: calc(var(--radius) * 2.2);
--radius-4xl: calc(var(--radius) * 2.6);
```

`--radius` 默认 `1rem`（16px）。预设会带自己的默认值（见 03-theming.md）。

## 2.6 间距 Token

- **无自定义 spacing scale**，依赖 Tailwind 默认（`p-1` = 0.25rem ...）
- `--spacing` 仅在密度轴非 default 时被覆盖：
  - `sm`（紧凑）：`--spacing: 0.225rem`
  - `lg`：`--spacing: 0.28rem`
  - `xl`（最宽松）：`--spacing: 0.3rem`

## 2.7 字号 Token（密度轴覆盖，theme-presets.css）

密度轴（`data-theme-scale`）会覆盖完整字号表：

| 档 | `--text-base` | `--spacing` |
|----|---------------|-------------|
| `default` | 1rem（Tailwind 默认） | Tailwind 默认 |
| `sm` | 0.88rem | 0.225rem |
| `lg` | 1.075rem | 0.28rem |
| `xl` | 1.125rem | 0.3rem |

同时覆盖 `--text-xs/sm/base/lg/xl/2xl/3xl` 全套（见 theme-presets.css:700-729）。

## 2.8 阴影 Token

**项目没有定义 `--shadow-*` CSS 变量。** 阴影以硬编码 `box-shadow` 形式散落在 index.css 中（Vercel 风格的 `0 4px 12px rgb(0 0 0 / 0.06)` 等）。**升级建议**：如果未来要全面主题化阴影，应新增 `--shadow-sm/md/lg/xl` token，并迁移这些硬编码值。

## 2.9 容器宽度与布局尺寸

```css
@utility container { margin-inline: auto; padding-inline: 2rem; }
@utility max-w-container { max-width: 1280px; }    /* = xl 断点 */
@utility max-w-container-lg { max-width: 1536px; } /* = 2xl 断点 */
```

其他布局常量（theme.css:97-105）：

```css
--radius: 1rem;
--app-header-height: 3rem;                  /* 48px，Header 固定高度 */
--app-rev: '2k6e8r7p';                      /* 构建版本回退（JS 未启动时显示） */
--font-body: var(--font-sans);              /* 字体轴切换的目标 */
```

## 2.10 响应式断点

**完全使用 Tailwind v4 默认断点，无任何自定义。**

| 前缀 | 触发像素 | 含义 |
|------|----------|------|
| (base) | 0px | 移动端默认 |
| `sm` | **640px** | 大手机 / 小平板竖屏 |
| `md` | **768px** | 平板 |
| `lg` | **1024px** | 笔记本（TopNav 桌面切换点） |
| `xl` | **1280px** | 桌面（`max-w-container` 上限） |
| `2xl` | **1536px** | 大屏（`max-w-container-lg` 上限） |

### 关键 JS 阈值（与 CSS 断点对齐）

- `hooks/use-mobile.ts:21`：`MOBILE_BREAKPOINT = 768`，判定 `window.innerWidth < 768`（即 `< md`）
- Sidebar 在 < 768px 切换为 Sheet 抽屉模式
- `index.css:79`：`@media (max-width: 767px)` 强制 input/select/textarea `font-size:16px`，防 iOS 聚焦缩放
- `index.css:493`：`@media (min-width: 641px)` 才启用 card hover translateY，避免移动端误触

### 容器查询

Tailwind v4 的 `@container` 已被采用：

- `SidebarInset` 标 `@container/content`（authenticated-layout.tsx:47）
- `Main` 用 `@7xl/content:`（main.tsx:31）

容器查询目前只用在布局壳层，业务组件主要用视口断点。

## 2.11 暗色变体声明

```css
@custom-variant dark (&:is(.dark *));
```

`.dark` class 加在 `<html>` 上，`dark:` 工具类即可生效。

## 2.12 全局基础样式（@layer base，index.css:50-86）

```css
* {
  @apply border-border outline-ring/50;
  scrollbar-width: thin;
  scrollbar-color: var(--border) transparent;
}
html { @apply overflow-x-hidden font-sans; }
body {
  @apply bg-background text-foreground has-[div[data-variant='inset']]:bg-sidebar min-h-svh w-full;
  font-family: var(--font-body);
}

/* Sticky headers 不受 body scroll lock 影响 */
body[data-scroll-locked] { overflow: unset !important; }

/* 按钮默认 cursor pointer */
button:not(:disabled), [role='button']:not(:disabled) { cursor: pointer; }

/* 移动端防 input 聚焦缩放 */
@media screen and (max-width: 767px) {
  input, select, textarea { font-size: 16px !important; }
}
```

## 2.13 Shiki 代码高亮双主题

```css
.shiki span { color: var(--shiki-light) !important; ... }
.dark .shiki span { color: var(--shiki-dark) !important; ... }
```

代码块背景保持 `bg-background`，token 颜色随亮暗切换。
