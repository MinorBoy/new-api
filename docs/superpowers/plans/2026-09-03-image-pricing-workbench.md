# 图像定价工作台 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将图像定价页面改造成可直接编辑 SKU 用户售价的表格式工作台，同时保留高级 JSON、路由预览和兼容性验收能力。

**Architecture:** 继续以 `ImageModelCatalog` 和 `ImageRoutingPolicy` 系统选项为唯一事实来源。后端提供不含密钥的 active 成本摘要，前端用纯函数展平/回写目录、用定点字符串计算毛利，并由一个工作台组件负责编辑、dirty 状态和保存；旧 JSON 编辑器及路由工具放在折叠高级设置中。

**Tech Stack:** Go 1.22、Gin、GORM、shopspring/decimal、React 19、TypeScript、React Query、Zod、Base UI、Bun/Vitest。

---

## 文件边界

- Create: `dto/image_pricing.go`，管理员成本摘要响应 DTO。
- Create: `service/image_pricing.go`，从 active 成本规则和渠道模型映射构造 SKU 成本摘要。
- Create: `service/image_pricing_test.go`，成本匹配、最低成本和未知成本回归测试。
- Create: `controller/image_pricing.go`，管理员只读接口处理器。
- Create: `controller/image_pricing_test.go`，接口响应、鉴权错误映射和敏感字段边界测试。
- Modify: `router/cost-accounting-router.go`，注册只读成本摘要路由。
- Modify: `router/cost_accounting_router_test.go`，断言该路由使用 `CostAccountingRead`。
- Modify: `web/src/features/system-settings/models/image-settings-card.tsx`，改为工作台容器并保留高级设置。
- Create: `web/src/features/system-settings/models/image-pricing-catalog.ts`，目录行模型、展平、价格校验、毛利计算和回写纯函数。
- Create: `web/src/features/system-settings/models/image-pricing-api.ts`，前端成本摘要请求和响应类型。
- Create: `web/src/features/system-settings/models/image-pricing-workbench.tsx`，表格编辑、保存、刷新、dirty 状态和高级设置布局。
- Create: `web/src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts`，纯函数行为测试。
- Create: `web/src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx`，用户视角交互测试。
- Modify: `web/src/i18n/static-keys.ts` 及各语言 JSON，登记新增界面文案。
- Create: `web/e2e/image-pricing-workbench.pw.ts`，登录后的桌面/移动浏览器验收流程。

## Task 1: 新增只读图像成本摘要接口

**Files:**
- Create: `dto/image_pricing.go`
- Create: `service/image_pricing.go`
- Test: `service/image_pricing_test.go`
- Create: `controller/image_pricing.go`
- Test: `controller/image_pricing_test.go`
- Modify: `router/cost-accounting-router.go`
- Modify: `router/cost_accounting_router_test.go`

- [ ] **Step 1: Write service tests for SKU matching and aggregation**

在 `service/image_pricing_test.go` 建立最小 SQLite fixture，使用现有 `model.Channel`、`model.ChannelModelCostRule`、`model.Ability` 表。测试数据包含：

```go
func TestImagePricingCostSummaryUsesMappedActiveRules(t *testing.T) {
	prepareImagePricingDB(t)
	seedImagePricingChannel(t, 41, `{"gpt-image-2":"vendor-cheap"}`, true)
	seedImagePricingChannel(t, 42, `{"gpt-image-2":"vendor-expensive"}`, true)
	seedActiveImageRule(t, 41, "vendor-cheap", "gen-1024x1024-medium", "0.008")
	seedActiveImageRule(t, 42, "vendor-expensive", "gen-1024x1024-medium", "0.012")

	summary, err := service.ListImagePricingCostSummary([]string{"gpt-image-2"})
	require.NoError(t, err)
	item := requireImagePricingItem(t, summary, "gpt-image-2", "gen-1024x1024-medium")
	assert.Equal(t, "0.008", item.MinimumCostUSD)
	assert.Equal(t, 2, item.SourceCount)
	assert.True(t, item.Known)
}

func TestImagePricingCostSummaryIgnoresDraftRetiredAndUnmappedRules(t *testing.T) {
	prepareImagePricingDB(t)
	seedImagePricingChannel(t, 51, `{"other-model":"vendor-model"}`, true)
	seedImagePricingChannel(t, 52, `{"gpt-image-2":"vendor-draft"}`, true)
	seedImagePricingChannel(t, 53, `{"gpt-image-2":"vendor-disabled"}`, false)
	seedDraftImageRule(t, 52, "vendor-draft", "gen-1024x1024-medium", "0.001")
	seedActiveImageRule(t, 53, "vendor-disabled", "gen-1024x1024-medium", "0.002")

	summary, err := service.ListImagePricingCostSummary([]string{"gpt-image-2"})
	require.NoError(t, err)
	item := requireImagePricingItem(t, summary, "gpt-image-2", "gen-1024x1024-medium")
	assert.False(t, item.Known)
	assert.Equal(t, 0, item.SourceCount)
}
```

测试 helper 必须通过 `require` 初始化数据库和 fixture，通过 `assert` 检查值；不得把密钥写入响应或 fixture 断言。

- [ ] **Step 2: Run the failing service tests**

Run: `go test ./service -run ImagePricingCostSummary -count=1`

Expected: FAIL because `ListImagePricingCostSummary` and its DTO do not exist。

- [ ] **Step 3: Define the response DTO and service contract**

在 `dto/image_pricing.go` 定义：

```go
type ImagePricingCostItem struct {
	Model           string `json:"model"`
	SKU             string `json:"sku"`
	Known           bool   `json:"known"`
	MinimumCostUSD  string `json:"minimum_cost_usd,omitempty"`
	SourceCount     int    `json:"source_count"`
}

type ImagePricingCostSummary struct {
	Items []ImagePricingCostItem `json:"items"`
}
```

在 `service/image_pricing.go` 定义 `func ListImagePricingCostSummary(models []string) (dto.ImagePricingCostSummary, error)`。实现要求：

1. 去除空模型、去重并稳定排序；空输入返回空 `items`。
2. 从 `image_setting.Snapshot()` 读取每个公共模型的 SKU key，不能信任客户端任意 SKU。
3. 只读取启用渠道及其安全字段（id、status、models、model_mapping、other_settings），不得调用会返回 `Key` 的全量选择。
4. 解析每个渠道的 `model_mapping`，将公共模型映射到 billable upstream model；无映射时仅当渠道 `Models` 明确包含公共模型才使用同名模型。
5. 用 `model.WalkCostCatalogRows` 获取 active 规则，候选键为 channel、映射后模型、SKU；只接受 `free`、`per_request`、`per_image`。用 `service.CalculateAttemptCost` 和 `types.CostMeter{ImageCount: 1}` 计算一个图片的 normalized USD 成本，金额用 `decimal` 比较。
6. 同一公共模型/SKU 保留最低已知成本，`SourceCount` 统计所有已知 active 规则来源；无候选时 `Known=false` 且不填成本。
7. 对成本规则配置解析失败、未知 meter 或负数计算结果跳过该来源并继续，不能将未知成本当作 0；数据库错误原样返回给 controller 包装。

为避免暴露敏感信息，DTO 不包含 channel id、channel name、billable model、rule id、config 或任何密钥。成本摘要是供应商成本规则配置值，不宣称为真实上游账单。

- [ ] **Step 4: Run service tests and cross-database model tests**

Run: `go test ./service -run ImagePricingCostSummary -count=1`

Expected: PASS，最低成本为 `0.008`、来源数为 `2`，draft/retired/disabled/unmapped 来源不参与聚合。

Run: `go test ./model -run CostCatalogQueryConfiguredDatabases -count=1`

Expected: PASS；新增查询不使用数据库专属 SQL。

- [ ] **Step 5: Add controller and route tests before implementation**

在 `controller/image_pricing_test.go` 覆盖：

```go
func TestGetImagePricingCostSummaryReturnsStableContract(t *testing.T) {
	prepareImagePricingControllerDB(t)
	ctx, recorder := imagePricingTestContext(http.MethodGet, "/api/cost-accounting/image-pricing?model=gpt-image-2")
	controller.GetImagePricingCostSummary(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "secret")
	assert.Contains(t, recorder.Body.String(), `"items"`)
}
```

给过长模型查询、服务错误和空模型查询分别增加确定性断言。更新 `router/cost_accounting_router_test.go`，要求 `GET /image-pricing` 的 permission 为 `authz.CostAccountingRead`，handler 为 `controller.GetImagePricingCostSummary`。

- [ ] **Step 6: Implement handler and route**

在 `controller/image_pricing.go`：

1. 从重复的 `model` 查询参数读取公共模型；限制单个模型长度 191、总数量 100，空查询使用当前目录的全部模型。
2. 调用 `service.ListImagePricingCostSummary`，失败时使用现有 `writeCostAccountingError`，不把内部 SQL 暴露给客户端。
3. 成功时用 `common.ApiSuccess(c, summary)` 返回 `items` 数组，即使没有条目也返回空数组。

在 `router/cost-accounting-router.go` 添加：

```go
{method: http.MethodGet, path: "/image-pricing", permission: authz.CostAccountingRead, handler: controller.GetImagePricingCostSummary},
```

不得新增数据库迁移或写接口。

- [ ] **Step 7: Run backend tests and commit the bounded backend change**

Run: `go test ./controller ./router ./service -run 'ImagePricing|CostAccountingRouter' -count=1`

Expected: PASS。

Run: `git diff --check`

Expected: no output。

Commit: `git add dto/image_pricing.go service/image_pricing.go service/image_pricing_test.go controller/image_pricing.go controller/image_pricing_test.go router/cost-accounting-router.go router/cost_accounting_router_test.go; git commit -m "feat: expose image pricing cost summary"`

## Task 2: 建立目录展平、价格校验和毛利纯函数

**Files:**
- Create: `web/src/features/system-settings/models/image-pricing-catalog.ts`
- Test: `web/src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts`

- [ ] **Step 1: Write pure-function tests**

测试固定目录 fixture，保护以下用户可见契约：

```ts
test('flattens and sorts every configured SKU', () => {
  const rows = flattenImagePricingCatalog(catalogFixture)
  expect(rows.map((row) => row.id)).toEqual([
    'gpt-image-2|gen-1024x1024-medium',
    'gpt-image-2|gen-4096x4096-medium',
  ])
})

test('updates only sale prices and preserves catalog capabilities', () => {
  const updated = updateCatalogSalePrices(catalogFixture, {
    'gpt-image-2|gen-1024x1024-medium': '0.035',
  })
  expect(updated.models['gpt-image-2'].skus['gen-1024x1024-medium'].sale_price_usd).toBe('0.035')
  expect(updated.models['gpt-image-2'].endpoints).toEqual(catalogFixture.models['gpt-image-2'].endpoints)
})

test('rejects invalid sale prices and calculates safe margins', () => {
  expect(validateSalePrice('')).toBe('Price is required')
  expect(validateSalePrice('-0.01')).toBe('Price must be non-negative')
  expect(validateSalePrice('0.1234567891')).toBe('Price has too many decimal places')
  expect(calculateExpectedMarginBps('0.03', '0.02')).toBe(3333)
  expect(calculateExpectedMarginBps('0', '0.02')).toBeNull()
})
```

测试文件只能测试该模块的导出行为，不复制生产实现来计算 expected。

- [ ] **Step 2: Run the failing frontend tests**

Run: `bun test src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts`

Expected: FAIL because the catalog module and exported functions do not exist。

- [ ] **Step 3: Implement typed catalog adapter**

在 `image-pricing-catalog.ts` 定义并导出：

```ts
export type ImagePricingRow = {
  id: string
  model: string
  endpoint: string
  size: string
  quality: string
  skuKey: string
  salePriceUSD: string
  upstreamCostUSD?: string
  sourceCount: number
  expectedMarginBps: number | null
  enabled: boolean
}

export function flattenImagePricingCatalog(raw: string | ImageCatalog): ImagePricingRow[]
export function updateCatalogSalePrices(raw: string | ImageCatalog, prices: Record<string, string>): ImageCatalog
export function validateSalePrice(value: string): string | null
export function calculateExpectedMarginBps(salePriceUSD: string, costUSD?: string): number | null
export function formatImagePrice(value: string): string
```

实现要求：

- 解析失败返回显式错误结果给调用方，禁止把损坏目录静默变成空目录。
- 只遍历已定义端点和 SKU，按模型名、端点、SKU key 稳定排序。
- `id` 固定为 `${model}|${skuKey}`，回写只修改 `sale_price_usd`。
- 售价允许非负十进制定点字符串，限制 8 位小数；不接受空值、指数、NaN、Infinity 或负数。
- 毛利使用 `decimal.js`，售价为 0、成本缺失、成本非法时返回 `null`；结果以 basis points 截断到整数，不能出现 NaN/Infinity。
- `enabled` 由端点 capability 的 `enabled !== false` 和 SKU 存在共同决定；本工作台不写入启用状态。

- [ ] **Step 4: Run pure-function tests, formatter and lint**

Run: `bun test src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts`

Expected: PASS。

Run: `bunx oxfmt --check src/features/system-settings/models/image-pricing-catalog.ts src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts`

Expected: PASS。

Commit: `git add web/src/features/system-settings/models/image-pricing-catalog.ts web/src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts; git commit -m "feat: add image pricing catalog helpers"`

## Task 3: 实现前端工作台和高级设置布局

**Files:**
- Create: `web/src/features/system-settings/models/image-pricing-api.ts`
- Create: `web/src/features/system-settings/models/image-pricing-workbench.tsx`
- Modify: `web/src/features/system-settings/models/image-settings-card.tsx`
- Test: `web/src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx`

- [ ] **Step 1: Write component interaction tests**

使用项目现有 `happy-dom`/React 测试配置和用户可见查询，mock 仅外部 API 边界：

```tsx
test('editing a sale price marks the row dirty and enables save', async () => {
  render(<ImagePricingWorkbench catalog={catalogJSON} routing={routingJSON} />)
  const price = screen.getByRole('textbox', { name: 'gpt-image-2 gen-1024x1024-medium sale price' })
  await userEvent.clear(price)
  await userEvent.type(price, '0.035')
  expect(screen.getByText('1 unsaved change')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
})

test('failed save keeps the edited value and dirty state', async () => {
  mockUpdateOption.mockRejectedValueOnce(new Error('permission denied'))
  render(<ImagePricingWorkbench catalog={catalogJSON} routing={routingJSON} />)
  await editSalePrice('gpt-image-2 gen-1024x1024-medium sale price', '0.035')
  await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(screen.getByRole('textbox', { name: 'gpt-image-2 gen-1024x1024-medium sale price' })).toHaveValue('0.035')
  expect(screen.getByText('1 unsaved change')).toBeVisible()
})

test('advanced settings are collapsed and can run routing preview', async () => {
  render(<ImagePricingWorkbench catalog={catalogJSON} routing={routingJSON} />)
  expect(screen.queryByLabelText('Global image model catalog JSON')).not.toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
  expect(screen.getByLabelText('Global image model catalog JSON')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Preview image routing' }))
  expect(mockPreviewImageRoute).toHaveBeenCalledTimes(1)
})
```

补充窄屏契约：渲染表格容器时断言存在水平滚动策略，价格输入具有稳定宽度和 `aria-invalid`，不会依赖完整 Tailwind class 快照。

- [ ] **Step 2: Run the failing component tests**

Run: `bun test src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx`

Expected: FAIL because the workbench component does not exist。

- [ ] **Step 3: Add cost summary API client and query types**

在 `image-pricing-api.ts` 定义：

```ts
export type ImagePricingCostItem = {
  model: string
  sku: string
  known: boolean
  minimum_cost_usd?: string
  source_count: number
}

export async function getImagePricingCostSummary(models: string[]): Promise<{
  success: boolean
  message: string
  data: { items: ImagePricingCostItem[] }
}> {
  const response = await api.get('/api/cost-accounting/image-pricing', {
    params: { model: models },
  })
  return response.data
}
```

请求失败时由工作台显示成本暂不可用，不阻止售价保存；不把响应写入系统选项。

- [ ] **Step 4: Implement the workbench shell**

在 `image-pricing-workbench.tsx`：

1. 读取目录并调用 `flattenImagePricingCatalog`；解析失败时显示页面级错误和高级 JSON 入口，不初始化空目录。
2. 用 `useQuery` 获取成本摘要，按 `model|skuKey` 建索引，填充只读上游成本、来源数和预计毛利率。
3. 用局部 `Record<string, string>` 保存编辑售价；点击售价单元格进入输入，`Enter` 提交单元格，`Escape` 恢复原值；dirty 行显示状态标记。
4. 点击“保存更改”时先校验全部编辑值，再调用现有 `useUpdateOption` 更新 `ImageModelCatalog`。成功后 invalidate `system-options`、重新获取成本摘要、清除 dirty；失败保留输入和 dirty 状态。
5. 使用 `FormNavigationGuard` 防止带 dirty 状态离开，保存期间禁用重复提交和输入。
6. 工具栏显示总 SKU、启用数量、未保存数量、刷新按钮和保存按钮；刷新仅在无 dirty 时接受，带 dirty 时先使用现有确认对话框。
7. 主表使用语义化 `<table>` 和可滚动容器，列为模型、端点、尺寸、质量、上游成本、用户售价、预计毛利率、启用状态；售价输入用 `Input`、正确 label、`aria-invalid` 和字段错误。

- [ ] **Step 5: Move existing JSON/routing tools into advanced settings**

将 `image-settings-card.tsx` 中现有 JSON 表单、`parseImageRoutingPolicy`、预览选择器和 `/api/routing-policies/image/preview` 调用拆入 `ImagePricingWorkbench` 的 `<Accordion>` 高级面板：

- “目录 JSON”面板保留 `JsonCodeEditor`，允许新增能力/SKU；导入只改变本地 dirty 状态，必须保存。
- “路由与验收”面板保留策略 JSON、分组/模型/端点/尺寸/质量选择、候选渠道和排除原因。
- 表格保存只更新 `ImageModelCatalog`；高级设置保存同时更新两个系统选项，并保持现有顺序和错误提示。
- 不改变 `ImageSettingsCard` 对外 props，`section-registry.tsx` 无需变更。

如果组件超过约 200 行，把目录 JSON 面板抽为 `image-pricing-advanced-settings.tsx`，但不要复制保存逻辑。

- [ ] **Step 6: Run component tests, typecheck and lint**

Run: `bun test src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx`

Expected: PASS。

Run: `bun run typecheck`

Expected: exit code 0。

Run: `bunx oxlint -c .oxlintrc.json src/features/system-settings/models/image-pricing-api.ts src/features/system-settings/models/image-pricing-catalog.ts src/features/system-settings/models/image-pricing-workbench.tsx src/features/system-settings/models/image-settings-card.tsx`

Expected: no lint errors。

Commit: `git add web/src/features/system-settings/models/image-pricing-api.ts web/src/features/system-settings/models/image-pricing-catalog.ts web/src/features/system-settings/models/image-pricing-workbench.tsx web/src/features/system-settings/models/image-settings-card.tsx web/src/features/system-settings/models/__tests__; git commit -m "feat: add inline image pricing workbench"`

## Task 4: 完成 i18n、格式检查和浏览器验收

**Files:**
- Modify: `web/src/i18n/static-keys.ts`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`
- Create: `web/e2e/image-pricing-workbench.pw.ts`

- [ ] **Step 1: Register all user-facing strings**

登记并翻译：`SKU count`、`Enabled SKUs`、`Unsaved changes`、`Save changes`、`Refresh`、`Model`、`Endpoint`、`Size`、`Quality`、`Upstream cost`、`Sale price`、`Expected margin`、`Enabled`、`Not configured`、`Unavailable`、`Advanced settings`、`Catalog JSON`、`Routing & acceptance`、`Price is required`、`Price must be non-negative`、`Price has too many decimal places`、`Unable to load image pricing costs`。组件所有展示文本必须通过 `t()` 使用。

- [ ] **Step 2: Run i18n sync and formatting checks**

Run from `web/`: `bun run i18n:sync`

Expected: locale files contain every new key without overwriting unrelated translations。

Run from `web/`: `bun run format:check`

Expected: PASS。

- [ ] **Step 3: Add Playwright acceptance flow**

在 `web/e2e/image-pricing-workbench.pw.ts` 使用现有登录 fixture 或环境变量登录管理员，测试：

1. 打开 `/system-settings/billing/image-pricing`，等待 `gpt-image-2`、1K 和 4K 行出现。
2. 修改 1K 售价为测试值，断言未保存计数增加且保存按钮可用。
3. 保存后重新加载页面，断言新售价仍存在；成本/毛利列有值或明确显示未配置。
4. 展开高级设置并执行一次路由预览，断言候选/选中渠道区域出现。
5. 使用 390px 视口，断言表格容器可横向滚动、售价输入和按钮文字不重叠。

测试不能提交真实生产价格：使用专用测试目录 fixture 或在 `E2E_IMAGE_PRICING_TEST` 开关下保存后立即恢复原 JSON；若没有测试环境变量则跳过写入步骤并报告前置条件。

- [ ] **Step 4: Run final frontend checks and browser tests**

Run from `web/`: `bun test src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx`

Expected: PASS。

Run from `web/`: `bun run typecheck`

Expected: exit code 0。

Run from `web/`: `bun run lint`

Expected: no lint errors。

Run from `web/`: `bun run build`

Expected: production build succeeds。

Run from `web/`: `bunx playwright test e2e/image-pricing-workbench.pw.ts --config=playwright.config-import.config.ts`

Expected: desktop and mobile projects pass；没有测试凭据时必须明确记录 skipped，而不是伪造成功。

- [ ] **Step 5: Inspect diff and commit documentation/test artifacts**

Run: `git diff --check; git status --short`

Expected: no whitespace errors；仅包含本计划列出的实现、测试和翻译文件。

Commit: `git add web/src/i18n web/e2e/image-pricing-workbench.pw.ts; git commit -m "test: verify image pricing workbench"`

## Verification Matrix

完成实现后必须按以下顺序记录最新输出：

| 范围 | 命令 | 通过标准 |
| --- | --- | --- |
| 目录与价格纯函数 | `bun test src/features/system-settings/models/__tests__/image-pricing-catalog.test.ts` | 展平、定点校验、毛利和回写全部通过 |
| 表格交互 | `bun test src/features/system-settings/models/__tests__/image-pricing-workbench.test.tsx` | 编辑、dirty、保存失败保留、高级折叠全部通过 |
| 后端摘要 | `go test ./controller ./router ./service -run 'ImagePricing|CostAccountingRouter' -count=1` | API、成本匹配和权限路由通过 |
| 类型与 lint | `bun run typecheck`、`bun run lint` | 无错误 |
| 生产构建 | `bun run build` | 构建成功 |
| 浏览器验收 | Playwright 命令 | 桌面/移动行为通过或明确 skipped 原因 |

实现过程中不得改变 `ImageModelCatalog`/`ImageRoutingPolicy` 的版本号、SKU key 规则、成本规则 active/draft/retired 状态或历史计费日志快照。

