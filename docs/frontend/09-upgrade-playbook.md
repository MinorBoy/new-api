# 09 · 升级迭代手册

> 本章是后续前端页面设计升级迭代的操作手册，覆盖常见升级场景的规范、检查清单和反模式。

## 9.1 新增主题预设

### 步骤

1. **注册元数据**（`lib/theme-customization.ts` 的 `THEME_PRESETS`）：
   ```ts
   {
     value: 'my-brand',
     name: 'My Brand',
     swatches: ['oklch(主色亮)', 'oklch(辅色亮)'],
   }
   ```
2. **写 CSS**（`theme-presets.css` 追加）：
   ```css
   [data-theme-preset='my-brand'] {
     --primary: oklch(0.6 0.18 250);
     --primary-foreground: oklch(1 0 0);
     --secondary: oklch(0.8 0.05 250);
     --secondary-foreground: oklch(0.2 0 0);
     --ring: oklch(0.6 0.18 250);
     --chart-1: oklch(0.6 0.18 250);
     --chart-2: oklch(0.7 0.12 200);
     --chart-3: oklch(0.65 0.15 280);
     --chart-4: oklch(0.68 0.18 325);
     --chart-5: oklch(0.7 0.15 155);
     --sidebar-primary: oklch(0.6 0.18 250);
     --sidebar-accent: oklch(0.95 0.02 250);
     --sidebar-accent-foreground: oklch(0.3 0.1 250);
     --sidebar-ring: oklch(0.6 0.18 250);
     --radius: 0.75rem;  /* 自带默认圆角 */
   }
   .dark [data-theme-preset='my-brand'] {
     /* 暗色一套，色相可微调，亮度通常调高 0.05~0.1 */
   }
   ```
3. **决定是否染色表面**：
   - **染色**（推荐，主色调统一感）：什么都不做，自动走 Semantic Surface Bridge（`theme-presets.css:415-472`）
   - **不染色**（保留中性表面）：把 `'my-brand'` 加入 opt-out 选择器组（与 `default` / `anthropic` / `simple-large` 并列）
4. **决定默认字体**（可选）：如要 serif，在 `PRESET_DEFAULT_FONT` 加 `my-brand: 'serif'`
5. **类型自动收窄**：`ThemePreset` 由 `THEME_PRESETS` 推导，无需手改类型

### 检查清单

- [ ] 亮/暗两套色值都定义
- [ ] 至少包含：`--primary(-foreground)` / `--ring` / `--chart-1~5` / `--sidebar-primary/-accent/-ring`
- [ ] 自带 `--radius`
- [ ] 色值用 OKLCH，不用 HEX/HSL
- [ ] 暗色亮度通常调高 0.05~0.1（保证对比度）
- [ ] swatches 用于切换 UI 预览，能代表预设氛围
- [ ] 在亮/暗模式 + 各字体/圆角/密度组合下视觉验证

## 9.2 新增 shadcn 组件

```bash
cd web/default
bunx shadcn add <component-name>
```

会自动：

- 从 `base-nova` registry 拉组件到 `src/components/ui/<name>.tsx`
- 使用 hugeicons 作为图标
- 用项目 CSS 变量（无需改 theme.css）

### 检查清单

- [ ] 组件落在 `src/components/ui/`（业务无关基元）
- [ ] 用项目 `cn()` 合并类名
- [ ] 用项目主题 token（`bg-primary` / `text-foreground`），不要硬编码颜色
- [ ] 图标用 hugeicons（保持 shadcn 注册一致性）
- [ ] 暴露 `data-slot` 属性（如需被全局 CSS 选中）
- [ ] 通过 a11y 审查（键盘可达、ARIA）

## 9.3 新增工作台页面

### 决策树

```
是否需要登录？
├── 是 → 是否 SUPER_ADMIN 且是系统设置？
│       ├── 是 → routes/_authenticated/system-settings/$section（drill-in sidebar）
│       └── 否 → routes/_authenticated/<feature>（AuthenticatedLayout + SectionPageLayout）
└── 否 → routes/<feature>/index（PublicLayout）
```

### 步骤（以「积分商城」为例）

1. **创建 feature 模块**：
   ```
   features/points-shop/
   ├── components/
   ├── hooks/
   ├── lib/
   ├── api.ts          # TanStack Query 封装
   ├── types.ts
   └── constants.ts
   ```

2. **创建路由文件**（薄壳）：
   ```tsx
   // routes/_authenticated/points-shop/index.tsx
   import { createFileRoute } from '@tanstack/react-router'
   import { PointsShopPage } from '@/features/points-shop/components/points-shop-page'

   export const Route = createFileRoute('/_authenticated/points-shop/')({
     component: PointsShopPage,
   })
   ```

3. **页面外层用 SectionPageLayout**：
   ```tsx
   <SectionPageLayout>
     <SectionPageLayout.Title>积分商城</SectionPageLayout.Title>
     <SectionPageLayout.Actions>
       <Button>兑换记录</Button>
     </SectionPageLayout.Actions>
     <SectionPageLayout.Content>
       {/* 商品网格 */}
       <div className='grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 lg:grid-cols-3'>
         {items.map(...)}
       </div>
     </SectionPageLayout.Content>
   </SectionPageLayout>
   ```

4. **加入 sidebar 导航**：参考 classic 的 4 段分组（聊天/控制台/个人中心/管理员），决定归属哪一段。

### 检查清单

- [ ] 路由放在正确的守卫层下
- [ ] feature 模块结构完整（components/hooks/api.ts/types.ts/constants.ts）
- [ ] 外层用 SectionPageLayout（工作台）或 PublicLayout（公开页）
- [ ] 响应式栅格从 `grid-cols-1` 起步
- [ ] i18n：所有用户可见文案走 `t()`，常量值即 i18n 键
- [ ] 表格型页面用 `components/data-table/` 系统
- [ ] 表单用 react-hook-form + zod
- [ ] 错误处理用 `handle-server-error.ts`
- [ ] 加载态用 `ContentSkeleton` / `TableSkeleton`

## 9.4 新增动效

### 决策：CSS 还是 motion？

| 场景 | 选择 |
|------|------|
| 微交互（hover / active / focus） | CSS |
| 进场 / 退场 / 切换 | motion |
| 持续循环（骨架屏、loading） | CSS keyframes |
| 错峰列表 | motion variants（封装组件） |

### 步骤

1. **优先复用** `lib/motion.ts` 中的现有 variants
2. **新增 variant** 时：
   ```ts
   // lib/motion.ts
   export const MOTION_VARIANTS = {
     // ...existing
     myEffect: {
       initial: { opacity: 0, y: 12 },
       animate: { opacity: 1, y: 0 },
       exit: { opacity: 0, y: -12 },
     },
   } as const
   ```
3. **CSS 动画**新增时：
   - 放 `index.css`（不要分散到组件 CSS）
   - 用主题 token，不要硬编码颜色
   - 必须写 `@media (prefers-reduced-motion: reduce)` 降级

### 检查清单

- [ ] 用 `MOTION_TRANSITION.*` 而非硬编码时长
- [ ] motion 组件尊重 `useReducedMotion()`
- [ ] CSS 动画有 reduced-motion 降级
- [ ] 不在大量元素上动画 `box-shadow` / `filter: blur`
- [ ] 路由切换用 `AnimatePresence mode='wait'`

## 9.5 新增图标

### 选用决策树

```
是否是 LLM 提供商/模型品牌？
├── 是 → @lobehub/icons（通过 lib/lobe-icon.tsx）
└── 否 → 是否是 shadcn ui/ 原语内部使用？
        ├── 是 → @hugeicons/react
        └── 否 → lucide-react
```

### 使用约定

```tsx
// lucide-react（主力）
import { ArrowRight } from 'lucide-react'
<ArrowRight className='size-4' aria-hidden />

// hugeicons（shadcn ui 原语）
import { HugeiconsIcon, Cancel01Icon } from '@hugeicons/react'
<HugeiconsIcon icon={Cancel01Icon} />

// lobehub（品牌）
import { LibeIcon } from '@/lib/lobe-icon'
<LibeIcon provider='openai' />
```

### 检查清单

- [ ] 装饰图标加 `aria-hidden`
- [ ] 重要信息配文本，不能只用图标
- [ ] 尺寸用 `size-*`（`size-4` = 16px），不用 `w-4 h-4`
- [ ] 不混用不同库的同类图标（同一个 UI 区域保持一致）

## 9.6 新增字体

1. `package.json` 加 `@fontsource-variable/<font-name>`
2. `index.css` 加 `@import '@fontsource-variable/<font-name>'`
3. `theme.css` 的 `@theme inline` 块加 token：
   ```css
   --font-<name>: '<Font Name Variable>', <fallback>;
   ```
4. 如要加入字体轴，扩展 `ThemeFont` 类型 + `theme-presets.css` 的 `[data-theme-font='...']` 块 + `lib/theme-customization.ts` 的 `THEME_FONT_VALUES`

### 检查清单

- [ ] 字体含所需所有字形（拉丁、CJK、西里尔等，依支持语言）
- [ ] variable font（支持字重轴）或明确字重档
- [ ] license 兼容（OFL / Apache / MIT）
- [ ] 已通过 `@fontsource-variable` 安装，不依赖 CDN
- [ ] Fallback 链合理（避免字体未加载时布局跳动）

## 9.7 新增响应式布局

### 断点使用规范

**完全使用 Tailwind v4 默认断点**，不要自定义 `--breakpoint-*`：

| 前缀 | 触发 | 典型用途 |
|------|------|---------|
| (base) | 0px | 移动端默认样式 |
| `sm:` | 640px | 大手机/小平板增强 |
| `md:` | 768px | 平板（Sidebar → Sheet 切换点） |
| `lg:` | 1024px | 笔记本（TopNav 桌面切换点） |
| `xl:` | 1280px | 桌面 |
| `2xl:` | 1536px | 大屏 |

### Mobile-first 原则

```tsx
// ✅ 正确：mobile-first
<div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3'>

// ❌ 错误：desktop-first（默认 3 列，小屏降级）
<div className='grid grid-cols-3 lg:grid-cols-2 md:grid-cols-1'>
```

### 移动端 JS 阈值

```tsx
import { useIsMobile } from '@/hooks/use-mobile'
const isMobile = useIsMobile()  // < 768px
```

不要写新的断点常量，复用 `MOBILE_BREAKPOINT = 768`。

### 检查清单

- [ ] 默认样式是移动端（最小屏）
- [ ] `sm:` / `md:` / `lg:` 递进增强
- [ ] 不自定义断点
- [ ] JS 阈值用 `useIsMobile()`，不写新的 768 常量
- [ ] 表格在移动端有 card 视图（`use-data-table-view-mode`）
- [ ] sidebar 在 < 768px 切换为 Sheet

## 9.8 反模式（不要做）

### ❌ 直接调用 `encoding/json`

仅 backend 规则，前端无关。略。

### ❌ 在业务组件里 inline transition

```tsx
// ❌ 错误
<motion.div
  initial={{ opacity: 0, y: 8 }}
  animate={{ opacity: 1, y: 0 }}
  transition={{ duration: 0.3, ease: [0.33, 1, 0.68, 1] }}
>

// ✅ 正确：复用 lib/motion.ts
import { MOTION_VARIANTS, MOTION_TRANSITION } from '@/lib/motion'
<motion.div
  variants={MOTION_VARIANTS.slideUp}
  transition={MOTION_TRANSITION.default}
>
```

### ❌ 硬编码颜色

```tsx
// ❌ 错误
<div className='bg-blue-500 text-white'>
<button className='bg-[#4f7eff]'>

// ✅ 正确：用主题 token
<div className='bg-primary text-primary-foreground'>
```

### ❌ 分散的 CSS 文件

```tsx
// ❌ 错误：每个组件一个 CSS 文件
import './my-component.css'

// ✅ 正确：用 Tailwind 工具类；全局样式集中到 styles/{index,theme,theme-presets}.css
```

### ❌ 自定义 spacing scale

```css
/* ❌ 错误：新增 spacing token */
--spacing-my: 13px;

/* ✅ 正确：用 Tailwind 默认（除非通过密度轴系统覆盖） */
```

### ❌ 用 localStorage 存主题

```tsx
// ❌ 错误
localStorage.setItem('theme', 'dark')

// ✅ 正确：用 cookie（通过 @/lib/cookies）
setCookie('vite-ui-theme', 'dark', { maxAge: oneYear })
```

### ❌ 在 ui/ 组件里塞业务逻辑

```tsx
// ❌ 错误：ui/button.tsx 里读 useAuth()
// ✅ 正确：ui/ 只放业务无关基元；业务逻辑放 features/<feature>/
```

### ❌ 忽略 reduced-motion

```css
/* ❌ 错误：没有降级 */
.my-animation { animation: spin 1s linear infinite; }

/* ✅ 正确：写降级 */
.my-animation { animation: spin 1s linear infinite; }
@media (prefers-reduced-motion: reduce) {
  .my-animation { animation: none; }
}
```

## 9.9 升级前的自检清单

在提交任何前端改动前，确认：

### 类型与规范

- [ ] `bun run typecheck`（tsgo）通过
- [ ] `bun run lint`（oxlint）通过
- [ ] `bun run check`（knip，死代码检测）通过

### i18n

- [ ] 所有用户可见文案走 `t()`
- [ ] 常量值即 i18n 键（如 `SUCCESS_MESSAGES.CREATED = 'Created successfully'`）
- [ ] 新增的 key 在 `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi,zh-TW}.json` 都有对应翻译（或用 `bun run i18n:sync`）

### a11y

- [ ] 装饰图标 `aria-hidden`
- [ ] 重要信息有文本（不只靠图标/颜色）
- [ ] 键盘可达（Tab 顺序、Enter/Space 触发）
- [ ] 动画有 reduced-motion 降级
- [ ] 表单 label 关联 input
- [ ] 颜色对比度 ≥ WCAG AA

### 跨主题验证

- [ ] 亮色模式视觉正确
- [ ] 暗色模式视觉正确
- [ ] 至少在 `default` + `anthropic` + 1 个染色预设下验证
- [ ] serif 字体模式下视觉正确
- [ ] sm 密度（紧凑）下视觉正确

### 跨断点验证

- [ ] 移动端（375px iPhone SE）
- [ ] 平板（768px iPad）
- [ ] 桌面（1280px）
- [ ] 大屏（1536px+）

### 构建验证

- [ ] `bun run build` 成功
- [ ] bundle size 无异常增长（检查 splitChunks）
- [ ] 无 console.log 残留（prod 会移除）

## 9.10 设计系统演进路线建议

基于本次梳理，建议的长期演进方向：

### 短期（保持现状）

- 维持 3 个 CSS 文件分层（index/theme/theme-presets）
- 维持双 Provider 主题架构
- 继续用 shadcn CLI 注入新组件

### 中期（待 default 完全替代 classic 后）

- 把 classic 的「应保留资产」（8.3 节）逐步迁移：SelectableButtonGroup 5 色变体、马卡龙装饰球、CardPro 6 插槽抽象
- 引入 `--shadow-*` token，统一散落的硬编码 box-shadow
- 考虑 Storybook 或 Ladle 做组件文档

### 长期（如需）

- 评估 next-themes 替换自研 ThemeProvider（依赖已在，可简化代码）
- 评估容器查询在更多业务组件的应用（目前只在布局壳用）
- 评估 design tokens 同步到 Figma（双向同步工具如 Tokens Studio）

---

**文档维护约定**：

- 每次大的前端架构变动（新增/移除依赖、改变主题系统、重构布局）后，更新对应章节
- 在 PR 描述中引用本文档章节，说明改动遵循的设计规范
- 如发现文档与实际代码不符，以代码为准，并修正文档
