# 成本核算单价展示实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在使用日志的供应商成本核算详情中，按冻结成本规则展示供应商原币和标准化美元的按次、按秒或每 1M Token 单价。

**Architecture:** 新增成本核算领域的纯函数模块，负责解析尝试账本中的 `rule_config_json`、选择与 `cost_mode`/`token_mode` 对应的冻结单价字段，并生成稳定的展示行。`CostRequestDetail` 只负责翻译标签和渲染，不重新计算供应商成本，也不修改后端账本接口。

**Tech Stack:** React 19、TypeScript、i18next、Bun test、happy-dom、Tailwind CSS。

---

## 文件结构

- 新建 `web/src/features/cost-accounting/lib/unit-price.ts`：解析冻结规则并格式化双币种单位价格。
- 新建 `web/src/features/cost-accounting/lib/__tests__/unit-price.test.ts`：覆盖全部成本模式、Token 子模式和无效冻结规则。
- 修改 `web/src/features/cost-accounting/components/cost-request-detail.tsx`：在每次尝试的详情网格中渲染单位价格行。
- 修改 `web/src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx`：保护管理员可见的最终展示行为。
- 修改 `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`：增加五个单位价格标签。

### Task 1：冻结规则单价解析

**Files:**
- Create: `web/src/features/cost-accounting/lib/unit-price.ts`
- Test: `web/src/features/cost-accounting/lib/__tests__/unit-price.test.ts`

- [ ] **Step 1: 编写按次和按秒的失败测试**

```ts
import assert from 'node:assert/strict'
import test from 'node:test'

import type { CostAccountingAttemptLedger } from '../../types'
import {
  formatFrozenUnitPrice,
  getFrozenUnitPriceRows,
} from '../unit-price'

function attempt(
  costMode: CostAccountingAttemptLedger['cost_mode'],
  config: Record<string, unknown>
): CostAccountingAttemptLedger {
  return {
    id: 1,
    cost_request_id: 1,
    attempt_no: 1,
    channel_id: 1,
    channel_name: 'supplier',
    channel_type: 1,
    predicted_upstream_model: 'predicted',
    billable_upstream_model: 'billable',
    rule_id: 1,
    rule_version: 1,
    cost_mode: costMode,
    schema_version: 1,
    rule_config_json: JSON.stringify(config),
    charge_event: 'response_succeeded',
    meter_source: 'upstream_usage',
    billable_request_count: 1,
    request_meter_json: '{}',
    actual_meter_json: '{}',
    original_cost: '0',
    upstream_accepted: true,
    http_status: 200,
    result_code: '',
    failure_code: '',
    status: 'settled',
    reconciliation_status: 'none',
    prepared_at: 1,
    created_at: 1,
    updated_at: 1,
  }
}

test('returns the frozen original and USD price for a per-request rule', () => {
  const rows = getFrozenUnitPriceRows(
    attempt('per_request', {
      currency: 'CNY',
      unit_price: '2.9',
      normalized_usd_prices: { unit_price: '0.397260274' },
    })
  )

  assert.deepEqual(rows, [
    {
      labelKey: 'Unit price per request',
      unitKey: 'Per request',
      currency: 'CNY',
      nativePrice: '2.9',
      normalizedUSDPrice: '0.397260274',
    },
  ])
  assert.equal(
    formatFrozenUnitPrice(rows[0], 'Unavailable', 'Per request'),
    '¥2.9 / Per request · $0.397260274 / Per request'
  )
})

test('returns the frozen original and USD price for a duration rule', () => {
  const rows = getFrozenUnitPriceRows(
    attempt('per_duration', {
      currency: 'CNY',
      price_per_second: '0.5',
      normalized_usd_prices: { price_per_second: '0.068493151' },
    })
  )

  assert.equal(rows[0]?.labelKey, 'Unit price per second')
  assert.equal(rows[0]?.unitKey, 'Per second')
  assert.equal(
    formatFrozenUnitPrice(rows[0], 'Unavailable', 'Per second'),
    '¥0.5 / Per second · $0.068493151 / Per second'
  )
})
```

- [ ] **Step 2: 运行测试并确认 RED**

Run: `cd web && bun test src/features/cost-accounting/lib/__tests__/unit-price.test.ts`

Expected: FAIL，提示无法找到 `../unit-price`。

- [ ] **Step 3: 编写 Token 模式和降级行为的失败测试**

```ts
test('selects the total-token price per million', () => {
  const rows = getFrozenUnitPriceRows(
    attempt('per_token', {
      currency: 'CNY',
      token_mode: 'total_tokens',
      total_per_million: '2.9',
      normalized_usd_prices: { total_per_million: '0.397260274' },
    })
  )
  assert.equal(rows[0]?.labelKey, 'Price per 1M tokens')
  assert.equal(rows[0]?.nativePrice, '2.9')
})

test('selects the completion-token price per million', () => {
  const rows = getFrozenUnitPriceRows(
    attempt('per_token', {
      currency: 'USD',
      token_mode: 'completion_tokens',
      completion_per_million: '4',
      normalized_usd_prices: { completion_per_million: '4' },
    })
  )
  assert.equal(rows[0]?.labelKey, 'Output price per 1M tokens')
  assert.equal(rows[0]?.nativePrice, '4')
})

test('returns separate input and output prices for a split-token rule', () => {
  const rows = getFrozenUnitPriceRows(
    attempt('per_token', {
      currency: 'CNY',
      token_mode: 'input_output',
      input_per_million: '7.3',
      output_per_million: '14.6',
      normalized_usd_prices: {
        input_per_million: '1',
        output_per_million: '2',
      },
    })
  )
  assert.deepEqual(
    rows.map((row) => row.labelKey),
    ['Input price per 1M tokens', 'Output price per 1M tokens']
  )
})

test('does not guess prices from malformed or free frozen rules', () => {
  const malformed = attempt('per_token', {})
  malformed.rule_config_json = '{'

  const rows = getFrozenUnitPriceRows(malformed)
  assert.equal(rows.length, 1)
  assert.equal(rows[0]?.labelKey, 'Price per 1M tokens')
  assert.equal(
    formatFrozenUnitPrice(rows[0], 'Unavailable', 'Per 1M tokens'),
    'Unavailable'
  )
  assert.deepEqual(getFrozenUnitPriceRows(attempt('free', {})), [])
})
```

- [ ] **Step 4: 实现最小冻结规则解析模块**

```ts
import type {
  CostAccountingAttemptLedger,
  CostRuleConfigV1,
  CostRulePricesV1,
} from '../types'

const CANONICAL_DECIMAL = /^(?:0|[1-9]\d*)(?:\.\d*[1-9])?$/

export type FrozenUnitPriceRow = {
  labelKey:
    | 'Unit price per request'
    | 'Unit price per second'
    | 'Price per 1M tokens'
    | 'Input price per 1M tokens'
    | 'Output price per 1M tokens'
  unitKey: 'Per request' | 'Per second' | 'Per 1M tokens'
  currency?: string
  nativePrice?: string
  normalizedUSDPrice?: string
}

function decimal(value: unknown): string | undefined {
  return typeof value === 'string' && CANONICAL_DECIMAL.test(value)
    ? value
    : undefined
}

function config(value: string): Partial<CostRuleConfigV1> {
  try {
    const parsed = JSON.parse(value) as unknown
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? (parsed as Partial<CostRuleConfigV1>)
      : {}
  } catch {
    return {}
  }
}

function row(
  labelKey: FrozenUnitPriceRow['labelKey'],
  unitKey: FrozenUnitPriceRow['unitKey'],
  rule: Partial<CostRuleConfigV1>,
  field: keyof CostRulePricesV1
): FrozenUnitPriceRow {
  return {
    labelKey,
    unitKey,
    currency:
      typeof rule.currency === 'string' ? rule.currency.toUpperCase() : undefined,
    nativePrice: decimal(rule[field]),
    normalizedUSDPrice: decimal(rule.normalized_usd_prices?.[field]),
  }
}

export function getFrozenUnitPriceRows(
  attempt: CostAccountingAttemptLedger
): FrozenUnitPriceRow[] {
  if (attempt.cost_mode === 'free') return []
  const rule = config(attempt.rule_config_json)
  if (attempt.cost_mode === 'per_request') {
    return [row('Unit price per request', 'Per request', rule, 'unit_price')]
  }
  if (attempt.cost_mode === 'per_duration') {
    return [
      row(
        'Unit price per second',
        'Per second',
        rule,
        'price_per_second'
      ),
    ]
  }
  if (rule.token_mode === 'completion_tokens') {
    return [
      row(
        'Output price per 1M tokens',
        'Per 1M tokens',
        rule,
        'completion_per_million'
      ),
    ]
  }
  if (rule.token_mode === 'input_output') {
    return [
      row(
        'Input price per 1M tokens',
        'Per 1M tokens',
        rule,
        'input_per_million'
      ),
      row(
        'Output price per 1M tokens',
        'Per 1M tokens',
        rule,
        'output_per_million'
      ),
    ]
  }
  return [
    row(
      'Price per 1M tokens',
      'Per 1M tokens',
      rule,
      'total_per_million'
    ),
  ]
}

function currencyPrefix(currency: string | undefined): string | undefined {
  if (!currency) return undefined
  try {
    const symbol = new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency,
      currencyDisplay: 'narrowSymbol',
    })
      .formatToParts(0)
      .find((part) => part.type === 'currency')?.value
    return symbol === currency ? `${currency} ` : symbol
  } catch {
    return `${currency} `
  }
}

export function formatFrozenUnitPrice(
  row: FrozenUnitPriceRow | undefined,
  unavailable: string,
  unit: string
): string {
  if (!row) return unavailable
  const prefix = currencyPrefix(row.currency)
  const native =
    prefix && row.nativePrice ? `${prefix}${row.nativePrice}` : unavailable
  const usd = row.normalizedUSDPrice
    ? `$${row.normalizedUSDPrice}`
    : unavailable
  if (native === unavailable && usd === unavailable) return unavailable
  return `${native} / ${unit} · ${usd} / ${unit}`
}
```

- [ ] **Step 5: 运行单元测试并确认 GREEN**

Run: `cd web && bun test src/features/cost-accounting/lib/__tests__/unit-price.test.ts`

Expected: 所有新增测试 PASS，0 fail。

- [ ] **Step 6: 提交解析模块**

```bash
git add web/src/features/cost-accounting/lib/unit-price.ts web/src/features/cost-accounting/lib/__tests__/unit-price.test.ts
git commit -m "feat: parse frozen supplier unit prices"
```

### Task 2：在成本尝试详情中展示单价

**Files:**
- Modify: `web/src/features/cost-accounting/components/cost-request-detail.tsx`
- Modify: `web/src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: 编写组件失败测试**

将测试夹具中的冻结规则补充原币价格：

```ts
rule_config_json: JSON.stringify({
  currency: 'CNY',
  token_mode: 'input_output',
  input_per_million: '7.3',
  output_per_million: '14.6',
  normalized_usd_prices: {
    input_per_million: '1',
    output_per_million: '2',
  },
}),
```

新增用户可见行为测试：

```ts
test('renders frozen supplier prices in original currency and normalized USD', async () => {
  const queryClient = createQueryClient()
  queryClient.setQueryData(costAccountingQueryKeys.request(101), {
    success: true,
    message: '',
    data: requestDetail,
  })

  const mounted = await mount(
    <CostRequestDetail requestID={101} isAdmin open />,
    queryClient
  )
  try {
    const text = browserWindow.document.body.textContent ?? ''
    assert.match(text, /Input price per 1M tokens/)
    assert.match(text, /¥7.3 \/ Per 1M tokens · \$1 \/ Per 1M tokens/)
    assert.match(text, /Output price per 1M tokens/)
    assert.match(text, /¥14.6 \/ Per 1M tokens · \$2 \/ Per 1M tokens/)
  } finally {
    await unmount(mounted)
  }
})
```

- [ ] **Step 2: 运行组件测试并确认 RED**

Run: `cd web && bun test src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx`

Expected: FAIL，因为页面尚未渲染单位价格标签和值。

- [ ] **Step 3: 在尝试时间线中渲染单价行**

在 `cost-request-detail.tsx` 中导入：

```ts
import {
  formatFrozenUnitPrice,
  getFrozenUnitPriceRows,
} from '../lib/unit-price'
```

在 `AttemptTimelineItem` 内计算冻结单价行：

```ts
const unitPriceRows = getFrozenUnitPriceRows(attempt)
```

在“计费上游模型”和“冻结规则”等既有字段所在的 `<dl>` 中加入：

```tsx
{unitPriceRows.map((row) => (
  <DetailValue
    key={row.labelKey}
    label={t(row.labelKey)}
    value={formatFrozenUnitPrice(
      row,
      t('Unavailable'),
      t(row.unitKey)
    )}
    mono
  />
))}
```

- [ ] **Step 4: 增加全部语言的标签翻译**

添加以下键值：

| Key | en | zh | zh-TW | fr | ja | ru | vi |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `Unit price per request` | Unit price per request | 每次单价 | 每次單價 | Prix unitaire par requête | リクエスト単価 | Цена за запрос | Đơn giá mỗi yêu cầu |
| `Unit price per second` | Unit price per second | 每秒单价 | 每秒單價 | Prix unitaire par seconde | 1秒あたりの単価 | Цена за секунду | Đơn giá mỗi giây |
| `Price per 1M tokens` | Price per 1M tokens | 每 1M Token 单价 | 每 1M Token 單價 | Prix par 1 M de tokens | 100万トークン単価 | Цена за 1 млн токенов | Giá mỗi 1 triệu token |
| `Input price per 1M tokens` | Input price per 1M tokens | 每 1M 输入 Token 单价 | 每 1M 輸入 Token 單價 | Prix d'entrée par 1 M de tokens | 入力100万トークン単価 | Цена входных 1 млн токенов | Giá đầu vào mỗi 1 triệu token |
| `Output price per 1M tokens` | Output price per 1M tokens | 每 1M 输出 Token 单价 | 每 1M 輸出 Token 單價 | Prix de sortie par 1 M de tokens | 出力100万トークン単価 | Цена выходных 1 млн токенов | Giá đầu ra mỗi 1 triệu token |

- [ ] **Step 5: 运行组件测试并确认 GREEN**

Run: `cd web && bun test src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx`

Expected: 所有测试 PASS，0 fail。

- [ ] **Step 6: 运行成本核算相关测试**

Run: `cd web && bun test src/features/cost-accounting`

Expected: 所有成本核算测试 PASS，0 fail。

- [ ] **Step 7: 提交界面和翻译改动**

```bash
git add web/src/features/cost-accounting/components/cost-request-detail.tsx web/src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx web/src/i18n/locales
git commit -m "feat: show supplier unit prices in cost details"
```

### Task 3：回归与页面验收

**Files:**
- Verify: `web/src/features/cost-accounting/lib/unit-price.ts`
- Verify: `web/src/features/cost-accounting/components/cost-request-detail.tsx`
- Verify: `web/src/i18n/locales/*.json`

- [ ] **Step 1: 对涉及文件执行格式检查**

Run:

```bash
cd web
bunx oxfmt --check src/features/cost-accounting/lib/unit-price.ts src/features/cost-accounting/lib/__tests__/unit-price.test.ts src/features/cost-accounting/components/cost-request-detail.tsx src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx src/i18n/locales/*.json
```

Expected: 退出码 0；若失败，执行相同路径的 `bunx oxfmt --write` 后重新检查。

- [ ] **Step 2: 对涉及代码执行 lint**

Run:

```bash
cd web
bunx oxlint -c .oxlintrc.json src/features/cost-accounting/lib/unit-price.ts src/features/cost-accounting/lib/__tests__/unit-price.test.ts src/features/cost-accounting/components/cost-request-detail.tsx src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx
```

Expected: 0 errors。

- [ ] **Step 3: 运行前端类型检查和生产构建**

Run: `cd web && bun run typecheck`

Expected: 退出码 0。

Run: `cd web && bun run build`

Expected: Rsbuild 完成并退出码为 0。

- [ ] **Step 4: 检查 Git 差异质量**

Run: `git diff --check`

Expected: 无输出，退出码 0。

- [ ] **Step 5: 在本地控制台执行真实展示验收**

打开 `http://127.0.0.1:3000/usage-logs/common?type=["2"]`，进入包含供应商成本核算的消费日志详情，确认：

- `per_token` 规则展示对应 Token 模式的每 1M Token 原币和 USD 单价。
- `per_duration` 规则展示每秒原币和 USD 单价。
- `per_request` 规则展示每次原币和 USD 单价。
- 展示值来自冻结规则，与成本规则列表一致，且不会根据最终总金额反推。
- 弹窗在桌面和窄屏视口中没有文本重叠或横向撑破。

- [ ] **Step 6: 提交验收中产生的必要修正**

仅当格式、lint、构建或页面验收产生修正时执行：

```bash
git add web/src/features/cost-accounting web/src/i18n/locales
git commit -m "fix: finalize supplier unit price display"
```
