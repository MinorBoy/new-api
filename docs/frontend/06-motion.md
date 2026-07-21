# 06 · 动效系统

## 6.1 主库与原则

**主库：`motion`（即 Framer Motion v12，入口 `motion/react`）**。`package.json:54`。未使用 `@animate-ui`。

### 设计原则

1. **集中定义，分散使用**：所有 variants 在 `lib/motion.ts` 一次定义，全项目复用
2. **尊重用户偏好**：所有 motion 组件统一尊重 `useReducedMotion()`，开启系统「减少动效」时退化为静态
3. **性能优先**：路由切换不重挂载（`AnimatedOutlet` 用 `routeId` 作 key）
4. **CSS 与 motion 分工**：微交互（hover/active）用 CSS，进场/切换用 motion

## 6.2 全局动效 Token

来源：`src/lib/motion.ts`

### 缓动与时长

```ts
const EASE_OUT_CUBIC = [0.33, 1, 0.68, 1] as const  // motion.ts:21

const DURATION = {
  instant: 0,
  fast: 0.15,      // 150ms
  normal: 0.25,    // 250ms
  slow: 0.35,      // 350ms
} as const
```

### Transition 预设（motion.ts:30-36）

```ts
export const MOTION_TRANSITION = {
  default: { duration: 0.25,  ease: EASE_OUT_CUBIC },
  fast:    { duration: 0.15,  ease: EASE_OUT_CUBIC },
  slow:    { duration: 0.35,  ease: EASE_OUT_CUBIC },
  spring:  { type: 'spring', damping: 20, stiffness: 300 },
  none:    { duration: 0 },
}
```

## 6.3 Variants 清单（motion.ts:38-122）

### 单元素 variants（MOTION_VARIANTS）

| 名称 | initial | animate | exit | 用途 |
|------|---------|---------|------|------|
| `pageEnter` | opacity:0, y:8, blur(4px) | opacity:1, y:0, blur(0) | opacity:0, y:-4, blur(2px) | 路由切换（带 4px 模糊入场） |
| `fadeIn` | opacity:0 | opacity:1 | opacity:0 | 简单淡入 |
| `scaleIn` | opacity:0, scale:0.96 | opacity:1, scale:1 | opacity:0, scale:0.96 | 缩放淡入 |
| `slideUp` | opacity:0, y:16 | opacity:1, y:0 | opacity:0, y:16 | 上滑入场 |
| `slideDown` | opacity:0, y:-16 | opacity:1, y:0 | opacity:0, y:-16 | 下滑入场 |
| `tableRow` | opacity:0, y:4 | opacity:1, y:0 | — | 表格行 |
| `cardItem` | opacity:0, y:12, scale:0.98 | opacity:1, y:0, scale:1 | — | 卡片 |
| `sidebarSlide` | opacity:0, x:-8 | opacity:1, x:0 | opacity:0, x:-8 | sidebar 视图切换 |

### 错峰容器 variants

| 名称 | children stagger | delayChildren | 用途 |
|------|-----------------|---------------|------|
| `STAGGER_VARIANTS` | 0.04s | — | 通用错峰 |
| `TABLE_STAGGER_VARIANTS` | 0.03s | — | 表格行 |
| `CARD_STAGGER_VARIANTS` | 0.05s | — | 卡片网格 |
| `SIDEBAR_STAGGER_VARIANTS` | 0.03s | 0.05s | sidebar 导航项 |

子元素 variants 配套：

- `STAGGER_ITEM_VARIANTS`：opacity:0 y:8 → opacity:1 y:0（用 default transition）
- `TABLE_ROW_VARIANTS`：opacity:0 y:4 → opacity:1 y:0（用 fast transition）
- `CARD_ITEM_VARIANTS`：opacity:0 y:12 scale:0.98 → opacity:1 y:0 scale:1（用 default transition）
- `SIDEBAR_ITEM_VARIANTS`：opacity:0 x:-8 → opacity:1 x:0（用 fast transition）

## 6.4 封装组件

来源：`src/components/page-transition.tsx`

| 组件 | 行 | 职责 |
|------|----|----|
| `PageTransition` | 39-56 | 路由切换淡入（包装 pageEnter variant） |
| `AnimatedOutlet` | 64-87 | **关键**：用 `routeId`（不是 pathname）作 key，使 `/dashboard/$section` 之间切换不重挂载，保留组件状态 |
| `StaggerContainer` | — | 错峰容器（STAGGER_VARIANTS） |
| `StaggerItem` | — | 错峰子项（STAGGER_ITEM_VARIANTS） |
| `TableStaggerContainer` / `TableStaggerRow` | — | 表格专用（`motion.tbody`/`motion.tr`） |
| `CardStaggerContainer` / `CardStaggerItem` | — | 卡片网格专用 |
| `FadeIn` | — | 简单淡入 |

### AnimatedOutlet 的关键设计

```tsx
// 简化示意
<AnimatePresence mode='wait'>
  <motion.div key={routeId} variants={MOTION_VARIANTS.pageEnter} ...>
    <Outlet />
  </motion.div>
</AnimatePresence>
```

为什么用 `routeId` 而非 `pathname`？`/dashboard/channels` 和 `/dashboard/keys` 共享同一个 `routeId`（`/dashboard/$section`），切换 section 时**不会触发路由级动画**，但内部组件可以有自己的过渡。这避免了 tab 切换时的不必要重挂载。

## 6.5 CSS 动画

来源：`src/styles/index.css`

### 骨架屏 shimmer（index.css:88-131）

```css
.skeleton-shimmer {
  background: linear-gradient(90deg,
    var(--skeleton-base) 0%,
    var(--skeleton-base) 33%,
    var(--skeleton-highlight) 50%,
    var(--skeleton-base) 67%,
    var(--skeleton-base) 100%);
  background-size: 300% 100%;
  animation: skeleton-shimmer 1.8s ease-in-out infinite;
}

@keyframes skeleton-shimmer {
  0%   { background-position: 100% 50%; }
  100% { background-position: -100% 50%; }
}
```

- **Vercel Geist 风格**：渐变带宽 33%/17%/33%，中间高光扫过
- **覆盖 auto-skeleton-react**：`.auto-skeleton-fade [class^='skeleton-']` 把库的内联样式统一为主题 token 驱动（index.css:111-123）
- **reduced-motion 降级**：动画关闭，背景固定为 `--skeleton-base`

### Landing 入场动画（index.css:408-483）

```css
.landing-animate-fade-up    { /* Y(20) → 0 */ }
.landing-animate-fade-in    { /* 纯淡入 */ }
.landing-animate-fade-left  { /* X(-20) → 0 */ }
.landing-animate-fade-right { /* X(20) → 0 */ }
.landing-scale-in           { /* scale(0.95) → 1 */ }
```

统一 `cubic-bezier(0.16, 1, 0.3, 1)` 0.6s（emeric easing，快速减速）。配合 `.animation-delay-100/300/700/1000`（index.css:317-339）做错峰。

### 通用入场

- `.animate-appear`：opacity + blur(8px) → 0（index.css:317-339）
- `.animate-appear-zoom`：opacity + scale(0.96) → 1

### 滚动列表（index.css:368-392）

```css
.animate-scroll-up   { animation: scroll-up 20s linear infinite; }
.animate-scroll-down { animation: scroll-down 20s linear infinite; }
/* hover 暂停 */
```

用于 home 的滚动图标墙。

### 表格行错峰入场（index.css:514-547）

```css
[data-slot='table'] tbody tr {
  animation: tableRowEnter 0.25s ease-out backwards;
}
[data-slot='table'] tbody tr:nth-child(1) { animation-delay: 0ms; }
[data-slot='table'] tbody tr:nth-child(2) { animation-delay: 25ms; }
/* ... 最多 10 行，每行 +25ms */
```

### Hero 终端 demo（index.css:572-606）

- `.terminal-demo-blink`：光标闪烁
- `.terminal-demo-spin`：spinner 旋转
- `.terminal-demo-pulse`：脉冲

### 微交互（index.css:486-547）

```css
/* Card hover：上浮 1px + 阴影 */
[data-slot='card']:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgb(0 0 0 / 0.06);
}

/* Button active：缩小到 98% */
button:active { transform: scale(0.98); }

/* Table row hover：背景色过渡 */
[data-slot='table'] tbody tr {
  transition: background-color 120ms ease;
}
```

**移动端保护**（index.css:493）：`@media (min-width: 641px)` 才启用 card hover translateY，避免移动端误触触发的视觉跳动。

### Glass Morphism 与 Fade Mask（虽是装饰，但与动效配合）

- `.glass-1` ~ `.glass-5`（index.css:243-261）：5 级玻璃态
- `.fade-x/-y/-top/-bottom/-left/-right/...`（index.css:264-314）：9 方向 mask 渐隐

## 6.6 加载与骨架屏

| 组件 | 文件 | 用途 |
|------|------|------|
| `NavigationProgress` | `components/navigation-progress.tsx` | 路由 pending 时顶部 2px 进度条（`react-top-loading-bar`），色 `var(--muted-foreground)` |
| `ContentSkeleton` / `QuerySkeleton` | `components/auto-skeleton.tsx` | 包装 `auto-skeleton-react`，自动取主题 `--skeleton-base/highlight` + `useThemeRadiusPx()` 圆角 |
| `Skeleton` | `components/ui/skeleton.tsx` | shadcn 标准 Skeleton |
| `TableSkeleton` | `components/data-table/core/table-skeleton.tsx` | 表格专用 |
| `LoadingSkeleton` | `features/pricing/components/loading-skeleton.tsx` | pricing 专用 |

## 6.7 reduced-motion 处理

所有动画在 `@media (prefers-reduced-motion: reduce)` 下降级为静态：

| 位置 | 降级内容 |
|------|---------|
| index.css:125-131 | 骨架屏 shimmer 关闭 |
| index.css:395-405 | 滚动列表动画关闭 |
| index.css:473-483 | landing 入场动画关闭 |
| index.css:608-614 | 终端 demo 动画关闭 |
| motion 组件 | `useReducedMotion()` hook 自动退化为静态 div |

**升级约定**：新增任何动画都必须同步写 reduced-motion 降级。这是 a11y 硬要求（AGENTS.md §3.x）。

## 6.8 动效升级建议

### 新增动效

1. **优先复用** `lib/motion.ts` 中的现有 variants，不要在业务组件里写 inline transition
2. **新增 variant** 时：
   - 在 `lib/motion.ts` 加导出
   - 用 `MOTION_TRANSITION.*` 而非硬编码时长
   - 如有可能，封装到 `components/page-transition.tsx`
3. **CSS 动画**新增时：
   - 放 `index.css`（不要分散到组件 CSS）
   - 用主题 token（`var(--skeleton-*)` 等），不要硬编码颜色
   - 必须写 `@media (prefers-reduced-motion: reduce)` 降级

### 性能红线

- 避免在大量元素上同时触发 `box-shadow` 或 `filter: blur` 动画（GPU 成本高）
- 表格行错峰上限 10 行（index.css 已限制），更多行直接静态
- 路由切换动画用 `AnimatePresence mode='wait'`，避免新旧页面同时渲染
- motion 组件优先用 `transform` / `opacity`（合成层属性），不要动画 `width`/`height`/`top`/`left`
