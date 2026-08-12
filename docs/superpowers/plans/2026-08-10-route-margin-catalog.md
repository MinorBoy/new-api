# 路由毛利目录实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在成本核算页面新增可按最低毛利率筛选的路由毛利目录，默认复现 4 秒、分组倍率 1、`no_video/with_video` 矩阵，并支持高级场景参数和 CSV 导出。

**Architecture:** 后端新增独立的路由目标投影查询和场景计算服务，批量加载活动成本规则，并复用现有 `PreviewRoutingRevenue`、`EstimateSeedanceTokens`、`CalculateAttemptCost` 与 `EvaluateProfitEligibility` 口径。前端新增独立标签页，将筛选条件持久化到 URL，通过 TanStack Query 查询分页结果；现有供应商成本目录保持不变。

**Tech Stack:** Go 1.22、Gin、GORM v2、testify、React 19、TypeScript、TanStack Router/Query/Table、Base UI、Tailwind CSS、Bun、i18next。

---

## 文件结构

- 新建 `dto/route_margin_catalog.go`：API 请求结果、汇总和场景枚举。
- 新建 `model/route_margin_catalog.go`：读取当前启用的 `config_import` 路由目标及关联策略、渠道。
- 新建 `model/route_margin_catalog_test.go`：跨数据库兼容的 GORM 查询行为测试。
- 修改 `service/profit_routing.go`：给现有收入预览输入增加可选分组倍率覆盖值。
- 修改 `relay/helper/cost_preview.go`、`relay/helper/profit_preview.go`、`main.go`：在目录预览时应用倍率覆盖值，真实请求保持 `nil` 路径。
- 新建 `service/route_margin_catalog.go`：参数归一化、场景展开、收入/成本/毛利计算、筛选、排序、分页和汇总。
- 新建 `service/route_margin_catalog_test.go`：覆盖阈值、成本模式、场景、缺失规则和分页排序。
- 新建 `service/route_margin_catalog_export.go`、`service/route_margin_catalog_export_test.go`：CSV 导出。
- 新建 `controller/route_margin_catalog.go`、`controller/route_margin_catalog_test.go`：查询参数解析、HTTP 400 和导出响应。
- 修改 `router/cost-accounting-router.go`、`router/cost_accounting_router_test.go`：注册只读接口并验证权限。
- 修改 `web/src/features/cost-accounting/types.ts`、`api.ts`、`lib/report.ts`：前端数据契约、查询键和 URL 状态。
- 新建 `web/src/features/cost-accounting/lib/route-margin-catalog.ts` 及测试：URL 参数映射、筛选重置和格式化。
- 新建 `web/src/features/cost-accounting/components/route-margin-catalog*.tsx` 及测试：筛选、表格、移动端、分页和导出。
- 修改 `web/src/features/cost-accounting/index.tsx` 和路由搜索 schema：接入“路由毛利”标签页。
- 修改七个 `web/src/i18n/locales/*.json`：补全新增界面文案。
- 新建 `docs/superpowers/reports/2026-08-10-route-margin-catalog-acceptance.md`：记录本机 30% 矩阵复核结果。

### Task 1：建立活动路由目标查询和 API 契约

**Files:**
- Create: `dto/route_margin_catalog.go`
- Create: `model/route_margin_catalog.go`
- Create: `model/route_margin_catalog_test.go`

- [ ] **Step 1：先写失败的模型查询测试**

测试夹具创建：一个已启用且未退役的 `config_import` 目标、一个手工目标、一个已退役目标、一个策略关闭目标，并验证只返回第一个目标；同时验证渠道名、策略模型、默认值和约束 JSON 被投影。

```go
func TestListActiveImportedRouteMarginTargets(t *testing.T) {
    setupRouteMarginCatalogDB(t)
    active := seedRouteMarginTarget(t, routeMarginTargetFixture{ManagedBy: "config_import", Enabled: true})
    seedRouteMarginTarget(t, routeMarginTargetFixture{ManagedBy: "manual", Enabled: true})
    retiredAt := int64(1700000000)
    seedRouteMarginTarget(t, routeMarginTargetFixture{ManagedBy: "config_import", Enabled: false, RetiredAt: &retiredAt})

    rows, err := ListActiveImportedRouteMarginTargets(RouteMarginTargetQuery{})

    require.NoError(t, err)
    require.Len(t, rows, 1)
    assert.Equal(t, active.ID, rows[0].TargetID)
    assert.Equal(t, "doubao-seedance-2-0-260128", rows[0].CanonicalModel)
    assert.Equal(t, "channel-a", rows[0].ChannelName)
}
```

- [ ] **Step 2：运行测试并确认失败**

Run: `go test ./model -run TestListActiveImportedRouteMarginTargets -count=1`

Expected: FAIL，提示 `ListActiveImportedRouteMarginTargets` 或相关类型未定义。

- [ ] **Step 3：实现 DTO 和 GORM 投影查询**

DTO 使用整数 nano-USD/PPM，避免浮点金额进入 API 契约：

```go
type RouteMarginCatalogItem struct {
    TargetID                    int    `json:"target_id"`
    TargetName                  string `json:"target_name"`
    PolicyID                    int    `json:"policy_id"`
    GroupName                   string `json:"group_name"`
    CanonicalModel              string `json:"canonical_model"`
    ChannelID                   int    `json:"channel_id"`
    ChannelName                 string `json:"channel_name"`
    ChannelType                 int    `json:"channel_type"`
    UpstreamModel               string `json:"upstream_model"`
    CostVariantKey              string `json:"cost_variant_key"`
    Resolution                  string `json:"resolution"`
    DurationSeconds             int    `json:"duration_seconds"`
    Scenario                    string `json:"scenario"`
    GroupRatio                  string `json:"group_ratio"`
    CostMode                    types.CostMode `json:"cost_mode,omitempty"`
    RuleID                      int64  `json:"rule_id,omitempty"`
    RuleVersion                 int    `json:"rule_version,omitempty"`
    EstimatedRevenueNanoUSD     *int64 `json:"estimated_revenue_nano_usd,omitempty"`
    EstimatedCostNanoUSD        *int64 `json:"estimated_cost_nano_usd,omitempty"`
    EstimatedProfitNanoUSD      *int64 `json:"estimated_profit_nano_usd,omitempty"`
    GrossMarginPPM              *int64 `json:"gross_margin_ppm,omitempty"`
    RequestedMinimumMarginPPM   int64  `json:"requested_minimum_margin_ppm"`
    ConfiguredMinimumMarginBPS  *int   `json:"configured_minimum_margin_bps,omitempty"`
    Eligible                    bool   `json:"eligible"`
    FailureReason               string `json:"failure_reason,omitempty"`
    CostSource                  string `json:"cost_source,omitempty"`
    RevenueSource               string `json:"revenue_source"`
}
```

模型查询只使用 GORM 和参数绑定，条件固定为：策略启用、目标启用、`retired_at IS NULL`、`managed_by = config_import`。渠道使用 `LEFT JOIN`，缺失渠道仍返回，以便服务层给出失败原因。

```go
func ListActiveImportedRouteMarginTargets(query RouteMarginTargetQuery) ([]RouteMarginTargetRow, error) {
    policyTable := DB.NamingStrategy.TableName("RoutingPolicy")
    targetTable := DB.NamingStrategy.TableName("RouteTarget")
    channelTable := DB.NamingStrategy.TableName("Channel")
    db := DB.Table(targetTable+" AS targets").
        Select("targets.id AS target_id, targets.name AS target_name, targets.policy_id, " +
            "targets.channel_id, targets.upstream_model, targets.cost_variant_key, " +
            "targets.minimum_expected_margin_bps, targets.constraints, " +
            "policies.group_name, policies.model AS canonical_model, " +
            "policies.default_resolution, policies.default_duration, policies.default_ratio, " +
            "COALESCE(channels.name, '') AS channel_name, COALESCE(channels.type, 0) AS channel_type").
        Joins("JOIN "+policyTable+" AS policies ON policies.id = targets.policy_id").
        Joins("LEFT JOIN "+channelTable+" AS channels ON channels.id = targets.channel_id").
        Where("policies.enabled = ? AND targets.enabled = ?", true, true).
        Where("targets.retired_at IS NULL AND targets.managed_by = ?", string(types.RouteTargetManagedByConfigImport))
    // channel/model/target/resolution filters are appended with bound parameters.
    return rows, db.Order("targets.id ASC").Scan(&rows).Error
}
```

- [ ] **Step 4：运行模型测试**

Run: `go test ./model -run 'TestListActiveImportedRouteMarginTargets' -count=1`

Expected: PASS。

- [ ] **Step 5：提交查询契约**

```bash
git add dto/route_margin_catalog.go model/route_margin_catalog.go model/route_margin_catalog_test.go
git commit -m "feat: add route margin target query"
```

### Task 2：让收入预览支持只读分组倍率覆盖

**Files:**
- Modify: `service/profit_routing.go`
- Modify: `relay/helper/cost_preview.go`
- Modify: `relay/helper/profit_preview.go`
- Modify: `relay/helper/profit_preview_test.go`
- Modify: `main.go`
- Modify: `cmd/ark-video-material-seed/main.go`

- [ ] **Step 1：先写失败的倍率覆盖测试**

在现有收入预览测试中，用相同模型和场景分别传 `nil`、`1`、`1.5`，断言覆盖值只改变最终收入，不改变默认路径。

```go
func TestPreviewRoutingRevenueAppliesExplicitGroupRatio(t *testing.T) {
    configurePreviewPricing(t)
    one := 1.0
    oneAndHalf := 1.5
    duration := 4

    base, _, err := PreviewRoutingRevenueWithSeedanceInputAndGroupRatio(
        modelrouting.Seedance20, "default", "/v1/video/generations",
        relayconstant.RelayModeVideoSubmit, &duration, 0, "720p", false, 0, &one,
    )
    require.NoError(t, err)
    increased, _, err := PreviewRoutingRevenueWithSeedanceInputAndGroupRatio(
        modelrouting.Seedance20, "default", "/v1/video/generations",
        relayconstant.RelayModeVideoSubmit, &duration, 0, "720p", false, 0, &oneAndHalf,
    )
    require.NoError(t, err)
    assert.Equal(t, decimal.NewFromInt(base).Mul(decimal.RequireFromString("1.5")).IntPart(), increased)
}
```

- [ ] **Step 2：运行测试并确认失败**

Run: `go test ./relay/helper -run TestPreviewRoutingRevenueAppliesExplicitGroupRatio -count=1`

Expected: FAIL，提示覆盖函数未定义。

- [ ] **Step 3：实现可选覆盖值，保持真实请求路径不变**

给 `RoutingRevenuePreviewInput` 增加 `GroupRatioOverride *float64`。现有调用不赋值，行为保持不变。新增 helper 包装函数，并在 `ModelPriceHelper`/`ModelPriceHelperPerCall` 已经构建 `info.PriceData` 后覆盖：

```go
if groupRatioOverride != nil {
    ratio := *groupRatioOverride
    if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 {
        return 0, "", fmt.Errorf("group ratio override must be a positive finite number")
    }
    info.PriceData.GroupRatioInfo.GroupRatio = ratio
    info.PriceData.GroupRatioInfo.GroupSpecialRatio = 0
    info.PriceData.GroupRatioInfo.HasSpecialRatio = false
}
```

`main.go` 和种子命令的 hook 将 `input.GroupRatioOverride` 传给新函数；旧的 `PreviewRoutingRevenue` 和 `PreviewRoutingRevenueWithSeedanceInput` 继续传 `nil`。

- [ ] **Step 4：运行收入预览和利润路由回归**

Run: `go test ./relay/helper ./service -run 'TestPreviewRoutingRevenue|TestEvaluateProfitEligibility|TestFilterProfitEligibleChannels' -count=1`

Expected: PASS，原有无覆盖路径断言不变。

- [ ] **Step 5：提交倍率覆盖能力**

```bash
git add service/profit_routing.go relay/helper/cost_preview.go relay/helper/profit_preview.go relay/helper/profit_preview_test.go main.go cmd/ark-video-material-seed/main.go
git commit -m "feat: support route margin group ratio preview"
```

### Task 3：实现路由毛利计算、筛选、排序和汇总

**Files:**
- Create: `service/route_margin_catalog.go`
- Create: `service/route_margin_catalog_test.go`

- [ ] **Step 1：先写服务层行为测试**

使用确定性夹具和注入的收入预览 hook，覆盖：阈值等于 30% 时通过；29.9999% 不通过；`per_request`、`per_duration`、`per_token` 三种成本；两个场景分别通过/失败；缺失成本规则；非法参数；按毛利率降序和分页。

```go
func TestListRouteMarginCatalogFiltersAtInclusiveThreshold(t *testing.T) {
    fixture := setupRouteMarginCatalogService(t)
    fixture.SetRevenueNanoUSD(1000)
    fixture.SeedPerRequestCostNanoUSD(700)

    page, err := ListRouteMarginCatalog(context.Background(), RouteMarginCatalogFilter{
        MinimumMarginPPM: 300000,
        DurationSeconds:  4,
        GroupRatio:       1,
        Scenario:         RouteMarginScenarioNoVideo,
        Page:             1,
        PageSize:         50,
        SortBy:           "gross_margin_ppm",
        SortOrder:        "desc",
    })

    require.NoError(t, err)
    require.Len(t, page.Items, 1)
    assert.True(t, page.Items[0].Eligible)
    assert.Equal(t, int64(300000), *page.Items[0].GrossMarginPPM)
}
```

- [ ] **Step 2：运行测试并确认失败**

Run: `go test ./service -run 'TestListRouteMarginCatalog' -count=1`

Expected: FAIL，提示目录服务未定义。

- [ ] **Step 3：实现参数归一化和场景展开**

归一化规则。HTTP 控制器负责在参数缺失时传入默认毛利率、时长和分组倍率，因此服务层收到的 `0` 始终是显式输入，不会被默认值覆盖：

```go
const (
    defaultRouteMarginPPM      int64 = 300000
    defaultRouteMarginDuration      = 4
    maxRouteMarginGroupRatio         = 100
)

func normalizeRouteMarginCatalogFilter(filter RouteMarginCatalogFilter) (RouteMarginCatalogFilter, error) {
    if filter.MinimumMarginPPM < -1_000_000 || filter.MinimumMarginPPM > 1_000_000 {
        return filter, errors.New("minimum margin must be between -100% and 100%")
    }
    if filter.DurationSeconds < 1 || filter.DurationSeconds > relaycommon.MaxTaskDurationSeconds {
        return filter, errors.New("duration is out of range")
    }
    if math.IsNaN(filter.GroupRatio) || math.IsInf(filter.GroupRatio, 0) || filter.GroupRatio <= 0 || filter.GroupRatio > maxRouteMarginGroupRatio {
        return filter, errors.New("group ratio is out of range")
    }
    // page_size 只允许 25、50、100；scenario 只允许 all/no_video/with_video。
    return filter, nil
}
```

未指定分辨率时，对每个目标约束中的分辨率生成行；指定分辨率时只保留支持该值的目标。`scenario=all` 生成两行。`with_video` 设置 `HasReferenceVideo=true`，静态矩阵使用 4000ms（4 秒）的代表性参考视频时长，以复现已批准的统计口径；真实请求仍会使用实际元数据时长。

- [ ] **Step 4：实现批量规则加载和精确计算**

先把所有目标转换为 `CostRuleCandidate`，一次调用 `ActiveCostRules(candidates, true)`。每个场景先调用 `PreviewRoutingRevenue`，再构造 `ProfitRoutingFacts`、调用 `EstimateSeedanceTokens`，最后复用 `evaluateCandidateProfit`：

```go
evaluation := evaluateCandidateProfit(ProfitChannelFilterInput{
    Ctx: ctx,
    Facts: facts,
    RevenueNanoUSD: revenueNanoUSD,
    HasRevenue: revenueErr == nil,
    // evaluator 用最低可选阈值完成金额计算；目录用 PPM 做最终精确比较。
    GlobalMarginBPS: -10000,
}, rules, ProfitRoutingCandidate{
    ChannelID: row.ChannelID,
    PredictedUpstreamModel: row.UpstreamModel,
    CostVariantKey: row.CostVariantKey,
}, 0, false, nil)
```

因为筛选阈值以 PPM 接收，不能丢失非整 BPS 精度：成本和利润字段沿用 evaluator，最终 `eligible` 必须再用 `*GrossMarginPPM >= filter.MinimumMarginPPM` 判断；金额可用但低于请求阈值时写入 `margin_below_threshold`。缺少收入、规则、计量或计算失败时保留行并写入稳定原因码。`RevenueSource` 固定为 `runtime_billing_settings`，`CostSource` 取活动成本规则的 `Source`。

- [ ] **Step 5：实现结果状态筛选、稳定排序、分页和按目标汇总**

`status` 取值为 `all`、`eligible`、`ineligible`，表示计算结果状态。排序字段支持 `target_name`、`channel_name`、`upstream_model`、`gross_margin_ppm`、`estimated_profit_nano_usd`，空金额始终排在末尾，并用 `target_id + resolution + scenario` 作为稳定次序。

汇总在状态过滤前按当前场景条件计算：目标数、场景行数、至少一场景通过、全场景通过、部分场景通过、全场景不通过、通过场景行数。这样默认查询可以直接复核 `75/60/15/81/135`。

- [ ] **Step 6：运行服务测试**

Run: `go test ./service -run 'TestListRouteMarginCatalog|TestNormalizeRouteMarginCatalogFilter' -count=1`

Expected: PASS。

- [ ] **Step 7：提交计算服务**

```bash
git add service/route_margin_catalog.go service/route_margin_catalog_test.go
git commit -m "feat: calculate route margin catalog"
```

### Task 4：增加查询、导出接口和权限路由

**Files:**
- Create: `service/route_margin_catalog_export.go`
- Create: `service/route_margin_catalog_export_test.go`
- Create: `controller/route_margin_catalog.go`
- Create: `controller/route_margin_catalog_test.go`
- Modify: `router/cost-accounting-router.go`
- Modify: `router/cost_accounting_router_test.go`

- [ ] **Step 1：先写失败的控制器和 CSV 测试**

控制器测试验证默认值、`min_margin_ppm=0` 的显式零值、非法时长/倍率/排序返回 400；CSV 测试验证 BOM、列顺序、金额和毛利率格式、失败原因。

```go
func TestRouteMarginCatalogFilterFromQueryDefaults(t *testing.T) {
    c, _ := gin.CreateTestContext(httptest.NewRecorder())
    c.Request = httptest.NewRequest(http.MethodGet, "/?status=eligible", nil)

    filter, err := routeMarginCatalogFilterFromQuery(c)

    require.NoError(t, err)
    assert.Equal(t, int64(300000), filter.MinimumMarginPPM)
    assert.Equal(t, 4, filter.DurationSeconds)
    assert.Equal(t, 1.0, filter.GroupRatio)
    assert.Equal(t, "all", filter.Scenario)
}
```

- [ ] **Step 2：运行测试并确认失败**

Run: `go test ./controller ./service -run 'TestRouteMarginCatalog|TestWriteRouteMarginCatalogCSV' -count=1`

Expected: FAIL，提示控制器或导出函数未定义。

- [ ] **Step 3：实现查询和导出控制器**

注册：

```go
{method: http.MethodGet, path: "/route-margin-catalog", permission: authz.CostAccountingRead, handler: controller.ListRouteMarginCatalog},
{method: http.MethodGet, path: "/route-margin-catalog/export", permission: authz.CostAccountingRead, handler: controller.ExportRouteMarginCatalog},
```

查询参数严格解析，文本字段去空白并限制长度；导出复用相同 filter，将分页移除，输出 `route-margin-catalog-YYYYMMDD-HHMMSS.csv` 和中文兼容文件名，并设置 `X-Exported-Row-Count`。

- [ ] **Step 4：运行接口和路由测试**

Run: `go test ./controller ./router ./service -run 'TestRouteMarginCatalog|TestCostAccountingPermissionRoutes|TestWriteRouteMarginCatalogCSV' -count=1`

Expected: PASS。

- [ ] **Step 5：提交 API**

```bash
git add service/route_margin_catalog_export.go service/route_margin_catalog_export_test.go controller/route_margin_catalog.go controller/route_margin_catalog_test.go router/cost-accounting-router.go router/cost_accounting_router_test.go
git commit -m "feat: expose route margin catalog api"
```

### Task 5：建立前端类型、URL 状态和 API 客户端

**Files:**
- Modify: `web/src/features/cost-accounting/types.ts`
- Modify: `web/src/features/cost-accounting/api.ts`
- Modify: `web/src/features/cost-accounting/lib/report.ts`
- Modify: `web/src/routes/_authenticated/cost-accounting/index.tsx`
- Create: `web/src/features/cost-accounting/lib/route-margin-catalog.ts`
- Create: `web/src/features/cost-accounting/lib/__tests__/route-margin-catalog.test.ts`

- [ ] **Step 1：先写失败的 URL 参数映射测试**

```ts
test('maps route margin URL state to API parameters', () => {
  assert.deepEqual(
    routeMarginParamsFromSearch({
      tab: 'route-margin',
      marginMinimumPercent: 30,
      marginDurationSeconds: 4,
      marginGroupRatio: 1.25,
      marginScenario: 'with_video',
      marginResolution: '720p',
      marginStatus: 'eligible',
      marginPage: 2,
    }),
    {
      min_margin_ppm: 300000,
      duration_seconds: 4,
      group_ratio: 1.25,
      scenario: 'with_video',
      resolution: '720p',
      status: 'eligible',
      page: 2,
      page_size: 50,
      sort_by: 'gross_margin_ppm',
      sort_order: 'desc',
    }
  )
})
```

- [ ] **Step 2：运行测试并确认失败**

Run: `cd web && bun test src/features/cost-accounting/lib/__tests__/route-margin-catalog.test.ts`

Expected: FAIL，提示映射函数或类型未定义。

- [ ] **Step 3：实现类型、查询键、API 和搜索 schema**

扩展标签页联合类型为 `profit | catalog | route-margin | anomalies`。新增 `RouteMarginCatalogItem/Page/Params/Summary`，字段与 Go DTO 完全一致；新增：

```ts
routeMarginCatalogs: () => [...costAccountingQueryKeys.all, 'route-margin-catalog'] as const,
routeMarginCatalog: (params: RouteMarginCatalogParams) =>
  [...costAccountingQueryKeys.routeMarginCatalogs(), params] as const,
```

API 函数：

```ts
export async function getRouteMarginCatalog(params: RouteMarginCatalogParams) {
  const response = await api.get<CostAccountingApiResponse<RouteMarginCatalogPage>>(
    `${COST_ACCOUNTING_PATH}/route-margin-catalog`, { params }
  )
  return response.data
}
```

导出函数复用现有安全文件名解析器。路由 schema 对百分比、时长、倍率、页码和排序字段使用 Zod 明确约束，非法 URL 值回退到默认值。

- [ ] **Step 4：运行 URL/API 聚焦测试**

Run: `cd web && bun test src/features/cost-accounting/lib/__tests__/route-margin-catalog.test.ts src/features/cost-accounting/lib/__tests__/catalog.test.ts`

Expected: PASS，并确认现有 `catalog` URL 状态不受影响。

- [ ] **Step 5：提交前端数据层**

```bash
git add web/src/features/cost-accounting/types.ts web/src/features/cost-accounting/api.ts web/src/features/cost-accounting/lib/report.ts web/src/features/cost-accounting/lib/route-margin-catalog.ts web/src/features/cost-accounting/lib/__tests__/route-margin-catalog.test.ts web/src/routes/_authenticated/cost-accounting/index.tsx
git commit -m "feat: add route margin catalog client"
```

### Task 6：实现路由毛利标签页、筛选、表格和导出

**Files:**
- Create: `web/src/features/cost-accounting/components/route-margin-catalog.tsx`
- Create: `web/src/features/cost-accounting/components/route-margin-catalog-filters.tsx`
- Create: `web/src/features/cost-accounting/components/route-margin-catalog-columns.tsx`
- Create: `web/src/features/cost-accounting/components/route-margin-catalog-mobile.tsx`
- Create: `web/src/features/cost-accounting/components/route-margin-catalog-summary.tsx`
- Create: `web/src/features/cost-accounting/components/__tests__/route-margin-catalog.test.tsx`
- Modify: `web/src/features/cost-accounting/index.tsx`

- [ ] **Step 1：先写失败的组件测试**

测试默认查询、30% 汇总、快速/高级切换、修改时长后页码重置、状态筛选、错误重试、CSV 导出以及移动端无横向溢出。

```tsx
test('shows the default 30 percent route margin matrix', async () => {
  const mounted = await mountRouteMarginCatalog({
    summary: {
      target_count: 156,
      scenario_count: 312,
      eligible_target_count: 75,
      fully_eligible_target_count: 60,
      partially_eligible_target_count: 15,
      ineligible_target_count: 81,
      eligible_scenario_count: 135,
    },
  })

  assert.match(mounted.container.textContent ?? '', /75/)
  assert.match(mounted.container.textContent ?? '', /60/)
  assert.match(mounted.container.textContent ?? '', /15/)
})
```

- [ ] **Step 2：运行测试并确认失败**

Run: `cd web && bun test src/features/cost-accounting/components/__tests__/route-margin-catalog.test.tsx`

Expected: FAIL，提示组件未定义。

- [ ] **Step 3：实现页面容器和查询状态**

`RouteMarginCatalog` 使用 `enabled={tab === 'route-margin'}`，查询键由完整参数组成；保留上次数据避免筛选时闪烁；分页和排序写回 URL。错误态使用现有 `Empty/EmptyHeader/EmptyDescription` 和重试按钮。

- [ ] **Step 4：实现快速/高级筛选**

快速模式只显示最低毛利率、结果状态、渠道、模型、路由目标；高级模式显示分组倍率、时长、分辨率和素材场景。控件类型：模式使用 segmented control，素材场景使用 Select，倍率和时长使用有边界的数字 Input。所有筛选变化调用 `updateRouteMarginSearch` 并把页码重置为 1。

- [ ] **Step 5：实现表格、移动端和汇总**

桌面表格列为路由目标、渠道/上游模型、分辨率/场景、预计收入、成本、利润、毛利率、状态/原因、规则来源。金额由 nano-USD 精确格式化，毛利率由 PPM 格式化；缺失金额显示 `—`。移动端使用单层条目列表，不嵌套卡片，保证长模型名换行或截断。

- [ ] **Step 6：接入标签页和导出**

在 `COST_ACCOUNTING_TABS` 和 `TabsList` 中加入 `route-margin`，并渲染：

```tsx
<TabsContent value='route-margin' className='min-h-0 overflow-hidden pr-1 pb-2'>
  <RouteMarginCatalog
    enabled={tab === 'route-margin'}
    search={search}
    onSearchChange={updateSearch}
  />
</TabsContent>
```

导出按钮只导出当前筛选结果，下载逻辑复用 `downloadCostCatalogExport` 的 URL 回收和文件名清洗能力。

- [ ] **Step 7：运行组件测试**

Run: `cd web && bun test src/features/cost-accounting/components/__tests__/route-margin-catalog.test.tsx src/features/cost-accounting/components/__tests__/supplier-cost-catalog.test.tsx`

Expected: PASS，供应商成本目录回归不变。

- [ ] **Step 8：提交界面**

```bash
git add web/src/features/cost-accounting/components/route-margin-catalog*.tsx web/src/features/cost-accounting/components/__tests__/route-margin-catalog.test.tsx web/src/features/cost-accounting/index.tsx
git commit -m "feat: add route margin catalog view"
```

### Task 7：补全七种语言并验证 i18n

**Files:**
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1：加载并遵循项目 `i18n-translate` skill**

在修改 locale 前完整读取 `.agents/skills/i18n-translate/SKILL.md`，按其同步和检查流程执行。

- [ ] **Step 2：添加新增英文键及七语翻译**

至少覆盖：`Route margin`、`Minimum margin`、`Quick mode`、`Advanced mode`、`Group ratio`、`Duration`、`Resolution`、`Material scenario`、`No video`、`With video`、`Eligible`、`Ineligible`、`Estimated revenue`、`Estimated cost`、`Estimated profit`、`Gross margin`、`Failure reason`、`Runtime billing settings` 和所有稳定失败原因。

- [ ] **Step 3：运行 i18n 同步和聚焦测试**

Run: `cd web && bun run i18n:sync && bun test src/features/cost-accounting`

Expected: locale 检查无缺失键，成本核算测试全部 PASS。

- [ ] **Step 4：提交翻译**

```bash
git add web/src/i18n/locales/*.json
git commit -m "feat: translate route margin catalog"
```

### Task 8：全量验证、本机矩阵验收和报告

**Files:**
- Create: `docs/superpowers/reports/2026-08-10-route-margin-catalog-acceptance.md`

- [ ] **Step 1：运行 Go 聚焦测试和全仓测试**

Run: `go test ./model ./service ./controller ./router ./relay/helper -count=1`

Expected: PASS。

Run: `go test ./...`

Expected: PASS；如存在与本改动无关的既有失败，在报告中记录准确包名和错误，不得写成通过。

- [ ] **Step 2：运行前端测试、类型检查和构建**

Run: `cd web && bun test src/features/cost-accounting`

Expected: PASS。

Run: `cd web && bun run build`

Expected: 构建成功，无 TypeScript 或 i18n 错误。

- [ ] **Step 3：启动本地服务并复核 30% 矩阵**

使用当前工作树的既有 Docker/本地开发启动方式，不修改数据库设置。请求：

```text
GET /api/cost-accounting/route-margin-catalog?min_margin_ppm=300000&duration_seconds=4&group_ratio=1&scenario=all&page=1&page_size=100&sort_by=gross_margin_ppm&sort_order=desc
```

翻页汇总后断言：目标 156、场景行 312、至少一个场景通过 75、全场景通过 60、部分通过 15、全不通过 81、通过场景行 135。

- [ ] **Step 4：使用 Playwright 检查桌面和移动端**

访问 `http://localhost:3000/cost-accounting?tab=route-margin`，检查 1440×900 和 390×844：首屏非空、筛选控件不重叠、长模型名不溢出、切换高级模式能刷新结果、导出按钮可用、供应商成本目录仍可访问。

- [ ] **Step 5：编写简体中文验收报告**

报告记录提交、测试命令和结果、矩阵数量、API 样例、桌面/移动截图路径、与原统计报告的差异；不得包含凭据、媒体 URL 或用户数据。

- [ ] **Step 6：检查差异并提交验收报告**

Run: `git diff --check`

Expected: 无输出，退出码 0。

```bash
git add docs/superpowers/reports/2026-08-10-route-margin-catalog-acceptance.md
git commit -m "docs: report route margin catalog acceptance"
```
