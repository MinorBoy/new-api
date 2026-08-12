# 利润报表可编辑下拉筛选实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为利润报表的渠道 ID、计费上游模型、原始模型、用户分组和使用分组提供与使用日志一致的可搜索、可清除、可自由输入下拉筛选。

**Architecture:** 后端在成本报表服务层基于账本查询候选值，并通过只读报告候选接口返回。前端通过 React Query 按已提交的报表搜索参数加载候选，将统一 `Combobox` 字段嵌入现有本地 draft / “应用筛选”流程。

**Tech Stack:** Go、Gin、GORM、SQLite 测试数据库、React 19、TypeScript、TanStack Query、Base UI Combobox、Bun node:test。

---

### Task 1: 后端候选查询的失败测试

**Files:**
- Modify: `service/cost_report_test.go`
- Modify: `controller/cost_accounting_test.go`
- Modify: `router/cost_accounting_router_test.go`

- [x] **Step 1: 添加服务层候选类型与查询契约测试**

在 `service/cost_report_test.go` 增加 SQLite fixture，写入两条请求和三条尝试，覆盖：去除空字符串、去重、按字符串稳定排序；请求筛选 `channel_id`、`billable_upstream_model`、`origin_model`、`user_group`、`using_group` 和时间范围会约束其它字段候选，但生成某字段候选时忽略该字段自身条件。

断言 `ListCostReportFilterOptions(CostReportFilter{...})` 返回的五个集合分别为精确的候选值，且渠道同时保留 ID 和账本中的渠道名。

- [x] **Step 2: 添加控制器响应契约测试**

在 `controller/cost_accounting_test.go` 增加对候选响应转换函数的测试，断言 JSON 字段为 `channels`、`billable_upstream_models`、`origin_models`、`user_groups`、`using_groups`，空列表序列化为 `[]` 而不是 `null`。

- [x] **Step 3: 添加专用路由权限测试**

在 `router/cost_accounting_router_test.go` 添加 `GET /reports/filter-options` 使用 `authz.CostAccountingRead` 且绑定 `controller.GetCostReportFilterOptions` 的断言。

- [x] **Step 4: 运行新增测试确认先失败**

运行：

```powershell
go test ./service ./controller ./router -run 'CostReport(FilterOptions|Responses)|CostAccountingPermissionRoutes'
```

预期：因候选服务、控制器处理函数和路由尚不存在而失败。

### Task 2: 实现后端候选服务与 API

**Files:**
- Modify: `service/cost_report.go`
- Modify: `controller/cost_accounting.go`
- Modify: `router/cost-accounting-router.go`

- [x] **Step 1: 定义候选响应领域类型**

在 `service/cost_report.go` 增加：

```go
type CostReportFilterChannel struct { ID int `json:"id"`; Name string `json:"name"` }
type CostReportFilterOptions struct {
    Channels []CostReportFilterChannel `json:"channels"`
    BillableUpstreamModels []string `json:"billable_upstream_models"`
    OriginModels []string `json:"origin_models"`
    UserGroups []string `json:"user_groups"`
    UsingGroups []string `json:"using_groups"`
}
```

初始化每个切片为空数组，保证没有数据时仍返回 JSON `[]`。

- [x] **Step 2: 抽取可复用的请求过滤条件构造**

将当前 `costReportRequestQuery` 的日期、时间口径、请求字段筛选构造为可选择忽略某一字段的内部查询函数；保留 `costReportRequestQuery` 现有行为。候选查询调用该函数五次，每次忽略自身筛选字段，并对 attempts 使用同一请求子查询条件。

- [x] **Step 3: 实现跨数据库候选查询**

使用 GORM `Select`、`Joins`、`Distinct`、`Order`、`Rows`/`ScanRows` 读取：

- 渠道与计费上游模型：`cost_accounting_attempts`，渠道按 ID 聚合并保留首个非空名称；
- 原始模型、用户分组、使用分组：`cost_accounting_requests`。

不使用数据库专用字符串聚合；在 Go 中 trim、过滤空值、去重和排序。渠道按 ID 升序，字符串按 `sort.Strings` 升序。

- [x] **Step 4: 暴露控制器处理函数**

在 `controller/cost_accounting.go` 增加 `GetCostReportFilterOptions`：复用 `costReportFilterFromQuery` 解析查询参数，调用服务并通过 `common.ApiSuccess` 返回候选结构；错误沿用 `writeCostAccountingError`。

- [x] **Step 5: 注册只读路由**

在 `router/cost-accounting-router.go` 添加：

```go
{method: http.MethodGet, path: "/reports/filter-options", permission: authz.CostAccountingRead, handler: controller.GetCostReportFilterOptions},
```

- [x] **Step 6: 运行后端测试确认通过**

运行 Task 1 命令及完整成本报表测试：

```powershell
go test ./service ./controller ./router -run 'CostReport|CostAccounting'
```

预期：全部通过。

### Task 3: 前端 API、类型与候选 hook 的失败测试

**Files:**
- Modify: `web/src/features/cost-accounting/types.ts`
- Modify: `web/src/features/cost-accounting/api.ts`
- Create: `web/src/features/cost-accounting/hooks/use-profit-filter-options.ts`
- Create: `web/src/features/cost-accounting/hooks/__tests__/use-profit-filter-options.test.ts`

- [x] **Step 1: 添加候选类型和 API 测试**

定义 `CostReportFilterOptions`、`CostReportFilterChannel` 和 `getCostReportFilterOptions(params)`，测试 API 使用 `/api/cost-accounting/reports/filter-options` 并将 `CostReportParams` 原样作为 query 参数传递。由于现有前端测试通过直接替换 `api.get` 验证请求，本任务使用同一方式，不引入新的 mock 框架。

- [x] **Step 2: 编写 hook 候选映射测试**

使用最小 QueryClient fixture，mock API 返回重复、空值和无序候选，断言 hook 输出：渠道选项为 `{ value: '7', label: '7 - Primary' }`，其它选项 value/label 相同，空值被过滤，结果稳定排序。

- [x] **Step 3: 运行前端测试确认失败**

运行：

```powershell
cd web
bun test src/features/cost-accounting/hooks/__tests__/use-profit-filter-options.test.ts
```

预期：因类型、API 和 hook 尚未实现而失败。

### Task 4: 实现前端候选加载

**Files:**
- Modify: `web/src/features/cost-accounting/types.ts`
- Modify: `web/src/features/cost-accounting/api.ts`
- Create: `web/src/features/cost-accounting/hooks/use-profit-filter-options.ts`

- [x] **Step 1: 增加 API 类型和查询函数**

实现候选接口类型、`costAccountingQueryKeys.reportFilterOptions(params)` 和 `getCostReportFilterOptions`，沿用成本核算 API 的 `CostAccountingApiResponse<T>` 包装。

- [x] **Step 2: 实现 hook**

`useProfitFilterOptions(search)` 将 `costReportParamsFromSearch(search)` 作为查询参数和 query key；使用 `useQuery`、30 秒 stale time；将返回数据转换为五组 `LogFilterOption` 兼容的 `{value,label}`，并对当前已提交值追加一个缺失的选项以保持显示。

- [x] **Step 3: 运行 hook 测试确认通过**

运行 Task 3 命令，预期 PASS。

### Task 5: 利润筛选区替换为可编辑 Combobox

**Files:**
- Modify: `web/src/features/cost-accounting/components/profit-filters.tsx`
- Create: `web/src/features/cost-accounting/components/__tests__/profit-filters.test.tsx`

- [x] **Step 1: 编写交互回归测试**

挂载 `ProfitFilters` 并传入候选选项，断言五个字段具备对应 label、`aria-expanded` 和可输入行为；选择渠道标签后 draft 值为 ID；自由输入候选外模型；清除字段；点击应用前 `onChange` 未调用，点击后一次性提交所有值。

- [x] **Step 2: 运行测试确认失败**

运行：

```powershell
cd web
bun test src/features/cost-accounting/components/__tests__/profit-filters.test.tsx
```

预期：当前 `Input` 不满足组合框行为，测试失败。

- [x] **Step 3: 实现统一可编辑筛选字段**

引入 `Combobox` 和 `CompositionEvent`，新增 `ComboboxFilter` 组件。五个字段设置 `allowCustomValue`、`openOnFocus`、`showClear`、候选 options 与受控 draft value；IME 期间只更新 draft，不触发应用。保留日期和两个普通 `Select`。

- [x] **Step 4: 运行组件测试确认通过**

运行 Task 5 命令，预期 PASS。

### Task 6: 接入利润页面并完成验证

**Files:**
- Modify: `web/src/features/cost-accounting/index.tsx`
- Modify: `web/src/features/cost-accounting/components/profit-filters.tsx`
- Modify: `web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx`

- [x] **Step 1: 在利润页面加载候选 hook**

仅在 `tab === 'profit'` 时启用 `useProfitFilterOptions(search)`，将五组 options 传入 `ProfitFilters`；候选请求失败时保持空 options，报表查询不受影响。

- [x] **Step 2: 增加页面查询参数回归断言**

扩展利润页面测试，断言候选请求包含已提交时间范围和其它筛选，且草稿输入不会立即改变报表 query 参数。

- [x] **Step 3: 运行前端检查**

```powershell
cd web
bun test src/features/cost-accounting/components/__tests__/profit-report.test.tsx src/features/cost-accounting/components/__tests__/profit-filters.test.tsx src/features/cost-accounting/hooks/__tests__/use-profit-filter-options.test.ts
bun run typecheck
bun run lint -- web/src/features/cost-accounting
bun run build
```

预期：测试、类型检查、相关 lint 和生产构建均通过。

- [x] **Step 4: 检查工作树并提交实现**

运行 `git diff --check` 和 `git status --short`，确认只包含本功能相关文件后提交：

```powershell
git add service/cost_report.go service/cost_report_test.go controller/cost_accounting.go controller/cost_accounting_test.go router/cost-accounting-router.go router/cost_accounting_router_test.go web/src/features/cost-accounting
git commit -m "feat: add editable profit report filters"
```
