# 4stoken Seedance Channel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Add 4stoken as a task-only Seedance channel whose users keep the Ark SDK /api/v3/contents/generations/tasks/* contract while the gateway translates to 4stoken /v1/videos and polls /v1/videos/{taskId}.

**Architecture:** Reuse relay/channel/task/newapivideo and add a dedicated 4stoken protocol profile that preserves the Ark content array and encodes the provider's snake_case fields. Keep public task IDs, Ark response conversion, task persistence, billing, settlement, refund, ownership and list filtering in the shared pipeline. Register 4stoken as channel type 209 and migrate the existing CH-4STOKEN import identity away from generic OpenAI type 1.

**Tech Stack:** Go 1.22+, Gin, httptest, GORM fixtures, testify require/assert, React/TypeScript, Bun, the existing channel-config converter, and common.Marshal/common.Unmarshal.

---

## Scope and contract

Source material:

- docs/new-channels/cn-4stoken-create-task.html: upstream Base URL https://api.4stoken.cn, Ark-compatible create documentation, provider request example POST /v1/videos, and task-only semantics.
- docs/new-channels/cn-4stoken-get-task.html: provider poll example GET /v1/videos/{taskId}; response fields id, model, status, error, content.video_url, content.last_frame_url, and usage.
- docs/channel/ark-video-generation-post.md, ark-video-generation-get.md, and ark-video-generation-gets.md: public Ark request/single/list contracts.
- e2e/testdata/channel-config-v1.json: existing CH-4STOKEN mappings and cost rows. This fixture is evidence for import migration, not a provider model catalog.

Public endpoints never change:

~~~text
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{public_task_id}
GET  /api/v3/contents/generations/tasks
~~~

The user changes only the Ark SDK base_url and gateway token. The upstream ID remains private in Task.PrivateData.UpstreamTaskID. Public responses use the local task_* ID, client-visible model, Ark status, and content.video_url.

Upstream request mapping:

| Ark field | 4stoken field | Rule |
| --- | --- | --- |
| model | model | mapped upstream model is required |
| content | content | preserve text and media objects, order, and roles |
| generate_audio | generate_audio | pointer, omit when absent |
| ratio | ratio | pointer/string, omit when absent |
| duration | duration | pointer integer, bounded by MaxTaskDurationSeconds |
| watermark | watermark | pointer, omit when absent |
| resolution | resolution | preserve only after fixture confirms provider acceptance |
| seed | seed | signed 64-bit parse, enforce -1 through uint32 maximum |
| callback_url | none | reject explicitly; the gateway does not proxy provider callbacks |

The provider document lists content task_id blocks and callback_url, but the current gateway has no safe public-to-upstream task-ID resolver or callback relay. Reject these fields with a deterministic 400 instead of silently dropping or forwarding a private-ID reference. Reject draft, non-empty tools, non-default service_tier, unknown content types, invalid roles, non-public media, empty prompt, and unsupported durations before pre-consume. Do not add generic chat/image routing for this channel.

The docs do not provide an authoritative 4stoken model catalog. Keep backend and frontend supportedModels empty and require administrators to map a client-visible Ark model to a verified upstream model. The existing import fixture contains these unique examples for acceptance fixtures: 4sdance_fast431, 4sdance431, 4sdance_v2.0_900, 4sdance933_fast, 4sdance_fast_933face, 4sdance933, and 4sdance_431_480p. Do not claim that list is complete.

Shared registry ownership: only the designated shared owner edits constant/channel.go, relay registry files, cost matrices, and common channel UI registries. The OmegaAI plan reserves type 208; this plan reserves type 209 and moves ChannelTypeDummy to 210 in the same registry change. The 4stoken branch owns its provider profile, request/response tests, E2E tests, import-converter tests, and form-specific tests, but not shared registry edits.

## Task 1: 4stoken request profile and Ark-to-provider encoding

Files:
- Modify relay/channel/task/newapivideo/profile.go, dto.go, adaptor.go
- Create relay/channel/task/newapivideo/fourstoken_request.go and fourstoken_request_test.go

- [ ] Write failing tests for exact POST /v1/videos JSON, snake_case generate_audio/watermark, ratio/duration/resolution, bounded seed, preserved content roles and media order, image/video/audio count limits, invalid task_id/callback_url rejection, and omission of absent optional scalars. Use JSON fixtures with a client model mapped to 4sdance_v2.0_900.
- [ ] Run: go test ./relay/channel/task/newapivideo -run 'TestFourSToken|TestBuildFourSToken' -count=1. Expect failure because the profile and dialect do not exist.
- [ ] Add ChannelNameFourSToken and videoRequestDialectFourSToken. Add a profile with submitPath /v1/videos, pollPath /v1/videos/{task_id}, application/json, public HTTP media enforcement, content-array dialect, and an empty model list.
- [ ] Use this exact DTO/profile shape so optional zero/false values survive JSON re-encoding and the unverified model catalog stays empty:

~~~go
type fourSTokenRequest struct {
	Model         string       `json:"model"`
	Content       []arkContent `json:"content"`
	GenerateAudio *bool        `json:"generate_audio,omitempty"`
	Ratio         *string      `json:"ratio,omitempty"`
	Duration      *int         `json:"duration,omitempty"`
	Watermark     *bool        `json:"watermark,omitempty"`
	Resolution    *string      `json:"resolution,omitempty"`
	Seed          *int64       `json:"seed,omitempty"`
}

func fourSTokenProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:            ChannelNameFourSToken,
		modelList:              []string{},
		submitPath:             "/v1/videos",
		pollPath:               "/v1/videos/{task_id}",
		contentType:            "application/json",
		requestDialect:         videoRequestDialectFourSToken,
		requirePublicHTTPMedia: true,
	}
}
~~~
- [ ] Depend on the OmegaAI shared foundation migration that makes Ark Ratio and Resolution pointer fields. Extend the DTO with Seed as a bounded pointer integer and CallbackURL as a pointer string so explicit fields can be rejected or translated deterministically. Add accepted field names to the parser, make every existing profile reject them when unsupported, and keep every optional scalar as a pointer with omitempty.
- [ ] Implement buildFourSTokenRequest with a private DTO that uses snake_case JSON tags, preserves the Ark content array, writes only the mapped upstream model, and uses common.Marshal. Keep content task_id unsupported with InvalidParameter.content; reject callback_url with InvalidParameter.callback_url.
- [ ] Validate seed with decimal/range checks before any integer conversion: allow -1 through 4294967295 and reject larger or malformed values. Enforce relaycommon.MaxTaskDurationSeconds and existing media SSRF/public-URL checks. Set ProviderValidationComplete before pre-consume and require it in BuildRequestBody.
- [ ] Dispatch the dialect in ValidateRequestAndSetAction and BuildRequestBody. Polling must substitute url.PathEscape(taskID) into /v1/videos/{task_id} exactly once. Run gofmt and focused tests, then commit feat(video): add 4stoken request profile.

## Task 2: Parse create and poll responses

Files:
- Modify relay/channel/task/newapivideo/dto.go, response.go, response_test.go

- [ ] Write response tests for create bodies with id, task_id, and taskId variants; reject conflicting IDs; poll queued/running/succeeded/failed/cancelled/expired statuses; content.video_url success; optional content.last_frame_url; nested error code/message; usage; missing success URL; malformed JSON; and sanitization of upstream IDs from public errors.
- [ ] Extend upstreamSubmitResponse to include the observed taskId spelling without changing the shared public response. Prefer task_id, then taskId, then id, and reject conflicting non-empty values.
- [ ] Reuse the shared direct task projection for the documented fields content.video_url, content.last_frame_url, model, status, created_at, updated_at, error, and usage. Map queued/running to in-progress internal states, succeeded to success, and failed/cancelled/expired to failure. A succeeded task without content.video_url must fail closed.
- [ ] The Ark converter must use local public ID and origin model and must not expose upstream model, task ID, key, channel ID, raw data, or quota fields. Preserve only allowlisted content URLs and bounded usage/duration.
- [ ] Run gofmt and go test ./relay/channel/task/newapivideo -run 'TestFourSToken|TestParse.*Task|TestConvert' -count=1; commit feat(video): parse 4stoken task responses.

## Task 3: Register backend type 209 and migrate Ark task routing

Files:
- Modify (shared registry owner): constant/channel.go, constant/channel_test.go
- Modify (shared registry owner): relay/relay_adaptor.go, relay/seedance_task.go, relay/relay_task.go
- Modify: relay/relay_task_seedance_test.go, relay/cost_accounting_adaptor_test.go
- Modify: controller/channel-test.go, controller/channel_test_internal_test.go

- [ ] Add failing assertions for ChannelTypeFourSToken=209, ChannelTypeDummy=210, base URL https://api.4stoken.cn, display name 4stoken, no ChannelType2APIType mapping, GetTaskAdaptor("209") implementing Ark conversion and cost accounting, task-platform inclusion, and generic-channel-test exclusion.
- [ ] Require the shared constants reservation from the OmegaAI plan: ChannelTypeFourSToken=209 and ChannelTypeDummy=210 must already exist. The designated shared registry owner now returns newapivideo.NewFourSTokenTaskAdaptor and adds 209 to isSeedanceTaskPlatform, seedanceTaskPlatformValues, Ark converter enforcement, relay task routing, cost-accounting capability tests, and generic channel-test exclusions. Keep it out of generic OpenAI/video routes.
- [ ] Add public/private task fixtures and list filters. Assert all Ark single/list responses use public task IDs, origin model, allowlisted content fields, and ownership checks without leaking 4stoken private data.
- [ ] Run gofmt and go test ./constant ./relay ./controller -run 'TestFourSToken|TestSeedanceTask|TestSupportsGenericChannelTest' -count=1; commit feat(4stoken): register Seedance task channel.

## Task 4: Management form and import-converter migration

Files:
- Modify web/src/features/channels/constants.ts
- Modify web/src/features/channels/lib/channel-type-config.ts and channel-utils.ts
- Modify web/tests/channel-type-config.test.ts
- Modify web/src/channel-config-converter/document.ts
- Modify web/src/channel-config-converter/__tests__/v1.test.ts
- Modify web/src/i18n/locales/en.json, zh.json, zh-TW.json, fr.json, ru.json, ja.json, vi.json

- [ ] Add failing UI tests asserting type 209, label 4stoken, NewAPI icon, default URL https://api.4stoken.cn, empty static model list, task-only status, disabled generic test/model fetch, mapping hint, and proxy URL preservation.
- [ ] Configure 209 with supportedModels empty and hints that administrators must map client-visible Ark names to verified 4stoken upstream models. Add 209 to channel display order, task-only/generic-test sets, managed default URL set, key prompt, and task-only warning. Do not invent a provider model catalog.
- [ ] Add V1 converter mapping CH-4STOKEN from type 1 to 209. Extend v1.test.ts to assert typesByChannel.get('CH-4STOKEN') equals 209 and that imported 4stoken model mappings retain their source line and upstream_model. Do not map an unreferenced CH-OMEGA identity; OmegaAI has no authoritative workbook channel reference in the current source.
- [ ] Add translated channel name, default URL, raw key/mapping hints, and task-only warning in all seven locales. Run bun run i18n:sync, bun test tests/channel-type-config.test.ts, bun test for the converter package, and bun run typecheck.
- [ ] Commit only the listed UI/converter files with feat(web): add 4stoken channel configuration.

## Task 5: Ark lifecycle, billing, refunds, and invalid-input isolation

Files:
- Create e2e/fourstoken_upstream_e2e_test.go
- Modify relay/relay_task_billing_test.go
- Modify relay/channel/task/newapivideo/fourstoken_request_test.go and response_test.go

- [ ] Build a deterministic mock upstream. Assert POST /v1/videos, Bearer auth, mapped model, exact content array, snake_case fields, and omitted absent fields. Return a create response using task_id, poll first queued then running then succeeded with content.video_url and optional last_frame_url.
- [ ] Assert Ark create returns only the local public ID. Assert single/list returns client-visible model, succeeded state, content.video_url, and no upstream ID/model/key/channel/quota/routing data. Verify task IDs with reserved URL characters are escaped exactly once.
- [ ] Add failures for upstream rejected requests and failed polling; assert one refund only. Add invalid callback_url, task_id content, out-of-range seed, unsupported content role, non-public media, and duration overflow; assert HTTP 400 before upstream calls, task row creation, or quota mutation.
- [ ] Extend the shared billing matrix for ChannelTypeFourSToken. Cover per-request and per-duration imported cost rows, request-duration snapshot, successful settlement, failed refund, upstream usage when present, and bounded actual duration. Use the centralized quota helpers and never write OtherRatios directly.
- [ ] Run gofmt, go test ./e2e -run TestFourSToken -count=1 -v, and go test ./relay -run 'TestFourSToken|TestTaskBilling' -count=1 -v; commit test(4stoken): cover Ark video lifecycle.

## Task 6: Real upstream acceptance and release gate

Files:
- Create after success: docs/superpowers/reports/2026-08-01-4stoken-channel-acceptance.md

- [ ] Read only FOURSTOKEN_API_KEY and optional FOURSTOKEN_BASE_URL from the environment. Configure one disabled new-api channel type 209 and explicit model mappings for verified upstream names from the current source fixture. Do not place keys in Go tests, frontend fixtures, docs, or git history.
- [ ] Through the gateway Ark endpoints, submit one per-request and one per-duration model, including text-only and at least one reference image. Poll with public task IDs, verify MP4 URL/readability, and check queued/running/succeeded and failed flows.
- [ ] Cross-check that the upstream receives /v1/videos, the request fields have exact snake_case names, the public Ark response includes content.video_url, billing settles/refunds once, and imported CH-4STOKEN data no longer routes through generic OpenAI type 1.
- [ ] Run go test ./relay/channel/task/newapivideo ./constant ./relay ./controller ./e2e -count=1, go build ./..., the focused Bun tests, bun run typecheck, bun run build, and git diff --check.
- [ ] Keep the channel disabled until real create/poll/failed/refund checks pass. Commit docs: record 4stoken channel acceptance.

## Self-review

| Requirement | Covered |
| --- | --- |
| Ark SDK zero-code create/single/list | Scope, Tasks 3 and 5 |
| 4stoken /v1/videos submit and poll | Tasks 1, 2, and 5 |
| Documented content/field semantics | Scope and Task 1 |
| Public/private task-ID isolation | Tasks 2, 3, and 5 |
| CH-4STOKEN import no longer type 1 | Task 4 and Task 6 |
| Billing/settlement/refund safety | Task 5 |
| Real upstream contract gate | Task 6 |

No implementation may bypass the shared Ark task router, and no real API key may be committed.
