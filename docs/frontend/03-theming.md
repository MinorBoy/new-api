# 03 · 主题系统

## 3.1 架构总览：双 Provider

主题系统由两个独立 React Context 组成，职责严格分离：

| Provider | 文件 | 管理内容 | DOM 写入位置 |
|----------|------|----------|--------------|
| `ThemeProvider` | `context/theme-provider.tsx` | 亮/暗/system 三态 | `<html>` 的 class（`light`/`dark`） |
| `ThemeCustomizationProvider` | `context/theme-customization-provider.tsx` | 预设/字体/圆角/密度/布局 5 轴 | `<body>` 的 5 个 data 属性 |

**Provider 嵌套**（`main.tsx` 第 165–176 行）：

```
QueryClientProvider > ThemeProvider > FontProvider > DirectionProvider > RouterProvider
```

`ThemeCustomizationProvider` 在路由根组件 `routes/__root.tsx` 内挂载（不在 main.tsx 里），这样它能在路由 ready 后才应用，避免阻塞首屏。

## 3.2 持久化策略

- **使用 cookie，不用 localStorage、不用 next-themes**（虽然 `next-themes` 在依赖里但未使用）
- 所有 cookie `maxAge = 1 年`，通过 `@/lib/cookies` 工具设置/读取

| Cookie 名 | 控制轴 | 取值 |
|-----------|--------|------|
| `vite-ui-theme` | 亮/暗 | `light` / `dark` / `system` |
| `theme_preset` | 预设 | `default` / `anthropic` / `simple-large` / `underground` / `rose-garden` / `lake-view` / `sunset-glow` / `forest-whisper` / `ocean-breeze` / `lavender-dream` |
| `theme_font` | 字体 | `default` / `sans` / `serif` |
| `theme_radius` | 圆角 | `default` / `none` / `sm` / `md` / `lg` / `xl` |
| `theme_scale` | 密度 | `default` / `sm` / `lg` / `xl` |
| `theme_content_layout` | 布局 | `full` / `centered` |

cookie 优势：SSR 友好（虽然当前是 SPA，但为未来迁移留余地）、跨子域共享、不会因 JS 禁用失效。

## 3.3 ThemeProvider：亮/暗/system

文件：`context/theme-provider.tsx`

### 状态

```ts
type Theme = 'dark' | 'light' | 'system'
// 暴露：theme（用户偏好）+ resolvedTheme（已解析 system 后的实际值）
```

### 行为

- 切换方式：`document.documentElement.classList.remove('light','dark')` + `add(nextResolvedTheme)`
- 监听 `matchMedia('(prefers-color-scheme: dark)')` 变化，自动响应系统切换（仅当 `theme === 'system'`）
- hook：`useTheme()` 暴露 `{ theme, resolvedTheme, setTheme, resetTheme, defaultTheme }`

### 浏览器主题色联动

`<meta name="theme-color">` 在 `theme-switch.tsx` 中动态更新：暗色 `#020817` / 亮色 `#fff`。

### VChart 联动

`useChartTheme()`（`lib/use-chart-theme.ts`）懒加载 `@visactor/vchart` 的 `ThemeManager`，按 `resolvedTheme` 调用 `setCurrentTheme('dark' | 'light')`，避免图表白屏闪。

## 3.4 ThemeCustomizationProvider：5 个独立可调轴

文件：`context/theme-customization-provider.tsx`

通过 `document.body.setAttribute('data-theme-preset', ...)` 等 data 属性写到 `<body>`。CSS 在 `theme-presets.css` 中用属性选择器响应。

### 轴清单

```ts
type ThemeCustomization = {
  preset: ThemePreset         // 10 个预设之一
  font: ThemeFont             // 'default' | 'sans' | 'serif'
  radius: ThemeRadius         // 'default' | 'none' | 'sm' | 'md' | 'lg' | 'xl'
  scale: ThemeScale           // 'default' | 'sm' | 'lg' | 'xl'
  contentLayout: ContentLayout // 'full' | 'centered'
}

const DEFAULT_THEME_CUSTOMIZATION = {
  preset: 'default',
  font: 'default',
  radius: 'default',
  scale: 'default',
  contentLayout: 'full',
}
```

来源：`lib/theme-customization.ts:110-124`。

### 字体轴的 resolve 逻辑

用户选 `font: 'default'` 时，根据当前 preset 解析为具体的 `sans` 或 `serif`：

```ts
const PRESET_DEFAULT_FONT: Partial<Record<ThemePreset, ResolvedThemeFont>> = {
  default: 'sans',
  anthropic: 'serif',
}

function resolveThemeFont(font: ThemeFont, preset: ThemePreset): ResolvedThemeFont {
  if (font === 'default') return PRESET_DEFAULT_FONT[preset] ?? 'sans'
  return font
}
```

来源：`lib/theme-customization.ts:176-197`。含义：

- `default` 预设默认 sans（Public Sans）
- `anthropic` 预设默认 serif（Lora）—— 呼应 Anthropic 品牌的 editorial 字体语言
- 其他颜色预设默认 sans，让强调色清晰可读，不与衬线字体竞争

Provider 总是把 `data-theme-font` 设为解析后的具体值（`sans` 或 `serif`），CSS 只需简单属性选择器，无需 `:not()` 或 per-preset 分支。

## 3.5 主题预设（10 套）

注册表：`lib/theme-customization.ts:26-80`，CSS 实现：`theme-presets.css`。

| value | 名称 | 主色（亮 / 暗） | 自带 radius | 字体默认 |
|-------|------|----------------|-------------|----------|
| `default` | Default | 蓝 `oklch(0.692 0.141 243.716)` / `oklch(0.54 0.142 248.516)` | 1rem | sans |
| `anthropic` | Anthropic | clay 珊瑚 `oklch(0.685 0.142 38)` / `oklch(0.72 0.135 40)` ≈ #d97757 | 0.625rem | serif |
| `simple-large` | Simple Large-font | 高对比黑白 `oklch(0.22 0 0)` / `oklch(0.94 0 0)` | 0.5rem | — |
| `underground` | Underground | 绿 `oklch(0.5315 0.0694 156.19)` / `oklch(0.6147 0.0867 154.73)` | 0.5rem | sans |
| `rose-garden` | Rose Garden | 玫瑰红 `oklch(0.5827 0.2418 12.23)` | 1rem | sans |
| `lake-view` | Lake View | 青绿 `oklch(0.765 0.177 163.22)` | 0.75rem | sans |
| `sunset-glow` | Sunset Glow | 橙红 `oklch(0.5591 0.1882 25.33)` | 1rem | sans |
| `forest-whisper` | Forest Whisper | 青 `oklch(0.5276 0.1072 182.22)` | 0.5rem | sans |
| `ocean-breeze` | Ocean Breeze | 蓝紫 `oklch(0.5461 0.2152 262.88)` | 0.3rem | sans |
| `lavender-dream` | Lavender Dream | 薰衣草紫 `oklch(0.5709 0.1808 306.89)` | 1rem | sans |

每个预设注册时带 `swatches`（2 个色样），用于主题切换 UI 的预览。

## 3.6 预设的设计机制：Semantic Surface Bridge

`theme-presets.css` 第 415–472 行的「语义表面桥接」是预设系统的核心：

```css
/* 除 default / anthropic / simple-large 外，所有预设自动派生表面 */
[data-theme-preset='rose-garden'] [data-slot='card'],
[data-theme-preset='rose-garden'] [data-slot='popover'] {
  background: color-mix(in oklch, var(--primary) 8%, var(--background));
  /* ... */
}
```

机制：

- **default**：使用 `:root` 的中性表面（纯白/纯灰），主色只在按钮/链接出现
- **anthropic**：opt-out 桥接，保留其暖奶油色表面（`oklch(0.984 0.004 95)` ≈ #faf9f5），不染珊瑚色
- **simple-large**：opt-out 桥接，保留高对比黑白
- **其他 7 个预设**：自动用 `color-mix(in oklch, var(--primary) N%, var(--background))` 派生 `--card` / `--popover` / `--muted` / `--accent` / `--border` / `--input` / `--sidebar` 等，让整个 UI 染上主色调

**升级建议**：新增预设时，如果想让它「全局染色」，无需手写所有表面 token，只要不放进 opt-out 名单即可。

## 3.7 字体轴 CSS（theme-presets.css:624-629）

```css
[data-theme-font='sans']  { --font-body: var(--font-sans); }
[data-theme-font='serif'] {
  --font-body: var(--font-serif);
  font-feature-settings: 'kern', 'liga', 'calt', 'tnum';
}
[data-theme-font='serif'] h1,
[data-theme-font='serif'] h2,
[data-theme-font='serif'] h3 {
  font-weight: 500;
  letter-spacing: -0.012em;
}
[data-theme-font='serif'] h1 { letter-spacing: -0.02em; }  /* 大标题更紧 */
```

serif 模式启用 OpenType 特性（kerning/连字/等宽数字），标题字重和字距更精致。

## 3.8 圆角轴 CSS（theme-presets.css:682-696）

```css
[data-theme-radius='none'] { --radius: 0rem; }
[data-theme-radius='sm']   { --radius: 0.3rem; }
[data-theme-radius='md']   { --radius: 0.5rem; }
[data-theme-radius='lg']   { --radius: 0.75rem; }
[data-theme-radius='xl']   { --radius: 1rem; }
/* 'default' 不写，沿用预设自带的 --radius */
```

**关键设计**：圆角轴块在 CSS 中**位于预设块之后**，所以用户显式选择会覆盖预设自带的 radius。

## 3.9 密度/字号轴 CSS（theme-presets.css:700-729）

详见 [02-design-tokens.md §2.7](./02-design-tokens.md)。除字号外，还覆盖 `--spacing`，影响 Tailwind 的 `p-*` / `m-*` / `gap-*` 等所有间距工具类。

## 3.10 内容布局轴 CSS（theme-presets.css:736-742）

```css
[data-theme-content-layout='centered'] {
  /* 在 min-width: 1280px 时把 sidebar 内容区限宽居中 */
}
@media (min-width: 1280px) {
  [data-theme-content-layout='centered'] [data-slot='sidebar-inset'] > * {
    max-width: var(--max-content-width, 1280px);
    margin-inline: auto;
  }
}
```

- `full`（默认）：内容撑满侧边栏右侧
- `centered`：宽屏下内容限宽 1280px 居中（适合阅读型页面）

## 3.11 主题切换 UI

| 组件 | 文件 | 说明 |
|------|------|------|
| `ThemeSwitch` | `components/theme-switch.tsx` | 下拉菜单：亮/暗/system + 5 轴定制入口 |
| `ThemeQuickSwitcher` | `components/theme-quick-switcher.tsx` | 快速切换器（键盘快捷键） |

## 3.12 自定义主题的标准流程（升级迭代时）

新增一个主题预设「my-brand」：

1. **注册元数据**：在 `lib/theme-customization.ts` 的 `THEME_PRESETS` 数组追加：
   ```ts
   {
     value: 'my-brand',
     name: 'My Brand',
     swatches: ['oklch(主色亮)', 'oklch(辅色亮)'],
   }
   ```
2. **写 CSS**：在 `theme-presets.css` 追加：
   ```css
   [data-theme-preset='my-brand'] {
     --primary: oklch(...);
     --primary-foreground: oklch(...);
     --ring: oklch(...);
     --chart-1 ~ --chart-5: ...;
     --sidebar-primary: ...;
     --sidebar-accent: ...;
     --sidebar-ring: ...;
     --radius: 0.75rem;  /* 自带默认圆角 */
   }
   .dark [data-theme-preset='my-brand'] { /* 暗色一套 */ }
   ```
3. **决定是否染色表面**：
   - 染色：什么都不做（自动走 Semantic Surface Bridge）
   - 不染色：把 `'my-brand'` 加入 opt-out 名单（`theme-presets.css:415` 附近的选择器组）
4. **决定默认字体**：如要 serif，在 `PRESET_DEFAULT_FONT` 加 `my-brand: 'serif'`
5. **类型自动收窄**：`ThemePreset` 由 `THEME_PRESETS` 推导，无需手改类型

新增预设的检查清单见 [09-upgrade-playbook.md](./09-upgrade-playbook.md)。
