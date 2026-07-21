# 01 · 架构与技术栈

## 1.1 双前端定位

`web/` 是一个 Bun workspace，包含两个独立子包：

| 子包 | 角色 | 语言 | UI 库 | 状态 |
|------|------|------|------|------|
| `web/default` | **当前主推**「新版前端」 | TypeScript | shadcn "base-nova"（基于 `@base-ui/react`） | 活跃开发 |
| `web/classic` | **已标记弃用**「经典/旧版前端」 | JavaScript | Semi Design（`@douyinfe/semi-ui`） | 最小维护 |

判定依据：

- `web/classic/src/components/layout/PageLayout.jsx` 中有 `ClassicFrontendDeprecationBanner`（弃用横幅）。
- 项目提供 `classic-to-default-sync` skill，工作流为「将 classic 某次提交的功能/修复移植到 default」。
- `web/default/AGENTS.md` 写入了完整工程规范（i18n、类型、测试、a11y、lint、format、knip），classic 无对应文档。

**结论：所有新设计、新页面、新组件都应该在 `web/default` 中实现。** 本文档后续章节若不特别说明，均指 `web/default`。

## 1.2 web/default 目录结构

feature-based + 文件式路由：

```
web/default/src/
├── main.tsx                    # 入口（QueryClient + TanStack Router + 多层 Provider）
├── routeTree.gen.ts            # 自动生成的路由树（@tanstack/router-plugin）
├── routes/                     # 路由文件（薄壳）
│   ├── __root.tsx              #   根布局：ThemeCustomizationProvider + Toaster + NavigationProgress
│   ├── index.tsx               #   / → Home（落地页）
│   ├── (auth)/                 #   认证路径组（sign-in/sign-up/forgot-password/...）
│   ├── (errors)/               #   错误页 401/403/404/500/503
│   ├── _authenticated/         #   受保护布局（带守卫）
│   │   ├── dashboard/$section
│   │   ├── channels, keys, models, usage-logs, users
│   │   ├── redemption-codes, subscriptions, wallet, profile
│   │   ├── playground, chat/$chatId, chat2link
│   │   ├── system-info, errors/$error
│   │   └── system-settings/$section   # 仅 SUPER_ADMIN
│   ├── about, pricing, rankings, setup, oauth/$provider, console/...
│   └── ...
├── features/                   # 24 个功能模块（真正的页面实现）
│   └── <feature>/
│       ├── components/
│       ├── hooks/
│       ├── lib/
│       ├── api.ts              #   TanStack Query 封装
│       ├── types.ts
│       └── constants.ts
├── components/
│   ├── ui/                     # shadcn 60+ 基元组件（Base UI 驱动）
│   ├── data-table/             # 基于 @tanstack/react-table 二次封装的工作台表格系统
│   ├── ai-elements/            # 来自 @ai-elements registry
│   ├── layout/                 # 布局壳（AuthenticatedLayout / PublicLayout）
│   └── ...                     # 业务无关的通用组件
├── lib/                        # 35+ 工具模块（api.ts/utils.ts/motion.ts/theme-customization.ts/...）
├── stores/                     # zustand stores（auth/notification/system-config）
├── context/                    # 6 个 Provider（theme/font/direction/layout/search/theme-customization）
├── hooks/                      # ~20 通用 hook（use-sidebar-*/use-system-config/use-dialog）
├── i18n/                       # config.ts/languages.ts/static-keys.ts/locales/{en,zh,fr,ja,ru,vi,zh-TW}
├── styles/                     # index.css + theme.css + theme-presets.css（仅这 3 个 CSS 文件）
├── config/
├── assets/
└── env.d.ts
```

## 1.3 关键依赖（package.json 摘要）

| 类别 | 依赖 | 版本 |
|------|------|------|
| 框架 | `react` / `react-dom` | 19 |
| 构建 | `@rsbuild/core` | 2.x |
| 路由 | `@tanstack/react-router` | 1.170 |
| 数据 | `@tanstack/react-query` | 5 |
| 状态 | `zustand` | 5 |
| HTTP | `axios` | catalog |
| UI 基元 | `@base-ui/react` | — |
| UI 工具 | `shadcn` CLI | 4.12 |
| 类名 | `class-variance-authority` + `clsx` + `tailwind-merge` | — |
| CSS | `tailwindcss` | **4.3+**（CSS-first，无 JS 配置） |
| 表单 | `react-hook-form` + `zod` + `@hookform/resolvers` | 7 / 4 / — |
| 表格 | `@tanstack/react-table` + `@tanstack/react-virtual` | — |
| 图表 | `@visactor/vchart` + `react-vchart` + `recharts` | 2.1 / — / 3 |
| i18n | `i18next` + `react-i18next` + `i18next-browser-languagedetector` | 26 / 17 / 8 |
| 通知 | `sonner` | 2 |
| 动效 | `motion`（Framer Motion v12） | 12 |
| 字体 | `@fontsource-variable/public-sans` / `@fontsource-variable/lora` | 5.2 |
| 图标 | `lucide-react` / `@hugeicons/react` + `@hugeicons/core-free-icons` / `@lobehub/icons` | — |
| Markdown | `marked` + `shiki` + `katex` + `dompurify` + `stream-markdown-parser` | 18 / 4 / — |
| 编辑器 | `codemirror` | — |
| 抽屉 | `vaul` | — |
| 轮播 | `embla-carousel-react` | — |
| 拖拽 | `react-resizable-panels` | — |
| AI | `ai`（Vercel AI SDK） | 7 |
| 骨架屏 | `auto-skeleton-react` | — |
| 进度条 | `react-top-loading-bar` | — |
| Lint | `oxlint` + `oxfmt` + `knip` | — |

## 1.4 构建配置要点（rsbuild.config.ts）

- 入口：`./src/main.tsx`，`@` 别名 → `./src`
- 插件：`pluginReact()` + `pluginTailwindcss({ optimize: false })`
- **手动 splitChunks**（`rsbuild.config.ts:28-54`）：
  - `vendor-react`（react/react-dom/react-router/react-query）
  - `vendor-ui-primitives`（@base-ui/@radix-ui）
  - `vendor-tanstack`
- **TanStack Router 插件**（`rsbuild.config.ts:91-98`）：
  - `target: 'react'`
  - dev：`autoCodeSplitting: false`（防白屏闪烁）
  - prod：`autoCodeSplitting: true`（按路由分包）
- dev 代理：`/api` `/mj` `/pg` → `VITE_REACT_APP_SERVER_URL`（默认 `http://localhost:3000`）
- prod：移除 `console.log`，`buildCache: false`（完全禁用缓存）

## 1.5 入口 Provider 嵌套（main.tsx）

```
StrictMode
└── QueryClientProvider              # TanStack Query，自定义 retry（401/403 不重试，500 跳 /500）
    └── ThemeProvider                # 亮/暗/system 三态，class 写到 <html>
        └── FontProvider             # 字体方向（LTR/RTL）
            └── DirectionProvider
                └── RouterProvider   # TanStack Router，defaultPreload: 'intent'
```

启动时副作用（main.tsx）：

- `initializeFrontendCache()` + `installBuildMetadata()`：注入构建版本（`--app-rev`）
- `initSystemBranding()`：localStorage 缓存优先设置 `document.title` / favicon，后台异步刷新 status
- `QueryClient` 全局错误拦截：401 → toast「Session expired」+ reset auth + 跳 `/sign-in`；500 → 跳 `/500`

## 1.6 路由树概览

详细路由结构见 `src/routeTree.gen.ts`（自动生成，勿手改）。关键约定：

- `routes/` 放薄路由文件（`createFileRoute` + 守卫 + 懒加载 feature 组件）
- `features/<feature>/` 放真正页面实现
- `(group)` = 路径不进 URL 的分组
- `_authenticated` = 受保护布局路由
- `$param` = 动态段
- `index.tsx` = `/`

主要顶层路由：

| 路径 | 模块 |
|------|------|
| `/` | Home（落地页） |
| `/about` `/pricing` `/rankings` `/setup` `/oauth/$provider` `/console/*` | 公开/混合 |
| `/sign-in` `/sign-up` `/forgot-password` `/reset` `/otp` `/register` | 认证 |
| `/401` `/403` `/404` `/500` `/503` | 错误页 |
| `/dashboard/$section` `/channels` `/keys` `/models/$section` `/usage-logs/$section` `/users` `/redemption-codes` `/subscriptions` `/wallet` `/profile` `/playground` `/chat/$chatId` `/chat2link` `/system-info` `/errors/$error` | 受保护 |
| `/system-settings/$section` | 受保护 + SUPER_ADMIN（drill-in sidebar 视图） |

24 个 feature 模块：`about, auth, channels, chat, dashboard, errors, home, keys, legal, models, performance-metrics, playground, pricing, profile, rankings, redemption-codes, setup, subscriptions, system-info, system-settings, usage-logs, users, wallet`。

## 1.7 状态管理分层

| 层 | 用途 | 实现 |
|----|------|------|
| 服务端状态 | API 数据、缓存、loading | TanStack Query |
| 全局客户端状态 | 认证、通知、系统配置 | zustand stores（`stores/auth-store.ts` 等） |
| 局部 UI 状态 | 单组件内 | React `useState`/`useReducer` |
| URL 状态 | 路由参数、查询字符串 | TanStack Router |
| 主题状态 | 主题/字体/圆角/密度 | React Context + cookie 持久化 |

## 1.8 TypeScript 配置

- 三件套：`tsconfig.json`（references 聚合 + `@/*` paths）、`tsconfig.app.json`（app）、`tsconfig.node.json`（node）
- `tsconfig.app.json`：`target: ES2020`、`moduleResolution: Bundler`、`strict: true`、`noUnusedLocals/Parameters`、`noFallthroughCasesInSwitch`、`noUncheckedSideEffectImports`、`jsx: react-jsx`、`noEmit: true`
- 类型检查用 `@typescript/native-preview`（tsgo），脚本 `bun run typecheck` → `tsgo -b`
