# 8yes Seedance 渠道接入实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 接入 8yes Seedance 视频渠道，使 Ark SDK 用户通过 `/api/v3/contents/generations/tasks/*` 零代码改动调用 8yes 的创建、轮询和视频内容代理。

**架构：** 在 `relay/channel/task/newapivideo` 增加 8yes provider profile 与 dialect，复用共享 Ark task 路由、任务持久化、公开任务 ID、所有权、计费结算和退款。8yes 轮询没有直链时使用既有 `/v1/videos/:task_id/content` 代理，并仅在代理层使用上游私有任务 ID。

**技术栈：** Go 1.22、Gin、httptest、testify、GORM 任务 fixtures、React/TypeScript、Bun、现有 i18n 和 channel-config converter。

---

## 任务 1：先写 provider 请求契约测试

**文件：**
- 创建：`relay/channel/task/newapivideo/eightyes_request_test.go`
- 修改：`relay/channel/task/newapivideo/profile.go`、`relay/channel/task/newapivideo/native.go`

- [ ] 写失败测试：profile 名称为 `8yes`，提交路径 `/v1/videos`，轮询路径 `/v1/videos/{task_id}`，内容路径由共享代理拼接，JSON content type，空静态模型目录，默认时长 5 秒。
- [ ] 写失败测试：Ark 文本和三类媒体按顺序编码为 `prompt`、`referenceImages`、`referenceVideos`、`referenceAudios`；不混用私有 `task_id` 内容。
- [ ] 写失败测试：`duration`、`ratio`、`seed` 的显式值保留，`generate_audio=true` 兼容接受但不转发；缺省字段不出现在 JSON；`seed` 接受 `-1..4294967295`，拒绝小数、负值越界和溢出。
- [ ] 运行 `go test ./relay/channel/task/newapivideo -run 'TestEightYes|TestBuildEightYes' -count=1`，确认因 dialect/profile 不存在而失败。

## 任务 2：实现 8yes 请求编码和校验

**文件：**
- 创建：`relay/channel/task/newapivideo/eightyes_request.go`
- 修改：`relay/channel/task/newapivideo/profile.go`、`relay/channel/task/newapivideo/native.go`、`relay/channel/task/newapivideo/adaptor.go`

- [ ] 增加 `ChannelNameEightYes`、`videoRequestDialectEightYes` 和 `eightYesProtocolProfile`；`defaultDurationSeconds=5`、`requirePublicHTTPMedia=true`、`singleFrameImagesAreReferences=true`。
- [ ] 定义带 `omitempty` 指针字段的私有 DTO：`model`、`prompt`、`duration`、`ratio`、`seed`、`referenceImages`、`referenceVideos`、`referenceAudios`；使用 `common.Marshal`。
- [ ] 将 `image_url` 的空角色、`first_frame`、`reference_image` 作为图片引用；明确拒绝 `last_frame`，因为 8yes 数组协议没有首尾帧角色字段。将视频/音频按现有 `reference_video`/`reference_audio` 角色写入对应数组。
- [ ] 接受 `generate_audio=true` 并省略上游不识别的字段；`generate_audio=false` 与参考音频冲突时返回 `InvalidParameter.generate_audio`。对 `watermark`、非 `default` 的 `service_tier`、启用 `draft`、非空 `tools`、`callback_url` 返回确定性 400。
- [ ] 扩展共享 seed 校验，使 8yes 进入同一安全整数范围；扩展 `arkRequest` 的 parser/profile 分派，使 8yes 不被 generic dialect 接管。保持 duration、媒体数量、SSRF 校验和计费前 provider validation 完成标记。
- [ ] 运行 `gofmt -w relay/channel/task/newapivideo/*.go` 与 `go test ./relay/channel/task/newapivideo -run 'TestEightYes|TestBuildEightYes' -count=1`，确认请求测试通过。

## 任务 3：实现创建/轮询响应和内容代理准备

**文件：**
- 修改：`relay/channel/task/newapivideo/dto.go`、`relay/channel/task/newapivideo/response.go`
- 修改：`controller/video_proxy.go`
- 修改：`relay/channel/task/taskcommon/helpers.go`（仅在需要时复用，不改变 URL 格式）
- 创建：`relay/channel/task/newapivideo/eightyes_response_test.go`

- [ ] 写失败测试：创建响应兼容 `id`、`task_id`、`taskId`，相互冲突时返回 `invalid_response`；轮询兼容 `queued`、`processing`、`running`、`completed`、`succeeded`、`failed`、`cancelled`、`expired`。
- [ ] 扩展 8yes 直接响应字段 `seconds`、`expires_at`、`metadata`，优先读取 `video_url`/`url`/`result_url`/metadata URL；成功无 URL 时对 8yes 允许返回空 TaskInfo URL，由共享轮询逻辑生成公开代理 URL，其他渠道继续 fail closed。
- [ ] 实现 `ConvertToArkVideoTask` 的公开投影，保留本地任务 ID、客户端模型、Ark 状态、允许的 content URL 和清理后的错误，不输出上游 ID、模型、Key、渠道、额度或原始响应。
- [ ] 在 `controller.VideoProxy` 增加 `ChannelTypeEightYes` 分支：拼接渠道 Base URL `/v1/videos/{task.GetUpstreamTaskID()}/content`，使用渠道 Key 的 Bearer 认证，保持公开请求仍为 `/v1/videos/{public_task_id}/content`。
- [ ] 运行 `go test ./relay/channel/task/newapivideo -run 'TestEightYes|TestParse.*Task|TestConvert' -count=1` 和内容代理 focused test。

## 任务 4：注册后端类型 210 并接入共享 Ark 生命周期

**文件：**
- 修改：`constant/channel.go`、`constant/channel_test.go`
- 修改：`relay/relay_adaptor.go`、`relay/seedance_task.go`、`relay/relay_task.go`
- 修改：`controller/channel.go`、`controller/channel-test.go`、`controller/channel_test_internal_test.go`
- 修改：`relay/relay_task_seedance_test.go`、`relay/cost_accounting_adaptor_test.go`、`relay/relay_task_billing_test.go`
- 修改：`service/config_import_stage.go`、`service/config_import_stage_test.go`

- [ ] 将 `ChannelTypeEightYes=210`、`ChannelTypeDummy=211`、默认 URL `https://8yes.cc`、显示名 `8yes` 加入共享注册；所有既有 Dummy=210 断言改为 211，不能覆盖已有渠道类型。
- [ ] `GetTaskAdaptor("210")` 返回 8yes adaptor；加入 Seedance task platform、Ark converter、任务路由、成本能力矩阵和 generic-channel-test 排除集合；不加入通用 OpenAI 模型发现路由。
- [ ] 在 `normalizedConfigImportBindingChannelType` 将 `CH-8YES` 从历史通用类型 1 迁移为 210；保留已有模型映射和 disabled 状态，不把 workbook 示例当作静态模型目录。
- [ ] 增加 Ark 创建、公开单查/列表、用户隔离、私有 ID/Key 清理、失败退款一次和重复轮询幂等的测试。
- [ ] 运行 `gofmt` 与 `go test ./constant ./relay ./controller ./service -run 'TestEightYes|TestSeedanceTask|TestSupportsGenericChannelTest|TestConfigImport' -count=1`。

## 任务 5：管理表单、i18n 和配置导入测试

**文件：**
- 修改：`web/src/features/channels/constants.ts`、`web/src/features/channels/lib/channel-type-config.ts`、`web/src/features/channels/lib/channel-utils.ts`、`web/src/features/channels/lib/channel-form.ts`
- 修改：`web/src/channel-config-converter/document.ts`、`web/src/channel-config-converter/__tests__/v1.test.ts`
- 修改：`web/src/i18n/locales/en.json`、`zh.json`、`zh-TW.json`、`fr.json`、`ru.json`、`ja.json`、`vi.json`
- 修改/创建：相应 `web` channel config tests

- [ ] 写失败测试：类型 210、NewAPI 图标、默认 URL `https://8yes.cc`、静态模型空列表、task-only、禁用 generic test/model fetch、映射提示和自定义代理 URL 保留。
- [ ] 类型切换到 210 时新建渠道默认 disabled；加入 managed default URL、key prompt、task-only warning、显示顺序和 `PRE_ACCEPTANCE_DISABLED_CHANNEL_TYPES`。
- [ ] 配置导入转换器将 `CH-8YES` 输出为 210；断言 `MAP-8YES-*` 映射仍存在且来源字段未被改写。
- [ ] 为新增用户文案补齐七种 locale，运行 `bun run i18n:sync`、受影响 Bun tests、`bun run typecheck` 和涉及文件 lint。

## 任务 6：Ark 生命周期、计费和真实验收门禁

**文件：**
- 创建：`relay/channel/task/newapivideo/eightyes_e2e_test.go`
- 修改：`relay/relay_task_billing_test.go`、`service/task_polling_test.go`
- 创建（真实凭据提供后）：`docs/superpowers/reports/2026-08-02-8yes-channel-acceptance.md`

- [ ] httptest 模拟 POST `/v1/videos`、GET `/v1/videos/{id}` 和 GET `/v1/videos/{id}/content`，断言 Bearer、字段名、请求体、公共 ID、代理内容和任务归属。
- [ ] 覆盖文本、图片、视频、音频组合；覆盖上游 4xx/5xx、未知状态、无 URL 成功、内容下载失败，确保预扣/结算/退款各执行一次。
- [ ] 使用 `EIGHTYES_API_KEY`（可选 `EIGHTYES_BASE_URL`）执行真实创建、轮询、MP4 可读性、Ark 单查/列表、内容代理和失败退款；凭据不落盘。
- [ ] 真实验收通过前保持类型 210 渠道 disabled；报告只写脱敏请求/响应和结论。

## 完成前验证

- [ ] `go test ./... -count=1`
- [ ] `go build ./...`
- [ ] `cd web; bun test; bun run typecheck; bun run build`
- [ ] `git diff --check`
- [ ] 逐项确认 Ark 创建、单查、列表、公开 ID、用户隔离、内容代理、计费结算和失败退款没有回退到通用 OpenAI 路由。
