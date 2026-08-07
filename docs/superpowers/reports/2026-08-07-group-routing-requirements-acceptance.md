# 分组路由要求验收报告

## 验收范围

本次验收覆盖分组级 `require_real_person` 要求，以及它与模型路由策略、配置导入、重试和自动分组选择的联动。

## 已实现行为

- 管理端分组定价编辑器支持查看和切换“Require real person”，并保留其他分组配置。
- 分组要求与请求级 `routing.require_real_person` 采用 OR 合并；任一方要求真人脸时，只允许声明支持真人脸的路由目标。
- 分组要求缺少能力路由策略或候选目标时默认拒绝，不回退到未声明能力的旧路由。
- `auto` 分组逐组评估要求；当前分组没有兼容目标时继续尝试下一个分组。
- 配置导入支持分组路由要求，暂存阶段校验分组，激活阶段在同一事务中写入要求、策略、成本和渠道状态，失败时回滚。
- 路由事实、目标和不匹配原因写入管理员诊断；普通用户响应和日志不暴露上游模型及路由内部字段。

## 验收用例

新增 `e2e/group_routing_requirements_e2e_test.go`，覆盖：

1. 分组强制真人脸时选择兼容目标。
2. 指定不兼容渠道时返回 `no_compatible_route`，且不上游。
3. 真人脸不匹配时记录 `MismatchRealPerson` 管理员诊断。
4. 重试时保持分组要求，并切换到下一个兼容渠道。
5. `auto` 分组跳过不兼容分组，选择后续分组。

## 验证结果

- `go test ./...`：通过。
- `bun test src/features/system-settings/models/__tests__/group-routing-requirements.test.ts`：4 个测试通过。
- `bun run typecheck`：通过。
- 定向 `oxlint`：通过。
- `bun run build`：通过。

## 集成记录

- `208619377 feat: add group routing requirements editor`
- `398f49563 test: cover group routing requirements end to end`
- 已 fast-forward 合并到本地 `ysr`。
- 合并过程保留 `ysr` 原有用户改动，未覆盖：
  - `cmd/ark-video-material-seed/main.go`
  - `cmd/ark-video-material-seed/main_test.go`
  - `docs/superpowers/reports/2026-08-07-ark-sdk-video-strict-margin-material-matrix-acceptance.md`
