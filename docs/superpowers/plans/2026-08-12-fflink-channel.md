
# FYLink Seedance 渠道接入实施计划

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 新增 FYLink（代码常量名 ChannelTypeFFLink，表格渠道名 fflink）task-only Seedance 视频渠道，使下游继续使用 Ark SDK 的 /api/v3/contents/generations/tasks/* 完成提交、列表、单查、取消和视频结果下载；参考素材只接受公网 HTTP(S) URL，new-api 不调用 FYLink 上传接口。

**Architecture:** 在 relay/channel/task/newapivideo 增加 FYLink 专属 profile、请求 DTO、响应投影和可选取消 adaptor，复用现有 Ark 任务 ID、轮询、计费、结算、退款和隐私投影。共享任务取消、视频代理 Range 支持、路由合同、配置导入和管理端注册分别落在现有边界内；渠道默认保持手动禁用，只有本地契约测试与真实 FYLink 凭据验收均通过后才允许启用。

**Tech Stack:** Go 1.22、Gin、GORM、common.Marshal/Unmarshal、httptest、testify、React 19、TypeScript、Bun、i18next、Ark 视频任务协议。

---

## 文件边界

FYLink 协议专属文件：

- 创建：relay/channel/task/newapivideo/fflink_request.go
- 创建：relay/channel/task/newapivideo/fflink_request_test.go
- 创建：relay/channel/task/newapivideo/fflink_response_test.go
- 创建：relay/channel/task/newapivideo/fflink_cancel.go
- 创建：relay/channel/task/newapivideo/fflink_cancel_test.go
- 修改：relay/channel/task/newapivideo/profile.go
- 修改：relay/channel/task/newapivideo/adaptor.go
- 修改：relay/channel/task/newapivideo/response.go

共享后端文件：

- 修改：constant/channel.go、constant/channel_test.go
- 修改：relay/channel/adapter.go
- 修改：relay/relay_adaptor.go、relay/relay_task.go、relay/seedance_task.go、relay/video_route_contract.go
- 修改：model/task.go、model/task_cas_test.go
- 修改：controller/relay.go、controller/video_proxy.go
- 创建：relay/seedance_task_cancel.go、relay/seedance_task_cancel_test.go
- 创建：controller/seedance_cancel.go、controller/seedance_cancel_test.go
- 修改：router/video-router.go、router/video_router_test.go
- 修改：service/seedance_task_response.go、service/config_import_stage.go、service/config_import_stage_test.go
- 修改：controller/channel.go、controller/channel-test.go、controller/channel_test_internal_test.go
- 创建：controller/video_proxy_fflink_test.go
- 修改：relay/video_route_contract_test.go、relay/cost_accounting_adaptor_test.go、relay/relay_task_billing_test.go

配置生成与前端文件：

- 修改：web/scripts/channel-model-template/types.ts
- 修改：web/scripts/channel-model-template/build.ts
- 修改：web/scripts/channel-model-template/conversion-rules.json
- 修改：web/scripts/channel-model-template/__tests__/build.test.ts
- 修改：web/src/channel-config-converter/document.ts、web/src/channel-config-converter/__tests__/v1.test.ts
- 修改：web/src/features/channels/constants.ts、web/src/features/channels/lib/channel-type-config.ts、web/src/features/channels/lib/channel-utils.ts、web/src/features/channels/lib/channel-form.ts
- 修改：web/tests/channel-type-config.test.ts、web/src/features/channels/lib/__tests__/new-api-channel.test.ts、web/src/i18n/static-keys.ts
- 通过脚本写入：web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json

不应修改：

- docs/new-channels/cn-fflink.html 只作为协议事实来源，不把示例模型复制到静态 modelList。
- 不新增 FYLink 上传调用，不接收本地文件、Base64、data: 或 file: 素材。
- 不把表格业务渠道代码 15 当成 ChannelTypeFFLink，不把 FYLink 加入 streamSupportedChannels、聊天模型拉取或普通 OpenAI 路由。

---

## Task 1：注册 FYLink 类型、profile 与默认禁用门禁

目标：先让类型、profile、task adaptor、Seedance 注册和管理端禁用门禁有可观察契约；实现仍不发送上游请求。

文件：constant/channel.go、constant/channel_test.go、relay/channel/task/newapivideo/profile.go、relay/relay_adaptor.go、relay/seedance_task.go、service/seedance_task_response.go、relay/relay_task.go、relay/video_route_contract.go、controller/channel.go、controller/channel-test.go、controller/channel_test_internal_test.go。

- [ ] 步骤 1：写失败测试。断言 ChannelTypeFFLink=214、ChannelTypeDummy=215、默认 URL 为 https://api.fflink.top、名称为 FYLink、common.ChannelType2APIType 不映射；GetTaskAdaptor("214") 非空且实现 ArkVideoTaskConverter；IsSeedanceTaskPlatform("214") 为 true；supportsGenericChannelTest(214) 为 false；新增或切换 FYLink 渠道时状态强制为 ChannelStatusManuallyDisabled。
- [ ] 步骤 2：运行失败测试。

~~~powershell
go test ./constant ./relay ./service ./controller -run 'FFLink|ChannelTypeDummy|SeedanceTaskPlatform|PreAcceptance|GenericChannelTest' -count=1
~~~

预期：失败于 ChannelTypeFFLink、默认 URL、名称和注册分支不存在，且 ChannelTypeDummy 仍为 213。

- [ ] 步骤 3：实现最小注册。在 constant/channel.go 保持已有 Z5API=211，为已占用的 212、213 保留编号，追加 ChannelTypeFFLink=214、ChannelTypeDummy=215；在 ChannelBaseURLs 和 ChannelTypeNames 加入 FYLink；在 profile.go 增加 ChannelNameFYLink、videoRequestDialectFFLink 和 profile：

~~~go
const ChannelNameFYLink = "FYLink"
const videoRequestDialectFFLink videoRequestDialect = "fflink"

func fflinkProtocolProfile() protocolProfile {
    return protocolProfile{
        channelName: ChannelNameFYLink,
        modelList: []string{},
        submitPath: "/v1/videos/generations",
        pollPath: "/v1/videos/jobs/{task_id}",
        contentType: "application/json",
        requestDialect: videoRequestDialectFFLink,
        requirePublicHTTPMedia: true,
        preferRespondAsync: true,
    }
}
~~~

新增 FYLinkTaskAdaptor 包装类型，并让 relay/relay_adaptor.go 只为 214 返回 NewFYLinkTaskAdaptor。把 214 加入 Seedance 平台值、Ark converter 分支、路由合同分支、普通渠道测试排除集合和 pre-acceptance disabled 集合；不要加入 OpenAI API 类型映射或流式列表。

- [ ] 步骤 4：运行通过测试并提交。

~~~powershell
gofmt -w constant relay/channel/task/newapivideo/profile.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/video_route_contract.go service/seedance_task_response.go controller/channel.go controller/channel-test.go controller/channel_test_internal_test.go
go test ./constant ./relay ./service ./controller -run 'FFLink|ChannelTypeDummy|SeedanceTaskPlatform|PreAcceptance|GenericChannelTest' -count=1
git add constant relay service controller
git commit -m "feat(fflink): register FYLink task channel"
~~~

---

## Task 2：FYLink 请求 DTO、公网素材与能力校验

目标：用类型化 DTO 精确编码 FYLink /v1/videos/generations，只透传公网 URL；所有边界在请求提交前返回 Ark InvalidParameter.*。

文件：relay/channel/task/newapivideo/fflink_request.go、fflink_request_test.go、profile.go、adaptor.go、native.go。

- [ ] 步骤 1：写失败测试。断言纯文本请求输出 model、prompt、resolution、duration、aspect_ratio、audio；显式 false 不丢失、缺省字段省略；first_frame/last_frame 分别进入 start_frame_url/end_frame_url；reference_image、reference_video、reference_audio 分别进入 guidances.image_reference、guidances.video_reference_base、guidances.audio_reference，媒体对象均为 url 加 type=UPLOADED；测试 server 的 /v1/videos/uploads 计数为 0。
- [ ] 步骤 2：在同一测试文件加入 400 契约：拒绝 data/file/asset/环回/私网/非 HTTP(S) URL、错误 role、重复首尾帧、参考图和首尾帧混用、图片 5 个、视频 4 个、音频 2 个、总素材 9 个、参考视频总时长大于 15 秒、仅首尾帧加音频、duration 3/16、标准 seedance-2.0 1080p duration 13；显式 watermark、seed、callback_url、draft、非空 tools、非 default service_tier 分别返回 InvalidParameter.<field>。
- [ ] 步骤 3：运行失败测试。

~~~powershell
go test ./relay/channel/task/newapivideo -run 'TestFFLinkRequest|TestBuildFFLink|TestFFLinkPublicURL' -count=1
~~~

预期：因 buildFFLinkRequest、profile 分派和 FYLink 专属校验尚不存在而失败。

- [ ] 步骤 4：实现 DTO 与验证。DTO 字段为 Model、Prompt、Resolution、Duration、AspectRatio、Audio、StartFrameURL、EndFrameURL、Guidances；Guidances 由 image_reference、video_reference_base、audio_reference 三个数组组成，数组元素分别包裹 image/video/audio 媒体对象。所有可选标量用指针和 omitempty，业务 JSON 只通过 common.Marshal。
- [ ] 步骤 5：实现 validateFFLinkRequest。复用 validateARKSemantics 的文本、URL SSRF 和整数安全边界，再执行参考图 4、视频 3、音频 1、总数 8、音频必须搭配 reference_image 或 reference_video、参考图不能与首尾帧共存、参考视频总时长 15 秒、标准 1080p 最大 12 秒；使用 service.ResolveReferenceVideoDurationMS，元数据不可得返回 503 reference_media_metadata_unavailable。若共享首尾帧逻辑误判首尾帧加参考视频，新增仅对 FYLink 生效的 profile 开关，再单独禁止首尾帧加参考图和仅首尾帧加音频；不放宽其他 provider。
- [ ] 步骤 6：在 adaptor.go 的 profile 分派中调用校验和构建函数，只有 ProviderValidationComplete 后才构建 body；BuildRequestHeader 为 FYLink 增加 Prefer: respond-async。运行请求测试并提交。

~~~powershell
gofmt -w relay/channel/task/newapivideo/fflink_request.go relay/channel/task/newapivideo/fflink_request_test.go relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/native.go
go test ./relay/channel/task/newapivideo -run 'TestFFLinkRequest|TestBuildFFLink|TestFFLinkPublicURL' -count=1
git add relay/channel/task/newapivideo
git commit -m "feat(fflink): encode public media task requests"
~~~

---

## Task 3：job_id 提交、轮询状态与 Ark 公开投影

目标：把 FYLink 的 job_id 和 pending/running/settling/completed/failed/canceled 生命周期投影到现有 Ark 任务模型；completed 没有结果 URL 时仍成功，并使用公开任务代理地址。

文件：relay/channel/task/newapivideo/adaptor.go、response.go、fflink_response_test.go、relay/relay_task.go、relay/seedance_task.go。

- [ ] 步骤 1：写失败响应测试。创建响应提取 job_id；pending 映射 QUEUED，running/settling 映射 IN_PROGRESS，completed 映射 SUCCESS，failed/canceled 映射 FAILURE；未知状态返回 invalid_response；completed 没有 URL 不报错；失败消息清理 Key、job ID 和渠道字段；ConvertToArkVideoTask 只返回公开 task ID、客户端模型、Ark status、content.video_url、duration、resolution、usage 和清理后的 error。
- [ ] 步骤 2：运行失败测试。

~~~powershell
go test ./relay/channel/task/newapivideo -run 'TestFFLinkResponse|TestFFLinkStatus|TestConvertToArkVideoTask' -count=1
~~~

预期：因 job_id 未加入提交 DTO、settling 未加入状态映射、FYLink 成功缺 URL 未放行而失败。

- [ ] 步骤 3：在 upstreamSubmitResponse 增加 JobID 字段，并把它加入 DoResponse 的 ID 一致性检查；FYLink 提交路径为 /v1/videos/generations，轮询路径为 /v1/videos/jobs/{url.PathEscape(job_id)}。在 mapUpstreamTaskStatus 增加 SETTLING 和 CANCELED 映射；ParseTaskResult 对 FYLink 放行成功无 URL；保持 common.Unmarshal/common.Marshal 和 checked quota helper。
- [ ] 步骤 4：在 relay/relay_task.go 的完成分支保持无 URL 时写入 taskcommon.BuildProxyURL(task.TaskID)，在 seedanceTaskPayload 中加入 214。失败/取消终态清理上游任务 ID、Key 和渠道私有信息的公开路径，但保留管理员审计数据。运行并提交。

~~~powershell
gofmt -w relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/fflink_response_test.go relay/relay_task.go relay/seedance_task.go
go test ./relay/channel/task/newapivideo ./relay -run 'TestFFLinkResponse|TestFFLinkStatus|TestConvertToArkVideoTask|SeedanceTask' -count=1
git add relay/channel/task/newapivideo relay/relay_task.go relay/seedance_task.go
git commit -m "feat(fflink): normalize job lifecycle for Ark tasks"
~~~

---

## Task 4：FYLink 取消、任务 Key 持久化与多状态 CAS

目标：增加 Ark DELETE 任务接口，只让 FYLink 通过可选取消 adaptor 调用上游；取消与轮询竞争时只允许一个终态 CAS 胜者退款。

文件：relay/channel/adapter.go、relay/channel/task/newapivideo/fflink_cancel.go、fflink_cancel_test.go、model/task.go、model/task_cas_test.go、controller/relay.go、relay/seedance_task_cancel.go、relay/seedance_task_cancel_test.go、controller/seedance_cancel.go、controller/seedance_cancel_test.go、router/video-router.go、router/video_router_test.go。

- [ ] 步骤 1：写失败测试。取消 adaptor 断言 DELETE /v1/videos/jobs/job%2Fprivate、Bearer 正确、路径经过 url.PathEscape；多状态 CAS 断言只有一个调用者成功；跨用户任务 404，已完成/已失败/重复取消返回 task_not_cancellable，不支持取消的 provider 返回 405 task_cancellation_not_supported；FYLink pending 取消后为 FAILURE/100%/task canceled，退款只产生一笔；退款失败时 quota 保留；提交持久化测试断言仅 214 写入 task.PrivateData.Key=relayInfo.ApiKey，公开响应和普通日志没有 Key。
- [ ] 步骤 2：运行失败测试。

~~~powershell
go test ./relay/channel/task/newapivideo ./model ./relay ./controller ./router -run 'FFLink.*Cancel|Task.*CAS|Seedance.*Cancel|VideoRouter.*Delete|Persist.*Task.*Key' -count=1
~~~

预期：因 TaskCancellationAdaptor、UpdateWithStatuses、DELETE handler 和 PrivateData.Key 分支不存在而失败。

- [ ] 步骤 3：实现可选接口，不改写共享 TaskAdaptor：

~~~go
type TaskCancellationAdaptor interface {
    CancelTask(ctx context.Context, baseURL string, key string, taskID string, proxy string) (*http.Response, error)
}
~~~

在 model.Task 增加 UpdateWithStatuses(fromStatuses []TaskStatus)，实现必须使用 GORM Model(t).Where("status IN ?", fromStatuses).Select("*").Updates(t)，空 task、空 ID 或空来源列表返回 false,nil。FYLinkTaskAdaptor 嵌入 TaskAdaptor 并单独实现 CancelTask，使用 http.NewRequestWithContext、url.PathEscape、service.GetHttpClientWithProxy 和 Bearer header。
- [ ] 步骤 4：在 controller/relay.go 的 persistSubmittedTask 中只为 214 保存 relayInfo.ApiKey。实现 relay.SeedanceTaskCancel：按当前用户查询公开 task ID；仅接受 NOT_START/SUBMITTED/QUEUED；类型断言 TaskCancellationAdaptor；优先 task.PrivateData.Key，空值回退 channel.Key；上游 DELETE 2xx 后使用 UpdateWithStatuses([NOT_START,SUBMITTED,QUEUED,IN_PROGRESS]) 写 FAILURE、100%、task canceled、FinishTime；只有 CAS 胜者调用 service.RefundTaskQuota；CAS 失败重新读取并返回 task_state_changed。新增 controller wrapper 和 DELETE 路由。
- [ ] 步骤 5：运行并提交。

~~~powershell
gofmt -w relay/channel/adapter.go relay/channel/task/newapivideo/fflink_cancel.go relay/channel/task/newapivideo/fflink_cancel_test.go model/task.go model/task_cas_test.go controller/relay.go relay/seedance_task_cancel.go relay/seedance_task_cancel_test.go controller/seedance_cancel.go controller/seedance_cancel_test.go router/video-router.go router/video_router_test.go
go test ./relay/channel/task/newapivideo ./model ./relay ./controller ./router -run 'FFLink.*Cancel|Task.*CAS|Seedance.*Cancel|VideoRouter.*Delete|Persist.*Task.*Key' -count=1
git add relay/channel/adapter.go relay/channel/task/newapivideo model/task.go model/task_cas_test.go controller/relay.go relay/seedance_task_cancel.go relay/seedance_task_cancel_test.go controller/seedance_cancel.go controller/seedance_cancel_test.go router/video-router.go router/video_router_test.go
git commit -m "feat(fflink): add cancellable Ark task lifecycle"
~~~

---

## Task 5：FYLink 视频内容代理、Range 和重定向凭据隔离

目标：成功任务没有上游结果 URL 时，公开任务 URL 仍能读取 FYLink /content；支持 Range/206，跨域重定向不得泄漏 Bearer。

文件：controller/video_proxy.go、controller/video_proxy_fflink_test.go、controller/video_proxy_eightyes_test.go。

- [ ] 步骤 1：写失败测试。FYLink task 使用 PrivateData.UpstreamTaskID="job/private" 和 PrivateData.Key="selected-fflink-key"，公开请求带 Range: bytes=10-19；origin 断言路径 /v1/videos/jobs/job%2Fprivate/content、Bearer 和 Range，返回 206、Content-Range、Accept-Ranges；跨域 sink 收不到 Authorization 和 Referer；公开响应不含上游 job ID、Key 或供应商敏感 header。
- [ ] 步骤 2：运行失败测试。

~~~powershell
go test ./controller -run 'FFLinkVideoProxy|VideoProxy.*Range|VideoProxy.*Authorization' -count=1
~~~

预期：当前代理不转发 Range、只接受 200，FYLink 分支不存在，跨域 redirect 不会删除 Authorization。
- [ ] 步骤 3：在 FYLink 分支构造 /v1/videos/jobs/{url.PathEscape(task.GetUpstreamTaskID())}/content；Key 使用 task.PrivateData.Key 优先、channel.Key 回退；client 使用 publicMediaHTTPClient("Authorization")。已有 8yes/OpenAI Bearer 分支也使用同一敏感 header 保护。转发 Range header，状态只接受 200 或 206，继续复制 Content-Range/Accept-Ranges，过滤 Cookie、ETag、Server 和供应商 request ID。
- [ ] 步骤 4：运行并提交。

~~~powershell
gofmt -w controller/video_proxy.go controller/video_proxy_fflink_test.go controller/video_proxy_eightyes_test.go
go test ./controller -run 'FFLinkVideoProxy|VideoProxy.*Range|VideoProxy.*Authorization|EightYesVideoProxy|GeminiVideoProxy' -count=1
git add controller/video_proxy.go controller/video_proxy_fflink_test.go controller/video_proxy_eightyes_test.go
git commit -m "fix(video): proxy FYLink ranges without leaking bearer keys"
~~~

---

## Task 6：路由合同、按秒成本与 Seedance 计费生命周期

目标：FYLink 只有经导入能力合同验证的模型/分辨率/时长才能路由；计费复用现有 Seedance 预扣、结算、失败退款和 quota 饱和审计。

文件：relay/video_route_contract.go、relay/video_route_contract_test.go、relay/cost_accounting_adaptor_test.go、relay/relay_task_billing_test.go、service/seedance_task_response.go。

- [ ] 步骤 1：写失败合同和计费测试。seedance-2.0 允许 480p/720p/1080p，fast 只允许 480p/720p，mini 只允许 720p；参考图/视频/音频/总数为 4/3/1/8，参考视频总时长 15 秒，生成时长 4-15 秒；标准 1080p 最大 12 秒。未声明模型、分辨率、13 秒 1080p、超过任何素材上限均返回 route_contract_*。Cost capability 断言 validated request、upstream actual、upstream usage，不加入 stream capability。billing 测试覆盖按秒预扣、成功结算、失败/取消退款、重复轮询不重复退款、checked quota saturation。
- [ ] 步骤 2：运行失败测试。

~~~powershell
go test ./relay ./service -run 'FFLink|VideoRouteContract|TaskBilling|QuotaClamp|Seedance' -count=1
~~~

预期：FYLink 路由合同没有分支，cost/billing matrix 没有 214，1080p 13 秒未被拒绝。
- [ ] 步骤 3：实现 validateFFLinkVideoRoute，并将 214 加入 IsSeedanceTaskPlatform 和 SeedanceTaskPlatformValues。能力只读取导入后的 modelrouting.Target，不从空 modelList 推断。FYLink adaptor 不写价格；实际 duration 通过现有 CostMeter/AdjustBillingOnComplete 结算，quota 使用 checked helper，clamp 写入 relayInfo.QuotaClamp 并进入管理员审计。失败、取消和超时只由终态 CAS 胜者退款。
- [ ] 步骤 4：运行并提交。

~~~powershell
gofmt -w relay/video_route_contract.go relay/video_route_contract_test.go relay/cost_accounting_adaptor_test.go relay/relay_task_billing_test.go service/seedance_task_response.go
go test ./relay ./service -run 'FFLink|VideoRouteContract|TaskBilling|QuotaClamp|Seedance' -count=1
git add relay/video_route_contract.go relay/video_route_contract_test.go relay/cost_accounting_adaptor_test.go relay/relay_task_billing_test.go service/seedance_task_response.go
git commit -m "feat(fflink): enforce route and billing contracts"
~~~

---

## Task 7：最新 sd收录 模板、1080p 12 秒覆盖与配置导入

目标：以 2026-08-11 最新 Google 表格下载物为事实来源，生成 FYLink 六条模型/分辨率映射和按秒成本，并把 CH-FFLINK 绑定到代码类型 214；不把表格业务 ID 15 写成代码类型。

文件：web/scripts/channel-model-template/types.ts、build.ts、conversion-rules.json、__tests__/build.test.ts、web/src/channel-config-converter/document.ts、web/src/channel-config-converter/__tests__/v1.test.ts、service/config_import_stage.go、service/config_import_stage_test.go。

- [ ] 步骤 1：写失败生成器测试。fixture 包含 seedance-2.0 480p/720p/1080p（0.12/0.25/0.60 元/秒）、seedance-2.0-fast 480p/720p（0.10/0.20 元/秒）、seedance-2.0-mini 720p（0.17 元/秒），参考图/视频/音频/总数 4/3/1/8、参考视频总时长 15 秒、时长 4-15 秒。断言 CH-FFLINK 生成 6 个 mapping、每个 mapping 有 no_video/with_video 两条 per_duration cost；增加源行断言确认 seedance-2.0/1080p 是 sd!R223。覆盖前 max=15，覆盖后 max=12，报告含 ROW_DURATION_OVERRIDE WARN。
- [ ] 步骤 2：运行失败测试。

~~~powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__/build.test.ts src/channel-config-converter/__tests__/v1.test.ts
Set-Location ..
~~~

预期：RowOverride 没有时长字段、FYLink 合同没有分支、V1 映射没有 214，1080p 行仍显示 15 秒。
- [ ] 步骤 3：扩展 RowOverride，加入 minDurationSeconds/maxDurationSeconds；build.ts 使用有效时长执行合同、mapping 和成本/利润计算，缩短上限时发出 WARN。channelContractIssues 增加 FYLink 模型/分辨率、4-15 秒、1080p 12 秒和 4/3/1/8/15 合同。conversion-rules.json 保留 "15": "CH-FFLINK"，新增 "15/R223": {"maxDurationSeconds": 12}。document.ts 增加 "CH-FFLINK": 214；config import 把 214 视为 task-only，导入不自动启用渠道。
- [ ] 步骤 4：从 C:\Users\880pro\Downloads\sd收录 (11).xlsx 复制到 outputs/2026-08-12-fflink-channel/sd收录.xlsx，记录 SHA-256、修改时间和有效行；生成模板：

~~~powershell
Set-Location web
bun run channel-model-template:generate --source "..\outputs\2026-08-12-fflink-channel\sd收录.xlsx" --rules "scripts\channel-model-template\conversion-rules.json" --base "src\channel-config-converter\__fixtures__\channel-config-v1-corrected.xlsx" --output "..\outputs\2026-08-12-fflink-channel\渠道模型成本与利润模板-v1.xlsx" --allow-warnings
Set-Location ..
~~~

预期：FAIL=0；6 个 mapping、每个 mapping 两个 scenario cost；R223 报告记录 15 秒被裁为 12 秒；绑定仍 disabled 或待审阅。
- [ ] 步骤 5：运行通过测试并提交。

~~~powershell
Set-Location web
bun test --parallel=1 scripts/channel-model-template/__tests__ src/channel-config-converter
bun run typecheck
Set-Location ..
go test ./service -run 'ConfigImport|FFLink' -count=1
git add web/scripts/channel-model-template web/src/channel-config-converter service/config_import_stage.go service/config_import_stage_test.go
git commit -m "feat(config): import FYLink model and cost contracts"
~~~

---

## Task 8：管理端 FYLink 类型、task-only 表单和七语言 i18n

目标：管理端能选择 FYLink、默认填充 https://api.fflink.top、不显示模型拉取、自动保持 disabled，所有新增用户文案有七种翻译。

文件：web/src/features/channels/constants.ts、channel-type-config.ts、channel-utils.ts、channel-form.ts、web/tests/channel-type-config.test.ts、web/src/features/channels/lib/__tests__/new-api-channel.test.ts、web/src/i18n/static-keys.ts、web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json。

- [ ] 步骤 1：写失败前端测试。断言 CHANNEL_TYPES[214] 为 FYLink、显示顺序在 211 后、icon 为 NewAPI；defaultBaseUrl 为 https://api.fflink.top、supportedModels 为空；214 不在 MODEL_FETCHABLE_TYPES、在 TASK_ONLY_CHANNEL_TYPES 和 GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES；key prompt/model hint 为 FYLink；切换到 214 返回 MANUAL_DISABLED。
- [ ] 步骤 2：运行失败测试。

~~~powershell
Set-Location web
bun test --parallel=1 tests/channel-type-config.test.ts src/features/channels/lib/__tests__/new-api-channel.test.ts
Set-Location ..
~~~

预期：类型 214、默认 URL、task-only 集合和强制禁用逻辑尚不存在。
- [ ] 步骤 3：按 8yes/Z5API 模式加入 214，保持空静态模型和不可拉取；通过 i18n 脚本写入七种 locale，禁止直接编辑 JSON。新增 source keys 为 FYLink、Default: https://api.fflink.top、Enter the raw API key issued by FYLink、Map client-visible Ark model names to verified FYLink upstream models、FYLink is task-only. Enable it only after real upstream contract acceptance.；登记 static-keys.ts。脚本使用下列精确值填充，不覆盖已有翻译：

| key | en | zh | zh-TW | fr | ja | ru | vi |
|---|---|---|---|---|---|---|---|
| `FYLink` | `FYLink` | `FYLink` | `FYLink` | `FYLink` | `FYLink` | `FYLink` | `FYLink` |
| `Default: https://api.fflink.top` | `Default: https://api.fflink.top` | `默认值：https://api.fflink.top` | `預設值：https://api.fflink.top` | `Par défaut : https://api.fflink.top` | `既定値: https://api.fflink.top` | `По умолчанию: https://api.fflink.top` | `Mặc định: https://api.fflink.top` |
| `Enter the raw API key issued by FYLink` | `Enter the raw API key issued by FYLink` | `输入 FYLink 签发的原始 API 密钥` | `輸入 FYLink 核發的原始 API 金鑰` | `Saisissez la clé API brute émise par FYLink` | `FYLink が発行した元の API キーを入力してください` | `Введите исходный ключ API, выданный FYLink` | `Nhập khóa API gốc do FYLink cấp` |
| `Map client-visible Ark model names to verified FYLink upstream models` | `Map client-visible Ark model names to verified FYLink upstream models` | `将客户端可见的 Ark 模型名称映射到已验证的 FYLink 上游模型` | `將用戶端可見的 Ark 模型名稱對應至已驗證的 FYLink 上游模型` | `Associez les noms de modèles Ark visibles par le client aux modèles amont FYLink vérifiés` | `クライアントに表示する Ark モデル名を検証済みの FYLink 上流モデルにマッピングします` | `Сопоставьте видимые клиенту имена моделей Ark с проверенными вышестоящими моделями FYLink` | `Ánh xạ tên mô hình Ark hiển thị cho khách hàng tới các mô hình thượng nguồn FYLink đã xác minh` |
| `FYLink is task-only. Enable it only after real upstream contract acceptance.` | `FYLink is task-only. Enable it only after real upstream contract acceptance.` | `FYLink 仅支持任务模式。仅在真实上游契约验收通过后启用。` | `FYLink 僅支援任務模式。僅在真實上游契約驗收通過後啟用。` | `FYLink est limité aux tâches. Activez-le uniquement après validation du contrat amont réel.` | `FYLink はタスク専用です。実際の上流契約の受け入れ後にのみ有効化してください。` | `FYLink поддерживает только задачи. Включайте его только после приемки реального вышестоящего контракта.` | `FYLink chỉ hỗ trợ tác vụ. Chỉ bật sau khi nghiệm thu hợp đồng thượng nguồn thực tế.` |
- [ ] 步骤 4：同步与验证。创建临时 web/scripts/add-missing-keys.mjs 填充七种翻译，运行 node scripts/add-missing-keys.mjs、bun run i18n:sync、focused Bun tests、bun run typecheck、bun run lint，随后删除临时脚本；不能手改 locale JSON。
- [ ] 步骤 5：提交。

~~~powershell
git add web/src/features/channels web/tests/channel-type-config.test.ts web/src/i18n/static-keys.ts web/src/i18n/locales
git commit -m "feat(web): add FYLink task channel configuration"
~~~

---

## Task 9：Mock Ark E2E、真实 FYLink 验收和发布门禁

目标：在没有供应商 Key 时完成可审计的 mock/contract 验收；有真实 Key 时再执行供应商 canary。无真实验收通过前，214 永远保持 disabled。

文件：e2e/fflink_upstream_e2e_test.go、e2e/seedance_material_matrix_test.go、docs/superpowers/reports/2026-08-12-fflink-channel-acceptance.md（真实验收后创建）。

- [ ] 步骤 1：写 mock E2E 失败测试。创建 `e2e/fflink_upstream_e2e_test.go`，用 httptest.Server 实现 POST /v1/videos/generations（校验 Authorization、Prefer、请求 JSON并返回 job_id）、GET /v1/videos/jobs/job_1（running、settling、completed且无 URL）、GET /v1/videos/jobs/job_1/content（Range/206）、DELETE /v1/videos/jobs/job_2（204）。通过真实 Ark POST、GET 列表、GET 单任务、DELETE 单任务和视频 content proxy，覆盖文本、首尾帧、参考图、参考视频、参考音频中的完整混合输入；断言公开响应不含 job ID、Key 或原始上游 body。仅在现有 `e2e/seedance_material_matrix_test.go` 已有白名单阻止 FYLink E2E 时修改该白名单；否则不修改该文件。
- [ ] 步骤 2：运行失败测试。

~~~powershell
go test ./e2e -run 'TestFFLink|TestSeedanceImportedMaterialMatrixFullFlowE2E' -count=1 -p=1
~~~

预期：FYLink 不在 mock 平台、导入 fixture、Ark 生命周期和 content proxy 白名单中。
- [ ] 步骤 3：实现 mock E2E 和账务断言。测试数据库显式初始化用户、token、channel quota、billing config 和导入 batch；任务实例使用 214 但状态保持 MANUAL_DISABLED，测试中显式构造路由目标。断言预扣、success settle、failed/canceled refund、重复轮询/重复 DELETE 不产生第二笔退款，quota_saturation 只出现在管理员 other.admin_info。无 Key 时只能报告 mock 协议通过，不能报告真实供应商通过。
- [ ] 步骤 4：运行完整本地验证。

~~~powershell
go test ./relay/channel/task/newapivideo -run 'FFLink|Submit|TaskResult' -count=1
go test ./model -run 'Task.*Status|Task.*CAS' -count=1
go test ./relay ./controller ./router -run 'FFLink|Seedance.*Cancel|VideoProxy' -count=1
go test ./service ./relay -run 'FFLink|Seedance|TaskBilling|ConfigImport' -count=1
go test ./e2e -run '^TestSeedanceImportedMaterialMatrixFullFlowE2E$' -count=1 -p=1
go test ./cmd/ark-video-material-seed ./e2e -count=1 -p=1
Set-Location web
bun test --parallel=1 tests/channel-type-config.test.ts src/features/channels/lib/__tests__/new-api-channel.test.ts
bun test --parallel=1 scripts/channel-model-template/__tests__ src/channel-config-converter
bun run typecheck
bun run lint
bun run build
Set-Location ..
go test ./...
git diff --check
~~~

预期：所有命令退出码为 0；没有真实 Key 时，只能在报告中标注真实 FYLink canary 未运行，不能启用渠道。
- [ ] 步骤 5：有凭据时执行受控真实验收。只从当前进程环境读取 FFLINK_BASE_URL、FFLINK_API_KEY，不把值写入 shell 历史、日志、fixture 或报告。至少执行一次文生、一次带公网参考素材、完整 pending -> running -> settling -> completed 轮询、视频 Range 下载、一次 pending 取消和账务核对。若供应商拒绝公网 URL、状态或 content 行为，先更新设计/失败契约测试，不启用 214。报告记录协议版本、Base URL（不含 Key）、场景、状态时间线、代理 Range、计费/退款、失败项、源表/模板/导入 JSON SHA-256、未覆盖项和 disabled 原因。
- [ ] 步骤 6：真实验收通过后提交报告和 E2E。

~~~powershell
git add e2e docs/superpowers/reports/2026-08-12-fflink-channel-acceptance.md
git commit -m "test(fflink): verify Ark lifecycle and acceptance gates"
~~~

---

## 完成前自检

- [ ] 对照 docs/superpowers/specs/2026-08-11-fflink-channel-design.md：Ark POST/GET/DELETE、job_id、Prefer、settling、无 URL success、content proxy、Range/206、重定向脱敏、公网 URL、媒体角色、4/3/1/8/15 限制、1080p 12 秒、Key 持久化、一次性退款、模板六行、类型 214、Dummy 215、默认 disabled、七语言 i18n 均有对应任务。
- [ ] 使用约定的占位词扫描表达式审阅计划；每个步骤都包含具体文件、命令、预期结果或提交命令。
- [ ] 检查类型一致性：ChannelTypeFFLink=214、ChannelTypeDummy=215、ChannelNameFYLink、videoRequestDialectFFLink、NewFYLinkTaskAdaptor、TaskCancellationAdaptor.CancelTask(ctx, baseURL, key, taskID, proxy)、UpdateWithStatuses([]TaskStatus) 在所有任务中名称一致。
- [ ] 检查隐私、数据库和计费边界：公开响应不含上游 job ID/Key；UpdateWithStatuses 只使用 GORM status IN；duration 和素材数量有上限；quota 转换使用 checked helper；退款由 CAS 胜者单次执行，失败时保留 quota 供 reconciliation。

## 执行选择

计划完成并保存到 docs/superpowers/plans/2026-08-12-fflink-channel.md 后，实施阶段二选一：

1. Subagent-Driven（推荐）：按 Task 1-9 每个任务分派新 agent，任务间由主 agent 做 focused review 和测试门禁。
2. Inline Execution：在当前 worktree 使用 superpowers:executing-plans，按任务批次执行并在每个提交后停点复核。
