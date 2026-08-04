# 任务日志多维筛选实施计划

> **执行要求：** 使用测试驱动开发逐项实施；每个生产变更前必须先运行对应失败测试。

**目标：** 为任务日志增加基于完整时间范围日志数据的渠道、状态、请求模型和用户下拉筛选，并在选择后立即查询。

**架构：** 后端新增任务筛选选项接口，并扩展现有任务列表查询参数。模型层统一构造任务查询和跨数据库请求模型 JSON 表达式，确保过滤先于分页。前端使用 React Query 获取时间范围选项，筛选状态继续通过 TanStack Router URL 管理。

**技术栈：** Go、Gin、GORM、React 19、TypeScript、TanStack Query、TanStack Router、Base UI Combobox、Vitest。

---

### 任务一：后端任务查询支持请求模型和用户

**文件：**

- 修改：`model/task.go`
- 测试：`model/task_filter_test.go`

- [ ] 新增失败测试，构造多个用户、渠道、状态、时间和 `Properties.OriginModelName` 不同的任务，断言 `RequestModel` 与 `UserID` 在计数和分页前生效。
- [ ] 运行 `go test ./model -run 'TestTaskQueryFilters' -count=1`，确认因缺少字段和过滤逻辑失败。
- [ ] 给 `SyncTaskQueryParams` 增加 `RequestModel string`，抽取复用的任务查询构造逻辑，并在管理员查询中支持 `UserID`、在管理员和用户查询中支持 `RequestModel`。
- [ ] 请求模型条件使用按数据库类型选择的 JSON 表达式：SQLite `json_extract`、MySQL `JSON_UNQUOTE(JSON_EXTRACT(...))`、PostgreSQL `->>`。
- [ ] 重新运行模型测试并确认通过。

### 任务二：提供完整时间范围筛选选项接口

**文件：**

- 修改：`dto/task.go`
- 修改：`model/task.go`
- 修改：`controller/task.go`
- 修改：`router/api-router.go`
- 测试：`controller/task_filter_options_test.go`

- [ ] 新增失败测试，断言管理员接口返回时间范围内去重并排序的渠道、状态、请求模型和用户，且忽略范围外任务。
- [ ] 新增失败测试，断言普通用户接口只基于当前用户任务返回状态和请求模型，不返回渠道或用户。
- [ ] 运行 `go test ./controller -run 'TestTaskFilterOptions' -count=1`，确认路由/控制器能力缺失导致失败。
- [ ] 新增结构化 DTO：`TaskFilterUserOption` 和 `TaskFilterOptions`。
- [ ] 模型层增加选项查询，管理员按时间范围查询四种维度；普通用户追加 `user_id` 条件并只返回允许维度。
- [ ] 控制器解析秒级时间戳，新增 `GetAllTaskFilterOptions`、`GetUserTaskFilterOptions`，路由注册 `/api/task/filter-options` 与 `/api/task/self/filter-options`。
- [ ] 重新运行控制器和模型定向测试并确认通过。

### 任务三：扩展前端筛选参数和选项查询

**文件：**

- 修改：`web/src/features/usage-logs/types.ts`
- 修改：`web/src/features/usage-logs/lib/filter.ts`
- 修改：`web/src/features/usage-logs/lib/utils.ts`
- 修改：`web/src/features/usage-logs/api.ts`
- 修改：`web/src/routes/_authenticated/usage-logs/$section.tsx`
- 新增：`web/src/features/usage-logs/hooks/use-task-log-filter-options.ts`
- 测试：`web/src/features/usage-logs/lib/__tests__/query-params.test.ts`
- 测试：`web/src/features/usage-logs/hooks/__tests__/task-log-filter-options.test.tsx`

- [ ] 新增失败测试，断言任务筛选将 `status`、`requestModel`、`userId` 写入 URL，并转换为 API 的 `status`、`request_model`、`user_id`。
- [ ] 新增失败测试，断言选项查询键只由管理员范围和时间范围组成，并将接口结果转换为稳定排序的下拉选项。
- [ ] 运行对应 Bun 测试，确认缺少类型、参数和 Hook 导致失败。
- [ ] 扩展 `TaskLogFilters`、路由搜索 schema 和 `GetTaskLogsParams`；在 `buildSearchParams`、`fetchLogs` 参数构造中接入三个字段。
- [ ] 新增筛选选项 API 类型和请求函数；Hook 使用 React Query 获取 `/api/task/filter-options` 或 `/api/task/self/filter-options`。
- [ ] 重新运行定向测试并确认通过。

### 任务四：实现下拉筛选和自动查询

**文件：**

- 修改：`web/src/features/usage-logs/components/task-logs-filter-bar.tsx`
- 新增：`web/src/features/usage-logs/components/__tests__/task-logs-filter-bar.test.tsx`

- [ ] 新增失败组件测试：管理员任务视图显示渠道、状态、请求模型和用户筛选；普通用户只显示状态和请求模型。
- [ ] 新增失败组件测试：用户下拉可以输入筛选候选项，但不能提交自定义用户。
- [ ] 新增失败组件测试：选择或清空每个下拉项都会立即导航到第一页；任务 ID 输入仍使用防抖查询；重置清空全部条件。
- [ ] 运行 `bun test src/features/usage-logs/components/__tests__/task-logs-filter-bar.test.tsx`，确认控件缺失导致失败。
- [ ] 使用现有 `Combobox` 构建渠道、状态、请求模型和用户控件。下拉 `onValueChange` 直接调用 `handleImmediateChange`；任务 ID 保持 `schedule`。
- [ ] 将新增筛选同步到 URL 恢复逻辑、激活状态、移动端抽屉和筛选计数。
- [ ] 重新运行组件测试并确认通过。

### 任务五：国际化与完整验证

**文件：**

- 按需修改：`web/scripts/add-missing-keys.mjs`
- 由脚本更新：`web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`

- [ ] 检查是否可复用现有 `Channel ID`、`Request Model`、`User` 和状态文案；仅对确实缺失的键通过 `add-missing-keys.mjs` 一次性写入七种语言。
- [ ] 运行 `node scripts/add-missing-keys.mjs`、删除临时脚本并运行 `bun run i18n:sync`。
- [ ] 运行受影响 Go 测试、前端定向测试、`bun run typecheck`、涉及文件 lint 和 `bun run build`。
- [ ] 在 `http://127.0.0.1:3000/usage-logs/task` 验证桌面与移动端：选项覆盖完整时间范围、选择后立即查询、清空恢复、管理员权限正确。
