# 04 · 布局系统

## 4.1 两条布局主线

`web/default` 区分两类完全不同的布局壳：

| 类型 | 布局组件 | 适用场景 |
|------|----------|----------|
| **AuthenticatedLayout**（工作台） | `components/layout/authenticated-layout.tsx` | 登录后的控制台（dashboard / channels / keys / models / ...） |
| **PublicLayout**（公开页） | `components/layout/public-layout.tsx` | 落地页、登录页、关于页、定价页等 |

布局代码集中在 `src/components/layout/`，对外通过 `src/components/layout/index.ts` 统一导出。

## 4.2 AuthenticatedLayout：工作台布局

来源：`components/layout/components/authenticated-layout.tsx`

```tsx
<LayoutProvider>
  <SearchProvider>
    <SidebarProvider defaultOpen={defaultOpen} className='flex-col'>
      <SkipToMain />
      <AppHeader />
      <div className='flex min-h-0 w-full flex-1'>
        <AppSidebar />
        <SidebarInset
          className={cn(
            '@container/content',                                          // 容器查询上下文
            'h-[calc(100svh-var(--app-header-height,0px))]',               // 减去 header 高度
            'min-h-0 overflow-hidden',
            'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]'
          )}
        >
          {props.children ?? <AnimatedOutlet />}
        </SidebarInset>
      </div>
    </SidebarProvider>
  </SearchProvider>
</LayoutProvider>
```

### 关键设计点

- **cookie 持久化 sidebar 状态**：`defaultOpen = getCookie('sidebar_state') !== 'false'`
- **视口高度计算**：`100svh - var(--app-header-height)`，使用 `svh`（small viewport height）避免移动端浏览器地址栏遮挡
- **容器查询上下文**：`SidebarInset` 标 `@container/content`，内部组件可用 `@7xl/content:` 等容器查询断点
- **inset 变体高度补偿**：`peer-data-[variant=inset]` 时多减 `var(--spacing)*4`（inset 布局上下留白）

### 组件职责清单

| 组件 | 文件 | 职责 |
|------|------|------|
| `AuthenticatedLayout` | `authenticated-layout.tsx` | 根布局壳（SidebarProvider + AppHeader + AppSidebar + SidebarInset） |
| `AppHeader` | `app-header.tsx` | 顶栏：SystemBrand + TopNav（仅 lg:）+ Search + NotificationPopover + LanguageSwitcher + ConfigDrawer + ProfileDropdown |
| `Header` | `header.tsx` | 纯展示 `<header sticky top-0 z-40 h-[var(--app-header-height,3rem)]>`，内置 SidebarTrigger |
| `AppSidebar` | `app-sidebar.tsx` | Vercel/Cloudflare 式 drill-in 侧边栏（URL 驱动视图切换） |
| `NavGroup` | `nav-group.tsx` | 导航分组 |
| `NavLinkItem` | `nav-link-item.tsx` | 单条导航链接 |
| `SidebarViewHeader` | `sidebar-view-header.tsx` | 嵌套视图的「← Back to Dashboard」头部 |
| `TopNav` | `top-nav.tsx` | lg: 横向 nav；lg: 以下降级为 DropdownMenu |
| `MobileDrawer` | `mobile-drawer.tsx` | 移动端抽屉式导航 |

### AppSidebar：drill-in 模式

来源：`components/layout/components/app-sidebar.tsx`

- **URL 驱动视图切换**：通过 `useSidebarView` + `sidebar-view-registry.ts` 注册的视图，按当前 URL 切换 sidebar 内容
- **横向滑入动画**：切换时用 `motion` + `AnimatePresence` 配 `MOTION_VARIANTS.sidebarSlide`（app-sidebar.tsx:56-71）
- **嵌套视图**：目前注册了 `SYSTEM_SETTINGS_VIEW`（系统设置二级导航）
- **注册表**：`components/layout/lib/sidebar-view-registry.ts:33`

## 4.3 PublicLayout：公开页布局

来源：`components/layout/components/public-layout.tsx`

```tsx
<div className='min-h-svh overflow-x-clip'>
  <PublicHeader />
  <main className='container px-4 py-6 pt-20 md:px-4'>
    {showMainContainer ? children : <>{children}</>}
  </main>
</div>
```

- **`overflow-x-clip`**：防止宽 Hero 元素触发水平滚动
- **`pt-20`**：为 fixed/sticky 的 PublicHeader 留出空间
- **`showMainContainer`** prop：落地页 Hero 等全宽场景设 `false` 关掉 `<main>` 包装

### PublicHeader：scroll-driven 玻璃态导航

来源：`components/layout/components/public-header.tsx`

滚动行为（public-header.tsx:178-191）：

- **未滚动**（scrollTop ≤ 20px）：`max-w-7xl px-4`，普通顶部导航
- **已滚动**（scrollTop > 20px）：收缩为 `max-w-[52rem] px-3 pt-3` + `rounded-2xl backdrop-blur-2xl ring`，变成浮岛

过渡统一：`duration-700 ease-[cubic-bezier(0.16,1,0.3,1)]`（emeric easing）。

- 桌面导航：`sm:flex`
- 移动端：自定义汉堡 + 全屏 overlay，菜单项带 `100+i*50ms` 的错峰 `transition-delay`

## 4.4 标准工作台页面骨架：SectionPageLayout

来源：`components/layout/components/section-page-layout.tsx`

复合组件 slots 模式（类似 Semi Layout）：

```tsx
<SectionPageLayout>
  <SectionPageLayout.Breadcrumb>...</SectionPageLayout.Breadcrumb>
  <SectionPageLayout.Title>渠道管理</SectionPageLayout.Title>
  <SectionPageLayout.Actions>
    <Button>新建</Button>
  </SectionPageLayout.Actions>
  <SectionPageLayout.Content>
    {/* 实际内容 */}
  </SectionPageLayout.Content>
</SectionPageLayout>
```

- **内边距响应式**：`px-3 pt-3 sm:px-4 sm:pt-5`（section-page-layout.tsx:82）
- **`fixedContent` prop**：控制内容区 `overflow-hidden` 还是 `overflow-auto`
- **底部 Portal 槽**：`PageFooterProvider` 提供 footer 槽（粘性底部按钮等）

## 4.5 主内容区 Main

来源：`components/layout/components/main.tsx`

```tsx
<main className='flex min-h-0 flex-1 flex-col overflow-hidden'>
  {/* fluid={false} 时加：@7xl/content:mx-auto @7xl/content:max-w-7xl */}
</main>
```

- **默认撑满侧边栏右侧**，不强制居中
- **容器查询限宽**：当容器宽度 ≥ 7xl 时，可选限宽 1280px 居中（main.tsx:31）
- `fluid` prop 控制是否启用限宽

## 4.6 栅格系统

**没有全局栅格 wrapper。** 策略：

- **Tailwind v4 原生 grid/flex 工具类**为主（AGENTS.md §3.10）
- 无 CSS-in-JS、无自定义 grid framework
- 无 12 列抽象（极少用 `lg:grid-cols-12`，仅 1 次）

### Container 居中（index.css:133-144）

```css
@utility container { margin-inline: auto; padding-inline: 2rem; }
@utility max-w-container { max-width: 1280px; }
@utility max-w-container-lg { max-width: 1536px; }
```

### 典型卡片栅格模式

来源：`components/data-table/layout/card-grid.tsx:75` 与 `features/pricing/components/model-card-grid.tsx:76`：

```tsx
<div className='grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 lg:grid-cols-3'>
  {items.map(...)}
</div>
```

### 响应式栅格使用频次（全 `src/**/*.tsx` 聚合）

| 工具类 | 出现次数 |
|--------|---------|
| `sm:grid-cols-2` | 42 |
| `md:grid-cols-2` | 25 |
| `sm:grid-cols-3` | 16 |
| `md:grid-cols-3` | 10 |
| `lg:grid-cols-2` | 10 |
| `lg:grid-cols-3` | 8 |
| `md:grid-cols-4` | 6 |
| `sm:grid-cols-4` | 4 |

### 双栏布局范例

`features/dashboard/components/overview-dashboard.tsx:254`：

```tsx
<div className='grid grid-cols-[minmax(0,1fr)_19rem]'>
  <Main />      {/* 弹性主区 */}
  <Aside />     {/* 固定 19rem 侧栏 */}
</div>
```

## 4.7 移动端适配策略

- **Mobile-first**：所有 utility 默认是移动端样式，`sm:`/`md:`/`lg:` 递进增强
- **TopNav**：`lg:` 显示横向；`lg:` 以下变 DropdownMenu
- **PublicHeader**：`sm:flex` 桌面导航；`sm:hidden` 自定义汉堡 + 全屏 overlay
- **Sidebar**：< 768px（`useIsMobile()`）切换为 `Sheet`（vaul-style drawer）
- **DataTable**：表格/卡片视图可手动切换（`use-data-table-view-mode.ts`），并有专门的 `mobile-card-list.tsx`
- **栅格**：卡片网格统一从 `grid-cols-1` 起步

## 4.8 全局路由壳

来源：`routes/__root.tsx`

```tsx
<>
  <ThemeCustomizationProvider>
    <NavigationProgress />    {/* 路由 pending 时顶部 2px 进度条 */}
    <Outlet />
    <Toaster position='top-center' richColors closeButton duration={5000} />
  </ThemeCustomizationProvider>
</>
```

- `beforeLoad` 做 setup 状态检查（带 localStorage 缓存）
- `errorComponent = GeneralError`
- `notFoundComponent = NotFoundError`

## 4.9 路由守卫分层

| 守卫层 | 文件 | 规则 |
|--------|------|------|
| 根 | `routes/__root.tsx` | 检查 setup 状态 |
| 认证 | `routes/_authenticated/route.tsx` | `beforeLoad` 走认证校验（localStorage 优先，每会话仅一次 `getSelf()` 验证），失败重定向 `/sign-in` |
| 超管 | `routes/_authenticated/system-settings/route.tsx` | 再叠加 `ROLE.SUPER_ADMIN` 守卫，否则 redirect `/403` |
| 路径组 | `routes/(auth)/route.tsx` | 空实现（给 sign-in/sign-up/... 共享前缀但不加布局） |

## 4.10 装饰元素：Glass Morphism 与 Fade Mask

虽然不属于布局结构，但常用于布局组件（公开页、Hero 区）：

- **Glass morphism 5 级**（index.css:243-261）：`.glass-1` ~ `.glass-5`，Launch UI 风格的半透明背景 + 模糊
- **Fade mask 9 方向**（index.css:264-314）：`.fade-x/-y/-top/-bottom/-left/-right/...`，CSS mask 渐隐
- **马卡龙模糊球**：classic 的 `.with-pastel-balls` 装饰尚未迁移到 default，可作为升级参考（见 [08-classic-baseline.md](./08-classic-baseline.md)）

## 4.11 页脚

| 组件 | 用途 |
|------|------|
| `components/layout/footer.tsx` | 落地页页脚 |
| `components/layout/page-footer.tsx` | SectionPageLayout 的粘性底部 Portal |

## 4.12 布局升级建议

新增页面时按以下决策树选布局：

```
是否需要登录？
├── 是 → 是否 SUPER_ADMIN 且是系统设置？
│       ├── 是 → _authenticated/system-settings/$section（drill-in sidebar）
│       └── 否 → _authenticated/<feature>（AuthenticatedLayout + SectionPageLayout）
└── 否 → 是否全宽 Hero 落地页？
        ├── 是 → PublicLayout + showMainContainer={false}
        └── 否 → PublicLayout + 默认 container
```

新增工作台页面建议：

1. 在 `routes/_authenticated/<feature>/` 加路由文件（薄壳，懒加载）
2. 在 `features/<feature>/components/` 实现页面，外层用 `<SectionPageLayout>` 包裹
3. 内容区用 `grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3` 等响应式栅格
4. 表格型页面用 `components/data-table/` 系统（见 07-components.md）
