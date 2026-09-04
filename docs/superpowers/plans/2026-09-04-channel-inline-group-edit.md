# 渠道列表内联分组编辑实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**目标：** 在渠道列表的分组列提供可搜索的多选下拉，并在选择变化后自动保存。

**架构：** 复用现有 `getGroups` 查询、`MultiSelect` 组件和 `updateChannel` API。分组单元格负责展示与本地编辑状态，保存成功使渠道列表查询失效，失败恢复快照。

**技术栈：** React 19、TypeScript、TanStack Query、Base UI Combobox、i18next、Vitest/React Testing Library。

---

### 任务 1：抽取可测试的内联分组保存逻辑

**文件：**
- 修改：`web/src/features/channels/lib/channel-actions.ts`
- 测试：`web/src/features/channels/lib/__tests__/channel-actions.test.ts`

- [ ] 新增 `handleUpdateChannelGroups(id, groups, queryClient, onError)`，使用 `formatGroups` 调用 `updateChannel`；成功失效渠道列表查询，失败调用回滚回调并显示统一错误。
- [ ] 为空数组直接返回失败，不发送空分组。
- [ ] 使用现有 `ERROR_MESSAGES.UPDATE_FAILED` 和 `SUCCESS_MESSAGES`，不新增无翻译文案。

### 任务 2：实现分组列多选编辑单元格

**文件：**
- 修改：`web/src/features/channels/components/channels-columns.tsx`
- 测试：`web/src/features/channels/components/__tests__/channels-columns.test.tsx`

- [ ] 新增 `ChannelGroupCell`，从 `parseGroupsList(channel.group)` 初始化本地值。
- [ ] 使用 `MultiSelect`，选项来自 `useChannels` 提供的全局分组选项；保留当前值并去重。
- [ ] 用稳定的 `isSaving` 状态禁用控件；保存成功更新本地快照，失败恢复旧值。
- [ ] 聚合标签行继续渲染现有 `GroupBadge` 列表。
- [ ] 保持敏感信息遮罩、列宽和移动端隐藏规则。

### 任务 3：提供分组选项上下文并接线

**文件：**
- 修改：`web/src/features/channels/components/channels-provider.tsx`
- 修改：`web/src/features/channels/components/channels-table.tsx`
- 修改：`web/src/features/channels/index.tsx`（如需传递查询数据）

- [ ] 在渠道表层复用 `getGroups` 查询结果，生成包含现有组和当前行历史值的选项。
- [ ] 不改变顶部“分组”筛选器的单选行为。
- [ ] 确保列表查询失效后编辑单元格不会因临时重渲染反复提交。

### 任务 4：验证

- [ ] 运行受影响 Vitest/React Testing Library 测试。
- [ ] 运行 `bun run typecheck`。
- [ ] 运行涉及文件 lint。
- [ ] 运行 `bun run build`。
