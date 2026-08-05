# 供应商敏感信息日志边界 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让普通用户的使用日志和任务日志只返回用户侧公开事实，彻底隔离渠道、上游模型、供应商成本、路由尝试、审计历史和供应商资源地址，同时保留管理员完整审计能力。

**Architecture:** 新增独立的公开 DTO 和服务层白名单投影，`/api/log/self`、`/api/log/token`、`/api/task/self` 在序列化前转换，未知字段默认丢弃；管理员接口继续使用现有完整模型/DTO。前端按 `isAdminView` 选择公开或管理员展示面，普通视图不声明筛选或渲染供应商字段；E2E 对三类接口的完整 JSON 做递归字段名和值扫描，并核对视频结果只能使用 new-api 代理 URL。

**Tech Stack:** Go 1.22、Gin、GORM、`common` JSON 包装器、Testify、React 19、TypeScript、TanStack Table/Query、Bun Test、happy-dom。

---

### Task 1: 使用日志公开 DTO 与白名单投影

**Files:**
- Create: `dto/log.go`
- Create: `service/public_log.go`
- Test: `service/public_log_test.go`
- Modify: `model/log_format_test.go`

- [ ] **Step 1: 写入公开日志投影失败测试**

测试构造包含渠道、上游请求 ID、分组、IP、价格、倍率、上游模型、成本请求、路由尝试和未知字段的 `model.Log`，序列化 `ProjectPublicLog` 后递归断言这些字段名和值均不存在，同时保留请求 ID、公共模型、最终额度、Token、响应时间、登录审计和用户侧计量。

```go
func TestProjectPublicLogDropsSupplierFactsByWhitelist(t *testing.T) {
    source := &model.Log{
        Id: 91, UserId: 10, Username: "alice", ChannelId: 40,
        ChannelName: "supplier-secret", Group: "internal-group", Ip: "10.0.0.8",
        RequestId: "req-public", UpstreamRequestId: "upstream-secret",
        ModelName: "public-model", TokenName: "user-token", Quota: 123,
        PromptTokens: 11, CompletionTokens: 22, UseTime: 3,
        Other: common.MapToJsonStr(map[string]interface{}{
            "cache_tokens": 7,
            "login_method": "password",
            "model_price": 0.2,
            "group_ratio": 1.25,
            "upstream_model_name": "provider-model",
            "admin_info": map[string]interface{}{
                "cost_accounting_request_id": 88,
                "use_channel": []int{40, 41},
            },
            "unknown_supplier_fact": "never-public",
        }),
    }

    payload, err := common.Marshal(ProjectPublicLog(source, 1))
    require.NoError(t, err)
    body := string(payload)
    assert.Contains(t, body, "req-public")
    assert.Contains(t, body, "public-model")
    assert.Contains(t, body, "cache_tokens")
    for _, forbidden := range []string{
        "supplier-secret", "internal-group", "upstream-secret", "provider-model",
        "model_price", "group_ratio", "cost_accounting_request_id",
        "use_channel", "unknown_supplier_fact", "never-public",
    } {
        assert.NotContains(t, body, forbidden)
    }
}
```

- [ ] **Step 2: 运行测试并确认因公开投影不存在而失败**

Run: `go test ./service -run 'TestProjectPublicLog' -count=1`

Expected: FAIL，错误指向 `ProjectPublicLog` 未定义。

- [ ] **Step 3: 新增强类型公开 DTO 与投影**

`dto/log.go` 定义 `PublicLog` 和 `PublicLogOther`，后者只包含登录审计、用户侧 Token/缓存/音频/图片计量、订阅消耗和公共任务 ID；价格、倍率、路径、模型映射、路由、成本和通用 `map` 不进入 DTO。所有可选标量使用指针与 `omitempty`，避免丢失显式零值。

```go
type PublicLog struct {
    ID               int            `json:"id"`
    CreatedAt        int64          `json:"created_at"`
    Type             int            `json:"type"`
    Content          string         `json:"content"`
    TokenName        string         `json:"token_name"`
    ModelName        string         `json:"model_name"`
    Quota            int            `json:"quota"`
    PromptTokens     int            `json:"prompt_tokens"`
    CompletionTokens int            `json:"completion_tokens"`
    UseTime          int            `json:"use_time"`
    IsStream         bool           `json:"is_stream"`
    RequestID        string         `json:"request_id,omitempty"`
    Other            PublicLogOther `json:"other"`
}

type PublicLogOther struct {
    LoginMethod           string `json:"login_method,omitempty"`
    UserAgent             string `json:"user_agent,omitempty"`
    WebSocket             *bool  `json:"ws,omitempty"`
    Audio                 *bool  `json:"audio,omitempty"`
    AudioInput            *int   `json:"audio_input,omitempty"`
    AudioOutput           *int   `json:"audio_output,omitempty"`
    TextInput             *int   `json:"text_input,omitempty"`
    TextOutput            *int   `json:"text_output,omitempty"`
    CacheTokens           *int   `json:"cache_tokens,omitempty"`
    CacheCreationTokens   *int   `json:"cache_creation_tokens,omitempty"`
    CacheCreationTokens5m *int   `json:"cache_creation_tokens_5m,omitempty"`
    CacheCreationTokens1h *int   `json:"cache_creation_tokens_1h,omitempty"`
    FirstResponseTime     *int   `json:"frt,omitempty"`
    Image                 *bool  `json:"image,omitempty"`
    ImageOutput           *int   `json:"image_output,omitempty"`
    WebSearchCount        *int   `json:"web_search_call_count,omitempty"`
    FileSearchCount       *int   `json:"file_search_call_count,omitempty"`
    BillingSource         string `json:"billing_source,omitempty"`
    SubscriptionConsumed  *int   `json:"subscription_consumed,omitempty"`
    SubscriptionRemain    *int   `json:"subscription_remain,omitempty"`
    SubscriptionTotal     *int   `json:"subscription_total,omitempty"`
    IsTask                *bool  `json:"is_task,omitempty"`
    TaskID                string `json:"task_id,omitempty"`
}
```

`service/public_log.go` 使用 `common.UnmarshalJsonStr` 将 `other` 直接解到 `dto.PublicLogOther`，未知键由结构体解码自动丢弃；`Content` 只执行 `common.MaskSensitiveInfo`，不把解析错误或原始 JSON写入返回值。

```go
func ProjectPublicLog(log *model.Log, displayID int) *dto.PublicLog {
    if log == nil {
        return nil
    }
    other := dto.PublicLogOther{}
    if log.Other != "" {
        _ = common.UnmarshalJsonStr(log.Other, &other)
    }
    return &dto.PublicLog{
        ID: displayID, CreatedAt: log.CreatedAt, Type: log.Type,
        Content: common.MaskSensitiveInfo(log.Content), TokenName: log.TokenName,
        ModelName: log.ModelName, Quota: log.Quota,
        PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens,
        UseTime: log.UseTime, IsStream: log.IsStream,
        RequestID: log.RequestId, Other: other,
    }
}
```

- [ ] **Step 4: 运行公开投影测试并清理旧删除列表测试**

Run: `go test ./service ./model -run 'TestProjectPublicLog|TestFormatUserLogs' -count=1`

Expected: PASS；删除或改写只证明 `formatUserLogs` 删除个别键的测试，避免继续把黑名单删除当作安全合同。

- [ ] **Step 5: 提交公开日志投影**

```bash
git add dto/log.go service/public_log.go service/public_log_test.go model/log_format_test.go
git commit -m "feat: add public usage log projection"
```

### Task 2: 普通使用日志 API 与查询探测边界

**Files:**
- Modify: `controller/log.go`
- Test: `controller/public_log_test.go`

- [ ] **Step 1: 写入 self/token API 合同和查询探测失败测试**

测试通过 SQLite 夹具写入两条同属当前用户、但渠道/分组/上游请求 ID 不同的日志。对 `GetUserLogs` 传入这些内部筛选参数后，断言总数仍为 2；完整响应递归扫描不得包含敏感字段名和值。`GetLogByKey` 使用相同公开 DTO。管理员 `GetAllLogs` 仍返回渠道和上游请求 ID。

```go
func TestGetUserLogsIgnoresSupplierFiltersAndReturnsPublicContract(t *testing.T) {
    setupPublicLogControllerDB(t)
    recorder := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(recorder)
    ctx.Set("id", 10)
    ctx.Request = httptest.NewRequest(http.MethodGet,
        "/api/log/self?p=1&page_size=20&channel=999&group=probe&upstream_request_id=probe", nil)

    GetUserLogs(ctx)

    payload := decodeJSONMap(t, recorder.Body.Bytes())
    assert.Equal(t, float64(2), nestedNumber(payload, "data", "total"))
    assertPublicPayloadHasNoSupplierFacts(t, payload)
}
```

- [ ] **Step 2: 运行测试并确认当前 self 响应泄漏而失败**

Run: `go test ./controller -run 'TestGet(UserLogs|LogByKey).*Public|TestGetAllLogsKeepsAdminAudit' -count=1`

Expected: FAIL，普通响应仍包含 `channel`、`group`、`upstream_request_id` 或价格字段。

- [ ] **Step 3: self/token 切换到白名单 DTO 并忽略内部维度**

`GetUserLogs` 只读取时间、类型、Token、公共模型和 new-api 请求 ID，调用模型查询时为 `group`、`upstreamRequestID` 传空值；返回前逐条调用 `service.ProjectPublicLog`。`GetLogByKey` 同样投影。`GetLogsSelfStat` 和 `GetLogSelfModels` 固定向模型层传 `channel=0`、`group=""`。

```go
publicLogs := make([]*dto.PublicLog, len(logs))
for i, log := range logs {
    publicLogs[i] = service.ProjectPublicLog(log, pageInfo.GetStartIdx()+i+1)
}
pageInfo.SetItems(publicLogs)
```

- [ ] **Step 4: 验证普通与管理员合同**

Run: `go test ./controller ./service ./model -run 'PublicLog|GetUserLogs|GetLogByKey|GetAllLogsKeepsAdminAudit' -count=1`

Expected: PASS；普通接口扫描为 0 泄漏，管理员接口仍包含完整审计。

- [ ] **Step 5: 提交使用日志 API 边界**

```bash
git add controller/log.go controller/public_log_test.go
git commit -m "fix: enforce public usage log API boundary"
```

### Task 3: 任务日志公开 DTO 与代理结果投影

**Files:**
- Modify: `dto/task.go`
- Create: `service/public_task.go`
- Test: `service/public_task_test.go`

- [ ] **Step 1: 写入任务公开投影失败测试**

测试任务同时携带渠道平台、内部分组、上游模型、上游任务 ID、原始供应商 URL、原始请求、上游响应、路由审计、成本上下文和带未知字段的终态响应。公开序列化必须保留公共任务 ID、公共模型、状态、完整 Ark 默认字段、Token 用量和 new-api 代理 URL，并排除所有私有字段名和值。

```go
func TestProjectPublicTaskUsesArkWhitelistAndProxyURL(t *testing.T) {
    system_setting.ServerAddress = "https://gateway.example"
    task := seededSupplierTaskForPublicProjection()

    public := ProjectPublicTask(task)
    payload, err := common.Marshal(public)
    require.NoError(t, err)
    body := string(payload)
    assert.Contains(t, body, "task_public")
    assert.Contains(t, body, "public-seedance")
    assert.Contains(t, body, "https://gateway.example/v1/videos/task_public/content")
    for _, forbidden := range []string{
        "supplier-platform", "internal-group", "provider-model", "cgt-secret",
        "supplier.example", "channel_id", "platform", "request_path",
        "user_request_data", "upstream_response_data", "routing", "billing_context",
    } {
        assert.NotContains(t, body, forbidden)
    }
}
```

- [ ] **Step 2: 运行测试并确认公开任务投影不存在**

Run: `go test ./service -run 'TestProjectPublicTask' -count=1`

Expected: FAIL，错误指向 `ProjectPublicTask` 未定义。

- [ ] **Step 3: 定义独立公开任务和 Ark 终态 DTO**

在 `dto/task.go` 新增 `PublicTaskDto`、`PublicTaskResult`、`PublicTaskContent`、`PublicTaskUsage`、`PublicTaskError`。公开任务不包含数据库 ID、用户、渠道、平台、分组、路径、原始请求/响应或 `Properties/Data`。

```go
type PublicTaskDto struct {
    CreatedAt        int64             `json:"created_at"`
    UpdatedAt        int64             `json:"updated_at"`
    TaskID           string            `json:"task_id"`
    Quota            int               `json:"quota"`
    Action           string            `json:"action"`
    Status           string            `json:"status"`
    FailReason       string            `json:"fail_reason,omitempty"`
    SubmitTime       int64             `json:"submit_time"`
    StartTime        int64             `json:"start_time"`
    FinishTime       int64             `json:"finish_time"`
    Progress         string            `json:"progress"`
    RequestModel     string            `json:"request_model,omitempty"`
    UserResponseData *PublicTaskResult `json:"user_response_data,omitempty"`
}

type PublicTaskResult struct {
    ID                    string             `json:"id"`
    Model                 string             `json:"model"`
    Status                string             `json:"status"`
    Content               *PublicTaskContent `json:"content,omitempty"`
    Usage                 PublicTaskUsage    `json:"usage"`
    CreatedAt             int64              `json:"created_at"`
    UpdatedAt             int64              `json:"updated_at"`
    Seed                  int64              `json:"seed"`
    Resolution            string             `json:"resolution"`
    Ratio                 string             `json:"ratio"`
    Duration              int64              `json:"duration"`
    FramesPerSecond       int64              `json:"framespersecond"`
    ServiceTier           string             `json:"service_tier"`
    ExecutionExpiresAfter int64              `json:"execution_expires_after"`
    GenerateAudio         bool               `json:"generate_audio"`
    Draft                 bool               `json:"draft"`
    Priority              int64              `json:"priority"`
    Error                 *PublicTaskError   `json:"error,omitempty"`
}
```

- [ ] **Step 4: 实现终态规范化后再白名单投影**

`service/public_task.go` 读取已保存 `UserResponseData`，仅在服务端内存中调用 `NormalizeSeedanceTaskResponse` 补全终态字段；成功任务预置/覆盖 `content.video_url=taskcommon.BuildProxyURL(task.TaskID)`，随后通过 `common.Marshal`/`common.Unmarshal` 解入 `dto.PublicTaskResult` 丢弃未知键。`id` 和 `model` 最后固定为公共任务 ID和 `OriginModelName`/冻结的公共模型；失败错误再次使用 `sanitizeSeedanceFailureText`，绝不回退上游模型或供应商 URL。

```go
func ProjectPublicTask(task *model.Task) *dto.PublicTaskDto {
    if task == nil {
        return nil
    }
    public := &dto.PublicTaskDto{
        CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt, TaskID: task.TaskID,
        Quota: task.Quota, Action: task.Action, Status: string(task.Status),
        SubmitTime: task.SubmitTime, StartTime: task.StartTime,
        FinishTime: task.FinishTime, Progress: task.Progress,
        RequestModel: publicTaskModel(task),
    }
    public.FailReason = publicTaskFailure(task)
    public.UserResponseData = projectPublicTaskResult(task, public.RequestModel)
    return public
}
```

- [ ] **Step 5: 验证成功、失败、历史供应商 URL和未知字段**

Run: `go test ./service -run 'TestProjectPublicTask' -count=1`

Expected: PASS；成功 URL 全部为 new-api 代理；失败无 `content` 且错误脱敏；历史供应商 URL 和未知字段均不出现。

- [ ] **Step 6: 提交任务公开投影**

```bash
git add dto/task.go service/public_task.go service/public_task_test.go
git commit -m "feat: add public task log projection"
```

### Task 4: 普通任务 API 与筛选边界

**Files:**
- Modify: `controller/task.go`
- Test: `controller/public_task_test.go`
- Modify: `controller/task_filter_options_test.go`

- [ ] **Step 1: 写入 `/api/task/self` 公开合同和平台探测失败测试**

测试写入两个当前用户任务和一个其他用户任务。普通请求携带 `platform`、`channel_id`、`user_id`、`group` 时仍返回当前用户两个任务；响应递归扫描不含敏感字段和值。管理员接口继续返回 `TaskDto` 的平台、渠道、路径、请求、上游响应和最终用户响应。

```go
func TestGetUserTaskIgnoresAdminDimensionsAndReturnsPublicDTO(t *testing.T) {
    setupPublicTaskControllerDB(t)
    recorder := httptest.NewRecorder()
    ctx, _ := gin.CreateTestContext(recorder)
    ctx.Set("id", 10)
    ctx.Request = httptest.NewRequest(http.MethodGet,
        "/api/task/self?p=1&page_size=20&platform=probe&channel_id=999&user_id=11&group=probe", nil)

    GetUserTask(ctx)

    payload := decodeJSONMap(t, recorder.Body.Bytes())
    assert.Equal(t, float64(2), nestedNumber(payload, "data", "total"))
    assertPublicPayloadHasNoSupplierFacts(t, payload)
}
```

- [ ] **Step 2: 运行测试并确认当前复用管理员 DTO 导致失败**

Run: `go test ./controller -run 'TestGet(UserTask|AllTask).*Public|TestGetUserTaskFilterOptions' -count=1`

Expected: FAIL，普通任务仍包含 `platform`、`channel_id`、`user_id` 或请求原文。

- [ ] **Step 3: self 接口切换公开任务投影**

`GetUserTask` 不再读取 `platform`、`channel_id`、`user_id`、`group`；保留公共任务 ID、公共模型、状态、操作和时间筛选。普通列表逐条调用 `service.ProjectPublicTask`；管理员 `tasksToDto(..., true)` 和 Relay 协议路径的 `TaskModel2Dto` 不变，避免破坏任务查询协议。

```go
result := make([]*dto.PublicTaskDto, len(items))
for i, task := range items {
    result[i] = service.ProjectPublicTask(task)
}
pageInfo.SetItems(result)
```

- [ ] **Step 4: 验证普通筛选项不返回渠道和用户维度**

Run: `go test ./controller -run 'TestGet(UserTask|AllTask)|TestGetUserTaskFilterOptions' -count=1`

Expected: PASS；普通筛选项只有 `statuses`、`request_models`，管理员筛选项仍有渠道和用户。

- [ ] **Step 5: 提交任务 API 边界**

```bash
git add controller/task.go controller/public_task_test.go controller/task_filter_options_test.go
git commit -m "fix: enforce public task log API boundary"
```

### Task 5: 普通使用日志前端隔离

**Files:**
- Modify: `web/src/features/usage-logs/data/schema.ts`
- Modify: `web/src/features/usage-logs/types.ts`
- Modify: `web/src/features/usage-logs/api.ts`
- Modify: `web/src/features/usage-logs/lib/format.ts`
- Modify: `web/src/features/usage-logs/lib/utils.ts`
- Modify: `web/src/features/usage-logs/hooks/use-common-log-filter-options.ts`
- Modify: `web/src/features/usage-logs/components/common-logs-filter-bar.tsx`
- Modify: `web/src/features/usage-logs/components/columns/common-logs-columns.tsx`
- Modify: `web/src/features/usage-logs/components/dialogs/details-dialog.tsx`
- Modify: `web/src/features/usage-logs/components/usage-logs-mobile-card.tsx`
- Test: `web/src/features/usage-logs/components/__tests__/public-common-log-boundary.test.tsx`
- Modify: `web/src/features/usage-logs/lib/__tests__/query-params.test.ts`

- [ ] **Step 1: 写入普通 UI 注入攻击和查询参数失败测试**

向 `DetailsDialog` 和普通列注入包含渠道、上游请求 ID、分组、上游模型、价格、倍率、成本请求和路由尝试的异常日志对象，`isAdmin=false` 时 DOM 不得出现对应标签和值；仍显示请求 ID、公共模型、Token、最终消耗和用量。参数测试断言普通请求不发送 `group`、`channel`、`upstream_request_id`，管理员请求仍发送。

```tsx
test('regular log details never render injected supplier fields', async () => {
  const log = supplierInjectedUsageLog()
  const mounted = await renderDetails(log, false)
  const text = mounted.container.textContent ?? ''
  for (const forbidden of [
    'supplier-channel', 'upstream-secret', 'provider-model',
    'Supplier Cost Accounting', 'Retry Chain', 'Model Mapping',
  ]) assert.equal(text.includes(forbidden), false)
  assert.match(text, /req-public/)
  assert.match(text, /public-model/)
})
```

- [ ] **Step 2: 运行前端定向测试并确认泄漏入口存在**

Run: `cd web && bun test --parallel=1 src/features/usage-logs/components/__tests__/public-common-log-boundary.test.tsx src/features/usage-logs/lib/__tests__/query-params.test.ts`

Expected: FAIL，普通详情仍显示上游请求 ID/分组/模型映射/计费明细，普通参数仍发送分组和上游请求 ID。

- [ ] **Step 3: 区分公开和管理员类型并兼容公开 `other` 对象**

`schema.ts` 定义公开字段和管理员扩展字段；`types.ts` 将价格、倍率、路由和管理员审计只放在管理员类型中。`parseLogOther` 接受公开对象或管理员 JSON 字符串，不执行任意透传。

```ts
export function parseLogOther(
  other: PublicLogOther | string | null | undefined
): LogOtherData | null {
  if (!other) return null
  if (typeof other === 'object') return other
  try {
    return JSON.parse(other) as LogOtherData
  } catch {
    return null
  }
}
```

- [ ] **Step 4: 普通筛选和组件移除供应商入口**

`buildApiParams` 仅在 `isAdmin` 时加入 `group`、`channel`、`username`、`upstream_request_id`；普通筛选栏不渲染分组、渠道、用户名、上游请求 ID和敏感值开关，`useCommonLogFilterOptions` 非管理员不请求分组/渠道。详情对渠道、上游请求 ID、重试链、分组、模型映射、计费拆分、动态价格和成本核算全部加 `props.isAdmin` 门禁；普通用户只显示最终额度，不显示单位价格或倍率。公共模型列禁止从异常 `other.upstream_model_name` 渲染实际模型。

- [ ] **Step 5: 验证普通与管理员使用日志 UI**

Run: `cd web && bun test --parallel=1 src/features/usage-logs/components/__tests__/public-common-log-boundary.test.tsx src/features/usage-logs/components/__tests__/common-logs-filter-bar.test.tsx src/features/usage-logs/components/__tests__/billing-breakdown.test.ts src/features/usage-logs/lib/__tests__/query-params.test.ts`

Expected: PASS；普通注入扫描为 0 泄漏，管理员完整区域仍可见。

- [ ] **Step 6: 提交使用日志前端边界**

```bash
git add web/src/features/usage-logs
git commit -m "fix: hide supplier audit from public usage logs"
```

### Task 6: 普通任务日志前端隔离和最终结果展示

**Files:**
- Modify: `web/src/features/usage-logs/types.ts`
- Modify: `web/src/features/usage-logs/components/columns/task-logs-columns.tsx`
- Modify: `web/src/features/usage-logs/components/usage-logs-mobile-card.tsx`
- Modify: `web/src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx`
- Modify: `web/src/features/usage-logs/components/__tests__/task-logs-mobile-card.test.tsx`

- [ ] **Step 1: 改写任务列测试为公开/管理员双合同**

普通列只允许公共模型、公共任务 ID、状态、进度、最终消耗和 `Task Details`，不得声明/渲染平台、渠道、请求原文、上游创建响应或上游任务 ID；管理员列继续展示完整审计。普通 `Task Details` 展示官方 Ark 结构和代理视频 URL。

```tsx
test('regular users see public task result but no supplier audit columns', async () => {
  const headers = await getHeaders(false)
  assert.equal(headers.includes('Request Model'), true)
  assert.equal(headers.includes('Task Details'), true)
  assert.equal(headers.includes('Request Data'), false)
  assert.equal(headers.includes('Upstream Response (Create Task)'), false)
  assert.equal(headers.includes('Channel'), false)
  assert.equal(headers.includes('Endpoint'), false)
})
```

- [ ] **Step 2: 运行测试并确认当前普通任务仍显示请求原文且缺少最终结果**

Run: `cd web && bun test --parallel=1 src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx src/features/usage-logs/components/__tests__/task-logs-mobile-card.test.tsx`

Expected: FAIL，普通列包含 `Request Data` 且没有 `Task Details`。

- [ ] **Step 3: 普通任务列只消费公开 DTO**

`TaskLog` 的管理员字段改为可选，新增强类型 `PublicTaskResult`。`useTaskLogsColumns(false)` 添加 `request_model` 和 `user_response_data`，不添加 `user_request_data`；任务 ID副标题普通视图只显示公开 action，不读取 platform。视频预览从 `user_response_data.content.video_url` 获取且只接受 `/v1/videos/{publicTaskID}/content` 或当前 new-api 地址，不读取 `fail_reason`/供应商 URL。

- [ ] **Step 4: 移动端按现有列集合展示公开结果**

移动端仅当列存在时展示请求原文、上游响应；普通列集合只产生 `Task Details`。测试向普通对象注入 `platform`、`channel_id`、`user_request_data`、`upstream_response_data`，DOM 仍不得出现敏感值。

- [ ] **Step 5: 验证任务日志 UI 和类型检查**

Run: `cd web && bun test --parallel=1 src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx src/features/usage-logs/components/__tests__/task-logs-mobile-card.test.tsx`

Run: `cd web && bun run typecheck`

Expected: 两条命令均 PASS。

- [ ] **Step 6: 提交任务日志前端边界**

```bash
git add web/src/features/usage-logs/types.ts web/src/features/usage-logs/components/columns/task-logs-columns.tsx web/src/features/usage-logs/components/usage-logs-mobile-card.tsx web/src/features/usage-logs/components/__tests__/task-audit-columns.test.tsx web/src/features/usage-logs/components/__tests__/task-logs-mobile-card.test.tsx
git commit -m "fix: expose only public task results to users"
```

### Task 7: 双角色 E2E、页面验收和报告

**Files:**
- Create: `e2e/supplier_confidential_log_boundary_e2e_test.go`
- Create: `docs/superpowers/reports/2026-08-05-supplier-confidential-log-boundary-acceptance.md`

- [ ] **Step 1: 写入 E2E 递归扫描测试**

复用现有 Seedance E2E 数据库和本地 mock 上游，创建包含完整供应商事实的成功任务、使用日志和成本账本。通过 Gin 路由分别请求普通和管理员 API；递归扫描普通 `GET /api/log/self`、`GET /api/log/token`、`GET /api/task/self` 的字段名和字符串值，并断言管理员 `GET /api/log/`、`GET /api/task/`、成本核算详情保留完整事实。

```go
func assertNoSupplierFacts(t *testing.T, payload []byte) {
    t.Helper()
    var value interface{}
    require.NoError(t, common.Unmarshal(payload, &value))
    forbiddenKeys := map[string]struct{}{
        "channel": {}, "channel_id": {}, "channel_name": {}, "platform": {},
        "group": {}, "upstream_request_id": {}, "upstream_model_name": {},
        "upstream_response_data": {}, "user_request_data": {}, "request_path": {},
        "model_price": {}, "duration_price": {}, "group_ratio": {},
        "cost_accounting_request_id": {}, "admin_info": {}, "audit_info": {},
    }
    scanJSONValue(t, value, forbiddenKeys, []string{
        "supplier-secret", "provider-model", "cgt-private", "supplier.example",
    })
}
```

- [ ] **Step 2: 运行 E2E 并确认完整双角色合同**

Run: `go test ./e2e -run 'TestSupplierConfidentialLogBoundaryE2E' -count=1 -p=1`

Expected: PASS；普通三类响应 0 敏感字段/值，管理员三类审计事实完整，普通视频 URL 为 new-api 代理。

- [ ] **Step 3: 运行后端、前端和构建全量验证**

Run: `go test ./controller ./model ./relay ./service ./e2e -count=1 -p=1`

Run: `cd web && bun test --parallel=1 src/features/usage-logs`

Run: `cd web && bun run typecheck`

Run: `cd web && bun run lint`

Run: `cd web && bun run build`

Run: `git diff --check`

Expected: 所有命令退出码为 0，无测试失败、类型错误、Lint error 或构建错误。

- [ ] **Step 4: 启动本地服务并执行桌面/移动双角色页面验收**

使用项目现有 Compose 或开发服务启动当前分支，访问：

```text
http://127.0.0.1:3000/usage-logs/common?type=["2"]
http://127.0.0.1:3000/usage-logs/task
http://127.0.0.1:3000/cost-accounting
```

普通账号检查页面文本、复制入口和浏览器网络响应；管理员检查完整渠道、供应商成本核算、尝试时间线和审计历史。桌面和移动视口都要确认没有重叠，普通任务详情能显示完整 Ark 公共结果和代理视频 URL。

- [ ] **Step 5: 生成简体中文验收报告**

报告记录：提交 SHA、角色和权限矩阵、三类普通接口递归扫描结果、管理员审计保留结果、筛选探测结果、任务响应字段完整率、代理 URL 结果、桌面/移动页面结果、测试命令和实际输出统计。不得在报告中写入真实 API Key 或完整供应商认证信息。

- [ ] **Step 6: 提交 E2E 与报告**

```bash
git add e2e/supplier_confidential_log_boundary_e2e_test.go docs/superpowers/reports/2026-08-05-supplier-confidential-log-boundary-acceptance.md
git commit -m "test: verify supplier log confidentiality boundary"
```

- [ ] **Step 7: 最终审查并合并到本地 ysr**

对设计文档逐条核对，执行 `git log --oneline --decorate -8` 和 `git status --short`，确认工作树干净；完成规格审查和代码质量审查后，将 `codex/supplier-log-confidentiality` 非交互合并到本地 `ysr`，再在 `ysr` 上重跑关键后端测试、前端类型检查和 `git diff --check`。
