# new-api 前端设计文档（Frontend Design Spec）

> 本文档系列是对 `new-api` 前端设计系统的全面梳理，作为后续页面设计升级迭代的依据。
>
> 调查基线日期：2026-07-19 · 调查对象：`web/default`（主推新版）与 `web/classic`（弃用旧版）。

## 文档结构

| 文档 | 内容 |
|------|------|
| [01-architecture.md](./01-architecture.md) | 双前端定位、目录结构、技术栈、构建配置、路由与状态管理 |
| [02-design-tokens.md](./02-design-tokens.md) | CSS 变量、OKLCH 配色、字体、圆角、间距、阴影、断点等所有 token |
| [03-theming.md](./03-theming.md) | 双 Provider 主题系统、暗黑模式、10 套预设、4 个独立可调轴 |
| [04-layout.md](./04-layout.md) | 工作台/公开页布局、栅格、容器、Sidebar/Header/Footer |
| [05-typography-icons.md](./05-typography-icons.md) | 字体系统、排版、图标库选用约定 |
| [06-motion.md](./06-motion.md) | motion variants、CSS 动画、骨架屏、reduced-motion |
| [07-components.md](./07-components.md) | shadcn "base-nova" 组件清单、DataTable、表单、Toast 等模式 |
| [08-classic-baseline.md](./08-classic-baseline.md) | classic 前端基线及迁移到 default 时应保留的设计资产 |
| [09-upgrade-playbook.md](./09-upgrade-playbook.md) | 升级迭代建议：新增预设/组件时的规范与检查清单 |
| [10-ysrouter-design-spec.md](./10-ysrouter-design-spec.md) | YSRouter 落地页设计规范（双主题 token、段落、组件、迁移路径） |

## 一句话概览

**Tailwind v4 CSS-first + OKLCH + shadcn "base-nova"（Base UI 基元）+ 10 套可切换主题预设 + 4 个独立可调轴（预设/字体/圆角/密度）+ motion v12 动效系统 + TanStack Router/Query + zustand。** 所有设计 token 以 CSS 变量定义在 3 个 CSS 文件中，组件库通过 shadcn CLI 注入 `src/components/ui/`，业务页面按 feature 模块组织在 `src/features/` 下。

## 速查：核心文件清单

| 文件 | 作用 |
|------|------|
| `web/default/components.json` | shadcn 配置（style: base-nova，iconLibrary: hugeicons，baseColor: neutral） |
| `web/default/src/styles/index.css` | 全局样式入口、Tailwind 导入、字体加载、CSS 动画 |
| `web/default/src/styles/theme.css` | `@theme inline` 桥接 + `:root`/`.dark` 配色 token |
| `web/default/src/styles/theme-presets.css` | 10 套主题预设 + 字体/圆角/密度/布局轴 |
| `web/default/src/context/theme-provider.tsx` | 亮/暗/system 三态 |
| `web/default/src/context/theme-customization-provider.tsx` | 预设/字体/圆角/密度/布局 5 轴 |
| `web/default/src/lib/theme-customization.ts` | 预设注册表与类型 |
| `web/default/src/lib/motion.ts` | 所有 motion variants 与 transition |
| `web/default/src/components/layout/` | 布局组件（AuthenticatedLayout / PublicLayout） |
| `web/default/src/components/ui/` | shadcn 60+ 基元组件 |
| `web/default/rsbuild.config.ts` | 构建、splitChunks、TanStack Router 插件 |
| `web/default/AGENTS.md` | 前端工程规范（权威） |

## 版权与许可

本项目前端代码遵循 AGPL-3.0 许可，版权归 QuantumNous（2023-2026）。本设计文档为衍生技术资料，同样遵循项目许可。引用源文件时请保留各文件顶部的版权声明。
