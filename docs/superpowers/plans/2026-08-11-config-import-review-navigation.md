# 导入审查返回能力实施计划

> **For agentic workers:** 本计划在当前工作树内执行，所有步骤使用测试驱动开发；无需创建新批次或读取凭据。

**目标：** 让管理员能从发布审查返回定价审查，修正价格分组后重新暂存、路由审查和校验。

**架构：** 复用现有 `ConfigImportWizard` 的本地 `reviewStep` 状态和 `PricingStep` 回调，不改变后端 API 或批次状态机。`PublishReviewStep` 只新增一个可选的用户回调和次要返回按钮，发布 blocker 逻辑保持不变。

**技术栈：** React 19、TypeScript、Base UI Button、react-i18next、Bun 测试。

---

### 任务 1：锁定发布审查返回行为

**文件：**
- 修改：`web/src/features/config-import/components/__tests__/publish-review-step.test.tsx`
- 修改：`web/src/features/config-import/components/__tests__/config-import-wizard.test.tsx`

- [ ] 增加组件测试：传入 `onBack` 时，用户可通过可访问名称 `Back` 找到按钮，点击后执行回调；未勾选发布确认时发布按钮仍不可用。
- [ ] 增加向导测试：从 `ready` 批次的发布审查点击 `Back` 后显示 `Pricing review`，确认默认分组并点击 `Continue` 后调用定价审查回调。
- [ ] 运行这两个测试文件，确认新断言在实现前失败，失败原因是当前没有 `Back` 控件或回调。

### 任务 2：实现最小返回入口

**文件：**
- 修改：`web/src/features/config-import/components/publish-review-step.tsx`
- 修改：`web/src/features/config-import/index.tsx`

- [ ] 在 `PublishReviewStepProps` 增加 `onBack?: () => void`。
- [ ] 在标题操作区、发布按钮之前渲染 `onBack` 存在时的 outline `Button`，使用 `t('Back')`，加入 `ArrowLeft` 装饰图标并保持键盘可操作。
- [ ] 在向导的 `PublishReviewStep` 传入 `onBack={() => setReviewStep('pricing')}`，不改变 `canPublish`、发布确认或异步 mutation。
- [ ] 运行任务 1 的测试，确认全部通过。

### 任务 3：前端验证

**文件：**
- 仅检查任务 2 修改的 TSX 文件及对应测试。

- [ ] 在 `web/` 运行 `bun test --parallel=1 src/features/config-import/components/__tests__/publish-review-step.test.tsx src/features/config-import/components/__tests__/config-import-wizard.test.tsx`。
- [ ] 在 `web/` 运行 `bun run typecheck`。
- [ ] 在 `web/` 运行 `bunx oxlint -c .oxlintrc.json src/features/config-import/components/publish-review-step.tsx src/features/config-import/index.tsx src/features/config-import/components/__tests__/publish-review-step.test.tsx src/features/config-import/components/__tests__/config-import-wizard.test.tsx`。
- [ ] 在 `web/` 运行 `bun run build`，确认生产构建成功。

### 任务 4：浏览器回归和刷新流程

**文件：**
- 修改：本轮输出目录的 `验收报告.md` 和 `e2e.log`。

- [ ] 启动当前工作树前端开发服务并在内置浏览器打开其地址，恢复管理员登录态。
- [ ] 在批次 `#26` 发布审查点击 `Back`，在定价审查勾选 `ark-sdk-material-matrix-local`，继续完成路由差异和校验。
- [ ] 检查发布审查 blocker=0 后确认发布，记录发布结果、实体计数、退休模型和激活预览。
- [ ] 勾选激活确认并激活批次；必要时按页面动作刷新缓存，记录结果。
- [ ] 运行本轮 Mock Ark SDK E2E 命令，生成简体中文验收报告；不访问真实供应商。
