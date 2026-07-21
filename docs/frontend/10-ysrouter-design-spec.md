# 10 · YSRouter 落地页设计规范

> 本文档是 YSRouter 主页/落地页设计的权威规范，作为后续 React 实现与迭代的依据。
>
> 设计稿：`docs/frontend/landing-ysrouter.html`（双主题，浏览器打开可切换）
> 亮色截图：`docs/frontend/landing-ysrouter-light.png`
> 暗色截图：`docs/frontend/landing-ysrouter-dark.png`
> 基础设计哲学：`docs/frontend/design-philosophy-switching-yard.md`

## 10.1 设计语言

**渐变科技感（Gradient Tech）**——参考 Vercel / Stripe / Linear 的现代 SaaS 美学：

- **深色优先**，亮色对偶（默认亮色，可切暗色）
- **aurora 渐变光晕**作为背景情绪层（紫 / 青 / 蓝三色径向晕染）
- **玻璃态卡片**（backdrop-blur + 半透明描边）
- **极淡 64px 网格背景**，径向 mask 让其只在视口顶部显形
- **渐变 accent**（紫→蓝→青 120° 线性渐变）作为唯一强调色，贯穿 logo / 按钮 / 标题强调词 / 价格

与项目设计系统的关系（详见 [03-theming.md](./03-theming.md)）：

- 配色策略遵循项目「单一主色 + OKLCH 派生」的设计哲学
- 可作为新主题预设 `ysrouter` 注册到 `THEME_PRESETS`（详见 §10.7 迁移路径）
- 双主题切换复用项目现有 `ThemeProvider` 的 `class` 策略

## 10.2 Token 系统（完整双主题）

所有 token 以 CSS 变量定义。亮色为默认（`:root`），暗色通过 `:root[data-theme="dark"]` 覆盖。

### 颜色 Token

| Token | 亮色 | 暗色 | 用途 |
|-------|------|------|------|
| `--bg` | `#fafafb` | `#07080c` | 全局背景 |
| `--bg-soft` | `#ffffff` | `#0c0e15` | 次级背景（备选） |
| `--card` | `rgba(10,12,30,.035)` | `rgba(255,255,255,.025)` | 卡片表面（半透明） |
| `--card-hi` | `rgba(10,12,30,.06)` | `rgba(255,255,255,.045)` | 卡片 hover 表面 |
| `--border` | `rgba(10,12,30,.10)` | `rgba(255,255,255,.08)` | 描边 |
| `--border-hi` | `rgba(10,12,30,.18)` | `rgba(255,255,255,.14)` | hover 描边 |
| `--ink` | `#0d1017` | `#f4f5f8` | 主前景文字 |
| `--ink-2` | `#4a5160` | `#a8adbb` | 次要文字 |
| `--ink-3` | `#6b7280` | `#6b7080` | 三级文字（标签/元信息） |
| `--ink-4` | `#9aa1ad` | `#3f4452` | 四级文字（极弱） |

### Accent 三色 + 渐变

| Token | 亮色 | 暗色 | 角色 |
|-------|------|------|------|
| `--accent` | `#6d3eff` | `#7c5cff` | 主强调（紫，亮色加深保对比） |
| `--accent-2` | `#0ea5a4` | `#2dd4bf` | 次强调（青，成功/正向） |
| `--accent-3` | `#2f6fd6` | `#60a5fa` | 三强调（蓝，信息） |
| `--grad` | `120deg #6d3eff → #3b7bff → #0ea5a4` | `120deg #7c5cff → #5b8cff → #2dd4bf` | 唯一强调渐变 |
| `--grad-soft` | 紫青低透明 | 紫青低透明 | 卡片表面渐变背景 |

### 光晕与背景层

| Token | 亮色 | 暗色 | 用途 |
|-------|------|------|------|
| `--glow-1` | `rgba(109,62,255,.18)` | `rgba(124,92,255,.22)` | 紫色径向光晕 |
| `--glow-2` | `rgba(14,165,164,.10)` | `rgba(45,212,191,.14)` | 青色径向光晕 |
| `--glow-3` | `rgba(59,123,255,.08)` | `rgba(91,140,255,.10)` | 蓝色径向光晕 |
| `--grid-line` | `rgba(10,12,30,.045)` | `rgba(255,255,255,.025)` | 64px 网格线 |

**背景构造**（`body` 上）：

```css
background-image:
  radial-gradient(900px 500px at 18% -8%, var(--glow-1), transparent 60%),
  radial-gradient(800px 500px at 92% 4%, var(--glow-2), transparent 60%),
  radial-gradient(700px 700px at 50% 110%, var(--glow-3), transparent 60%);
background-attachment: fixed;
```

三个光晕分别定位在左上、右上、底部中央，构成「三光晕环绕」的视觉场。

### 组件级 Token

| Token | 亮色 | 暗色 | 用途 |
|-------|------|------|------|
| `--nav-bg` | `rgba(255,255,255,.65)` | `rgba(10,12,18,.55)` | nav 玻璃态背景 |
| `--nav-shadow` | `0 1px 0 #fff inset, 0 18px 40px -22px rgba(20,24,60,.28)` | `0 1px 0 rgba(255,255,255,.05) inset, 0 20px 50px -20px rgba(0,0,0,.6)` | nav 阴影 |
| `--term-bg` | `linear-gradient(180deg,#fff,#f6f7fb)` | `linear-gradient(180deg,#0d0f17,#0a0c13)` | 终端窗口背景 |
| `--term-bar-bg` | `rgba(10,12,30,.025)` | `rgba(255,255,255,.02)` | 终端标题栏背景 |
| `--btn-grad-shadow` | `0 10px 30px -8px rgba(109,62,255,.45)` | `0 10px 30px -8px rgba(124,92,255,.55)` | 渐变按钮阴影 |

### 尺寸 Token

| Token | 值 | 用途 |
|-------|----|------|
| `--radius` | `16px` | 基础圆角（卡片/终端） |
| `--maxw` | `1200px` | 内容区最大宽度 |

### 字体 Token

| Token | 字族 | 用途 |
|-------|------|------|
| `--sans` | `'Inter', system-ui, sans-serif` | 主 UI 字体 |
| `--serif` | `'Instrument Serif', Georgia, serif` | 标题强调词（italic） |
| `--mono` | `'JetBrains Mono', ui-monospace, monospace` | 代码、量化数字、标签 |

**字号梯度**（响应式）：

| 用途 | 字号 | 字重 | letter-spacing |
|------|------|------|----------------|
| Hero 主标题 | `clamp(48px, 7vw, 88px)` | 800 | -.045em |
| 段落大标题 | `clamp(32px, 4.4vw, 48px)` | 700 | -.035em |
| 按用量价格数字 | `72px` | 800 | -.05em |
| Stat 数字 | `48px` | 700 | -.04em |
| 正文 | `14-19px` | 400 | -.01em |
| 等宽标签 | `11-13px` | 400 | +.04~.24em |

## 10.3 段落结构

落地页共 9 个段落，垂直堆叠，段落间距 `90px`（`.block` padding）。

| # | 段落 | ID | 高度约 | 关键元素 |
|---|------|----|----|---------|
| 1 | Nav | — | sticky | 药丸栏：品牌 + 导航 + 主题切换 + 双 CTA |
| 2 | Hero | — | ~600px | 徽章 + 双行大标题 + 副标题 + 双 CTA + meta |
| 3 | Logos 信任墙 | — | ~180px | 10 个 chip + 彩色 swatch |
| 4 | Terminal demo | `#demo` | ~600px | macOS 终端 + curl 代码 + 200 OK |
| 5 | Features bento | `#features` | ~700px | 6 卡片（2 wide + 3 normal + 1 wide-3） |
| 6 | Stats | — | ~160px | 4 宫格大数字（50+/100+/50+/10+） |
| 7 | Model wall | `#models` | ~500px | 18 cells（17 provider + dashed +30） |
| 8 | Pricing（按用量） | `#pricing` | ~700px | 主卡 + 账本卡 + 3 特性 |
| 9 | Final CTA | — | ~400px | 径向光晕卡 + 双 CTA |
| 10 | Footer | — | ~300px | 4 列链接 + 版权 |

段落间距统一用 `<section class="block">` 的 `padding: 90px 0`。

## 10.4 组件规范

### Nav（sticky 药丸栏）

```
┌─────────────────────────────────────────────────────────────┐
│ [◈] YSRouter   Features  Models  Pricing  Docs   [🌙][Sign in][Get started →] │
└─────────────────────────────────────────────────────────────┘
```

- 容器：`max-width: 1200px`，距顶 `14px`，`border-radius: 100px`
- 背景：`var(--nav-bg)` + `backdrop-filter: blur(18px) saturate(140%)`
- 描边 + 阴影：`1px solid var(--border)` + `var(--nav-shadow)`
- 内边距：`10px 14px 10px 22px`
- 元素：品牌（logo + 文字）/ nav-links / 主题切换 / Sign in / Get started

**主题切换按钮**：36×36px 图标按钮，亮色显示月亮，暗色显示太阳（CSS 驱动），点击循环 light → dark → system。

### Hero

```
                    ┌─────────────────────┐
                    │ ● v3.0 · streaming  │  ← pill 徽章
                    └─────────────────────┘
              One unified gateway,
              every AI model.              ← 大标题，第二行渐变 italic
                                               
        YSRouter aggregates 50+ upstream...   ← 副标题
                                               
           [Start routing →] [Read the docs]  ← 双 CTA
                                               
          // replace your base URL · zero...  ← mono meta
```

- 居中对齐
- 徽章 pill：`5px 14px 5px 10px`，`border-radius: 100px`，含 6×6px 状态点（青色发光）
- 标题强调词用 `<em class="grad-text">`（italic + 渐变文字）
- meta 行：等宽字体，`--ink-4` 色

### Terminal Demo

- macOS 风窗口：交通灯（红 #ff5f57 / 橙 #febc2e / 绿 #28c840）
- 标题栏：`~/app — bash` + 三 tab（curl / python / node）
- 代码语法色：
  - 注释 `# before...` → `var(--ink-4)`
  - prompt `$` → `var(--accent-2)`（青）
  - flag `-H -d` → `var(--accent)`（紫）
  - string `"..."` → `var(--accent)` opacity .85
  - 输出 `· 247 tokens...` → `var(--ink-2)`
  - 成功 `→ 200 OK` → `var(--accent-2)`（青）

### Features Bento

6 列网格，卡片 span 灵活：

| 卡片 | span | 内容 |
|------|------|------|
| 01 Lightning routing | `span 3`（wide） | 大特性卡 |
| 02 Secure by default | `span 3`（wide） | 大特性卡 |
| 03 Global coverage | `span 2` | 小卡 |
| 04 Developer-first | `span 2` | 小卡 |
| 05 Transparent billing | `span 2` | 小卡 |
| 06 Ten gateway primitives | `span 6`（wide-3） | 含 10 个 chip |

- 卡片：`var(--card)` 背景，`1px solid var(--border)`，`border-radius: 16px`
- hover：`translateY(-2px)` + 背景变 `var(--card-hi)` + 描边变 `var(--border-hi)` + `::before` 显出 `var(--grad-soft)`
- 卡片图标：38×38px 渐变方块，`box-shadow: var(--glow-1)`
- 序号 tag：右上角等宽字体，`--ink-4`
- 第 6 卡的 10 chips：蓝/紫/青三色循环边框（`color-mix(in srgb, var(--accent-N) 28%, transparent)`）

### Stats Strip

4 宫格，等分，分隔线竖向：

```
┌──────────┬──────────┬──────────┬──────────┐
│   50+    │   100+   │   50+    │   10+    │
│ upstream │  model   │compatible│scheduling│
│ services │ billing  │  routes  │ controls │
└──────────┴──────────┴──────────┴──────────┘
```

- 数字：`48px`，`--grad` 渐变文字，font-weight 700
- 标签：等宽字体 `12px`，`--ink-3`

### Model Wall

6 列网格，18 个等比方形 cell（aspect-ratio: 1）：

- 每个 cell：`var(--card)` 背景，中心一个 italic serif 渐变字母（provider 首字母）+ 下方等宽 provider 名
- 右上角：6×6px 青色状态点（`box-shadow: 0 0 8px var(--accent-2)` 发光）
- 最后一个 cell：`+30` 渐变大字 + "more integrated"，`border-style: dashed`
- hover：`translateY(-2px)` + 背景变 `var(--card-hi)`

### Pricing（按用量）

双列布局（主卡 1.35 : 账本卡 1）：

**主卡**（左，渐变光晕）：
- 标签 `PAY · AS · YOU · GO`（紫色等宽，letter-spacing .24em）
- 价格 `$0.0021 / 1K tokens`（72px 渐变数字）
- 说明（含 `<strong>no markup</strong>`）
- 双 CTA：渐变 `Top up & start routing` + `See model prices`
- `::before` 径向紫色光晕装饰

**账本卡**（右，等宽字体）：
- 标题 `sample usage · this month` + 青色状态灯
- 4 行模型用量（grid 1.4 : 0.8 : 0.6）
- 分隔线
- `balance after` + 渐变 `$80.10`（22px）
- 页脚 `no markup · no seat fees · no commitment`

**下方 3 特性卡**：等宽三列，含青色 checkmark 图标。

### Final CTA

径向光晕卡（`::before` 居中紫色 600px 径向渐变）：

```
            Ready to simplify your
              AI integration?          ← 大标题，第二行 italic 渐变
                                               
     Deploy your own gateway...        ← 副文
                                               
       [Get started →] [See pricing]   ← 双 CTA
```

### Footer

4 列（2 : 1 : 1 : 1）+ 底部版权行：

- 第 1 列：品牌 + 一句话描述
- 第 2-4 列：PRODUCT / RESOURCES / COMPANY 链接
- 底栏：`© 2023–2026 QuantumNous · AGPL-3.0 · self-hostable` + `build 2k6e8r7p · all systems clear`

## 10.5 按钮 Token

| 类 | 用途 | 样式 |
|----|------|------|
| `.btn` | 次要按钮 | `var(--card)` 背景 + `1px solid var(--border)` + `var(--ink)` 文字 |
| `.btn:hover` | hover | 背景 `var(--card-hi)`，描边 `var(--border-hi)` |
| `.btn-grad` | 主要 CTA | `var(--grad)` 背景 + 白字 + `var(--btn-grad-shadow)` |
| `.btn-grad:hover` | hover | `filter: brightness(1.08)` |
| `.btn-primary` | 备选主按钮 | `var(--ink)` 背景 + `var(--bg)` 文字 |

通用：`font-size: 13.5px`，`padding: 8px 16px`，`border-radius: 10px`，`transition: .15s`，inline-flex 居中带 7px gap。

## 10.6 主题切换实现

**HTML 单文件版**（当前 `landing-ysrouter.html`）：

```js
// 三态循环：light → dark → system → light
// localStorage 持久化 + 跟随 prefers-color-scheme
// 支持 ?theme=light|dark URL 参数
```

**切换按钮**：

```html
<button class="btn theme-toggle" id="theme-toggle">
  <svg class="icon-moon">...</svg>   <!-- 亮色下显示 -->
  <svg class="icon-sun">...</svg>     <!-- 暗色下显示 -->
</button>
```

```css
.theme-toggle .icon-sun { display: none }
.theme-toggle .icon-moon { display: block }
:root[data-theme="dark"] .theme-toggle .icon-sun { display: block }
:root[data-theme="dark"] .theme-toggle .icon-moon { display: none }
```

**平滑过渡**：

```css
html, body {
  transition: background-color .35s ease, color .35s ease;
}
```

## 10.7 迁移到 React 的路径

项目已有完整双 Provider 主题架构（见 [03-theming.md](./03-theming.md)），迁移步骤：

### Step 1：注册新主题预设

在 `web/default/src/lib/theme-customization.ts` 的 `THEME_PRESETS` 追加：

```ts
{
  value: 'ysrouter',
  name: 'YSRouter',
  swatches: ['oklch(0.5 0.25 280)', 'oklch(0.7 0.15 200)'],
}
```

### Step 2：写 CSS（`theme-presets.css`）

```css
[data-theme-preset='ysrouter'] {
  --primary: oklch(0.5 0.25 280);           /* 紫 */
  --primary-foreground: oklch(1 0 0);
  --ring: oklch(0.5 0.25 280);
  --chart-1: oklch(0.5 0.25 280);           /* 紫 */
  --chart-2: oklch(0.7 0.15 200);           /* 青 */
  --chart-3: oklch(0.55 0.2 250);           /* 蓝 */
  --chart-4: oklch(0.65 0.18 325);          /* 品红 */
  --chart-5: oklch(0.7 0.15 155);           /* 绿 */
  --sidebar-primary: oklch(0.5 0.25 280);
  --sidebar-accent: color-mix(in oklch, var(--primary) 12%, var(--background));
  --sidebar-ring: oklch(0.5 0.25 280);
  --radius: 1rem;
}
.dark [data-theme-preset='ysrouter'] { /* 暗色一套 */ }
```

### Step 3：落地页组件结构

对应到 `features/home/components/sections/`：

```
features/home/
├── index.tsx                      # 组合所有 section
└── components/sections/
    ├── hero.tsx                   # ← 对应 HTML Hero
    ├── logos.tsx                  # ← 新增（信任墙）
    ├── terminal-demo.tsx          # ← 新增（代码 demo）
    ├── features.tsx               # ← 对应 HTML Features bento
    ├── stats.tsx                  # ← 已有，改样式
    ├── model-wall.tsx             # ← 新增
    ├── pricing.tsx                # ← 改为按用量计费版
    └── cta.tsx                    # ← 已有，改样式
```

### Step 4：复用现有主题切换

无需新增切换逻辑——项目 `ThemeProvider` 的 `useTheme()` hook 已支持 light/dark/system 三态 + cookie 持久化。落地页直接消费即可。

### Step 5：字体替换

| HTML 设计稿 | 项目对应 |
|------------|----------|
| Inter | Public Sans（`--font-sans`） |
| Instrument Serif | Lora Italic（`--font-serif` italic） |
| JetBrains Mono | 项目默认 mono |

## 10.8 设计决策记录

| 决策 | 选择 | 理由 |
|------|------|------|
| 默认主题 | 亮色 | 截图/分享更友好；用户可切暗色 |
| 强调色 | 紫青蓝渐变（非项目默认蓝） | 与项目 `default` 预设区分，作为 YSRouter 品牌识别 |
| 亮色 accent 加深 | `#6d3eff`（非 `#7c5cff`） | 亮底下紫色的对比度要求更高 |
| 玻璃态 nav | sticky + backdrop-blur | 滚动时仍可见品牌 + CTA，且不遮挡内容 |
| 三光晕背景 | fixed 定位 | 滚动时光晕静止，营造空间深度 |
| 终端代码色 | 基于 accent 变量 | 主题切换时自动跟随，无需维护两套语法色 |
| 按用量计费（非套餐） | 主卡 + 账本卡 | 呼应「no markup」透明定价主张 |
| Model wall 字母 | italic serif 渐变 | 避免使用第三方品牌 logo（许可问题），用字母抽象表达 |

## 10.9 检查清单（迭代时）

新增/修改段落时确认：

- [ ] 用 CSS 变量（不硬编码颜色），确保双主题都正确
- [ ] 亮色下文字对比度 ≥ WCAG AA（4.5:1）
- [ ] 暗色下文字对比度 ≥ WCAG AA
- [ ] 渐变 accent 在两个主题下都视觉鲜明
- [ ] 玻璃态背景在两个主题下都可读（亮色半透白 / 暗色半透黑）
- [ ] 段落间距统一 90px
- [ ] 内容区不超 `--maxw: 1200px`
- [ ] hover 微交互 `transition: .15-.25s`
- [ ] 主题切换按钮图标正确切换（moon/sun）
- [ ] 截图验证两个主题
