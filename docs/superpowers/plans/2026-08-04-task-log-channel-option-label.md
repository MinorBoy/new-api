# 任务日志渠道筛选标签实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 让任务日志渠道筛选与使用日志页面保持一致，展示 `渠道 ID - 渠道名称`、支持搜索和手动输入，同时保留当前时间范围内任务日志去重取选项及自动筛选行为。

**架构：** 模型层继续负责从当前时间范围内任务日志提取唯一渠道 ID；控制器通过现有 `model.GetChannelsByIds` 一次性补齐渠道名称，并返回结构化选项。前端将结构化渠道数据归一化为 Combobox 选项，任务日志筛选栏复用使用日志页面的 Combobox 配置和自动搜索控制器。

**技术栈：** Go、Gin、GORM、React 19、TypeScript、TanStack Query、Base UI、Bun、`node:test`、`testify`

---

## 文件职责

- `dto/task.go`：定义任务筛选渠道选项的 API DTO。
- `controller/task.go`：将当前时间范围内的渠道 ID 批量映射为渠道名称。
- `controller/task_filter_options_test.go`：保护接口结构、名称补齐和缺失渠道回退。
- `web/src/features/usage-logs/types.ts`：声明前端渠道选项响应类型。
- `web/src/features/usage-logs/hooks/use-task-log-filter-options.ts`：将接口数据归一化为稳定的 Combobox 选项。
- `web/src/features/usage-logs/hooks/__tests__/task-log-filter-options.test.ts`：保护标签格式、去重、排序和缺失名称回退。
- `web/src/features/usage-logs/components/task-logs-filter-bar.tsx`：改用与使用日志相同的渠道 Combobox。
- `web/src/features/usage-logs/components/__tests__/task-logs-filter-bar.test.tsx`：保护搜索、自定义 ID 和自动筛选行为。

### 任务 1：后端返回结构化渠道选项

**文件：**

- 修改：`controller/task_filter_options_test.go`
- 修改：`dto/task.go`
- 修改：`controller/task.go`

- [ ] **步骤 1：先写失败的接口契约测试**

在控制器测试数据中只创建一个仍存在的渠道，另一个任务渠道保持为已删除状态：

```go
require.NoError(t, db.Create(&model.Channel{
	Id: 29, Name: "paipu", Key: "test-key",
}).Error)
```

将测试响应的渠道字段声明为测试专用结构，避免依赖尚未实现的生产 DTO：

```go
type taskFilterChannelOption struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type taskFilterOptionsData struct {
	Channels      []taskFilterChannelOption  `json:"channels"`
	Statuses      []string                   `json:"statuses"`
	RequestModels []string                   `json:"request_models"`
	Users         []dto.TaskFilterUserOption `json:"users"`
}

type taskFilterOptionsResponse struct {
	Success bool                  `json:"success"`
	Message string                `json:"message"`
	Data    taskFilterOptionsData `json:"data"`
}
```

断言当前时间范围内的渠道按 ID 排序，存在渠道带名称，已删除渠道保留 ID 并回退为空名称：

```go
assert.Equal(t, []taskFilterChannelOption{
	{ID: 29, Name: "paipu"},
	{ID: 40, Name: ""},
}, payload.Data.Channels)
```

- [ ] **步骤 2：运行测试并确认按预期失败**

运行：

```powershell
go test ./controller -run TestGetAllTaskFilterOptionsUsesAllLogsInTimeRange -count=1
```

预期：失败，现有接口返回整数数组，无法解码为 `{id,name}` 对象。

- [ ] **步骤 3：增加生产 DTO 并批量补齐渠道名称**

在 `dto/task.go` 增加：

```go
type TaskFilterChannelOption struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
```

并将 `TaskFilterOptions.Channels` 改为：

```go
Channels []TaskFilterChannelOption `json:"channels,omitempty"`
```

在 `controller/task.go` 的管理员维度分支中批量查询渠道并保持任务日志渠道 ID 的原始排序：

```go
channels, err := model.GetChannelsByIds(options.ChannelIDs)
if err != nil {
	common.ApiError(c, err)
	return
}
channelNames := make(map[int]string, len(channels))
for _, channel := range channels {
	channelNames[channel.Id] = channel.Name
}
response.Channels = make([]dto.TaskFilterChannelOption, 0, len(options.ChannelIDs))
for _, channelID := range options.ChannelIDs {
	response.Channels = append(response.Channels, dto.TaskFilterChannelOption{
		ID: channelID, Name: channelNames[channelID],
	})
}
```

当 `options.ChannelIDs` 为空时直接返回空切片，不调用批量查询。

- [ ] **步骤 4：格式化并运行后端测试**

运行：

```powershell
gofmt -w dto/task.go controller/task.go controller/task_filter_options_test.go
go test ./controller ./model -count=1
```

预期：相关测试全部通过，接口返回结构化渠道选项，已删除渠道仍可筛选。

- [ ] **步骤 5：提交后端契约变更**

```powershell
git add dto/task.go controller/task.go controller/task_filter_options_test.go
git commit -m "feat: 补充任务日志渠道筛选名称"
```

### 任务 2：前端生成 `ID - 名称` 选项

**文件：**

- 修改：`web/src/features/usage-logs/types.ts`
- 修改：`web/src/features/usage-logs/hooks/use-task-log-filter-options.ts`
- 修改：`web/src/features/usage-logs/hooks/__tests__/task-log-filter-options.test.ts`

- [ ] **步骤 1：先修改测试数据并写失败断言**

将渠道测试输入改为结构化数组，并覆盖重复 ID 和缺失名称：

```ts
channels: [
  { id: 40, name: '' },
  { id: 29, name: 'paipu' },
  { id: 40, name: 'stale duplicate' },
],
```

期望结果：

```ts
assert.deepEqual(options.channelOptions, [
  { value: '29', label: '29 - paipu' },
  { value: '40', label: '40' },
])
```

- [ ] **步骤 2：运行归一化测试并确认失败**

运行：

```powershell
bun test --parallel=1 src/features/usage-logs/hooks/__tests__/task-log-filter-options.test.ts
```

工作目录：`web/`

预期：TypeScript 或断言失败，因为生产类型和归一化逻辑仍要求 `number[]`。

- [ ] **步骤 3：实现结构化类型和稳定归一化**

在 `types.ts` 增加：

```ts
export interface TaskLogFilterChannel {
  id: number
  name: string
}
```

将响应字段改为：

```ts
channels?: TaskLogFilterChannel[]
```

在 `normalizeTaskLogFilterOptions` 中按渠道 ID 去重、排序并生成与使用日志一致的标签；首次出现的当前接口结果为权威值：

```ts
const channels = new Map<number, TaskLogFilterOption>()
for (const channel of data.channels ?? []) {
  if (channels.has(channel.id)) continue
  channels.set(channel.id, {
    value: String(channel.id),
    label: channel.name
      ? `${channel.id} - ${channel.name}`
      : String(channel.id),
  })
}
const channelOptions = [...channels.entries()]
  .sort(([left], [right]) => left - right)
  .map(([, option]) => option)
```

- [ ] **步骤 4：运行前端归一化测试和类型检查**

运行：

```powershell
bun test --parallel=1 src/features/usage-logs/hooks/__tests__/task-log-filter-options.test.ts
bun run typecheck
```

工作目录：`web/`

预期：测试与类型检查通过。

- [ ] **步骤 5：提交前端数据契约变更**

```powershell
git add web/src/features/usage-logs/types.ts web/src/features/usage-logs/hooks/use-task-log-filter-options.ts web/src/features/usage-logs/hooks/__tests__/task-log-filter-options.test.ts
git commit -m "feat: 格式化任务日志渠道筛选选项"
```

### 任务 3：渠道筛选改用与使用日志一致的 Combobox

**文件：**

- 修改：`web/src/features/usage-logs/components/task-logs-filter-bar.tsx`
- 修改：`web/src/features/usage-logs/components/__tests__/task-logs-filter-bar.test.tsx`

- [ ] **步骤 1：先写失败的组件交互测试**

将测试夹具渠道标签改为：

```ts
channelOptions: [
  { value: '29', label: '29 - paipu' },
  { value: '40', label: '40 - backup' },
],
```

增强管理员渲染断言，要求渠道控件是可编辑 Combobox：

```ts
const channelInput = mounted.container.querySelector(
  '[role="combobox"][aria-label="Channel ID"]'
) as HTMLInputElement | null
assert.ok(channelInput)
```

在组件动态导入之前，将自动搜索边界替换为同步测试控制器；真实的 350ms 防抖继续由 `hooks/__tests__/auto-search.test.ts` 保护：

```ts
mock.module('@/features/usage-logs/hooks/use-auto-search', () => ({
  useAutoSearch: <TValue,>(onSearch: (value: TValue) => void) => ({
    schedule: onSearch,
    flush: onSearch,
    cancel: () => {},
  }),
}))
```

增加手动输入测试，向渠道 Combobox 输入 `88`，触发 `input` 事件后断言：

```ts
await act(async () => {
  channelInput.focus()
  channelInput.value = '88'
  channelInput.dispatchEvent(new Event('input', { bubbles: true }))
})

const search = lastNavigate.current?.search as
  | Record<string, unknown>
  | undefined
assert.equal(search?.channel, '88')
assert.equal(search?.page, 1)
```

保留候选项选择测试，将选项文本改为 `40 - backup`，并继续断言提交值是 `40` 而非标签文本。

- [ ] **步骤 2：运行组件测试并确认失败**

运行：

```powershell
bun test --parallel=1 src/features/usage-logs/components/__tests__/task-logs-filter-bar.test.tsx
```

工作目录：`web/`

预期：失败，因为当前渠道控件是不可输入的 Select，且选项仍只显示 ID。

- [ ] **步骤 3：替换为与使用日志一致的 Combobox**

移除任务渠道使用的 `TaskLogSelectFilter`，改为：

```tsx
const channelFilter =
  isAdmin && taskFilters ? (
    <LogsFilterField>
      <Combobox
        options={taskFilterOptions.channelOptions}
        ariaLabel={t('Channel ID')}
        placeholder={t('Channel ID')}
        value={taskFilters.channel || ''}
        onValueChange={(value) => handleTextChange('channel', value ?? '')}
        onCompositionStart={() => handleCompositionStart('channel')}
        onCompositionEnd={(event: CompositionEvent<HTMLInputElement>) =>
          handleCompositionEnd('channel', event.currentTarget.value)
        }
        allowCustomValue
        openOnFocus
        className='h-8 min-w-0 text-sm leading-5'
      />
    </LogsFilterField>
  ) : null
```

状态和请求模型继续使用 Select，用户继续使用不允许自定义值的 Combobox。渠道输入复用现有 350ms 自动搜索控制器，最终仍调用 `submitFilters` 并设置 `page: 1`。

- [ ] **步骤 4：运行组件测试、用量日志测试和静态检查**

运行：

```powershell
bun test --parallel=1 src/features/usage-logs
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/usage-logs/components/task-logs-filter-bar.tsx src/features/usage-logs/components/__tests__/task-logs-filter-bar.test.tsx src/features/usage-logs/hooks/use-task-log-filter-options.ts src/features/usage-logs/hooks/__tests__/task-log-filter-options.test.ts src/features/usage-logs/types.ts
bun run build
```

工作目录：`web/`

预期：所有用量日志测试通过，类型检查和 lint 无错误，生产构建成功。

- [ ] **步骤 5：提交渠道 Combobox 交互变更**

```powershell
git add web/src/features/usage-logs/components/task-logs-filter-bar.tsx web/src/features/usage-logs/components/__tests__/task-logs-filter-bar.test.tsx
git commit -m "feat: 统一任务日志渠道筛选交互"
```

### 任务 4：容器与浏览器验收

**文件：** 无生产文件修改。

- [ ] **步骤 1：运行完整相关后端测试**

```powershell
go test ./model ./controller ./cmd/ark-video-material-seed -count=1
```

预期：全部通过。

- [ ] **步骤 2：重建本地容器**

```powershell
docker compose -f docker-compose.local.yml up -d --build new-api
docker compose -f docker-compose.local.yml ps new-api
```

预期：`new-api-local-new-api-1` 状态为 `healthy`，端口为 `127.0.0.1:3000`。

- [ ] **步骤 3：检查服务健康状态**

```powershell
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:3000/api/status
```

预期：HTTP 200。

- [ ] **步骤 4：完成桌面端浏览器验收**

打开 `http://127.0.0.1:3000/usage-logs/task`，确认：

- 渠道控件是可输入 Combobox。
- 候选项显示 `ID - 渠道名称`。
- 选中候选项后 URL 中保存渠道 ID，页码为 1。
- 手动输入未出现在候选项中的渠道 ID 后自动筛选。
- 状态、请求模型、用户筛选行为不变。

- [ ] **步骤 5：完成移动端浏览器验收**

使用 `390x844` 视口打开筛选抽屉，确认渠道标签不与其他控件重叠，长渠道名称可在输入框内正常显示或截断，筛选抽屉可滚动。
