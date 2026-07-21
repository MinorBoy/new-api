# 05 · 字体、排版与图标

## 5.1 字体系统

### 字族定义（theme.css:22-41）

| Token | 字族链 | 用途 |
|-------|--------|------|
| `--font-sans` | `'Public Sans', sans-serif` | 主 UI 字体（默认 body） |
| `--font-serif` | `'Lora Variable', 'Lora', 'Source Serif Pro', 'Source Serif 4', + 完整 CJK serif fallback 链 + Georgia, 'Times New Roman', serif | editorial 衬线（Anthropic 预设 + serif 字体轴） |
| `--font-inter` / `--font-manrope` | 系统 UI 栈 | 预留，当前未启用 |
| `--font-body` | `var(--font-sans)` 或 `var(--font-serif)` | body 实际使用，由字体轴切换 |

### 完整 CJK Serif Fallback 链

```
'Lora Variable', 'Lora',
'Source Serif Pro', 'Source Serif 4',
'Noto Serif SC', 'Noto Serif TC', 'Noto Serif JP', 'Noto Serif KR',
'Source Han Serif SC', 'Source Han Serif TC', 'Source Han Serif',
'Songti SC', 'STSong', 'STSongti-SC-Regular', 'PingFang SC', 'SimSun',
'NSimSun', '宋体', 'FangSong', '仿宋', 'KaiTi', '楷体',
Georgia, 'Times New Roman', Cambria, 'Liberation Serif', serif
```

**为什么这么长？** 注释（theme.css:23-28）解释：Lora 只含拉丁字形，浏览器在 Windows 上的 generic-serif 选择在小字号下不确定，常出现类 sans 渲染。显式列出 CJK 字体确保中日韩文字稳定显示。

### 字体加载方式

**全部本地字体**（`@fontsource-variable`，npm 安装），**无 Google Fonts CDN**：

- `@fontsource-variable/public-sans` (^5.2.7) — 主 UI sans
- `@fontsource-variable/lora` (^5.2.8) — editorial serif

加载（index.css:22-28）：

```css
@import '@fontsource-variable/public-sans';
@import '@fontsource-variable/lora';
```

`index.html` 中**没有任何** `<link>` Google Fonts。`<meta name="theme-color" content="#fff" />` 是初始值，运行时由 `theme-switch.tsx` 动态更新（暗色 `#020817` / 亮色 `#fff`）。

### 选择本地字体的理由

- **离线可用**：内网部署、私有化场景不需要外网
- **性能**：无 DNS 查询、无第三方 cookie
- **可控**：版本钉死在 package.json，字体更新走依赖管理流程
- **隐私**：不向 Google 泄露用户访问信息

## 5.2 字体轴切换效果（serif 模式）

来源：theme-presets.css:624-629

`[data-theme-font='serif']` 启用 OpenType 特性：

```css
[data-theme-font='serif'] {
  --font-body: var(--font-serif);
  font-feature-settings: 'kern', 'liga', 'calt', 'tnum';
  /* kern: 字距微调；liga: 标准连字；calt: 上下文替代；tnum: 等宽数字 */
}

[data-theme-font='serif'] h1,
[data-theme-font='serif'] h2,
[data-theme-font='serif'] h3 {
  font-weight: 500;            /* 标题不用 bold，更优雅 */
  letter-spacing: -0.012em;    /* 字距收紧 */
}

[data-theme-font='serif'] h1 {
  letter-spacing: -0.02em;     /* 大标题更紧 */
}
```

设计意图：serif 模式不是为了「换个字体」，而是模拟 editorial / 出版物的整体排版语言（紧凑字距、中等字重、等宽数字对齐表格）。

## 5.3 排版工具类

### Tailwind v4 默认字号表（除非密度轴覆盖）

| 工具类 | 字号 | 用途 |
|--------|------|------|
| `text-xs` | 0.75rem (12px) | 辅助文字、tag |
| `text-sm` | 0.875rem (14px) | 次要文字、表单提示 |
| `text-base` | 1rem (16px) | body 默认 |
| `text-lg` | 1.125rem (18px) | 强调正文 |
| `text-xl` ~ `text-3xl` | 1.25 ~ 1.875rem | 标题 |
| `text-4xl` ~ `text-7xl` | 更大 | Hero 大标题 |

密度轴（`data-theme-scale`）会覆盖这张表，详见 [02-design-tokens.md §2.7](./02-design-tokens.md)。

### 行高、字重

- 默认行高跟随 Tailwind（`leading-normal` = 1.5）
- 字重：`font-normal` (400) / `font-medium` (500) / `font-semibold` (600) / `font-bold` (700)
- Variable 字体（Public Sans Variable、Lora Variable）支持任意字重值，但项目内只用上述 4 档

### 文本截断

```tsx
<span className='truncate'>超长文本</span>            {/* 单行截断 */}
<p className='line-clamp-2'>多行描述</p>              {/* 2 行截断 */}
```

## 5.4 代码字体

代码块用 Tailwind 默认 mono 字族（`font-mono`），由 preflight 提供：

```
ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, 'Liberation Mono', monospace
```

未自定义 mono token。Shiki 代码高亮通过 `--shiki-light` / `--shiki-dark` 双主题切换（见 [02-design-tokens.md §2.13](./02-design-tokens.md)）。

## 5.5 图标系统

**多套并存，各司其职**：

| 库 | 用途 | 使用文件数 | 调用方式 |
|----|------|-----------|----------|
| **`lucide-react`** | 主力 UI 图标（按钮、表单、卡片、dashboard） | **268** | `<ArrowRight className='size-4' />` |
| **`@hugeicons/react`** + `@hugeicons/core-free-icons` | shadcn 注册的默认图标库（ui/ 原语：dialog 关闭、sonner toast、sidebar 折叠） | 21 | `<HugeiconsIcon icon={XxxIcon} />` |
| **`@lobehub/icons`** | LLM 提供商/模型品牌 logo（渠道、模型选择器） | 11 | 通过 `lib/lobe-icon.tsx` 统一封装 |
| **`react-icons`** | 少量补充 | 5 | 直接导入 |

### 选用决策树

```
是否是 LLM 提供商/模型品牌？
├── 是 → @lobehub/icons（通过 lib/lobe-icon.tsx）
└── 否 → 是否是 shadcn ui/ 原语内部使用？
        ├── 是 → @hugeicons/react（保持 shadcn 注册一致性）
        └── 否 → lucide-react（主力，覆盖 99% 业务场景）
```

### 使用约定（AGENTS.md §3.12）

- 装饰性图标加 `aria-hidden`（避免屏幕阅读器重复朗读）
- 重要信息必须配文本，不能只用图标表达
- 图标尺寸用 `size-*` 工具类（`size-4` = 16px，`size-5` = 20px），不用 `w-4 h-4`
- Lucide 直接使用：`<ArrowRight className='size-4' />`
- Hugeicons 统一封装：`<HugeiconsIcon icon={Cancel01Icon} />`

### `lib/lobe-icon.tsx`

品牌图标的统一封装，处理：

- LLM 提供商映射（OpenAI / Anthropic / Google / ...）
- 颜色变体（mono / color）
- 尺寸规范化
- fallback（未知 provider 显示通用图标）

## 5.6 字体升级建议

如果未来要新增字族（如 `--font-mono` 自定义、或新增中文 sans）：

1. 在 `package.json` 加 `@fontsource-variable/<font-name>`
2. 在 `index.css` 加 `@import '@fontsource-variable/<font-name>'`
3. 在 `theme.css` 的 `@theme inline` 块加 token：
   ```css
   --font-<name>: '<Font Name Variable>', <fallback>;
   ```
4. 如要加入字体轴，扩展 `ThemeFont` 类型和 `theme-presets.css` 的 `[data-theme-font='...']` 块
5. 更新 `lib/theme-customization.ts` 的 `THEME_FONT_VALUES`

### 字体升级检查清单

- [ ] 字体含所需的所有字形（拉丁、CJK、西里尔等，依支持语言）
- [ ] variable font（支持字重轴）或明确字重档
- [ ] license 兼容（OFL / Apache / MIT）
- [ ] 已通过 `@fontsource-variable` 安装，不依赖 CDN
- [ ] Fallback 链合理（避免字体未加载时布局跳动）
