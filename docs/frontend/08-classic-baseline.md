# 08 · classic 前端基线与迁移资产

> 本章梳理 `web/classic`（已弃用）的设计基线，重点是：迁移到 `web/default` 时**应该保留**的设计资产，以及**不需要继承**的负担。
>
> classic 是「最小维护」状态，所有新功能都在 default 实现。但 classic 经过长期迭代，沉淀了一些值得复用的设计语言。

## 8.1 classic 技术栈速览

| 维度 | classic | default |
|------|---------|---------|
| 组件库 | Semi Design（`@douyinfe/semi-ui` ^2.69） | shadcn "base-nova"（`@base-ui/react`） |
| 语言 | JSX (JS) | TSX (TypeScript) |
| Tailwind | v3（`tailwind.config.js`） | v4（`@theme inline`，无 JS 配置） |
| 路由 | react-router-dom v6 | TanStack Router |
| 数据 | axios + 自定义 context | TanStack Query + zustand |
| 主题切换 | 自研 ThemeProvider（三态） | ThemeProvider（class 策略） |
| 颜色空间 | hex/rgba（消费 Semi CSS 变量） | OKLCH |
| 色板 | 单一 Semi 默认 + 5 个 SBG 变体 | 10 套预设 + 4 个独立可调轴 |
| 圆角 | 全局硬编码 10px | `--radius: 1rem` + 派生 |
| 字体 | Lato + Microsoft YaHei | Public Sans + Lora Variable + CJK fallback |
| 图标 | lucide-react + semi-icons + semi-illustrations | hugeicons + lucide-react + lobehub |
| Toast | react-toastify + Semi Toast | sonner |
| Drawer | Semi SideSheet | vaul |
| Markdown | react-markdown + rehype | marked + shiki + stream-markdown-parser |
| 图表 | VChart 1.8 + vchart-semi-theme | VChart 2.1 + recharts |
| 文件位置 | 全局单文件 `src/index.css` 1095 行 | 分层 `styles/{index,theme,theme-presets}.css` |

## 8.2 classic 设计基线要点

### 主题实现

- 文件：`src/context/Theme/index.jsx`
- 三态：`light` / `dark` / `auto`，存 `localStorage['theme-mode']`
- **DOM 双驱动**：暗色时同时设 `body[theme-mode="dark"]`（驱动 Semi 变量）和 `<html class="dark">`（驱动 Tailwind dark: variant）
- 三个 context：`ThemeContext`（用户偏好）、`ActualThemeContext`（解析后实际值）、`SetThemeContext`（setter）
- **无独立 Semi 主题定制文件**（无 SCSS、无 DSM 产物），主题完全靠消费 Semi CSS 变量 + 选择性覆盖

### 关键 token（`src/index.css`）

```css
--sidebar-width: 180px;
--sidebar-width-collapsed: 60px;
--sidebar-current-width: var(--sidebar-width);  /* 折叠时由 body.sidebar-collapsed 覆盖 */
```

颜色：完全消费 Semi 变量（`--semi-color-text-0/1/2/3`、`--semi-color-bg-0/1/2/3`、`--semi-color-fill-0/1/2`、`--semi-color-border`、`--semi-color-primary`）。

圆角：**全局强制 10px**，覆盖在所有 Semi 组件（`.semi-button` / `.semi-input-wrapper` / `.semi-select` / ...）。`tailwind.config.js:136-146` 把 Semi 的 `--semi-border-radius-*` 暴露为 Tailwind 工具类。

### 布局（`components/layout/PageLayout.jsx`）

- Semi `Layout` / `Header` / `Sider` / `Content` / `Footer`
- 固定顶部 Header（`position:fixed, top:0, zIndex:100`），高 64px
- 固定左侧 Sider，宽 = `var(--sidebar-current-width)`
- Content 用 `marginLeft: var(--sidebar-current-width)` 让位
- **`cardProPages`**（9 个表格路由：channel/log/redemption/user/token/midjourney/task/models/pricing）：隐藏 footer、走 fixed layout
- isFixedLayout = 路径以 `/console` 开头 || 路径 === `/pricing'

### 响应式

- **单一断点 768px**：`hooks/common/useIsMobile.js:20` 定义 `MOBILE_BREAKPOINT = 768`
- CSS 所有 media query 都是 `@media (max-width: 767px)`
- PageLayout：< 768 时 sider 改 drawer，`marginLeft:0`

## 8.3 应保留并迁移到 default 的设计资产

### 8.3.1 Sidebar 信息架构

来源：`components/layout/SiderBar.jsx`

- 4 段分组：**聊天 / 控制台 / 个人中心 / 管理员**
- 每段配 lucide-react 图标（`getLucideIcon` helper）
- `useSidebar` hook 控制模块可见性（按角色/配置）
- localStorage 持久化折叠状态
- 分组标签样式：`uppercase, 12px, letterspacing 0.5px`

**价值**：业务结构清晰，default 重建时应照搬 IA（信息架构）。default 的 `app-sidebar.tsx` 已支持 drill-in 视图，但顶层分组可参考 classic 的 4 段划分。

### 8.3.2 `cardProPages` 9 路由分类

来源：`PageLayout.jsx:54-64`

明确列出哪些是「表格型全屏卡页、无 footer」：

```
channel, log, redemption, user, token, midjourney, task, models, pricing
```

**价值**：这是 default 规划 layout variant 的现成清单 —— 这些页面应该用 `SectionPageLayout` + `fixedContent` 模式（无 footer，内容区独立滚动）。

### 8.3.3 SelectableButtonGroup 5 色变体

来源：`src/index.css:447-516`

5 套变体（`violet` / `teal` / `amber` / `rose` / `green`），每套：

- 独立的亮/暗 `--semi-color-primary`
- 三档 light 状态（default / hover / active）

**价值**：这是 classic 最精致的设计语言资产。default 中可通过新增主题预设或 CVA variant 复刻。

**迁移建议**：转换为 default 的预设或 button variant，例如：

```ts
// 在 buttonVariants 的 variant 中新增
tone: {
  violet: 'bg-violet-500 text-white hover:bg-violet-600',
  teal: 'bg-teal-500 text-white hover:bg-teal-600',
  // ...
}
```

### 8.3.4 马卡龙模糊球装饰

来源：`src/index.css:854-906`，`.with-pastel-balls` 类

- 4 色 radial-gradient（粉 / 薰衣草 / 薄荷 / 桃）
- 暗色用 `mix-blend-mode: screen`（关键技巧：避免暗色下高亮突兀）
- Banner 装饰球 `.blur-ball-indigo/teal`（`index.css:817-852`）

**价值**：用于 Home / Hero / 卡片的柔和氛围光晕。default 的 `Underground` / `Anthropic` 预设可借鉴。`mix-blend-mode: screen` 的暗模式适配技巧是通用经验。

### 8.3.5 三态主题切换 UX

来源：`components/layout/headerbar/ThemeToggle.jsx`

- Semi `Dropdown` + Sun / Moon / Monitor（lucide-react）三选一
- 显示「当前跟随系统」提示行（当 theme === 'auto'）

**价值**：default 的 `theme-switch.tsx` 已具备能力（system 模式），但 classic 的交互细节（明确提示当前是跟随系统）值得作为 UX 参考。

### 8.3.6 HeaderBar 半透明毛玻璃

来源：`components/layout/headerbar/index.jsx:68`

```tsx
<header className='bg-white/75 dark:bg-zinc-900/75 backdrop-blur-lg'>
```

**价值**：default 已有类似实现（PublicHeader 的 scroll-driven 玻璃态），但 classic 在工作台 header 也用毛玻璃。可考虑在 default 的 AppHeader 上复用。

### 8.3.7 Pricing 双栏布局

来源：`src/index.css:965-1015`

```css
.pricing-sidebar {
  width: clamp(280px, 24vw, 520px) !important;
}
```

- 左侧 sticky search header + 右侧 model grid
- 独立的 mobile 容器

**价值**：直接迁移到 default 的 Pricing 路由。`clamp(280px, 24vw, 520px)` 是响应式宽度的好范式。

### 8.3.8 CardPro 三种 layout type

classic 的 `CardPro` 抽象支持 3 种 layout：

- 操作型（toolbar + table）
- 查询型（filter + table）
- 复杂型（stats + description + tabs + actions + search + pagination）

**价值**：抽象本身可重做，但「表格页有 6 个可组合插槽」的产品抽象值得在 default 复刻。default 的 `SectionPageLayout` 已有 4 个 slot（Breadcrumb / Title / Actions / Content），可扩展。

### 8.3.9 渠道亲和标签

来源：`src/index.css:1025-1050`，`.semi-tag.channel-affinity-tag`

- 青色描边 + hover ring

**价值**：业务语义化样式，default 里应该有对应 token 映射（如 `tag-variant='affinity'`）。

### 8.3.10 滚动条多套规则

classic 有三套滚动条规则：

- 通用隐藏（`.scrollbar-hide`）
- 表格 / sidesheet 6px 细滚动条
- 聊天 / 侧栏完全隐藏

**价值**：default 已有 `scrollbar-width: thin`（全局）和 `no-scrollbar` 工具类（index.css:146-154），但可参考 classic 的分场景策略。

## 8.4 不需要继承的负担

### Semi 组件级 `!important` 覆盖

classic 在 `index.css` 大量用 `!important` 修补 Semi 默认值：

```css
.semi-navigation-item { margin-bottom:4px!important; padding:4px 12px!important; }
.semi-card-header, .semi-card-body { padding:10px!important; }
.semi-tabs-content { padding:0!important; height:calc(100% - 40px)!important; }
```

**不要搬到 default**。default 用 shadcn，应直接调 token 或改组件源码（shadcn 组件就在 `src/components/ui/`，可直接编辑）。

### 全局单文件 CSS

classic 的 `src/index.css` 有 1095 行，混合了：

- Semi 变量消费
- 组件级覆盖
- 业务样式
- 装饰元素

default 已分层为 `index.css` + `theme.css` + `theme-presets.css`，更清晰。不要倒退回单文件。

### react-toastify / Semi Toast

default 已统一用 sonner，不要混用。

### vchart-semi-theme

default 用 VChart 2.1 + 自研 `useChartTheme` hook（基于 resolvedTheme 切换），不要再用 semi-theme 适配。

## 8.5 迁移工作流

项目已提供 `classic-to-default-sync` skill：

> Inspect a given commit's web/classic changes and sync all features/fixes to web/default.

工作流：

1. 指定 classic 的某次提交
2. skill 审计 default 是否已有对应功能
3. 移植缺失的功能/修复到 default

**迁移时的设计决策**：

- 不要 1:1 复制 classic 的样式（Semi → shadcn 范式不同）
- 保留业务语义（IA、字段、交互流程）
- 用 default 的设计系统重新表达（OKLCH token、shadcn 组件、motion variants）
- 参考本章 8.3 的「应保留资产」清单

## 8.6 classic 弃用时间线

- classic 仍可通过系统配置切换启用（向后兼容）
- 顶部有 `ClassicFrontendDeprecationBanner` 提示弃用
- 长期目标：classic 完全移除，所有用户迁移到 default
- 在 classic 完全移除前，本章的「应保留资产」清单应作为 default 升级的参考输入
