# 成本核算模式悬停说明实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为成本核算页面右上角的三个模式按钮增加支持鼠标悬停和键盘聚焦的多语言说明，同时保持现有模式切换与严格模式门禁行为。

**Architecture:** 将模式按钮组从页面容器提取为专属组件，由该组件负责模式枚举、Tooltip 和可访问交互，页面继续负责权限、覆盖校验、Toast 与设置更新。Tooltip 复用现有 Base UI 封装；翻译通过项目规定的脚本一次性写入七个语言文件。

**Tech Stack:** React 19、TypeScript、Base UI Tooltip、现有 ToggleGroup、react-i18next、Bun、node:test、happy-dom

---

## 文件结构

- 新建 `web/src/features/cost-accounting/components/cost-accounting-mode-toggle.tsx`：渲染三个模式按钮及其 Tooltip，并把合法模式变化回传给页面。
- 新建 `web/src/features/cost-accounting/components/__tests__/cost-accounting-mode-toggle.test.tsx`：保护悬停、聚焦、禁用和选择行为。
- 修改 `web/src/features/cost-accounting/index.tsx`：用新组件替换内联 `ToggleGroup`，保留现有 mutation 和严格模式覆盖校验。
- 临时新建并删除 `web/scripts/add-missing-keys.mjs`：按规范写入七个 locale 文件。
- 修改 `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`：新增三个模式说明键。

### Task 1: 用交互测试定义模式 Tooltip 契约

**Files:**
- Create: `web/src/features/cost-accounting/components/__tests__/cost-accounting-mode-toggle.test.tsx`
- Create: `web/src/features/cost-accounting/components/cost-accounting-mode-toggle.tsx`

- [ ] **Step 1: 编写失败的组件交互测试**

测试使用 `happy-dom` 建立浏览器全局对象，用 `I18nextProvider` 提供英文键值，并挂载以下受控组件：

```tsx
<CostAccountingModeToggle
  mode='tracking'
  canEnableStrict={false}
  disabled={false}
  onChange={(mode) => changes.push(mode)}
/>
```

写入四个明确行为：

```tsx
test('shows the matching description when an enabled mode receives focus', async () => {
  const tracking = findButton('Tracking')
  await act(async () => tracking.focus())
  assert.match(
    document.body.textContent ?? '',
    /Records revenue, provider cost, profit, and anomalies/
  )
})

test('shows the strict description on hover even when strict mode is disabled', async () => {
  const strict = findButton('Strict')
  assert.equal(strict.disabled, true)
  await hover(strict)
  assert.match(
    document.body.textContent ?? '',
    /Requires complete cost coverage/
  )
})

test('does not select strict mode while it is disabled', async () => {
  findButton('Strict').click()
  assert.deepEqual(changes, [])
})

test('reports an enabled mode selection', async () => {
  await act(async () => findButton('Disabled').click())
  assert.deepEqual(changes, ['disabled'])
})
```

`hover` 同步派发鼠标指针进入和 `mouseover` 事件，并等待 React `act` 完成；断言只观察可见 Tooltip 文案、原生禁用状态和对外回调，不检查组件内部状态或完整 class 字符串。

- [ ] **Step 2: 运行测试并确认因组件尚不存在而失败**

Run:

```powershell
cd web
bun test --parallel=1 src/features/cost-accounting/components/__tests__/cost-accounting-mode-toggle.test.tsx
```

Expected: FAIL，错误指出无法解析 `../cost-accounting-mode-toggle`。

- [ ] **Step 3: 实现最小可用模式 Tooltip 组件**

组件使用以下接口和模式定义：

```tsx
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import { isCostAccountingMode } from '../lib/mode'
import type { CostAccountingMode } from '../types'

type CostAccountingModeToggleProps = {
  mode: CostAccountingMode
  canEnableStrict: boolean
  disabled: boolean
  onChange: (mode: CostAccountingMode) => void
}

const modes: ReadonlyArray<{
  value: CostAccountingMode
  label: string
  description: string
}> = [
  {
    value: 'disabled',
    label: 'Disabled',
    description:
      'Turns off provider cost accounting and profit guardrails. Existing user billing continues.',
  },
  {
    value: 'tracking',
    label: 'Tracking',
    description:
      'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.',
  },
  {
    value: 'strict',
    label: 'Strict',
    description:
      'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.',
  },
]
```

渲染结构如下；`TooltipTrigger` 直接合成 `ToggleGroupItem`，避免额外包装破坏分段按钮的首尾圆角和边框。`Strict` 的原生按钮保持禁用，但 Tooltip trigger 本身不传 `disabled`，因此仍处理鼠标悬停：

```tsx
export function CostAccountingModeToggle(
  props: CostAccountingModeToggleProps
) {
  const { t } = useTranslation()

  return (
    <TooltipProvider delay={0}>
      <ToggleGroup
        value={[props.mode]}
        onValueChange={(selection) => {
          const nextMode = selection[0]
          if (isCostAccountingMode(nextMode)) props.onChange(nextMode)
        }}
        disabled={props.disabled}
        aria-label={t('Cost accounting mode')}
      >
        {modes.map((option) => (
          <Tooltip key={option.value}>
            <TooltipTrigger
              render={
                <ToggleGroupItem
                  value={option.value}
                  disabled={
                    option.value === 'strict' && !props.canEnableStrict
                  }
                >
                  {t(option.label)}
                </ToggleGroupItem>
              }
            />
            <TooltipContent
              side='bottom'
              className='max-w-72 items-start text-pretty'
            >
              {t(option.description)}
            </TooltipContent>
          </Tooltip>
        ))}
      </ToggleGroup>
    </TooltipProvider>
  )
}
```

- [ ] **Step 4: 运行组件测试并确认通过**

Run:

```powershell
cd web
bun test --parallel=1 src/features/cost-accounting/components/__tests__/cost-accounting-mode-toggle.test.tsx
```

Expected: 4 tests PASS。

- [ ] **Step 5: 提交独立组件和回归测试**

```powershell
git add -- web/src/features/cost-accounting/components/cost-accounting-mode-toggle.tsx web/src/features/cost-accounting/components/__tests__/cost-accounting-mode-toggle.test.tsx
git commit -m "feat: add cost accounting mode tooltips"
```

### Task 2: 接入页面并补齐七种语言

**Files:**
- Modify: `web/src/features/cost-accounting/index.tsx`
- Create then delete: `web/scripts/add-missing-keys.mjs`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: 运行 i18n 同步预检并记录基线报告**

Run:

```powershell
cd web
bun run i18n:sync
```

Expected: 命令成功，并生成或更新 `src/i18n/locales/_reports/_sync-report.json`；新增三个键前不得出现由本次改动造成的缺失项。

- [ ] **Step 2: 用专属组件替换页面内联按钮组**

从 `index.tsx` 删除 `ToggleGroup`、`ToggleGroupItem` 和 `isCostAccountingMode` 导入，增加：

```tsx
import { CostAccountingModeToggle } from './components/cost-accounting-mode-toggle'
```

用以下代码替换原内联 `ToggleGroup`：

```tsx
<CostAccountingModeToggle
  mode={mode}
  canEnableStrict={canEnableStrict}
  disabled={modeMutation.isPending || settingsQuery.isLoading}
  onChange={(value) => {
    if (value === 'strict' && !canEnableStrict) {
      toast.error(t('Resolve uncovered models before enabling strict mode'))
      return
    }
    modeMutation.mutate({
      mode: value,
      minimum_expected_margin_bps: minimumExpectedMarginBPS,
    })
  }}
/>
```

保留父组件的严格模式校验作为竞态保护，不修改请求参数、Toast 或 query invalidation。

- [ ] **Step 3: 通过脚本写入三个说明键的七语言翻译**

创建符合 `i18n-translate` 规范的 `web/scripts/add-missing-keys.mjs`，使用该技能规定的 `stableStringify`、locale 遍历和排序逻辑，并填入：

```javascript
const newKeys = {
  en: {
    'Turns off provider cost accounting and profit guardrails. Existing user billing continues.':
      'Turns off provider cost accounting and profit guardrails. Existing user billing continues.',
    'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.':
      'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.',
    'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.':
      'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.',
  },
  zh: {
    'Turns off provider cost accounting and profit guardrails. Existing user billing continues.':
      '关闭供应商成本核算和利润门禁，现有用户计费继续运行。',
    'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.':
      '记录收入、供应商成本、利润和异常，但不拦截成本缺失或低毛利路由。',
    'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.':
      '记录成本，并拦截成本未知、规则缺失或预计毛利低于最低要求的路由；启用前需要完整成本覆盖。',
  },
  'zh-TW': {
    'Turns off provider cost accounting and profit guardrails. Existing user billing continues.':
      '關閉供應商成本核算和利潤門檻，現有使用者計費繼續運作。',
    'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.':
      '記錄收入、供應商成本、利潤和異常，但不攔截成本缺失或低毛利路由。',
    'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.':
      '記錄成本，並攔截成本未知、規則缺失或預期毛利低於最低要求的路由；啟用前需要完整成本覆蓋。',
  },
  fr: {
    'Turns off provider cost accounting and profit guardrails. Existing user billing continues.':
      'Désactive le suivi des coûts fournisseur et les seuils de rentabilité. La facturation utilisateur existante continue.',
    'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.':
      'Enregistre les revenus, les coûts fournisseur, le bénéfice et les anomalies sans bloquer les routes sans coût ou à faible marge.',
    'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.':
      'Enregistre les coûts et bloque les routes au coût inconnu, sans règle ou dont la marge attendue est sous le minimum. Nécessite une couverture complète des coûts.',
  },
  ja: {
    'Turns off provider cost accounting and profit guardrails. Existing user billing continues.':
      'プロバイダー原価計算と利益ガードを無効にします。既存のユーザー課金は継続します。',
    'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.':
      '収益、プロバイダー原価、利益、異常を記録しますが、原価未設定または低利益率のルートはブロックしません。',
    'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.':
      '原価を記録し、原価不明、ルール未設定、または想定利益率が最低値を下回るルートをブロックします。有効化には完全な原価カバレッジが必要です。',
  },
  ru: {
    'Turns off provider cost accounting and profit guardrails. Existing user billing continues.':
      'Отключает учёт затрат поставщика и контроль прибыльности. Действующий биллинг пользователей продолжает работать.',
    'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.':
      'Учитывает выручку, затраты поставщика, прибыль и аномалии, не блокируя маршруты без данных о затратах или с низкой маржой.',
    'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.':
      'Учитывает затраты и блокирует маршруты с неизвестной стоимостью, отсутствующими правилами или ожидаемой маржой ниже минимума. Требуется полное покрытие затрат.',
  },
  vi: {
    'Turns off provider cost accounting and profit guardrails. Existing user billing continues.':
      'Tắt hạch toán chi phí nhà cung cấp và các rào chắn lợi nhuận. Việc tính phí người dùng hiện tại vẫn tiếp tục.',
    'Records revenue, provider cost, profit, and anomalies without blocking missing-cost or low-margin routes.':
      'Ghi nhận doanh thu, chi phí nhà cung cấp, lợi nhuận và bất thường nhưng không chặn tuyến thiếu chi phí hoặc có biên lợi nhuận thấp.',
    'Records cost and blocks routes with unknown cost, missing rules, or expected margin below the minimum. Requires complete cost coverage.':
      'Ghi nhận chi phí và chặn tuyến có chi phí chưa xác định, thiếu quy tắc hoặc biên lợi nhuận dự kiến dưới mức tối thiểu. Yêu cầu bao phủ chi phí đầy đủ.',
  },
}
```

执行脚本后，用 `apply_patch` 删除临时脚本：

```powershell
cd web
node scripts/add-missing-keys.mjs
bun run i18n:sync
```

Expected: 每个 locale 应用 3 个翻译；同步报告不包含这三个缺失键。

- [ ] **Step 4: 验证代码调用和七语言键完整性**

Run:

```powershell
cd web
rg -n -F "Turns off provider cost accounting" src/features src/i18n/locales
rg -n -F "Records revenue, provider cost" src/features src/i18n/locales
rg -n -F "Records cost and blocks routes" src/features src/i18n/locales
```

Expected: 每个键在组件中出现 1 次，并在 7 个 locale 文件中各出现 1 次。

- [ ] **Step 5: 提交页面接入和翻译**

```powershell
git add -- web/src/features/cost-accounting/index.tsx web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat: explain cost accounting modes"
```

### Task 3: 完整验证与页面视觉检查

**Files:**
- Verify: `web/src/features/cost-accounting/components/cost-accounting-mode-toggle.tsx`
- Verify: `web/src/features/cost-accounting/index.tsx`
- Verify: `web/src/i18n/locales/*.json`

- [ ] **Step 1: 运行受影响测试、类型检查和 lint**

Run:

```powershell
cd web
bun test --parallel=1 src/features/cost-accounting/components/__tests__/cost-accounting-mode-toggle.test.tsx src/features/cost-accounting/components/__tests__/profit-report.test.tsx
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/cost-accounting/index.tsx src/features/cost-accounting/components/cost-accounting-mode-toggle.tsx src/features/cost-accounting/components/__tests__/cost-accounting-mode-toggle.test.tsx
```

Expected: 所有测试 PASS；TypeScript 无错误；涉及文件无 lint error。

- [ ] **Step 2: 运行格式、i18n 和生产构建检查**

Run:

```powershell
cd web
bun run format:check
bun run i18n:sync
bun run build
```

Expected: 三条命令均以退出码 0 完成，i18n 报告中三个新增键在七个 locale 均完整。

- [ ] **Step 3: 在真实页面验证桌面和窄屏交互**

使用当前本地应用打开 `/cost-accounting`，分别在桌面和移动宽度执行：

1. 悬停 `Disabled`、`Tracking`、`Strict`，确认显示各自说明且 Tooltip 位于按钮下方。
2. 用键盘聚焦可用按钮，确认说明出现且焦点样式可见。
3. 覆盖不完整时确认 `Strict` 仍为禁用状态，同时鼠标悬停能够看到说明。
4. 检查 Tooltip 文案自然换行，不遮挡三个模式按钮、最低毛利输入框或页面标题。
5. 检查浅色与深色主题的文字对比度和边界位置。

- [ ] **Step 4: 检查提交和工作区状态**

Run:

```powershell
git diff --check
git status --short
git log -3 --oneline
```

Expected: `git diff --check` 无输出，工作区干净，日志包含设计文档提交和本计划中的功能提交。
