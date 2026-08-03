# 成本利润颜色与任务结果审计 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为成本利润字段增加正负语义颜色，并让任务日志稳定保存和展示异步任务最终公开结果。

**Architecture:** 前端复用一个基于规范整数字符串的符号样式函数。后端在视频任务轮询赢得终态 CAS 后，按原始请求路径调用已有公开响应转换器，并写入现有 `UserResponseData` 审计字段；持久化 seed 额外生成一条失败任务。

**Tech Stack:** Go、Gin、GORM、React 19、TypeScript、Tailwind CSS、Bun、Go `testing`、Node test runner。

---

### Task 1: 成本利润语义颜色

**Files:**
- Modify: `web/src/features/cost-accounting/lib/cost-rule.ts`
- Modify: `web/src/features/cost-accounting/components/profit-summary.tsx`
- Modify: `web/src/features/cost-accounting/components/profit-table.tsx`
- Modify: `web/src/features/cost-accounting/components/cost-request-detail.tsx`
- Test: `web/src/features/cost-accounting/components/__tests__/profit-report.test.tsx`
- Test: `web/src/features/cost-accounting/components/__tests__/cost-request-detail.test.tsx`

- [x] **Step 1: 写入正数、负数和零值颜色失败测试**
- [x] **Step 2: 运行受影响前端测试并确认因缺少语义 class 失败**
- [x] **Step 3: 实现规范整数符号解析和三个页面位置的 class 绑定**
- [x] **Step 4: 重跑前端测试并确认通过**

### Task 2: 轮询终态公开响应归档

**Files:**
- Create: `service/task_response_audit.go`
- Modify: `service/task_polling.go`
- Modify: `relay/relay_task.go`
- Test: `service/task_polling_test.go`
- Test: `relay/relay_task_response_audit_test.go`

- [x] **Step 1: 写入成功和失败终态自动归档失败测试**
- [x] **Step 2: 运行测试并确认 `UserResponseData` 为空**
- [x] **Step 3: 提取共用归档函数，并按 `request_path` 调用 OpenAI/ARK 转换器**
- [x] **Step 4: 在计费和成本结算完成后持久化公开响应，避免后续更新覆盖**
- [x] **Step 5: 重跑服务与 relay 测试并确认通过**

### Task 3: E2E 成功与失败 mock 数据

**Files:**
- Modify: `e2e/newapi_video_upstream_e2e_test.go`
- Modify: `cmd/ark-video-material-seed/main.go`
- Modify: `cmd/ark-video-material-seed/main_test.go`
- Modify: `docs/superpowers/reports/2026-08-03-seedance-official-sale-pricing-acceptance.md`

- [x] **Step 1: 增加 E2E 断言，要求轮询后成功与失败任务均已有公开终态结果**
- [x] **Step 2: 扩展 mock server，使指定任务返回稳定且已脱敏的失败响应**
- [x] **Step 3: 保留 110 条矩阵成功任务，额外通过真实链路生成 1 条失败展示任务**
- [x] **Step 4: 重跑 seed 并核对 110 条成功、1 条失败和 111 条终态结果**
- [x] **Step 5: 更新验收报告中的任务日志结果数据**

### Task 4: 全量验证与提交

**Files:**
- Verify only

- [x] **Step 1: 运行受影响 Go 测试与 `go test ./... -count=1 -p=1`**
- [x] **Step 2: 运行前端测试、`bun run typecheck` 和 `bun run build`**
- [x] **Step 3: 运行 `git diff --check`、`gofmt -l` 和私有字段扫描**
- [x] **Step 4: 提交并推送 `ysr`**
