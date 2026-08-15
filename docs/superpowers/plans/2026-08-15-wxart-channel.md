# WxArt Seedance 渠道接入实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不修改客户端调用方式的前提下，将 WxArt 的 `seedance2.0` 和 `seedance2.5` 接入现有 Ark `/api/v3/contents/generations/tasks/*` 视频任务链路，并保持渠道默认禁用。

**Architecture:** 复用 `relay/channel/task/newapivideo` 的异步提交、轮询、任务私有数据、Ark 转换和计费生命周期；通过独立 WxArt protocol profile 与强类型请求 DTO 表达模型白名单、字段方言、素材上限和失败响应语义。Seedance 2.5 的公共模型、路由约束和按秒能力在共享模型路由/定价层登记，价格仍由收录表模板和配置导入产生，不写入 adaptor。

**Tech Stack:** Go 1.22、Gin、GORM、`common` JSON/quota 包装、现有 Ark task relay、React 19、TypeScript、Bun、Vitest/Bun test、i18next。

---

## 工作区边界与现状

当前工作区包含用户已有的资产预览修改，必须保留且不得暂存到 WxArt 提交：

- `controller/asset_settings.go`
- `controller/asset_settings_test.go`
- `service/asset_service.go`
- `service/asset_service_test.go`
- `web/src/features/assets/index.tsx`
- `web/src/features/assets/__tests__/`
- `docs/superpowers/plans/2026-08-15-asset-image-preview.md`

本计划生成时，工作区又出现了一组未提交的 WxArt/Seedance 2.5 改动，包括 `constant/channel.go`、`relay/channel/task/newapivideo/wxart_request.go`、路由/定价/导入和前端文件。执行时将这些修改视为现有工作，不使用 `reset`、`checkout`、批量格式化或覆盖式写入；先逐项审计其是否满足本计划的测试契约，再补齐缺口。每次暂存必须按本计划任务的文件集合执行，禁止使用 `git add -A`。

## 执行记录（2026-08-15）

- [x] 完成渠道常量、任务注册、默认禁用和 Ark converter 发现。
- [x] 完成 WxArt 请求方言、模型白名单、素材安全校验、首尾帧比例约束和媒体时长校验。
- [x] 完成失败响应投影、Seedance 2.5 公共模型/能力/路由合同、导入映射和前端七语种配置。
- [x] 通过 WxArt/newapivideo、relay、service、controller、modelrouting、seedancepricing 相关 Go 测试。
- [x] 通过模板/导入/渠道配置前端定向测试、`bun run typecheck` 和 `bun run build`。
- [x] 完成本轮 Go 聚焦复核、WxArt 前端定向复核、`bun run i18n:sync` 和 `git diff --check`。
- [ ] 真实 WxArt Canary、账务对账和提交合并仍待 API Key 与工作区并发修改收敛后执行。

## 文件职责地图

- 渠道编号与默认地址：`constant/channel.go`、`constant/channel_test.go`。
- WxArt 协议 profile、请求方言和验证：`relay/channel/task/newapivideo/profile.go`、`native.go`、`adaptor.go`、新建 `wxart_request.go` 及对应测试。
- WxArt 失败投影与 Ark 任务响应：`relay/channel/task/newapivideo/response.go`、新建/扩展 response 测试。
- 任务注册与 Ark 生命周期：`relay/relay_adaptor.go`、`relay/relay_task.go`、`relay/seedance_task.go`、`service/seedance_task_response.go` 及现有任务测试。
- Seedance 2.5 公共模型与能力：`pkg/modelrouting/types.go`、`public.go`、`validate.go`、`pkg/seedancepricing/profile.go`、Doubao 共享 Seedance 处理文件及测试。
- WxArt 路由合同：`relay/video_route_contract.go` 与 `relay/video_route_contract_test.go`。
- 渠道默认禁用和通用渠道测试排除：`controller/channel.go`、`controller/channel-test.go`、`controller/channel_test_internal_test.go`。
- 服务端导入：`service/config_import_stage.go`、`service/config_import_publish.go` 及 `service/config_import_stage_test.go`。
- 模板生成：`web/scripts/channel-model-template/conversion-rules.json`、`build.ts` 及 `__tests__/build.test.ts`。
- 前端渠道配置：`web/src/channel-config-converter/document.ts`、其测试、`web/src/features/channels/constants.ts`、`lib/channel-form.ts`、`lib/channel-type-config.ts`、`lib/channel-utils.ts`、相关渠道测试。
- 前端文案：`web/src/i18n/static-keys.ts` 和七种 locale；locale 文件只能通过 `web/scripts/add-missing-keys.mjs` 写入并随后执行 `bun run i18n:sync`。
- 验收记录：新建 `docs/superpowers/reports/2026-08-15-wxart-channel-acceptance.md`，记录命令、结果和真实 Key 缺失造成的限制。

## Task 1: 固定渠道常量、任务注册和默认禁用行为

**Files:**

- Modify: `constant/channel.go` (`ChannelTypeWxArt`、`ChannelTypeDummy`、`ChannelBaseURLs`、`ChannelTypeNames`)
- Test: `constant/channel_test.go`
- Modify: `relay/relay_adaptor.go` (`GetTaskAdaptor`)
- Modify: `service/seedance_task_response.go` (`IsSeedanceTaskPlatform`、`SeedanceTaskPlatformValues`)
- Modify: `relay/relay_task.go`、`relay/seedance_task.go`（Ark converter/platform 分支）
- Modify: `controller/channel.go`、`controller/channel-test.go`、`controller/channel_test_internal_test.go`
- Test: `relay/relay_task_seedance_test.go`、`controller/channel_test_internal_test.go`

- [x] **Step 1: Write the failing registration tests.** Assert `ChannelTypeWxArt == 215`, `ChannelTypeDummy == 216`, default URL `https://api.wxart.space`, display name `WxArt`, and `common.ChannelType2APIType(215)` is not a chat API. Add tests that `GetTaskAdaptor("215")` returns an `ArkVideoTaskConverter`, `IsSeedanceTaskPlatform("215")` is true, and `supportsGenericChannelTest(215)` is false.

```go
func TestWxArtChannelConstants(t *testing.T) {
    require.Equal(t, 215, constant.ChannelTypeWxArt)
    require.Equal(t, 216, constant.ChannelTypeDummy)
    require.Equal(t, "https://api.wxart.space", constant.ChannelBaseURLs[constant.ChannelTypeWxArt])
    require.Equal(t, "WxArt", constant.GetChannelTypeName(constant.ChannelTypeWxArt))
    _, mapped := common.ChannelType2APIType(constant.ChannelTypeWxArt)
    require.False(t, mapped)
}
```

- [x] **Step 2: Run the focused tests and verify they fail on the clean WxArt baseline.**

Run: `go test ./constant ./relay ./controller -run 'TestWxArt|Test.*TaskAdaptor|TestAddChannelDisables' -count=1`

Expected: FAIL because the type, registration, or default-disabled list is not fully present. If an existing uncommitted WxArt change makes a test pass, retain it and identify the remaining failing assertion rather than rewriting it.

- [x] **Step 3: Implement the minimal registrations.** Add the constant at 215 and move the count sentinel to 216; register the base URL/name; return `NewWxArtTaskAdaptor()` from `GetTaskAdaptor`; add platform values and Ark converter branches; add WxArt to the pre-acceptance disabled and generic-test-unsupported sets. Do not add it to `streamSupportedChannels`, model-fetchable chat channels, or delete support.

- [x] **Step 4: Run the focused tests again.**

Run: `go test ./constant ./relay ./controller -run 'TestWxArt|Test.*TaskAdaptor|TestAddChannelDisables' -count=1`

Expected: PASS, with no change to unrelated asset tests.

- [ ] **Step 5: Commit only Task 1 files.**（本工作区包含并发改动，暂不提交）

```bash
git add constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/relay_task.go relay/seedance_task.go relay/relay_task_seedance_test.go service/seedance_task_response.go controller/channel.go controller/channel-test.go controller/channel_test_internal_test.go
git commit -m "feat: register WxArt Seedance task channel"
```

## Task 2: Build the WxArt protocol profile and request dialect

**Files:**

- Modify: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/channel/task/newapivideo/native.go`、`adaptor.go`
- Create: `relay/channel/task/newapivideo/wxart_request.go`
- Create: `relay/channel/task/newapivideo/wxart_request_test.go`
- Test: `relay/channel/task/newapivideo/adaptor_test.go`、`native_test.go`

- [x] **Step 1: Write failing provider contract tests.** Cover profile values (`/v1/videos`, `/v1/videos/{task_id}`, JSON, Bearer auth inherited from the shared adaptor, `defaultDurationSeconds == 4`, task-only model list), model normalization accepting only the two imported WxArt IDs/aliases, and rejecting every other model. Cover exact JSON for text-only, first/last frame, reference image/video/audio, and absent optional fields.

```go
func TestBuildWxArtRequestOmitsAbsentOptions(t *testing.T) {
    body, err := buildWxArtRequest(arkRequest{
        Model: "seedance2.0",
        Content: []arkContent{{Type: "text", Text: "a red kite"}},
    }, "seedance2.0")
    require.NoError(t, err)
    assert.JSONEq(t, `{"model":"seedance2.0","prompt":"a red kite"}`, string(body))
}
```

Use pointer fields for optional `ratio`, `duration`, and `resolution`; use `omitempty` on optional URL/slice fields. Verify an explicit `duration: 0` is rejected by validation rather than silently omitted, while an absent duration is omitted and later billed as 4 seconds. Verify `ratio: "Auto"` is sent as the provider's canonical `Auto` value and `4K` is normalized to `4k`.

- [x] **Step 2: Run only the new provider tests.**

Run: `go test ./relay/channel/task/newapivideo -run 'TestWxArt' -count=1`

Expected: FAIL on missing profile/dialect/DTO behavior or an incorrect JSON field name.

- [x] **Step 3: Implement the profile and DTO.** Add `videoRequestDialectWxArt`, `ChannelNameWxArt`, `wxartProtocolProfile`, `NewWxArtTaskAdaptor`, and a typed `wxArtRequest`. Map Ark fields as follows: `first_frame -> first_image`, `last_frame -> last_image`, `reference_image -> referenceImages`, `reference_video -> referenceVideos`, and `reference_audio -> referenceAudios`. Use `common.Marshal`; do not import `encoding/json` for runtime encoding.

- [x] **Step 4: Implement strict request validation.** Require exactly one non-empty text item; accept public HTTP(S) media only through the existing URL/SSRF validation; treat unroled images as references; require `last_frame` to have `first_frame`; reject frame/reference mixing; reject unsupported `watermark`, `generate_audio`, `seed`, `callback_url`, `draft`, `tools`, and non-default `service_tier` with `InvalidParameter.*`. Enforce model-specific duration, resolution, ratio, and media count limits: 2.0 is 4-15 seconds, 480p/720p/1080p/4k and 9/3/3/12; 2.5 is 4-30 seconds, 480p/720p and 30/10/10/50. Keep reference video/audio duration checks on the existing metadata and saturation path.

- [x] **Step 5: Run the provider tests and the shared native parser tests.**

Run: `go test ./relay/channel/task/newapivideo -run 'TestWxArt|TestParseARK|Test.*Request' -count=1`

Expected: PASS, including malformed content, private/data URL rejection, boundary counts, and explicit unsupported-field errors.

- [ ] **Step 6: Commit the protocol slice.**（本工作区包含并发改动，暂不提交）

```bash
git add relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/wxart_request.go relay/channel/task/newapivideo/wxart_request_test.go relay/channel/task/newapivideo/adaptor_test.go relay/channel/task/newapivideo/native_test.go
git commit -m "feat: add WxArt Seedance request dialect"
```

## Task 3: Implement response mapping and Ark task lifecycle

**Files:**

- Modify: `relay/channel/task/newapivideo/response.go`
- Create/Modify: `relay/channel/task/newapivideo/wxart_response_test.go`
- Test: `relay/relay_task_seedance_test.go`、`service/public_task_test.go`、`controller/public_task_test.go`

- [x] **Step 1: Write failing response and lifecycle tests.** Use a mock HTTP transport to assert Bearer submission to `POST /v1/videos`, poll `GET /v1/videos/{task_id}`, and conversion through the existing Ark create/list/single-task paths. Cover `queued`, `running`/`processing`, `completed` with a result URL, `failed` where `content.video_url` is the failure reason, HTTP 400/401/403/429/5xx, and an unknown status.

The failure assertion must prove both properties: `error.message` contains the sanitized failure reason and `content.video_url` is absent. The response must not expose the upstream task ID, upstream model ID, channel ID, user ID, API key, quota, private polling data, signed query parameters, or raw provider error headers.

- [x] **Step 2: Run the failing response tests.**

Run: `go test ./relay/channel/task/newapivideo ./relay -run 'TestWxArt|Test.*PublicTask|Test.*Seedance' -count=1`

Expected: FAIL if the generic parser treats a failed `video_url` as a downloadable result, if the adaptor is not discoverable, or if the Ark conversion branch is absent.

- [x] **Step 3: Implement WxArt failure projection.** In `ParseTaskResult`, when the active profile is WxArt and the parsed status is failure, sanitize `Nested.Content.VideoURL` into the failure reason, clear `Nested.Content` and `URL`, and fall back to a generic failure message if empty. In `ConvertToArkVideoTask`, never synthesize `content.video_url` from `PrivateData.ResultURL` for a WxArt failure. Preserve normal success URL handling and the existing private-data redaction.

- [x] **Step 4: Implement/verify task registration and status mapping.** Reuse the shared task state map for `queued`, `pending`, `in_progress`, `running`, `processing`, `succeeded`, `completed`, `failed`, `error`, `canceled`, `cancelled`, and `expired`; return a clear parse error for unknown states. Preserve multi-ID collision protection and public/private task ID separation.

- [x] **Step 5: Run the Ark lifecycle tests.**

Run: `go test ./relay/channel/task/newapivideo ./relay ./service ./controller -run 'TestWxArt|Test.*PublicTask|Test.*TaskPolling|Test.*SeedanceTask' -count=1`

Expected: PASS for submit -> list -> single-task -> success/failure conversion. No DELETE test is added because WxArt does not declare an upstream delete endpoint, and the channel is not added to streaming support.

- [ ] **Step 6: Commit the lifecycle slice.**（本工作区包含并发改动，暂不提交）

```bash
git add relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/wxart_response_test.go relay/relay_task_seedance_test.go service/public_task_test.go controller/public_task_test.go
git commit -m "feat: map WxArt task results to Ark"
```

## Task 4: Add Seedance 2.5 model routing, capability contracts, and billing bounds

**Files:**

- Modify/Test: `pkg/modelrouting/types.go`、`public.go`、`validate.go`、`public_test.go`
- Modify/Test: `pkg/seedancepricing/profile.go`、`profile_test.go`
- Modify/Test: `relay/channel/task/doubao/constants.go`、`adaptor.go`、`native.go`、their tests
- Modify/Test: `relay/video_route_contract.go`、`video_route_contract_test.go`
- Modify/Test: `relay/relay_task_billing_test.go`、`relay/relay_task_seedance_test.go` or the existing billing boundary fixture

- [x] **Step 1: Write failing capability and billing tests.** Assert canonical public model `doubao-seedance-2-5-260628`, family `seedance-2.5`, public model inclusion, 480p/720p support only, 4-30 second bounds, 30/10/10/50 media limits, and rejection of 1080p, 31 seconds, or 51 total references. Add a billing test proving omitted duration pre-consumes/settles at 4 seconds and explicit durations are bounded before quota conversion.

- [x] **Step 2: Run the focused capability tests.**

Run: `go test ./pkg/modelrouting ./pkg/seedancepricing ./relay/channel/task/doubao ./relay -run 'Test.*Seedance25|Test.*WxArt|TestValidate.*VideoRoute|Test.*Billing' -count=1`

Expected: FAIL before the canonical model/family and profile-aware limits are registered.

- [x] **Step 3: Implement shared model and capability support.** Add `Seedance25` to canonical model routing and public model validation; add `Family25` to pricing capability resolution; update shared Seedance native validation and billing family checks for 2.5. Do not loosen 2.0/other-provider limits. Keep the request duration as a bounded integer and use the existing `relaycommon.MaxTaskDurationSeconds`/centralized quota helpers.

- [x] **Step 4: Implement WxArt route contracts.** Validate upstream model aliases against the two WxArt models, enforce model-specific output resolutions, 4-15/4-30 duration bounds, and reference limit/total constraints. Ensure route targets cannot advertise unsupported 1080p for 2.5 or unsupported models. Preserve route validation for all existing channels.

- [x] **Step 5: Trace the full billing chain.** Confirm request validation -> `EstimateBilling`/`OtherRatios` -> pre-consume -> settlement/refund. Any new quota calculation must use `common.QuotaFromFloatChecked`, `common.QuotaRoundChecked`, or `common.QuotaFromDecimalChecked`, attach a clamp to `relayInfo.QuotaClamp`, and call `attachQuotaSaturation` before writing consume/task logs. Add a regression assertion that oversized inputs fail pre-consume instead of wrapping into a negative quota.

- [x] **Step 6: Run capability and billing tests again.**

Run: `go test ./pkg/modelrouting ./pkg/seedancepricing ./relay/channel/task/doubao ./relay -run 'Test.*Seedance25|Test.*WxArt|TestValidate.*VideoRoute|Test.*Billing' -count=1`

Expected: PASS, with no negative charge, no out-of-range duration reaching settlement, and unchanged existing Seedance tests.

- [ ] **Step 7: Commit the shared capability slice.**（本工作区包含并发改动，暂不提交）

```bash
git add pkg/modelrouting pkg/seedancepricing relay/channel/task/doubao relay/video_route_contract.go relay/video_route_contract_test.go relay/relay_task_billing_test.go relay/relay_task_seedance_test.go
git commit -m "feat: add Seedance 2.5 capability contracts"
```

## Task 5: Wire model template generation and server-side config import

**Files:**

- Modify: `web/scripts/channel-model-template/conversion-rules.json`、`build.ts`
- Test: `web/scripts/channel-model-template/__tests__/build.test.ts`、`rules.test.ts`
- Modify: `web/src/channel-config-converter/document.ts`
- Test: `web/src/channel-config-converter/__tests__/v1.test.ts`、`scope.test.ts`
- Modify/Test: `service/config_import_stage.go`、`service/config_import_publish.go`、`service/config_import_stage_test.go`

- [x] **Step 1: Write failing import/template tests.** Use a fixture for business channel `17`, name `wxart`, base URL `https://api.wxart.space`, and exactly `seedance2.0`/`seedance2.5`. Assert template mapping `17 -> CH-WXART`, canonical model mapping for 2.5, six pricing/capability rows, and document/import mapping `CH-WXART -> 215`. Assert no unapproved model such as Veo, Omni, MiniMax, or image models is generated.

- [x] **Step 2: Run the focused frontend/server import tests.**

Run: `cd web; bun test --parallel=1 scripts/channel-model-template src/channel-config-converter`

Run: `go test ./service -run 'Test.*ConfigImport|Test.*WxArt' -count=1`

Expected: FAIL on missing channel code/type mapping or missing Seedance 2.5 canonicalization.

- [x] **Step 3: Implement template and import mappings.** Add only `"17": "CH-WXART"`; map imported WxArt model rows to client-visible Seedance 2.5 without hardcoding price values in Go. Add the server-side task-channel allowlist and runtime canonical model mapping. Keep generated WxArt channels disabled/draft until key binding, price review, and acceptance.

- [x] **Step 4: Run the import tests again.**

Run: `cd web; bun test --parallel=1 scripts/channel-model-template src/channel-config-converter`

Run: `go test ./service -run 'Test.*ConfigImport|Test.*WxArt' -count=1`

Expected: PASS with `17 -> CH-WXART -> 215`, no unrelated channel mappings changed, and all generated records in the expected draft/disabled state.

- [ ] **Step 5: Commit the import slice.**（本工作区包含并发改动，暂不提交）

```bash
git add web/scripts/channel-model-template/conversion-rules.json web/scripts/channel-model-template/build.ts web/scripts/channel-model-template/__tests__ web/src/channel-config-converter/document.ts web/src/channel-config-converter/__tests__ service/config_import_stage.go service/config_import_publish.go service/config_import_stage_test.go
git commit -m "feat: import WxArt channel model rules"
```

## Task 6: Add management UI configuration and seven-locale translations

**Files:**

- Modify/Test: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-form.ts`、`channel-type-config.ts`、`channel-utils.ts`
- Test: `web/tests/channel-type-config.test.ts`、`web/src/features/channels/lib/__tests__/new-api-channel.test.ts`
- Modify: `web/src/i18n/static-keys.ts`
- Write through script only: `web/scripts/add-missing-keys.mjs`, then `web/src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json`

- [x] **Step 1: Write failing UI configuration tests.** Assert type 215 is displayed as WxArt, appears after the reserved YSR types, is task-only, is excluded from generic channel testing and model fetching, uses NewAPI icon, defaults to `https://api.wxart.space`, offers only `seedance2.0` and `seedance2.5`, and starts disabled for pre-acceptance channels.

- [x] **Step 2: Run the focused frontend tests before implementation.**

Run: `cd web; bun test --parallel=1 tests/channel-type-config.test.ts src/features/channels/lib/__tests__/new-api-channel.test.ts`

Expected: FAIL for missing type 215 configuration or missing warning/label key.

- [x] **Step 3: Implement the non-locale UI configuration.** Add type 215 to display order, task-only/unsupported sets, key prompt, warning key, default URL, supported model list, and NewAPI icon. Ensure no user-visible string is rendered directly without `t(...)`/the existing static-key mechanism.

- [x] **Step 4: Add translations via the mandated workflow.** First run `cd web; bun run i18n:sync` and inspect `_reports/_sync-report.json`. Confirm every new key is sourced from a `t(...)` key or `en.json`. Populate `newKeys` in `web/scripts/add-missing-keys.mjs` for `en`, `zh`, `zh-TW`, `fr`, `ja`, `ru`, and `vi`; run `node scripts/add-missing-keys.mjs`, then `bun run i18n:sync`. Do not edit any locale JSON directly. Keep brand/model names and URLs unchanged; translate the task-only warning and key prompt naturally and compactly.

- [x] **Step 5: Verify i18n and UI tests.**

Run: `cd web; node scripts/find-missing-keys.mjs`

Expected: `All t() keys found in en.json!`.

Run: `cd web; bun test --parallel=1 tests/channel-type-config.test.ts src/features/channels/lib/__tests__/new-api-channel.test.ts`

Expected: PASS for all seven locales and type 215 behavior.

- [x] **Step 6: Remove only the temporary translation helper after successful sync.** Preserve any pre-existing `web/scripts/add-missing-keys.mjs` if it belongs to the overlapping user change; otherwise remove the newly created helper as required by the i18n skill. Do not remove unrelated scripts.

- [ ] **Step 7: Commit the management/UI slice.**（本工作区包含并发改动，暂不提交）

```bash
git add web/src/features/channels/constants.ts web/src/features/channels/lib/channel-form.ts web/src/features/channels/lib/channel-type-config.ts web/src/features/channels/lib/channel-utils.ts web/tests/channel-type-config.test.ts web/src/features/channels/lib/__tests__/new-api-channel.test.ts web/src/i18n/static-keys.ts web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ja.json web/src/i18n/locales/ru.json web/src/i18n/locales/vi.json
git commit -m "feat: configure WxArt in channel management"
```

## Task 7: Complete Ark contract, accounting, and security regression coverage

**Files:**

- Modify/Test: `relay/channel/task/newapivideo/wxart_request_test.go`、`wxart_response_test.go`
- Modify/Test: `relay/relay_task_seedance_test.go`、`service/seedance_task_response_test.go`、`service/task_polling_test.go`
- Modify/Test: `relay/relay_task_billing_test.go`、`controller/cost_task_relay_test.go`
- Create: `docs/superpowers/reports/2026-08-15-wxart-channel-acceptance.md`

- [ ] **Step 1: Add the full deterministic material matrix.** Cover text-only, first/last frame, reference image, reference video, reference audio, mixed references, 2.0 and 2.5 boundaries, empty/duplicate frame roles, data/file/loopback/private URLs, unknown status, all declared HTTP errors, and failed `video_url` projection. Use explicit fixtures and `require`/`assert`; do not use random fuzz loops, sleeps, or log-only assertions.

- [ ] **Step 2: Add the Ark lifecycle accounting fixture.** Submit through `/api/v3/contents/generations/tasks`, list it, fetch it by public task ID, poll from queued to success/failure, and assert public responses contain only Ark fields. Verify 4-second default pre-consume, successful settlement, failure refund, timeout behavior, duplicate terminal updates, and quota saturation audit metadata. Use the existing test database/cache initialization and no real provider credentials.

- [x] **Step 3: Run the focused regression suites.**

Run: `go test ./relay/channel/task/newapivideo -run 'TestWxArt' -count=1`

Run: `go test ./relay ./service ./controller -run 'Test.*WxArt|Test.*Seedance|Test.*TaskPolling|Test.*Cost|Test.*PublicTask' -count=1`

Expected: PASS without private IDs/keys in response bodies and without negative or duplicate charges.

- [x] **Step 4: Record acceptance limits.** In `docs/superpowers/reports/2026-08-15-wxart-channel-acceptance.md`, record the exact mock/contract commands and results, the six model/capability rows imported from the sheet, and explicitly state that no real WxArt Canary was run because no API Key was supplied. Mark the channel as disabled and list the exact evidence required before enabling it.

- [ ] **Step 5: Commit the regression/report slice.**

```bash
git add relay/channel/task/newapivideo/wxart_request_test.go relay/channel/task/newapivideo/wxart_response_test.go relay/relay_task_seedance_test.go service/seedance_task_response_test.go service/task_polling_test.go relay/relay_task_billing_test.go controller/cost_task_relay_test.go docs/superpowers/reports/2026-08-15-wxart-channel-acceptance.md
git commit -m "test: verify WxArt Ark lifecycle and billing"
```

## Task 8: Full verification and handoff

- [x] **Step 1: Run the backend provider and integration suites.**

Run: `go test ./relay/channel/... ./relay/... ./router/... ./service/... ./controller/... -count=1`

Expected: PASS. If an existing unrelated test fails, record the first failing package, command, and whether it is caused by the current asset changes; do not modify unrelated code to hide it.

- [x] **Step 2: Run frontend focused tests, typecheck, lint, and build.**

Run: `cd web; bun test --parallel=1 scripts/channel-model-template src/channel-config-converter tests/channel-type-config.test.ts src/features/channels/lib/__tests__`

Run: `cd web; bun run typecheck`

Run: `cd web; bun run lint`

Run: `cd web; bun run build`

Expected: all commands exit 0. Any TypeScript/lint error in touched files must be fixed before completion.

- [x] **Step 3: Check formatting, secrets, and scope.**

Run: `git diff --check`

Run: `git status --short`

Inspect staged paths to ensure no API key, signed URL, unrelated asset change, generated build artifact, or direct locale edit is included. Confirm `streamSupportedChannels` does not contain 215, `MODEL_FETCHABLE_TYPES` does not contain 215, and the default status remains disabled.

- [ ] **Step 4: If a real Key is later supplied, run the live canary separately.** Exercise text, first/last frame, reference image/video/audio, queue polling, success URL, failure reason, and billing reconciliation against `https://api.wxart.space`; never place the key in source, logs, tests, or this report. Until then, the report must say real upstream acceptance is not executed.

- [x] **Step 5: Update this plan checkboxes and hand off.** Summarize changed files, focused/full test evidence, known residual risks, default-disabled status, and the commit sequence. Do not claim real upstream success without the canary evidence.

## Completion Criteria

- `ChannelTypeWxArt == 215`, `ChannelTypeDummy == 216`, base URL and name are registered.
- Only `seedance2.0` and `seedance2.5` are accepted; no unrelated WxArt models are generated or routable.
- Ark submit/list/single-task lifecycle works through the existing endpoints; WxArt implements `ArkVideoTaskConverter`.
- Success returns a result URL; failed `video_url` becomes a sanitized error message and never a public result URL.
- 2.0/2.5 duration, resolution, media count, URL safety, unsupported fields, and default 4-second billing bounds are enforced before upstream calls and quota conversion.
- Model routing, pricing capability, route contract, template generation, server import, management UI, and all seven locale keys agree on the same two-model scope.
- Focused and full verification commands have current passing evidence; real upstream acceptance is explicitly marked unavailable without a key.
- WxArt remains disabled until key binding, price review, and real contract/账务验收 are complete.
