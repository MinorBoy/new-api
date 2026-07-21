# 07 · 组件系统

## 7.1 组件库选型：shadcn "base-nova"

### 配置（components.json）

```json
{
  "style": "base-nova",
  "rsc": false,
  "tsx": true,
  "tailwind": {
    "config": "",
    "css": "src/styles/index.css",
    "baseColor": "neutral",
    "cssVariables": true,
    "prefix": ""
  },
  "iconLibrary": "hugeicons",
  "menuColor": "inverted",
  "menuAccent": "subtle",
  "registries": {
    "@ai-elements": "https://registry.ai-sdk.dev/{name}.json"
  }
}
```

### 关键决策

| 项 | 选择 | 含义 |
|----|------|------|
| `style` | `base-nova` | shadcn v4 的新风格，**基于 `@base-ui/react` 而非 Radix** |
| `rsc` | false | 非 React Server Components（SPA） |
| `tsx` | true | TypeScript |
| `baseColor` | neutral | 基础灰阶（但实际项目用 OKLCH 自定义） |
| `cssVariables` | true | 用 CSS 变量主题化 |
| `iconLibrary` | hugeicons | shadcn 默认图标库（用于 ui/ 原语） |
| `registries.@ai-elements` | ai-sdk.dev | AI 相关组件（shimmer 等）从 Vercel AI SDK registry 拉 |

### 基元来源分层

| 来源 | 组件 | 数量 |
|------|------|------|
| **Base UI**（`@base-ui/react/*`） | accordion, alert-dialog, avatar, badge, breadcrumb, button-group, button, checkbox, collapsible, combobox, context-menu, dialog, direction, dropdown-menu, form, hover-card, input, item, menubar, navigation-menu, popover, progress, radio-group, scroll-area, select, separator, sheet, sidebar, slider, switch, tabs, toggle-group, toggle, tooltip | 34 |
| **第三方库** | drawer (vaul), carousel (embla-carousel-react), command (cmdk), calendar (react-day-picker), chart (recharts), resizable (react-resizable-panels), input-otp (input-otp) | 7 |
| **完全自定义**（不在 shadcn registry） | titled-card, icon-badge, empty, markdown, kbd, spinner, sonner, direction; data-table/*; ai-elements/*; layout/*; page-transition, auto-skeleton, navigation-progress; rich-content, config-drawer, notification-popover, profile-dropdown, search, language-switcher, theme-switch | — |

## 7.2 通用约定

### `data-slot` 属性

所有 shadcn 组件带 `data-slot` 属性供 CSS 选择。例如：

```tsx
<button data-slot='button' className='...'>
```

CSS 中可用：

```css
[data-slot='button'] { ... }
[data-slot='card']:hover { transform: translateY(-1px); }
[data-slot='table'] tbody tr { animation: ...; }
```

### 类名合并：`cn()`

所有动态类名通过 `cn()`（clsx + tailwind-merge）合并：

```tsx
import { cn } from '@/lib/utils'

<div className={cn('base-class', condition && 'conditional-class', className)} />
```

### variant 系统：CVA

按钮、badge 等用 `class-variance-authority` 定义 variant：

```tsx
const buttonVariants = cva('base classes', {
  variants: {
    variant: { default: '...', destructive: '...', outline: '...', ... },
    size: { default: '...', sm: '...', lg: '...', icon: '...' },
  },
  defaultVariants: { variant: 'default', size: 'default' },
})
```

## 7.3 高频组件模式

### Card

来源：`ui/card.tsx`

```tsx
<Card>
  <CardHeader>
    <CardTitle>标题</CardTitle>
    <CardDescription>描述</CardDescription>
  </CardHeader>
  <CardContent>...</CardContent>
  <CardFooter>...</CardFooter>
</Card>
```

- 全局 CSS 自动加 hover 动效（`translateY(-1px)` + shadow）
- `data-card-hover='false'` 关闭 hover 动效
- **`TitledCard`**（`ui/titled-card.tsx`）：高频复合 = 标题 + 图标 + 描述 + action（带 `IconBadge` tone）

### Dialog / Modal

来源：`ui/dialog.tsx`（底层 Base UI）+ `components/dialog.tsx`（项目封装）

```tsx
// 项目封装（推荐）
<Dialog
  open={open}
  onOpenChange={setOpen}
  title='编辑'
  description='修改渠道配置'
  contentClassName='max-w-2xl'
  contentHeight='max-h-[85vh]'
  footer={<><Button variant='outline'>取消</Button><Button>保存</Button></>}
>
  <FormContent />
</Dialog>
```

关闭按钮用 Hugeicons `Cancel01Icon`。

### Drawer

来源：`ui/drawer.tsx`（**vaul**，非 Base UI）

```tsx
<Drawer>
  <DrawerTrigger asChild><Button>打开</Button></DrawerTrigger>
  <DrawerContent>
    <DrawerHeader>...</DrawerHeader>
    <DrawerBody>...</DrawerBody>
  </DrawerContent>
</Drawer>
```

常用于 row 编辑（如 `features/keys/components/api-keys-mutate-drawer.tsx`）。

### Sheet

来源：`ui/sheet.tsx`（Base UI）。Sidebar 在移动端降级为 Sheet。

### Table（工作台级）

**核心：`components/data-table/`** —— 基于 `@tanstack/react-table` + `@tanstack/react-virtual` 的二次封装系统。

```
data-table/
├── core/           # 核心：column-header / pagination / row-action-menu / table-skeleton
├── toolbar/        # 工具栏：faceted-filter / bulk-actions / view-options / view-mode-toggle
├── layout/         # 布局：card-grid / mobile-card-list / data-table-page
└── static/         # 静态数据表（非异步）
```

特性：

- **table/card 双视图**：由 `use-data-table-view-mode.ts` 管理（可 localStorage 持久化）
- **虚拟滚动**：长列表用 `@tanstack/react-virtual`
- **行错峰入场**：`motion.tr` 配 `TABLE_STAGGER_VARIANTS`
- **批量操作**：toolbar 的 `bulk-actions`
- **分面过滤**：toolbar 的 `faceted-filter`

基础 Table 原语：`ui/table.tsx`（普通语义 table）。

### Form

来源：`react-hook-form` + `zod` + `@hookform/resolvers`

模式：

```tsx
// features/<feature>/lib/schema.ts
const schema = z.object({
  name: z.string().min(1, '必填'),
  email: z.string().email('邮箱格式错误'),
})
type FormValues = z.infer<typeof schema>

// 组件
const form = useForm<FormValues>({
  resolver: zodResolver(schema),
  defaultValues: { name: '', email: '' },
})

<Form {...form}>
  <form onSubmit={form.handleSubmit(onSubmit)}>
    <FormField name='name' control={form.control} render={...} />
  </form>
</Form>
```

- `ui/form.tsx`（Base UI Field）+ `ui/field.tsx` / `ui/input-group.tsx`
- `ui/input-otp.tsx`（`input-otp` 库）：OTP 验证码
- `ui/combobox.tsx` / `combobox-input.tsx`（基于 `cmdk`）：搜索下拉

### Tabs

来源：`ui/tabs.tsx`（Base UI）

**特殊用法**：System Settings 的 section 切换**由路由 `$section` 驱动**，而非 Tabs 组件，让 URL 与 tab 状态同步。Settings 内部用 `settings-accordion.tsx` 做折叠分组。

### Toast

来源：`sonner`（`ui/sonner.tsx`）

```tsx
// 根路由挂载
<Toaster position='top-center' richColors closeButton duration={5000} />

// 业务调用
import { toast } from 'sonner'
toast.success(t(SUCCESS_MESSAGES.CREATED))
toast.error(t(ERROR_MESSAGES.NOT_FOUND))
```

约定（AGENTS.md §3.1）：

- 图标用 Hugeicons（CheckmarkCircle02 / Information / Alert02 / MultiplicationSignCircle / Loading03）
- 消息文案走 i18n：常量值即 i18n 键，例如 `SUCCESS_MESSAGES.CREATED = 'Created successfully'`
- 401 全局拦截 → toast「Session expired」（main.tsx QueryClient）

## 7.4 业务无关通用组件清单（components/）

| 组件 | 文件 | 用途 |
|------|------|------|
| `PageTransition` / `AnimatedOutlet` | `page-transition.tsx` | 路由切换动画 |
| `NavigationProgress` | `navigation-progress.tsx` | 顶部 2px 进度条 |
| `AutoSkeleton` / `ContentSkeleton` / `QuerySkeleton` | `auto-skeleton.tsx` | 骨架屏包装 |
| `SkipToMain` | `skip-to-main.tsx` | 无障碍跳转链接 |
| `RichContent` | `rich-content.tsx` | 富文本渲染 |
| `Markdown` | `ui/markdown.tsx` | marked + dompurify + Shiki + KaTeX |
| `Search` | `search.tsx` | 全局搜索 |
| `LanguageSwitcher` | `language-switcher.tsx` | 语言切换 |
| `ThemeSwitch` / `ThemeQuickSwitcher` | `theme-switch.tsx` | 主题切换 |
| `ConfigDrawer` | `config-drawer.tsx` | 配置抽屉 |
| `NotificationPopover` | `notification-popover.tsx` | 通知气泡 |
| `ProfileDropdown` | `profile-dropdown.tsx` | 个人菜单 |
| `SystemBrand` / `Logo` / `HeaderLogo` | layout/ | 品牌标识 |

## 7.5 组件升级建议

### 新增 shadcn 组件

```bash
cd web/default
bunx shadcn add <component-name>
```

会自动：

- 从 `base-nova` registry 拉组件到 `src/components/ui/<name>.tsx`
- 使用 hugeicons 作为图标
- 用项目 CSS 变量（无需改 theme.css）

### 自定义组件规范

- 放 `src/components/`（业务无关）或 `src/features/<feature>/components/`（业务相关）
- 用 `cn()` 合并类名
- 用 `data-slot` 暴露选择钩子（如需被全局 CSS 选中）
- 复用 `MOTION_*` variants（不要 inline transition）
- a11y：装饰图标 `aria-hidden`，重要信息配文本
