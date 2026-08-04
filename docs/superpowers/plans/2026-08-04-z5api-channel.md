# Z5API Seedance 渠道接入实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task with checkpoints.

**Goal:** 新增 Z5API task-only Seedance 渠道，使 Ark SDK `/api/v3/contents/generations/tasks/*` 无需客户端改动即可调用 Z5API 的 `/v1/videos`，并覆盖多模态请求、任务轮询、计费和管理配置。

**Architecture:** 在 `relay/channel/task/newapivideo` 中添加独立 Z5API protocol profile、请求编码和响应解析，复用现有 Ark 路由、公共任务 ID、轮询、计费、结算、退款和隐私投影。共享注册文件和公共前端注册由唯一维护者修改；Z5API 分支只负责 provider profile、请求校验/编码、响应测试、E2E 和渠道专属表单测试。

**Tech Stack:** Go 1.22、Gin、GORM、`common.Marshal/Unmarshal`、`httptest`、React 19、TypeScript、Bun、Ark 视频任务协议。

---

## 文件边界

**Z5API 专属文件：**

- Create: `relay/channel/task/newapivideo/z5api_request.go`
- Create: `relay/channel/task/newapivideo/z5api_request_test.go`
- Create: `relay/channel/task/newapivideo/z5api_response_test.go`
- Create: `e2e/z5api_upstream_e2e_test.go`
- Modify: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`
- Modify: `relay/channel/task/newapivideo/dto.go`
- Modify: `relay/channel/task/newapivideo/response.go`

**共享注册文件（指定唯一维护者）：**

- Modify: `constant/channel.go`, `constant/channel_test.go`
- Modify: `relay/relay_adaptor.go`, `relay/seedance_task.go`, `relay/relay_task.go`
- Modify: `relay/video_route_contract.go`, `relay/relay_task_seedance_test.go`, `relay/cost_accounting_adaptor_test.go`
- Modify: `controller/channel.go`, `controller/channel-test.go`, `controller/channel_test_internal_test.go`
- Modify: `service/config_import_stage.go`, `service/config_import_stage_test.go`

**公共前端注册文件（指定唯一维护者）：**

- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/channel-config-converter/document.ts`
- Modify: `web/tests/channel-type-config.test.ts`
- Modify: `web/src/channel-config-converter/__tests__/v1.test.ts`
- Modify: `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

不要改动 `docs/new-channels/cn-z5api.html`；不要把 HTML 示例模型复制为生产静态模型目录。

## Task 1: Z5API profile 与请求编码

**Files:** `relay/channel/task/newapivideo/profile.go`, `relay/channel/task/newapivideo/z5api_request.go`, `relay/channel/task/newapivideo/z5api_request_test.go`

- [ ] 在 `profile.go` 增加 `ChannelNameZ5API = "Z5API"`、`videoRequestDialectZ5APIMedia` 和 `z5apiProtocolProfile()`；配置提交/查询路径 `/v1/videos`、`/v1/videos/{task_id}`、JSON、公网 URL和最多 9 图片/3 视频/3 音频协议上限，不写静态模型列表。
- [ ] 先写失败测试，覆盖文生、首帧、首尾帧、图片/视频/音频混合请求的精确 JSON；缺省 `parameters` 字段省略，显式 `duration` 指针不丢失；显式 `watermark`、`seed`、`generate_audio`、`callback_url`、`draft`、非默认 `service_tier` 返回 `InvalidParameter.*`。
- [ ] 测试图片 9/10、视频 3/4、音频 3/4；允许 `first_frame`、`last_frame`、`reference_image`、`reference_video`、`reference_voice`，拒绝错误角色、`file://`、`asset://`、私网 URL和错误 data URI。
- [ ] 运行失败测试：

~~~~powershell
go test ./relay/channel/task/newapivideo -run 'TestZ5API|TestBuildZ5' -count=1
~~~~

Expected: 因 Z5API 方言和 profile 尚不存在而失败。
- [ ] 实现 `z5apiRequest`（`Model`、`Prompt`、`Media`、`Parameters`）；可选参数使用指针和 `omitempty`，通过 `common.Marshal` 编码。将 Ark 媒体角色映射为文档声明的 Z5API type，音频映射为 `reference_voice`。
- [ ] 实现 `validateZ5APIRequest`，复用 Ark 语义、角色、数量、公网 URL、比例和 `MaxTaskDurationSeconds` 校验；模型细分能力交给导入 route contract。
- [ ] 在 `TaskAdaptor.ValidateRequestAndSetAction` 与 `BuildRequestBody` 增加 Z5API 分支，并要求 `ProviderValidationComplete` 后编码。
- [ ] 运行：

~~~~powershell
gofmt -w relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/z5api_request.go relay/channel/task/newapivideo/z5api_request_test.go relay/channel/task/newapivideo/adaptor.go
go test ./relay/channel/task/newapivideo -run 'TestZ5API|TestBuildZ5|TestPaipu|TestEightYes' -count=1
~~~~
- [ ] 提交：`git commit -m "feat(z5api): add request protocol profile"`。

## Task 2: 响应解析与 Ark 转换

**Files:** `relay/channel/task/newapivideo/dto.go`, `relay/channel/task/newapivideo/response.go`, `relay/channel/task/newapivideo/z5api_response_test.go`

- [ ] 先写失败测试：`pending/processing` 为进行中，`completed + object` 为成功，`failed` 清理错误，`seconds` 进入 duration 和 CostMeter；成功无 URL、非法/超大 seconds 遵循现有错误和边界策略。
- [ ] 扩展 direct task DTO，保存 Z5API 的 `object` 和 `seconds` 字段；`directTaskVideoURL` 对 Z5API 方言优先读取 `object`，不覆盖既有 `url/video_url/result_url`。
- [ ] 在解析阶段使用 decimal 和公共边界检查，禁止裸 `int(float)`；失败消息走 `sanitizeUpstreamFailure`，不返回 Key、私有任务 ID或渠道字段。
- [ ] 在 `ConvertToArkVideoTask` 中输出用户模型、公开任务 ID、Ark status、`content.video_url`、duration、resolution和 usage；覆盖单查与列表隐私隔离。
- [ ] 运行：

~~~~powershell
gofmt -w relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/z5api_response_test.go
go test ./relay/channel/task/newapivideo -run 'TestZ5API|TestParse.*Task|TestConvertToArkVideoTask' -count=1
~~~~
- [ ] 提交：`git commit -m "feat(z5api): normalize video task responses"`。

## Task 3: 注册渠道类型 211

**Files:** `constant/channel.go`, `constant/channel_test.go`, `relay/relay_adaptor.go`, `relay/seedance_task.go`, `relay/relay_task.go`, `relay/video_route_contract.go`, `relay/relay_task_seedance_test.go`, `relay/cost_accounting_adaptor_test.go`, `controller/channel.go`, `controller/channel-test.go`, `controller/channel_test_internal_test.go`, `service/config_import_stage.go`, `service/config_import_stage_test.go`

- [ ] 先写失败测试，断言类型、名称、默认 URL、`GetTaskAdaptor("211")`、Ark converter、cost capabilities、Seedance allow-list、task-only 和配置导入 task protocol。
- [ ] 分配 `ChannelTypeZ5API = 211`，将 `ChannelTypeDummy` 移到 212，设置 `https://z5api.com` 与 `Z5API`；不加入 `common.ChannelType2APIType`。
- [ ] 注册 `newapivideo.NewZ5APITaskAdaptor()`，把 211 加入 Seedance 三个平台集合、Ark converter、relay task routing、video route contract、cost capability 和 generic-test exclusion。
- [ ] 配置导入把 `CH-Z5API` 绑定到 211，保留导入模型和 disabled 状态，不从 HTML 生成模型目录。
- [ ] 运行：

~~~~powershell
gofmt -w constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/video_route_contract.go relay/relay_task_seedance_test.go relay/cost_accounting_adaptor_test.go controller/channel.go controller/channel-test.go controller/channel_test_internal_test.go service/config_import_stage.go service/config_import_stage_test.go
go test ./constant ./relay ./controller ./service -run 'TestZ5API|TestSeedanceTask|TestSupportsGenericChannelTest|TestConfigImport' -count=1
~~~~
- [ ] 提交：`git commit -m "feat(z5api): register task-only channel"`。

## Task 4: 管理端配置与导入映射

**Files:** `web/src/features/channels/constants.ts`, `web/src/features/channels/lib/channel-type-config.ts`, `web/src/channel-config-converter/document.ts`, `web/tests/channel-type-config.test.ts`, `web/src/channel-config-converter/__tests__/v1.test.ts`, `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`

- [ ] 先写失败测试，断言 `CHANNEL_TYPES[211] === "Z5API"`、显示顺序、NewAPI 图标、默认 URL、task-only、不可拉取模型、空静态模型数组和 `CH-Z5API -> 211`。
- [ ] 增加 211 配置：默认 Base URL `https://z5api.com`、原始 Key 提示、模型由导入/手工映射维护；加入 managed default URL 集合，不加入 model fetch 集合。
- [ ] 在 V1 converter 表和 fixture 中增加 `CH-Z5API`，确保 provider hint 为 `z5api`，导入 upstream model 不被静态配置覆盖。
- [ ] 为七种 locale 增加 Z5API、默认地址、Key 提示和“仅在真实上游契约验收后启用”文案，用户可见文本全部走 `t()`。
- [ ] 运行：

~~~~powershell
Set-Location web
bun test tests/channel-type-config.test.ts src/channel-config-converter/__tests__/v1.test.ts
bun run i18n:sync
bun run typecheck
Set-Location ..
~~~~
- [ ] 提交：`git commit -m "feat(web): add Z5API channel configuration"`。

## Task 5: Ark 生命周期、路由和计费 E2E

**Files:** `e2e/z5api_upstream_e2e_test.go`, `relay/relay_task_billing_test.go`, `relay/video_route_contract_test.go`

- [ ] 先写 mock server 失败测试：Ark POST 收到 `/v1/videos`、Bearer Key和精确 `media/parameters` JSON；返回 pending，轮询 processing，终态 completed/object/seconds；Ark 单查和列表只返回公开投影。
- [ ] 增加失败 fixture：上游 failed 与 HTTP 4xx/5xx 只退款一次；不支持角色、数量或私网 URL 在本地 400 且上游 POST 计数为 0。
- [ ] 将 Z5API 加入 billing matrix：按请求、按时长、上游 seconds 结算、失败退款、超大 seconds 饱和；使用现有测试数据库和 `require/assert`，不直接写 `OtherRatios`。
- [ ] 增加 route contract：导入 capability 的图片/视频/音频上限和分辨率限制在路由选择阶段生效，未知模型不被 profile 静态列表误判。
- [ ] 运行：

~~~~powershell
gofmt -w e2e/z5api_upstream_e2e_test.go relay/relay_task_billing_test.go relay/video_route_contract_test.go
go test ./e2e -run 'TestZ5API' -count=1 -v
go test ./relay -run 'TestZ5API|TestTaskBilling|TestVideoRouteContract' -count=1 -v
~~~~
- [ ] 提交：`git commit -m "test(z5api): cover Ark lifecycle and billing"`。

## Task 6: 真实验收与发布门禁

**Files:** `docs/superpowers/reports/2026-08-04-z5api-channel-acceptance.md`（真实验收后创建）

- [ ] 只从本机环境变量读取 `Z5API_BASE_URL`、`Z5API_API_KEY`，不把值写入命令历史、日志、fixture或提交。
- [ ] 通过 Ark POST 验收已由导入配置映射的模型，至少覆盖文生、首尾帧和图片/视频/音频混合中的一个；记录请求字段、四种状态、object URL、seconds、视频可读性、计费和退款。
- [ ] 真实响应不符合设计时，先更新设计/计划并重跑受影响失败测试；不在实现中静默兼容未确认字段。
- [ ] 没有凭据时运行本地验证并在报告中写明真实验收阻塞，渠道保持 disabled，不创建虚假通过报告。
- [ ] 有真实结果时创建中文验收报告并脱敏，运行：

~~~~powershell
go test ./relay/channel/task/newapivideo ./constant ./relay ./controller ./service ./e2e -count=1
go vet ./...
go build ./...
Set-Location web
bun run typecheck
bun run build
Set-Location ..
git diff --check
~~~~
- [ ] 只有所有命令退出码为 0、Ark 生命周期和计费门禁通过、真实验收阻塞项解决后才允许启用 211。

## 合并与回滚

- 共享注册由唯一维护者先合并；Z5API provider 分支只合并 Task 1、2、5 的专属文件，避免与其他渠道同时修改公共注册文件。
- 合并前运行 `go test ./relay/channel/task/newapivideo ./relay ./router -run 'TestZ5API|TestSeedanceTask' -count=1` 和前端 focused suite。
- 上游验收失败时仅禁用类型 211 或回退 provider profile/注册提交；保留设计、失败测试和阻塞报告，不删除用户已有渠道数据。

## 自检矩阵

| 设计要求 | 计划任务 |
|---|---|
| Ark 提交、单查、列表零代码兼容 | Task 1、2、5 |
| 独立 Z5API 方言 | Task 1、2 |
| 多模态角色与公网 URL | Task 1、5 |
| 动态模型来源 | Task 3、4 |
| task-only、默认禁用 | Task 3、4、6 |
| 计费、结算、退款和饱和保护 | Task 2、5 |
| 真实上游验收门禁 | Task 6 |
