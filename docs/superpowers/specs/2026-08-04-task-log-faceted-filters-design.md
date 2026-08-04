# 任务日志多维筛选设计

## 目标

优化任务日志页面筛选交互，提供渠道、任务状态、请求模型和用户筛选。所有下拉选项均从当前时间范围内的全部任务日志去重获取，不受当前分页和其他已选筛选项影响。选择或清空下拉项后立即执行现有查询逻辑并回到第一页。

## 交互设计

- 渠道：管理员“全部”视图显示普通下拉框，选项为当前时间范围内出现过的渠道 ID。
- 状态：所有视图显示普通下拉框，选项为当前时间范围内出现过的任务状态，显示现有国际化状态名称。
- 请求模型：所有视图显示普通下拉框，选项来自任务记录的 `properties.origin_model_name`，即列表中的 `request_model`。
- 用户：仅管理员“全部”视图显示可输入搜索的下拉框，选项格式为 `用户 ID - 用户名`，提交值为用户 ID，不接受选项之外的自定义值。
- 选择或清空任一下拉框时直接调用现有立即查询逻辑，取消等待中的文本防抖查询，更新 URL 搜索参数并将页码重置为 1。
- 任务 ID 保持可输入和防抖自动查询；日期范围变化保持立即查询。
- 现有重置行为同时清空四个新增筛选项。
- 移动端筛选抽屉展示与桌面端相同的可用筛选项，并正确计算激活筛选数量。

## 数据与接口

新增管理员和用户任务筛选选项接口：

- `GET /api/task/filter-options`
- `GET /api/task/self/filter-options`

请求参数只接受 `start_timestamp`、`end_timestamp`。响应返回当前权限范围和时间范围内的去重数据：

```json
{
  "channels": [29, 30, 40],
  "statuses": ["FAILURE", "SUCCESS"],
  "request_models": ["doubao-seedance-2-0-260128"],
  "users": [
    { "id": 10, "username": "ark_sdk_matrix_user" }
  ]
}
```

普通用户接口不返回渠道和用户数据，避免暴露管理员信息。

任务列表接口新增：

- `request_model`：按客户端请求模型精确过滤。
- `user_id`：管理员按任务用户 ID 精确过滤。

现有 `status`、`channel_id`、`task_id` 和时间范围过滤继续使用。所有条件必须在数据库分页和计数之前生效。

## 数据库兼容

请求模型存储在 `tasks.properties` JSON 中。模型层提供稳定的请求模型 JSON 表达式：

- SQLite：`json_extract(properties, '$.origin_model_name')`
- MySQL：`JSON_UNQUOTE(JSON_EXTRACT(properties, '$.origin_model_name'))`
- PostgreSQL：`properties ->> 'origin_model_name'`

该表达式同时用于请求模型过滤和去重选项查询。其余筛选使用 GORM 的 `Where`、`Distinct`、`Pluck` 和关联查询，保持 SQLite、MySQL、PostgreSQL 一致。

## 前端数据流

筛选选项使用 React Query，以视图范围、开始时间和结束时间组成查询键。日期范围变化后重新获取选项；分页、任务 ID和其他筛选项变化不会重复请求选项。

筛选状态继续由 `TaskLogsFilterBar` 管理并同步 URL。下拉框 `onValueChange` 直接构造下一份筛选状态并调用 `flush`，不通过额外 `useEffect` 触发查询，避免重复请求和状态漂移。

## 测试

- 后端模型测试覆盖时间范围、权限范围、渠道、状态、请求模型和用户去重选项，以及请求模型/用户分页前过滤。
- 控制器测试覆盖查询参数映射和普通用户选项脱敏。
- 前端参数测试覆盖 `status`、`requestModel`、`userId` 写入 URL 和 API 参数。
- 前端组件测试覆盖四个下拉框渲染、管理员权限、可搜索用户、选择后立即查询、清空和重置、移动端激活数量。
- 最终运行相关 Go 测试、前端定向测试、类型检查、涉及文件 lint 和生产构建。
