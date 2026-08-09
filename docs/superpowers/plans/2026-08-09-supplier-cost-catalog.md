# 供应商成本目录 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有成本核算页面交付只读的全局供应商成本目录，支持服务端筛选、排序、分页、详情历史和安全 CSV 导出。

**Architecture:** 后端新增目录专用 DTO、GORM 左连接查询、成本规则投影和 CSV 导出服务，通过三个受 `CostAccountingRead` 保护的 GET 接口提供数据。前端在 `/cost-accounting?tab=catalog` 中复用现有 DataTable、Base UI Sheet 和 URL 搜索参数，桌面显示固定列宽表，移动端显示可展开紧凑列表；按次 15 秒等效价格只存在于目录投影和导出。

**Tech Stack:** Go 1.22、Gin、GORM、`shopspring/decimal`、`encoding/csv`、React 19、TypeScript、TanStack Router/Query/Table、Base UI、Tailwind CSS、Vitest、React Testing Library、Bun。

**Design:** `docs/superpowers/specs/2026-08-09-supplier-cost-catalog-design.md`

---

## 实施前置

正式实现前使用 `superpowers:using-git-worktrees`，从包含本计划的 `ysr` 创建 `.worktrees/supplier-cost-catalog` 和分支 `codex/supplier-cost-catalog`。所有任务在该 worktree 中顺序执行；每个任务通过后提交，不在主工作区直接实现。

## 文件结构

### 后端

- Create: `model/cost_catalog.go`：目录行左连接、普通列筛选、白名单排序、计数和分页。
- Create: `model/cost_catalog_test.go`：SQLite 行为合同及配置化 MySQL/PostgreSQL 查询合同。
- Create: `dto/cost_catalog.go`：列表、详情、价格项、摘要、facets 的 API 白名单 DTO。
- Create: `service/cost_catalog.go`：配置解析、价格投影、币种筛选、摘要、facets 和详情历史。
- Create: `service/cost_catalog_test.go`：四种成本模式、币种分支、未知数据和比较值合同。
- Create: `service/cost_catalog_export.go`：固定 CSV 列、安全单元格和导出上限。
- Create: `service/cost_catalog_export_test.go`：CSV 编码、公式注入、范围和敏感字段测试。
- Create: `controller/cost_catalog.go`：查询参数校验、列表/详情响应和临时文件下载。
- Create: `controller/cost_catalog_test.go`：HTTP 参数、响应头、下载和安全错误合同。
- Modify: `controller/cost_accounting.go`：把目录内部错误和导出超限映射为安全 HTTP 错误。
- Modify: `router/cost-accounting-router.go`：注册三个只读目录路由。
- Modify: `router/cost_accounting_router_test.go`：锁定三个路由的读取权限。
- Create: `e2e/supplier_cost_catalog_e2e_test.go`：管理员读取/导出与普通用户拒绝合同。

### 前端

- Modify: `web/src/features/cost-accounting/types.ts`：目录 API、筛选、详情和导出类型。
- Modify: `web/src/features/cost-accounting/api.ts`：目录 Query Key、列表、详情和 Blob 导出。
- Modify: `web/src/features/cost-accounting/lib/report.ts`：扩展 `CostAccountingSearch`，隔离 `catalog*` URL 状态。
- Create: `web/src/features/cost-accounting/lib/catalog.ts`：URL 到 API 参数、排序、价格和时间展示纯函数。
- Create: `web/src/features/cost-accounting/lib/catalog-export.ts`：安全解析下载文件名并触发浏览器下载。
- Create: `web/src/features/cost-accounting/lib/__tests__/catalog.test.ts`：URL、分页、排序和格式化合同。
- Create: `web/src/features/cost-accounting/lib/__tests__/catalog-export.test.ts`：文件名、Blob URL 创建和释放合同。
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog.tsx`：目录查询、表格状态和总交互编排。
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-filters.tsx`：六个筛选器和命令区。
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-columns.tsx`：桌面列定义、固定尺寸和排序表头。
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-mobile.tsx`：移动端紧凑可展开列表。
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-summary.tsx`：无嵌套卡片的摘要统计带。
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-pagination.tsx`：仅允许 25/50/100 的目录分页。
- Create: `web/src/features/cost-accounting/components/supplier-cost-detail-drawer.tsx`：只读配置和历史抽屉。
- Create: `web/src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx`：加载、筛选、表格、移动端和错误状态。
- Create: `web/src/features/cost-accounting/components/__tests__/supplier-cost-detail-drawer.test.tsx`：详情、历史、未知值和焦点恢复。
- Modify: `web/src/features/cost-accounting/index.tsx`：接入第三个 Tab。
- Modify: `web/src/routes/_authenticated/cost-accounting/index.tsx`：扩展 Zod 搜索参数。
- Modify via script: `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`：新增目录文案。
- Modify: `web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx`：第三个 Tab 和按需请求回归。

不修改 `web/src/components/data-table/`，目录通过现有 `DataTablePage` 的 `pinnedColumns`、`renderRow` 和 `mobile` 扩展点实现。

### Task 1: 建立跨数据库目录查询

**Files:**
- Create: `model/cost_catalog.go`
- Create: `model/cost_catalog_test.go`

- [ ] **Step 1: 编写 SQLite 失败测试，锁定左连接、字面量模型搜索和稳定分页**

在 `model/cost_catalog_test.go` 建立独立内存数据库，迁移 `Channel` 与 `ChannelModelCostRule`，写入两个正常渠道、一个已删除渠道对应规则，以及活动/草稿/退休版本。测试必须直接断言用户可见查询合同：

```go
func TestListCostCatalogRowsFiltersSortsAndKeepsMissingChannels(t *testing.T) {
	prepareCostCatalogQueryDB(t)
	seedCostCatalogQueryRows(t)

	rows, total, err := ListCostCatalogRows(CostCatalogQuery{
		BillableUpstreamModel: "100%_literal",
		Status:                string(types.CostRuleActive),
		SortBy:                "channel_name",
		SortOrder:             "asc",
		Offset:                0,
		Limit:                 25,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	assert.Equal(t, "Alpha", rows[0].ChannelName)
	assert.False(t, rows[0].ChannelMissing)
	assert.Equal(t, "", rows[1].ChannelName)
	assert.True(t, rows[1].ChannelMissing)

	page, pageTotal, err := ListCostCatalogRows(CostCatalogQuery{
		Status: "all", SortBy: "version", SortOrder: "desc", Offset: 1, Limit: 1,
	})
	require.NoError(t, err)
	assert.Greater(t, pageTotal, int64(1))
	require.Len(t, page, 1)
}
```

`seedCostCatalogQueryRows` 同时写入模型 `vendor-100%_literal` 与 `vendor-100x-literal`，确保 `%`、`_` 被视为普通字符而不是 SQL 通配符。

同文件增加 `TestListCostCatalogRowsFiltersChannelModeAndSource`，用 `ChannelID`、`CostMode=per_request`、`Source=config_import` 组合筛选并断言只返回目标规则；筛选模型使用大写输入，证明匹配不区分大小写。

再增加 `TestWalkCostCatalogRowsVisitsStableBatches`，传入批次大小 2，记录每批规则 ID，断言每个 ID 只出现一次且顺序与相同排序的非分页查询一致。

- [ ] **Step 2: 运行模型测试并确认因目录类型未定义而失败**

Run: `go test ./model -run TestListCostCatalogRowsFiltersSortsAndKeepsMissingChannels -count=1`

Expected: FAIL，错误包含 `undefined: ListCostCatalogRows` 或 `undefined: CostCatalogQuery`。

- [ ] **Step 3: 实现目录查询行、过滤和排序白名单**

在 `model/cost_catalog.go` 定义：

```go
type CostCatalogQuery struct {
	ChannelID             int
	BillableUpstreamModel string
	CostMode              string
	Status                string
	Source                string
	SortBy                 string
	SortOrder              string
	Offset                 int
	Limit                  int
}

type CostCatalogRow struct {
	ChannelModelCostRule
	ChannelName    string `gorm:"column:channel_name"`
	ChannelType    int    `gorm:"column:channel_type"`
	ChannelMissing bool   `gorm:"column:channel_missing"`
}

func ListCostCatalogRows(query CostCatalogQuery) ([]CostCatalogRow, int64, error)
func WalkCostCatalogRows(query CostCatalogQuery, batchSize int, visit func([]CostCatalogRow) error) error
func GetCostCatalogRow(id int64) (*CostCatalogRow, error)
func ListCostCatalogHistoryRows(channelID int, billableModel, variant string) ([]CostCatalogRow, error)
```

基础查询必须使用单次左连接和显式白名单列：

```go
func costCatalogBaseQuery(query CostCatalogQuery) *gorm.DB {
	ruleTable := DB.NamingStrategy.TableName("ChannelModelCostRule")
	channelTable := DB.NamingStrategy.TableName("Channel")
	db := DB.Table(ruleTable + " AS rules").
		Select("rules.*, COALESCE(channels.name, '') AS channel_name, COALESCE(channels.type, 0) AS channel_type, CASE WHEN channels.id IS NULL THEN ? ELSE ? END AS channel_missing", true, false).
		Joins("LEFT JOIN " + channelTable + " AS channels ON channels.id = rules.channel_id")

	if query.ChannelID > 0 {
		db = db.Where("rules.channel_id = ?", query.ChannelID)
	}
	if query.BillableUpstreamModel != "" {
		literal := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(query.BillableUpstreamModel))
		db = db.Where("LOWER(rules.billable_upstream_model) LIKE ? ESCAPE '!'", "%"+literal+"%")
	}
	if query.CostMode != "" {
		db = db.Where("rules.cost_mode = ?", query.CostMode)
	}
	if query.Status != "all" {
		db = db.Where("rules.status = ?", query.Status)
	}
	if query.Source != "" {
		db = db.Where("rules.source = ?", query.Source)
	}
	return db
}
```

排序使用 `gorm.io/gorm/clause.OrderByColumn` 和固定 map；`channel_name` 先用可移植的 `CASE WHEN channels.name IS NULL` 把缺失渠道放到末尾，再按名称排序。所有排序最后追加 `rules.id ASC`，禁止把 `sort_by` 或 `sort_order` 直接拼进 SQL。

`WalkCostCatalogRows` 固定按白名单稳定排序以 500 行为默认批次执行 `Offset/Limit`，每批调用一次 `visit`，`visit` 返回错误时立即停止。币种筛选、摘要、facets 和 CSV 共用该遍历，禁止先加载完整规则切片。

- [ ] **Step 4: 运行 SQLite 查询测试并修复至通过**

Run: `gofmt -w model/cost_catalog.go model/cost_catalog_test.go && go test ./model -run CostCatalog -count=1`

Expected: PASS，且 SQL 日志中没有逐行渠道查询。

- [ ] **Step 5: 增加配置化 MySQL/PostgreSQL 合同测试**

仿照 `model/cost_accounting_migration_test.go`，新增 `TestCostCatalogQueryConfiguredDatabases`，分别读取 `TEST_MYSQL_DSN` 与 `TEST_POSTGRES_DSN`。未配置时 `t.Skip`；配置时使用带随机表前缀的 GORM 连接执行同一个 `testCostCatalogQueryContract`，断言左连接、`ESCAPE '!'` 字面量搜索和排序均成功。

- [ ] **Step 6: 运行模型完整测试并提交**

Run: `go test ./model -count=1`

Expected: PASS；未配置的 MySQL/PostgreSQL 子测试显示 SKIP，而不是 FAIL。

```bash
git add model/cost_catalog.go model/cost_catalog_test.go
git commit -m "feat(cost-accounting): add supplier cost catalog query"
```

### Task 2: 实现目录 DTO、价格投影、摘要和 facets

**Files:**
- Create: `dto/cost_catalog.go`
- Create: `service/cost_catalog.go`
- Create: `service/cost_catalog_test.go`

- [ ] **Step 1: 编写服务失败测试，覆盖价格模式和 15 秒比较值**

在 `service/cost_catalog_test.go` 使用 SQLite fixture 写入以下规则：`free`、`per_request`、`per_duration`、三个 `per_token` 子模式、无效 JSON、缺失标准化价格和孤立渠道。至少定义并运行这些合同测试：

```go
func TestListSupplierCostCatalogProjectsAllCostModes(t *testing.T)
func TestListSupplierCostCatalogUsesFullPerRequestPriceAndComparisonOnly(t *testing.T)
func TestListSupplierCostCatalogFiltersCurrencyAfterStructuredParsing(t *testing.T)
func TestListSupplierCostCatalogReportsInvalidPricesWithoutZeroFallback(t *testing.T)
func TestListSupplierCostCatalogKeepsValidPriceForMissingChannel(t *testing.T)
func TestListSupplierCostCatalogSummaryIgnoresStatusFilter(t *testing.T)
func TestListSupplierCostCatalogFacetsIgnoreCurrentFilters(t *testing.T)
```

按次断言必须明确区分实际价格和比较值：

```go
page, err := ListSupplierCostCatalog(CostCatalogFilter{
	Status: "active", Page: 1, PageSize: 50,
})
require.NoError(t, err)
row := findCatalogItem(t, page.Items, "per-request-model")
require.Len(t, row.Prices, 1)
assert.Equal(t, "3", row.Prices[0].NormalizedUSDAmount)
require.NotNil(t, row.Comparison15SEquivalentUSDPerSecond)
assert.Equal(t, "0.2", *row.Comparison15SEquivalentUSDPerSecond)
assert.Equal(t, "available", row.PriceStatus)
```

无效规则必须断言 `PriceStatus == "unavailable"`、`Issues` 含 `invalid_config`，且所有价格字符串为空而不是 `"0"`。

- [ ] **Step 2: 运行服务测试并确认失败**

Run: `go test ./service -run 'TestListSupplierCostCatalog' -count=1`

Expected: FAIL，错误包含 `undefined: ListSupplierCostCatalog`。

- [ ] **Step 3: 定义 API 白名单 DTO**

在 `dto/cost_catalog.go` 定义以下稳定合同；列表 DTO 不包含 `ConfigJSON`、渠道密钥或渠道配置：

```go
type CostCatalogPrice struct {
	Key                 string `json:"key"`
	Unit                string `json:"unit"`
	NativeAmount        string `json:"native_amount"`
	NormalizedUSDAmount string `json:"normalized_usd_amount"`
}

type CostCatalogItem struct {
	RuleID                                  int64              `json:"rule_id"`
	ChannelID                               int                `json:"channel_id"`
	ChannelName                             string             `json:"channel_name"`
	ChannelType                             int                `json:"channel_type"`
	ChannelMissing                          bool               `json:"channel_missing"`
	BillableUpstreamModel                   string             `json:"billable_upstream_model"`
	CostVariantKey                          string             `json:"cost_variant_key"`
	Version                                 int                `json:"version"`
	Status                                  string             `json:"status"`
	CostMode                                types.CostMode     `json:"cost_mode"`
	SchemaVersion                           int                `json:"schema_version"`
	Currency                                string             `json:"currency"`
	Prices                                  []CostCatalogPrice `json:"prices"`
	Comparison15SEquivalentUSDPerSecond     *string            `json:"comparison_15s_equivalent_usd_per_second,omitempty"`
	ChargeEvent                             types.CostChargeEvent `json:"charge_event,omitempty"`
	MeterSource                             types.CostMeterSource `json:"meter_source,omitempty"`
	TokenMode                               types.CostTokenMode `json:"token_mode,omitempty"`
	Source                                  string             `json:"source"`
	Note                                    string             `json:"note"`
	EffectiveFrom                           *int64              `json:"effective_from,omitempty"`
	EffectiveTo                             *int64              `json:"effective_to,omitempty"`
	CreatedAt                               int64               `json:"created_at"`
	UpdatedAt                               int64               `json:"updated_at"`
	PriceStatus                             string              `json:"price_status"`
	Issues                                  []string            `json:"issues"`
}

type CostCatalogSummary struct {
	ChannelCount     int64 `json:"channel_count"`
	ActiveRuleCount  int64 `json:"active_rule_count"`
	DraftRuleCount   int64 `json:"draft_rule_count"`
	RetiredRuleCount int64 `json:"retired_rule_count"`
}

type CostCatalogChannelFacet struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Type    int    `json:"type"`
	Missing bool   `json:"missing"`
}

type CostCatalogFacets struct {
	Channels   []CostCatalogChannelFacet `json:"channels"`
	Currencies []string                  `json:"currencies"`
	Sources    []string                  `json:"sources"`
}

type CostCatalogPage struct {
	Items    []CostCatalogItem `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Summary  CostCatalogSummary `json:"summary"`
	Facets   CostCatalogFacets  `json:"facets"`
}

type CostCatalogHistoryEntry struct {
	CostCatalogItem
	CreatedBy   int `json:"created_by"`
	ActivatedBy int `json:"activated_by"`
}

type CostCatalogDetail struct {
	Rule    CostCatalogHistoryEntry   `json:"rule"`
	Config  *types.CostRuleConfigV1   `json:"config,omitempty"`
	History []CostCatalogHistoryEntry `json:"history"`
}
```

格式化时运行 `gofmt`；若字段对齐被格式化器调整，以 `gofmt` 输出为准。

- [ ] **Step 4: 实现服务过滤、投影和摘要**

在 `service/cost_catalog.go` 定义：

```go
var ErrCostCatalogUnavailable = errors.New("cost catalog unavailable")

type CostCatalogFilter struct {
	ChannelID             int
	BillableUpstreamModel string
	CostMode              string
	Status                string
	Currency              string
	Source                string
	Page                  int
	PageSize              int
	SortBy                 string
	SortOrder              string
}

func ListSupplierCostCatalog(filter CostCatalogFilter) (dto.CostCatalogPage, error)
func GetSupplierCostCatalogDetail(ruleID int64) (*dto.CostCatalogDetail, error)
```

服务内部使用一个同时保留结构化配置的投影，避免列表、详情和 CSV 各自解析一次规则：

```go
type costCatalogProjection struct {
	Item        dto.CostCatalogItem
	Config      *types.CostRuleConfigV1
	CreatedBy   int
	ActivatedBy int
}

func walkSupplierCostCatalogProjections(
	filter CostCatalogFilter,
	visit func(costCatalogProjection) error,
) error
```

实现规则：

1. 缺省状态规范为 `active`，缺省分页为 `1/50`，仅接受 `25/50/100`；计算 offset 前检查 `(page-1) * pageSize` 不超过 `math.MaxInt`，越界返回无数据而不发生整数回绕；
2. 无币种筛选时调用 `model.ListCostCatalogRows` 在数据库计数和分页；
3. 有币种筛选时调用 `walkSupplierCostCatalogProjections`，按大写币种精确匹配并计数，只保留当前页索引范围内的 projection；
4. 摘要复制非状态筛选并强制 `Status="all"`，通过同一批次遍历应用币种过滤；
5. facets 通过无筛选的批次遍历构建唯一集合，渠道按名称/ID、币种和来源按字典序排序；
6. 数据库错误包装 `ErrCostCatalogUnavailable`，无效单行配置只写入 issues，不使整页失败；
7. `Items`、`Prices`、`Issues`、facets 和 `History` 即使为空也初始化为空切片，JSON 不能返回 `null`。

价格项只读取规则冻结字段；每个原币价格和对应标准化 USD 价格都使用 `decimal.NewFromString` 验证有限、非负后输出 `decimal.String()`。按次比较值仅执行：

```go
comparison := normalized.Div(decimal.NewFromInt(15)).String()
item.Comparison15SEquivalentUSDPerSecond = &comparison
```

禁止把 comparison 写回规则、传入路由或结算类型。详情读取同一业务键历史，历史按版本倒序；无效 JSON 时 `Config=nil` 并保留 issues。

- [ ] **Step 5: 运行服务测试和现有成本规则回归**

Run: `gofmt -w dto/cost_catalog.go service/cost_catalog.go service/cost_catalog_test.go && go test ./service -run 'CostCatalog|CostRule' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交 DTO 和服务投影**

```bash
git add dto/cost_catalog.go service/cost_catalog.go service/cost_catalog_test.go
git commit -m "feat(cost-accounting): project supplier cost catalog"
```

### Task 3: 暴露只读列表和详情接口

**Files:**
- Create: `controller/cost_catalog.go`
- Create: `controller/cost_catalog_test.go`
- Modify: `controller/cost_accounting.go`
- Modify: `router/cost-accounting-router.go`
- Modify: `router/cost_accounting_router_test.go`

- [ ] **Step 1: 先扩展路由权限失败测试**

在 `TestCostAccountingRoutesUseDedicatedPermissions` 增加：

```go
assertCostRoute(t, http.MethodGet, "/catalog", authz.CostAccountingRead, controller.ListSupplierCostCatalog)
assertCostRoute(t, http.MethodGet, "/catalog/export", authz.CostAccountingRead, controller.ExportSupplierCostCatalog)
assertCostRoute(t, http.MethodGet, "/catalog/:rule_id", authz.CostAccountingRead, controller.GetSupplierCostCatalogDetail)
```

Run: `go test ./router -run TestCostAccountingRoutesUseDedicatedPermissions -count=1`

Expected: FAIL，三个 controller 函数尚不存在。

- [ ] **Step 2: 编写 controller 参数与详情响应失败测试**

在 `controller/cost_catalog_test.go` 覆盖：

```go
func TestCostCatalogFilterFromQueryUsesSafeDefaults(t *testing.T)
func TestCostCatalogFilterFromQueryAcceptsAllStatuses(t *testing.T)
func TestCostCatalogFilterFromQueryRejectsInvalidEnumsAndPageSize(t *testing.T)
func TestCostCatalogDetailEndpointReturnsNotFound(t *testing.T)
func TestCostCatalogInternalErrorIsRedacted(t *testing.T)
```

默认断言必须为 `status=active`、`page=1`、`page_size=50`、`sort_by=channel_name`、`sort_order=asc`。拒绝负数、非 `25/50/100` 页大小、未知状态/模式/排序字段、超过 191 字符的模型搜索和超过 32 字符的来源。

- [ ] **Step 3: 实现查询参数解析和两个 JSON 接口**

在 `controller/cost_catalog.go` 定义白名单和解析函数：

```go
var costCatalogSortFields = map[string]struct{}{
	"channel_name": {}, "channel_id": {}, "billable_upstream_model": {},
	"cost_variant_key": {}, "status": {}, "version": {},
	"cost_mode": {}, "source": {}, "effective_from": {},
}

func costCatalogFilterFromQuery(c *gin.Context) (service.CostCatalogFilter, error)
func ListSupplierCostCatalog(c *gin.Context)
func GetSupplierCostCatalogDetail(c *gin.Context)
```

`costCatalogFilterFromQuery` 使用现有 `optionalCostAccountingQueryInt` 读取数字，但对 `page` 强制大于零；币种 `strings.ToUpper`，其他文本 `TrimSpace`。`GetSupplierCostCatalogDetail` 用 `strconv.ParseInt(c.Param("rule_id"), 10, 64)` 严格校验正数。

响应直接使用 DTO：

```go
page, err := service.ListSupplierCostCatalog(filter)
if err != nil {
	writeCostAccountingError(c, err)
	return
}
common.ApiSuccess(c, page)
```

- [ ] **Step 4: 注册路由并保护内部错误**

在 `costAccountingPermissionRoutes` 注册三个 GET 路由，静态 `/catalog/export` 写在参数路由之前。修改 `writeCostAccountingError`：

```go
} else if errors.Is(err, service.ErrCostCatalogUnavailable) {
	status = http.StatusInternalServerError
	code = "cost_catalog_unavailable"
}
```

500 响应继续由现有逻辑改写为 `cost accounting operation failed`，不能暴露 SQL 或 `config_json`。

- [ ] **Step 5: 运行 controller 和 router 测试**

Run: `gofmt -w controller/cost_catalog.go controller/cost_catalog_test.go controller/cost_accounting.go router/cost-accounting-router.go router/cost_accounting_router_test.go && go test ./controller ./router -run 'CostCatalog|CostAccountingRoutes' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交只读 API**

```bash
git add controller/cost_catalog.go controller/cost_catalog_test.go controller/cost_accounting.go router/cost-accounting-router.go router/cost_accounting_router_test.go
git commit -m "feat(cost-accounting): expose supplier cost catalog api"
```

### Task 4: 实现安全 CSV 导出

**Files:**
- Create: `service/cost_catalog_export.go`
- Create: `service/cost_catalog_export_test.go`
- Modify: `controller/cost_catalog.go`
- Modify: `controller/cost_catalog_test.go`
- Modify: `controller/cost_accounting.go`

- [ ] **Step 1: 编写 CSV 失败测试**

新增以下服务合同测试：

```go
func TestWriteSupplierCostCatalogCSVUsesFixedColumnsAndBOM(t *testing.T)
func TestWriteSupplierCostCatalogCSVNeutralizesSpreadsheetFormulas(t *testing.T)
func TestWriteSupplierCostCatalogCSVLeavesUnknownPricesBlank(t *testing.T)
func TestWriteSupplierCostCatalogCSVLabelsComparisonOnly(t *testing.T)
func TestWriteSupplierCostCatalogCSVHonorsFilteredAndAllScopes(t *testing.T)
func TestWriteSupplierCostCatalogCSVExcludesSensitiveChannelFields(t *testing.T)
```

公式测试至少使用 `=cmd|' /C calc'!A0`、` +SUM(1,1)`、制表符后接 `@SUM(1,1)`，通过 `encoding/csv.Reader` 读回并断言单元格以单引号开头。敏感测试写入渠道 Key、Base URL、HeaderOverride 和 ConfigJSON 标记值，断言 CSV 字节不包含这些值。

- [ ] **Step 2: 运行 CSV 测试并确认失败**

Run: `go test ./service -run 'TestWriteSupplierCostCatalogCSV' -count=1`

Expected: FAIL，错误包含 `undefined: WriteSupplierCostCatalogCSV`。

- [ ] **Step 3: 实现固定列、公式防护和导出范围**

在 `service/cost_catalog_export.go` 定义：

```go
const CostCatalogExportMaxRows = 100_000

var ErrCostCatalogExportTooLarge = errors.New("cost catalog export exceeds 100000 rows")

type CostCatalogExportScope string

const (
	CostCatalogExportFiltered CostCatalogExportScope = "filtered"
	CostCatalogExportAll      CostCatalogExportScope = "all"
)

func WriteSupplierCostCatalogCSV(w io.Writer, filter CostCatalogFilter, scope CostCatalogExportScope) (int, error)
```

实现必须写入 UTF-8 BOM，再用 `csv.NewWriter` 写固定中文表头。`scope=filtered` 调用 `walkSupplierCostCatalogProjections` 复用目录筛选但忽略分页；`scope=all` 清空六个筛选并强制状态 `all` 后调用同一遍历，保留白名单排序。每个 projection 立即编码到 controller 的临时文件，只在内存保留当前批次；CSV 从结构化 `Config` 读取换算参数，不解析或输出原始 JSON。计数超过 100,000 时返回 sentinel，controller 删除临时文件且不发送 HTTP 文件响应。

单元格防护函数按以下合同实现：

```go
func safeCatalogCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
```

非适用和不可用价格写空字符串；issues 使用分号连接；15 秒列名固定为 `15 秒等效 USD/秒（仅比较）`。不使用规则原始 JSON 生成任意列。

- [ ] **Step 4: 编写并实现 HTTP 下载合同**

在 `controller/cost_catalog_test.go` 增加：

```go
func TestExportSupplierCostCatalogRequiresExplicitScope(t *testing.T)
func TestExportSupplierCostCatalogReturnsExcelCompatibleHeaders(t *testing.T)
func TestExportSupplierCostCatalogMapsTooLargeTo413(t *testing.T)
```

超限测试直接用 Gin recorder 调用 `writeCostAccountingError(ctx, service.ErrCostCatalogExportTooLarge)`，断言状态码 `413`、错误码 `cost_catalog_export_too_large`，不需要构造 100,001 条数据库记录。

`ExportSupplierCostCatalog` 要求显式 `scope=filtered|all`，使用 `os.CreateTemp("", "new-api-cost-catalog-*.csv")` 写完后再设置 HTTP 头并发送文件，`defer os.Remove` 清理精确临时文件。响应头必须包含：

```go
c.Header("Content-Type", "text/csv; charset=utf-8")
c.Header("Content-Disposition", fmt.Sprintf(
	"attachment; filename=%q; filename*=UTF-8''%s", asciiName, url.PathEscape(chineseName),
))
c.Header("X-Exported-Row-Count", strconv.Itoa(count))
```

在 `writeCostAccountingError` 把 `ErrCostCatalogExportTooLarge` 映射为 `413` 和 `cost_catalog_export_too_large`。临时文件创建、写入、关闭或发送失败统一包装 `ErrCostCatalogUnavailable`。

- [ ] **Step 5: 运行导出和成本核算回归**

Run: `gofmt -w service/cost_catalog_export.go service/cost_catalog_export_test.go controller/cost_catalog.go controller/cost_catalog_test.go controller/cost_accounting.go && go test ./service ./controller ./router -run 'CostCatalog|CostAccountingRoutes' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交 CSV 导出**

```bash
git add service/cost_catalog_export.go service/cost_catalog_export_test.go controller/cost_catalog.go controller/cost_catalog_test.go controller/cost_accounting.go
git commit -m "feat(cost-accounting): export supplier cost catalog"
```

### Task 5: 建立前端 URL、API 和下载合同

**Files:**
- Modify: `web/src/features/cost-accounting/types.ts`
- Modify: `web/src/features/cost-accounting/api.ts`
- Modify: `web/src/features/cost-accounting/lib/report.ts`
- Create: `web/src/features/cost-accounting/lib/catalog.ts`
- Create: `web/src/features/cost-accounting/lib/catalog-export.ts`
- Create: `web/src/features/cost-accounting/lib/__tests__/catalog.test.ts`
- Create: `web/src/features/cost-accounting/lib/__tests__/catalog-export.test.ts`
- Modify: `web/src/routes/_authenticated/cost-accounting/index.tsx`

- [ ] **Step 1: 编写 URL 与下载辅助失败测试**

`catalog.test.ts` 精确覆盖：

```ts
test('maps catalog URL state to trimmed API parameters', () => {
  assert.deepEqual(
    costCatalogParamsFromSearch({
      tab: 'catalog',
      catalogChannelId: 23,
      catalogModel: '  vendor-model  ',
      catalogCostMode: 'per_request',
      catalogStatus: 'all',
      catalogCurrency: 'cny',
      catalogSource: 'config_import',
      catalogPage: 2,
      catalogPageSize: 100,
      catalogSort: 'version',
      catalogOrder: 'desc',
    }),
    {
      channel_id: 23,
      billable_upstream_model: 'vendor-model',
      cost_mode: 'per_request',
      status: 'all',
      currency: 'CNY',
      source: 'config_import',
      page: 2,
      page_size: 100,
      sort_by: 'version',
      sort_order: 'desc',
    }
  )
})
```

再覆盖缺省 `active/1/50/channel_name/asc`、筛选更新重置第一页、价格项格式化、未知价格不显示 `$0` 和 Unix 时间空值。

`catalog-export.test.ts` 使用可控 `URL.createObjectURL`、`URL.revokeObjectURL` 和 `<a>.click()`，断言优先解析 RFC 5987 `filename*`、拒绝目录穿越字符、触发一次下载并始终释放 Blob URL。

- [ ] **Step 2: 运行纯函数测试并确认失败**

Run: `cd web && bun test src/features/cost-accounting/lib/__tests__/catalog.test.ts src/features/cost-accounting/lib/__tests__/catalog-export.test.ts`

Expected: FAIL，模块尚不存在。

- [ ] **Step 3: 扩展目录 TypeScript 类型**

在 `types.ts` 增加与后端 JSON 一致的类型：

```ts
export type CostCatalogPriceStatus = 'available' | 'unavailable'
export type CostCatalogSort =
  | 'channel_name'
  | 'channel_id'
  | 'billable_upstream_model'
  | 'cost_variant_key'
  | 'status'
  | 'version'
  | 'cost_mode'
  | 'source'
  | 'effective_from'

export interface CostCatalogPrice {
  key: string
  unit: string
  native_amount: string
  normalized_usd_amount: string
}

export interface CostCatalogItem {
  rule_id: number
  channel_id: number
  channel_name: string
  channel_type: number
  channel_missing: boolean
  billable_upstream_model: string
  cost_variant_key: string
  version: number
  status: CostRuleStatus
  cost_mode: CostMode
  schema_version: number
  currency: string
  prices: CostCatalogPrice[]
  comparison_15s_equivalent_usd_per_second?: string
  charge_event?: CostChargeEvent
  meter_source?: CostMeterSource
  token_mode?: CostTokenMode
  source: string
  note: string
  effective_from?: number
  effective_to?: number
  created_at: number
  updated_at: number
  price_status: CostCatalogPriceStatus
  issues: string[]
}

export interface CostCatalogHistoryEntry extends CostCatalogItem {
  created_by: number
  activated_by: number
}

export interface CostCatalogDetail {
  rule: CostCatalogHistoryEntry
  config?: CostRuleConfigV1
  history: CostCatalogHistoryEntry[]
}

export interface CostCatalogSummary {
  channel_count: number
  active_rule_count: number
  draft_rule_count: number
  retired_rule_count: number
}

export interface CostCatalogChannelFacet {
  id: number
  name: string
  type: number
  missing: boolean
}

export interface CostCatalogFacets {
  channels: CostCatalogChannelFacet[]
  currencies: string[]
  sources: string[]
}

export interface CostCatalogPage {
  items: CostCatalogItem[]
  total: number
  page: number
  page_size: number
  summary: CostCatalogSummary
  facets: CostCatalogFacets
}

export interface CostCatalogExportResult {
  blob: Blob
  filename: string
  rowCount: number
}

export interface CostCatalogParams {
  channel_id?: number
  billable_upstream_model?: string
  cost_mode?: CostMode
  status?: CostRuleStatus | 'all'
  currency?: string
  source?: string
  page?: number
  page_size?: 25 | 50 | 100
  sort_by?: CostCatalogSort
  sort_order?: 'asc' | 'desc'
}
```

`CostCatalogPage` 必须始终把 `items`、`facets` 数组视为非空数组合同，即使结果为空也不使用 `undefined`。

- [ ] **Step 4: 实现 URL 映射、格式化和下载辅助**

把 `CostAccountingSearch.tab` 扩展为 `'profit' | 'catalog' | 'anomalies'`，并增加设计中的十个 `catalog*` 字段。`catalog.ts` 导出：

```ts
export function costCatalogParamsFromSearch(
  search: CostAccountingSearch
): CostCatalogParams

export function updateCatalogSearch(
  search: CostAccountingSearch,
  patch: Partial<CostAccountingSearch>
): CostAccountingSearch

export function formatCatalogPrice(
  amount: string,
  currency: string,
  unitLabel: string
): string

export function formatCatalogTimestamp(value?: number): string
```

`CostAccountingSearch` 增加准确字段：

```ts
catalogChannelId?: number
catalogModel?: string
catalogCostMode?: CostMode
catalogStatus?: CostRuleStatus | 'all'
catalogCurrency?: string
catalogSource?: string
catalogPage?: number
catalogPageSize?: 25 | 50 | 100
catalogSort?: CostCatalogSort
catalogOrder?: 'asc' | 'desc'
```

`updateCatalogSearch` 在筛选或排序改变时固定写入 `catalogPage: 1`。`formatCatalogPrice` 先用 `decimal.js` 验证数值；空值或非有限值返回空字符串，不能猜测为零。

`catalog-export.ts` 导出：

```ts
export function filenameFromContentDisposition(
  header: string | undefined,
  fallback: string
): string

export function downloadCostCatalogExport(
  result: CostCatalogExportResult
): void
```

文件名解析后通过 `decoded.split(/[\\/]/).at(-1)` 只保留 basename，替换 `/\\:*?"<>|` 为 `_`；下载函数在 `finally` 中调用 `URL.revokeObjectURL`，不引入 Node `path` 到浏览器包。

- [ ] **Step 5: 实现 API 和 Query Key**

在 `costAccountingQueryKeys` 增加：

```ts
catalogs: () => [...costAccountingQueryKeys.all, 'catalog'] as const,
catalog: (params: CostCatalogParams) =>
  [...costAccountingQueryKeys.catalogs(), params] as const,
catalogDetail: (id: number) =>
  [...costAccountingQueryKeys.catalogs(), id] as const,
```

新增 `getSupplierCostCatalog`、`getSupplierCostCatalogDetail` 和：

```ts
export async function exportSupplierCostCatalog(
  scope: 'filtered' | 'all',
  params: Omit<CostCatalogParams, 'page' | 'page_size'>
): Promise<CostCatalogExportResult> {
  const response = await api.get<Blob>(
    `${COST_ACCOUNTING_PATH}/catalog/export`,
    { params: { ...params, scope }, responseType: 'blob' }
  )
  return {
    blob: response.data,
    filename: filenameFromContentDisposition(
      response.headers['content-disposition'],
      'supplier-cost-catalog.csv'
    ),
    rowCount: validExportRowCount(
      response.headers['x-exported-row-count']
    ),
  }
}
```

`validExportRowCount` 只接受非负安全整数，缺失、NaN、Infinity 或负数均返回 0：

```ts
function validExportRowCount(value: unknown): number {
  const count = Number(value)
  return Number.isSafeInteger(count) && count >= 0 ? count : 0
}
```

- [ ] **Step 6: 扩展路由 Zod schema**

在路由文件添加 `tab: z.enum(['profit', 'catalog', 'anomalies'])`，以及严格枚举的 `catalogCostMode`、`catalogStatus`、`catalogPageSize`、`catalogSort` 和 `catalogOrder`。数字参数用 `z.number().int().positive()`；无效值使用 `.catch(default)` 回退，不污染利润参数。

- [ ] **Step 7: 运行纯函数测试、类型检查并提交**

Run: `cd web && bun test src/features/cost-accounting/lib/__tests__/catalog.test.ts src/features/cost-accounting/lib/__tests__/catalog-export.test.ts && bun run typecheck`

Expected: PASS。

```bash
git add web/src/features/cost-accounting/types.ts web/src/features/cost-accounting/api.ts web/src/features/cost-accounting/lib/report.ts web/src/features/cost-accounting/lib/catalog.ts web/src/features/cost-accounting/lib/catalog-export.ts web/src/features/cost-accounting/lib/__tests__/catalog.test.ts web/src/features/cost-accounting/lib/__tests__/catalog-export.test.ts web/src/routes/_authenticated/cost-accounting/index.tsx
git commit -m "feat(cost-accounting): add supplier catalog client contract"
```

### Task 6: 构建目录摘要、筛选、桌面表格和移动列表

**Required skill:** `shadcn-ui`，先确认 `base=base`、Tailwind v4 和 Hugeicons 项目上下文。

**Files:**
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog.tsx`
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-filters.tsx`
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-columns.tsx`
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-mobile.tsx`
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-summary.tsx`
- Create: `web/src/features/cost-accounting/components/supplier-cost-catalog-pagination.tsx`
- Create: `web/src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx`

- [ ] **Step 1: 编写组件失败测试**

测试通过 React Query wrapper 和 API mock 覆盖以下用户行为：

```ts
test('renders summary and active catalog rows from URL defaults')
test('updates URL filters and resets the page to one')
test('keeps previous rows visible during background refresh')
test('distinguishes an empty catalog from no filter matches')
test('shows a retry action after the catalog request fails')
test('renders unknown prices without a zero dollar fallback')
test('uses pinned channel and model columns on desktop')
test('exposes sorting and filters with accessible names and aria-sort')
test('expands mobile metadata without opening the detail drawer')
test('offers only 25, 50, and 100 rows per page')
test('moves to the last valid page after filtered results shrink')
```

固定使用角色、可访问名称、`aria-expanded` 和可见状态断言；不对完整 Tailwind class 字符串做快照。

- [ ] **Step 2: 运行组件测试并确认失败**

Run: `cd web && bun test src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx`

Expected: FAIL，组件模块尚不存在。

- [ ] **Step 3: 实现无卡片嵌套的摘要和筛选区**

`SupplierCostCatalogSummary` 使用一个 `grid grid-cols-2 lg:grid-cols-4` 的上下边框统计带，四项固定为渠道数、活动、草稿、已退休；加载时使用 `Skeleton` 保持高度。

`SupplierCostCatalogFilters` 使用现有 `Combobox` 选择 facets 渠道，使用 Base UI `Select` 时必须传 `items` 且把 `SelectItem` 放入 `SelectGroup`。模型输入在 Blur 或 Enter 时应用；成本模式、状态、币种和来源选择立即应用。统一清除操作恢复活动状态、第一页和默认排序。

刷新、导出按钮使用项目配置的 Hugeicons：

```tsx
import {
  Download04Icon,
  RefreshIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'

<Button type='button' variant='outline' onClick={props.onRefresh}>
  <HugeiconsIcon icon={RefreshIcon} data-icon='inline-start' strokeWidth={2} />
  {t('Refresh')}
</Button>
```

不安装新组件或图标依赖。

- [ ] **Step 4: 实现列定义和服务端 DataTable 状态**

`useSupplierCostCatalogColumns` 定义以下稳定列 ID 和尺寸：`channel_name`、`billable_upstream_model`、`cost_variant_key`、`status`、`cost_mode`、`native_price`、`normalized_price`、`billing_semantics`、`source`、`effective_from`。渠道和模型列分别固定约 190/240px；渠道单元格使用现有 `getChannelTypeLabel` 显示名称、`#ID` 和类型，不读取渠道配置。使用 `DataTableColumnHeader` 暴露可排序字段。

`SupplierCostCatalog` 使用：

```tsx
const catalogQuery = useQuery({
  queryKey: costAccountingQueryKeys.catalog(params),
  queryFn: () => getSupplierCostCatalog(params),
  enabled: props.enabled,
  placeholderData: (previous) => previous,
})

const { table } = useDataTable({
  data: page.items,
  columns,
  totalCount: page.total,
  pagination,
  sorting,
  onPaginationChange: handlePaginationChange,
  onSortingChange: handleSortingChange,
  manualPagination: true,
  manualSorting: true,
  manualFiltering: true,
  getRowId: (row) => String(row.rule_id),
  ensurePageInRange: (pageCount) => {
    if (pagination.pageIndex >= pageCount) {
      props.onSearchChange({
        ...props.search,
        catalogPage: Math.max(1, pageCount),
      })
    }
  },
})
```

`DataTablePage` 配置 `pinnedColumns` 固定渠道和模型并启用 `applyHeaderSize`。错误时渲染 `Empty` + Retry；系统无数据与筛选无结果使用不同标题和动作。`isFetching` 只降低旧数据强调度，不清空列表。Task 7 再接入详情行事件，避免在抽屉不存在时留下无效交互。

不要使用共享 `DataTablePagination`，因为它包含目录 API 不接受的 10/20/30/40。新增 `SupplierCostCatalogPagination`，固定选项为：

```ts
const CATALOG_PAGE_SIZES = [25, 50, 100] as const
```

分页组件显示总行数、当前页/总页、上一页、下一页和页大小 Select，所有图标按钮有 `aria-label`。`DataTablePage` 设置 `showPagination={false}`，通过 `afterTable` 渲染该组件。

- [ ] **Step 5: 实现移动端紧凑可展开列表**

`SupplierCostCatalogMobile` 使用单个 `divide-y rounded-lg border` 容器，每行一个 Base UI `Collapsible`。Task 6 只实现独立展开按钮并设置 `aria-expanded`；Task 7 再把行主操作连接到详情抽屉。默认展示渠道、模型、状态、模式和标准化 USD 主价格，展开后展示变体、原币价格、计费语义、来源和有效期。禁止在移动行内再嵌套 Card。

- [ ] **Step 6: 运行测试、类型检查和涉及文件 lint**

Run:

```bash
cd web
bun test src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/cost-accounting/components/supplier-cost-catalog.tsx src/features/cost-accounting/components/supplier-cost-catalog-filters.tsx src/features/cost-accounting/components/supplier-cost-catalog-columns.tsx src/features/cost-accounting/components/supplier-cost-catalog-mobile.tsx src/features/cost-accounting/components/supplier-cost-catalog-summary.tsx src/features/cost-accounting/components/supplier-cost-catalog-pagination.tsx
```

Expected: 全部 PASS，无 lint error。

- [ ] **Step 7: 提交目录主视图**

```bash
git add web/src/features/cost-accounting/components/supplier-cost-catalog.tsx web/src/features/cost-accounting/components/supplier-cost-catalog-filters.tsx web/src/features/cost-accounting/components/supplier-cost-catalog-columns.tsx web/src/features/cost-accounting/components/supplier-cost-catalog-mobile.tsx web/src/features/cost-accounting/components/supplier-cost-catalog-summary.tsx web/src/features/cost-accounting/components/supplier-cost-catalog-pagination.tsx web/src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx
git commit -m "feat(cost-accounting): build supplier cost catalog view"
```

### Task 7: 增加只读详情抽屉和双范围导出

**Files:**
- Create: `web/src/features/cost-accounting/components/supplier-cost-detail-drawer.tsx`
- Create: `web/src/features/cost-accounting/components/__tests__/supplier-cost-detail-drawer.test.tsx`
- Modify: `web/src/features/cost-accounting/components/supplier-cost-catalog.tsx`
- Modify: `web/src/features/cost-accounting/components/supplier-cost-catalog-filters.tsx`
- Modify: `web/src/features/cost-accounting/components/supplier-cost-catalog-mobile.tsx`
- Modify: `web/src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx`

- [ ] **Step 1: 编写详情和导出失败测试**

详情测试覆盖：

```ts
test('loads complete price components and conversion parameters on open')
test('renders invalid configuration as unavailable without raw JSON')
test('renders version history newest first')
test('retries only the failed detail request')
test('opens the selected rule from keyboard activation')
test('returns focus to the originating row after close')
```

目录测试再覆盖：

```ts
test('exports all filtered rows without page parameters')
test('exports all supplier costs without current filters')
test('disables only the export action currently in progress')
test('downloads the blob and reports the exported row count')
test('shows an actionable error when export fails')
```

- [ ] **Step 2: 运行新增测试并确认失败**

Run: `cd web && bun test src/features/cost-accounting/components/__tests__/supplier-cost-detail-drawer.test.tsx src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx`

Expected: FAIL，详情抽屉和导出 mutation 尚未实现。

- [ ] **Step 3: 实现只读 Sheet 详情**

使用现有 `Sheet` 与 `sideDrawer*ClassName`，必须包含 `SheetTitle` 和 `SheetDescription`：

```tsx
<Sheet open={props.ruleId !== null} onOpenChange={props.onOpenChange}>
  <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
    <SheetHeader className={sideDrawerHeaderClassName()}>
      <SheetTitle>{t('Supplier cost rule details')}</SheetTitle>
      <SheetDescription>{description}</SheetDescription>
    </SheetHeader>
    <div className={sideDrawerFormClassName()}>{content}</div>
  </SheetContent>
</Sheet>
```

内容分成价格组成、换算参数、计费语义、元数据和规则历史五个 `SideDrawerSection`。只展示结构化 `config` 与 DTO 字段，不能显示原始 JSON。加载、错误和重试只影响抽屉。关闭后由父组件保存的触发元素引用恢复焦点。

父组件通过 `DataTablePage.renderRow` 给桌面行增加点击、Enter 和 Space 打开详情；移动列表把行主按钮连接到同一 `openRule(ruleId, triggerElement)`，扩展图标仍只控制 `Collapsible`。打开时保存触发元素，关闭时显式调用 `focus()`，不能依赖没有 `SheetTrigger` 的隐式恢复。

- [ ] **Step 4: 实现两个独立导出 mutation**

目录组件维护 `exportingScope: 'filtered' | 'all' | null`，调用 `exportSupplierCostCatalog`。filtered 参数来自当前筛选和排序并明确删除 `page/page_size`；all 只传排序与 `scope=all`。成功后：

```ts
downloadCostCatalogExport(result)
toast.success(t('Export completed: {{count}} rows', { count: result.rowCount }))
```

失败调用 `toast.error(t('Export failed'))`。只有 `exportingScope` 对应的按钮禁用并显示 `Spinner`；刷新和另一导出按钮保持可用。

- [ ] **Step 5: 运行组件测试、类型检查和 lint**

Run:

```bash
cd web
bun test src/features/cost-accounting/components/__tests__/supplier-cost-detail-drawer.test.tsx src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/cost-accounting/components/supplier-cost-detail-drawer.tsx src/features/cost-accounting/components/supplier-cost-catalog.tsx src/features/cost-accounting/components/supplier-cost-catalog-filters.tsx src/features/cost-accounting/components/supplier-cost-catalog-mobile.tsx
```

Expected: PASS。

- [ ] **Step 6: 提交详情和导出交互**

```bash
git add web/src/features/cost-accounting/components/supplier-cost-detail-drawer.tsx web/src/features/cost-accounting/components/__tests__/supplier-cost-detail-drawer.test.tsx web/src/features/cost-accounting/components/supplier-cost-catalog.tsx web/src/features/cost-accounting/components/supplier-cost-catalog-filters.tsx web/src/features/cost-accounting/components/supplier-cost-catalog-mobile.tsx web/src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx
git commit -m "feat(cost-accounting): inspect and export supplier costs"
```

### Task 8: 接入第三个 Tab 并完成七种语言

**Required skill:** `i18n-translate`，语言文件只能通过脚本写入。

**Files:**
- Modify: `web/src/features/cost-accounting/index.tsx`
- Modify: `web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx`
- Temporary create/delete: `web/scripts/add-missing-keys.mjs`
- Modify via script: `web/src/i18n/locales/en.json`
- Modify via script: `web/src/i18n/locales/zh.json`
- Modify via script: `web/src/i18n/locales/zh-TW.json`
- Modify via script: `web/src/i18n/locales/fr.json`
- Modify via script: `web/src/i18n/locales/ja.json`
- Modify via script: `web/src/i18n/locales/ru.json`
- Modify via script: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: 扩展页面失败测试**

在 `profit-report.test.tsx` 增加：

```ts
test('renders supplier catalog as the third URL-driven tab')
test('does not request supplier catalog while profit or anomalies is active')
test('preserves profit filters when switching to and from catalog')
```

Mock 目录 API，不 Mock `SupplierCostCatalog`；断言 `tab=catalog` 才发出目录请求，从而覆盖真实 `enabled` 传递。

- [ ] **Step 2: 运行页面测试并确认失败**

Run: `cd web && bun test src/features/cost-accounting/components/__tests__/profit-report.test.tsx`

Expected: FAIL，页面仍只有两个 Tab。

- [ ] **Step 3: 接入第三个 Tab**

在 `index.tsx`：

```tsx
<TabsTrigger value='profit'>{t('Profit report')}</TabsTrigger>
<TabsTrigger value='catalog'>{t('Supplier cost catalog')}</TabsTrigger>
<TabsTrigger value='anomalies'>{t('Anomalies')}</TabsTrigger>
```

`onValueChange` 接受三个枚举；新增 `TabsContent value='catalog'`，只渲染：

```tsx
<SupplierCostCatalog
  enabled={tab === 'catalog'}
  search={search}
  onSearchChange={updateSearch}
/>
```

利润 summary/breakdown 继续只在 `tab === 'profit'` 时启用，异常队列继续只在 `tab === 'anomalies'` 时启用。切换 Tab 不删除其他 Tab 的查询参数。

不修改侧边栏配置，也不在渠道页面增加首版入口。

- [ ] **Step 4: 运行 i18n 预检并创建唯一允许的写入脚本**

Run: `cd web && bun run i18n:sync`

Expected: 报告列出新目录键缺失，现有 JSON 仍有效。

创建 `web/scripts/add-missing-keys.mjs`，使用 `i18n-translate` 技能规定的 `stableStringify`、七语言 `newKeys` 和按键排序写入逻辑。脚本中的实际翻译必须完全来自下表：

| Key | zh | zh-TW | fr | ja | ru | vi |
| --- | --- | --- | --- | --- | --- | --- |
| Supplier cost catalog | 供应商成本目录 | 供應商成本目錄 | Catalogue des coûts fournisseurs | サプライヤーコスト一覧 | Каталог затрат поставщиков | Danh mục chi phí nhà cung cấp |
| Active rules | 活动规则 | 啟用中規則 | Règles actives | 有効なルール | Активные правила | Quy tắc đang hoạt động |
| Draft rules | 草稿规则 | 草稿規則 | Règles en brouillon | 下書きルール | Черновики правил | Quy tắc nháp |
| Retired rules | 已退休规则 | 已停用規則 | Règles retirées | 廃止済みルール | Архивные правила | Quy tắc đã ngừng |
| Export current results | 导出当前结果 | 匯出目前結果 | Exporter les résultats actuels | 現在の結果をエクスポート | Экспортировать текущие результаты | Xuất kết quả hiện tại |
| Export all supplier costs | 导出全部供应商成本 | 匯出全部供應商成本 | Exporter tous les coûts fournisseurs | すべてのサプライヤーコストをエクスポート | Экспортировать все затраты поставщиков | Xuất toàn bộ chi phí nhà cung cấp |
| Billing semantics | 计费语义 | 計費語意 | Règles de facturation | 課金ルール | Правила тарификации | Quy tắc tính phí |
| Effective period | 有效期 | 有效期間 | Période de validité | 有効期間 | Период действия | Thời gian hiệu lực |
| No supplier cost rules | 暂无供应商成本规则 | 暫無供應商成本規則 | Aucune règle de coût fournisseur | サプライヤーコストルールがありません | Нет правил затрат поставщиков | Không có quy tắc chi phí nhà cung cấp |
| Failed to load supplier costs | 加载供应商成本失败 | 載入供應商成本失敗 | Échec du chargement des coûts fournisseurs | サプライヤーコストの読み込みに失敗しました | Не удалось загрузить затраты поставщиков | Không thể tải chi phí nhà cung cấp |
| Supplier cost rule details | 供应商成本规则详情 | 供應商成本規則詳情 | Détails de la règle de coût fournisseur | サプライヤーコストルールの詳細 | Сведения о правиле затрат поставщика | Chi tiết quy tắc chi phí nhà cung cấp |
| 15-second equivalent USD/second (comparison only) | 15 秒等效 USD/秒（仅比较） | 15 秒等效 USD/秒（僅供比較） | Équivalent USD/seconde sur 15 secondes (comparaison uniquement) | 15秒換算 USD/秒（比較専用） | Эквивалент USD/с для 15 секунд (только для сравнения) | USD/giây quy đổi theo 15 giây (chỉ để so sánh) |
| Price components | 价格组成 | 價格組成 | Composants du prix | 価格構成 | Составляющие цены | Thành phần giá |
| Conversion parameters | 换算参数 | 換算參數 | Paramètres de conversion | 換算パラメーター | Параметры пересчёта | Tham số quy đổi |
| Rule history | 规则历史 | 規則歷史 | Historique des règles | ルール履歴 | История правил | Lịch sử quy tắc |
| No rule history | 暂无规则历史 | 暫無規則歷史 | Aucun historique de règle | ルール履歴がありません | История правил отсутствует | Không có lịch sử quy tắc |
| Export completed: {{count}} rows | 已导出 {{count}} 行 | 已匯出 {{count}} 筆 | Export terminé : {{count}} lignes | {{count}} 行をエクスポートしました | Экспортировано строк: {{count}} | Đã xuất {{count}} dòng |
| Export failed | 导出失败 | 匯出失敗 | Échec de l’export | エクスポートに失敗しました | Ошибка экспорта | Xuất thất bại |
| Channel count | 渠道数 | 渠道數 | Nombre de canaux | チャネル数 | Количество каналов | Số kênh |
| Effective from | 生效时间 | 生效時間 | En vigueur à partir du | 有効開始 | Действует с | Có hiệu lực từ |
| Effective to | 失效时间 | 失效時間 | En vigueur jusqu’au | 有効終了 | Действует до | Có hiệu lực đến |

`en` 对象中 key 与 value 完全相同。保留 `{{count}}` 插值变量，不修改现有翻译值。

- [ ] **Step 5: 通过脚本写入、同步并删除临时脚本**

Run:

```powershell
Set-Location web
node scripts/add-missing-keys.mjs
bun run i18n:sync
Remove-Item -LiteralPath 'scripts/add-missing-keys.mjs'
```

Expected: 七个 locale 的新增/更新数量一致；`_reports/_sync-report.json` 中每种语言 `missingCount` 为 `0`。临时脚本不进入 Git。

- [ ] **Step 6: 运行成本核算前端测试和构建检查**

Run:

```bash
cd web
bun test src/features/cost-accounting/components/__tests__
bun run typecheck
bun run i18n:sync
bun run build
```

Expected: 全部 PASS。

- [ ] **Step 7: 提交 Tab 和国际化**

```bash
git add web/src/features/cost-accounting/index.tsx web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat(cost-accounting): integrate supplier catalog tab"
```

### Task 9: 完成权限 E2E、全量验证和视觉验收

**Files:**
- Create: `e2e/supplier_cost_catalog_e2e_test.go`

- [ ] **Step 1: 添加管理员/普通用户 E2E 验收测试**

复用 `e2e/supplier_confidential_log_boundary_e2e_test.go` 同包中的用户、管理员、渠道和请求辅助，新增测试：

```go
func TestSupplierCostCatalogAuthorizationAndExportE2E(t *testing.T) {
	seedConfidentialBoundaryData(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelModelCostRule{}))

	config, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
		UnitPrice: common.GetPointer("3"), ChargeEvent: types.CostChargeTaskSucceeded,
	})
	require.NoError(t, err)
	configJSON, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: 40, BillableUpstreamModel: confidentialBoundaryModel,
		CostVariantKey: "default", Version: 1, Status: string(types.CostRuleActive),
		CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual",
	}).Error)

	engine := gin.New()
	engine.GET("/api/cost-accounting/catalog", middleware.AdminAuth(), middleware.RequirePermission(authz.CostAccountingRead), controller.ListSupplierCostCatalog)
	engine.GET("/api/cost-accounting/catalog/export", middleware.AdminAuth(), middleware.RequirePermission(authz.CostAccountingRead), controller.ExportSupplierCostCatalog)

	userStatus, userBody := performConfidentialBoundaryRequest(t, engine, "/api/cost-accounting/catalog", confidentialBoundaryUserPAT)
	assert.Equal(t, http.StatusForbidden, userStatus, string(userBody))

	adminStatus, adminBody := performConfidentialBoundaryRequest(t, engine, "/api/cost-accounting/catalog?status=active&page=1&page_size=50", confidentialBoundaryAdminPAT)
	require.Equal(t, http.StatusOK, adminStatus, string(adminBody))
	assert.Contains(t, string(adminBody), confidentialBoundaryChannel)
	assert.Contains(t, string(adminBody), confidentialBoundaryModel)
	assert.NotContains(t, string(adminBody), "supplier-api-key-secret")
	assert.NotContains(t, string(adminBody), "https://supplier.example")

	exportStatus, exportBody := performConfidentialBoundaryRequest(t, engine, "/api/cost-accounting/catalog/export?scope=all", confidentialBoundaryAdminPAT)
	require.Equal(t, http.StatusOK, exportStatus, string(exportBody))
	assert.True(t, bytes.HasPrefix(exportBody, []byte{0xEF, 0xBB, 0xBF}))
	assert.NotContains(t, string(exportBody), "supplier-api-key-secret")
	assert.NotContains(t, string(exportBody), "https://supplier.example")
}
```

为该文件补齐 `bytes`、controller、middleware、service、authz 和 types 导入，禁止复制真实密钥或真实供应商数据。

- [ ] **Step 2: 运行 E2E 验收测试**

Run: `go test ./e2e -run TestSupplierCostCatalogAuthorizationAndExportE2E -count=1`

Expected: PASS。若失败，修复只能落在本计划已列文件中；若需要改变设计，先更新 spec 和 plan。

- [ ] **Step 3: 运行后端完整相关测试**

Run:

```bash
go test ./model ./service ./controller ./router -count=1
go test ./e2e -run 'TestSupplierCostCatalogAuthorizationAndExportE2E|TestSupplierConfidentialLogBoundaryE2E' -count=1
```

Expected: PASS；MySQL/PostgreSQL 配置化测试在未配置 DSN 时明确 SKIP。

- [ ] **Step 4: 运行前端完整相关验证**

Run:

```bash
cd web
bun test src/features/cost-accounting
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/cost-accounting src/routes/_authenticated/cost-accounting/index.tsx
bun run format:check
bun run i18n:sync
bun run build
```

Expected: 全部 PASS，七种语言 missingCount 为 0。

- [ ] **Step 5: 启动隔离前端并进行桌面/移动视觉验收**

在后端 `http://localhost:3000` 正常运行时，从 worktree 的 `web/` 启动：

Run: `bun run dev -- --port 3001`

使用 `browser:control-in-app-browser` 技能打开 `http://localhost:3001/cost-accounting?tab=catalog`，复用已有管理员会话完成：

1. 1440x900：检查摘要、六筛选器、固定渠道/模型列、横向滚动、详情抽屉和两种导出；
2. 390x844：检查紧凑列表、展开按钮、详情按钮、长模型名换行和无重叠；
3. 键盘：Tab 到表格行，Enter 打开，Esc 关闭并确认焦点返回；
4. 空状态：使用一个不存在的模型筛选，验证空结果和清除筛选；错误/Retry 已由 Task 6 组件测试覆盖；
5. 下载：打开 filtered/all CSV，核对 UTF-8 中文、行数、未知值为空和“仅比较”列。

截图和下载产物保存在任务输出目录，不提交到仓库。完成后停止该 worktree 的开发服务器。

- [ ] **Step 6: 执行最终差异与严格模式隔离检查**

Run:

```bash
git diff --check
rg -n "comparison_15s_equivalent_usd_per_second" dto service web/src
go test ./service -run 'Profit|Routing|CostCatalog' -count=1
```

再用 PowerShell 断言禁止目录不存在任何命中：

```powershell
$forbiddenMatches = rg -n 'comparison_15s_equivalent_usd_per_second' relay middleware model 2>$null
if ($LASTEXITCODE -eq 0) {
  $forbiddenMatches
  throw 'comparison-only price leaked into routing or settlement code'
}
Write-Output 'NO_ROUTING_OR_SETTLEMENT_MATCHES'
```

Expected: 第一条 `rg` 只命中目录 DTO、目录服务、导出和前端；PowerShell 输出 `NO_ROUTING_OR_SETTLEMENT_MATCHES`，证明字段不在 `profit_routing.go`、`model_routing.go`、attempt 快照或结算输入；测试 PASS。

- [ ] **Step 7: 提交 E2E 并进行完成前审查**

```bash
git add e2e/supplier_cost_catalog_e2e_test.go
git commit -m "test(cost-accounting): verify supplier catalog boundary"
git status --short
```

Expected: 提交成功，最终 `git status --short` 无输出。

使用 `superpowers:requesting-code-review` 检查权限、CSV 注入、跨数据库查询、按次完整成本、响应式布局和缺失测试；修复发现后重新运行 Step 3-6。最后使用 `superpowers:verification-before-completion` 核对最新命令输出，再进入 `superpowers:finishing-a-development-branch`。
