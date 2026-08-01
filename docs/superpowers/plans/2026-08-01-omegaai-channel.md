# OmegaAI Seedance Channel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Add OmegaAI as a task-only Seedance channel while users continue using the Ark SDK /api/v3/contents/generations/tasks/* endpoints with zero client code changes.

**Architecture:** Extend relay/channel/task/newapivideo with an OmegaAI media-array request dialect. Keep the shared Ark create, public-ID persistence, polling, single-task/list conversion, billing, settlement, refund, ownership, and privacy paths. Provider-specific code owns request validation/encoding and response normalization. Include all four documented video models: klsdpro2-720p, seedance-v2-720p, dola-seedance-2.0, lingjing-video-v1.

**Tech Stack:** Go 1.22+, Gin, httptest, GORM fixtures, testify require/assert, React/TypeScript, Bun, and common.Marshal/common.Unmarshal.

---

## Contract and boundaries

Evidence files:

- docs/new-channels/cn-omegaai.html: Base URL https://omegaai.xin, Bearer auth, POST /v1/media/generate, GET /v1/tasks/{taskId}, and the four model examples.
- docs/channel/ark-video-generation-post.md: Ark create request.
- docs/channel/ark-video-generation-get.md: Ark single-task response.
- docs/channel/ark-video-generation-gets.md: Ark list response.
- docs/channel/ark-video-generation-delete.md: deletion is not part of this provider plan.

Public endpoints remain:

~~~text
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{public_task_id}
GET  /api/v3/contents/generations/tasks
~~~

Only the gateway base URL and token change for the user. Upstream IDs remain private in Task.PrivateData.UpstreamTaskID; Ark responses expose local task_* IDs and the client-visible model.

Provider request mapping:

| Ark | OmegaAI | Contract |
| --- | --- | --- |
| model | model | mapped upstream model |
| one text content item | prompt | exact text |
| reference image items | images array | preserve order, max 9 |
| reference video items | videos array | only klsdpro2-720p, max 3 |
| reference audio items | audios array | only klsdpro2-720p, max 3 |
| ratio | aspect_ratio | omit when absent |
| duration | duration | explicit integer, bounded by MaxTaskDurationSeconds |
| resolution | omitted | no unverified upstream field |

Reject first_frame/last_frame roles, unsupported generate_audio/watermark/seed/callback_url, non-default service_tier, draft, non-empty tools, empty prompt, private/non-public media, and unsupported media for a model. Do not silently discard an explicit Ark field. The lingjing prompt preserves caller-supplied @参考图N references.

Shared registry ownership: only the designated shared owner edits constant/channel.go, relay registry files, cost matrices, and common channel UI registries. Reserve type 208 for OmegaAI, 209 for 4stoken, and move ChannelTypeDummy to 210 in one registry change. OmegaAI provider work must not edit those files independently.

## Task 1: OmegaAI profile and request translation

Files:
- Modify relay/channel/task/newapivideo/profile.go, dto.go, adaptor.go
- Create relay/channel/task/newapivideo/omega_request.go and omega_request_test.go

- [ ] Write failing tests for exact array translation, image-only/video-audio rejection, nine-image acceptance/ten-image rejection, frame-role rejection, unsupported-field rejection, duration bounds, and omitted optional scalar fields. The core fixture must assert model klsdpro2-720p, prompt, duration, aspect_ratio, images, videos, and audios in exact JSON.
- [ ] Run: go test ./relay/channel/task/newapivideo -run 'TestOmegaAI|TestBuildOmega' -count=1. Expect failure because the dialect and profile do not exist.
- [ ] Add ChannelNameOmegaAI, videoRequestDialectOmegaMediaArrays, the four-model list, and omegaRequestProfile with per-model media capabilities. Add omegaMediaRequest using pointer duration and omitempty arrays.
- [ ] In the same shared foundation change, migrate optional Ark Ratio and Resolution fields from string to *string and update existing Lucen/MegaByAI/Cangyuan/Paipu/Secure validators and builders. Tests must distinguish absent fields from explicit empty strings; explicit empty is rejected, while absent remains nil and is omitted upstream. OmegaAI outbound AspectRatio and every other optional scalar use pointer types with omitempty.
- [ ] Use this exact profile shape; keep provider capability details inside omegaRequestProfile instead of adding channel-name conditionals throughout the adaptor:

~~~go
func omegaAIProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:            ChannelNameOmegaAI,
		modelList:              []string{"klsdpro2-720p", "seedance-v2-720p", "dola-seedance-2.0", "lingjing-video-v1"},
		submitPath:             "/v1/media/generate",
		pollPath:               "/v1/tasks/{task_id}",
		contentType:            "application/json",
		requestDialect:         videoRequestDialectOmegaMediaArrays,
		requirePublicHTTPMedia: true,
		omegaRequest:           &omegaRequestProfile{MaxImages: 9, MaxVideos: 3, MaxAudios: 3},
	}
}

func NewOmegaAITaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: omegaAIProtocolProfile()}
}
~~~
- [ ] Return a profile with submitPath /v1/media/generate, pollPath /v1/tasks/{task_id}, application/json, public HTTP media enforcement, max nine images, and model-specific video/audio capability.
- [ ] Implement buildOmegaAIRequest with common.Marshal. Preserve text and media order, translate ratio to aspect_ratio, omit absent fields, and never add provider defaults. Validate mapped model capability before pricing/pre-consume; set ProviderValidationComplete and require it again in BuildRequestBody.
- [ ] Route the dialect through ValidateRequestAndSetAction and BuildRequestBody. Polling must replace the task template using url.PathEscape exactly once. Return local HTTP 400 errors with Ark field-specific codes.
- [ ] Run gofmt on changed files, the focused test package, and commit feat(video): add OmegaAI request profile.

## Task 2: OmegaAI response normalization

Files:
- Modify relay/channel/task/newapivideo/dto.go, response.go, response_test.go

- [ ] Read OMEGA_AI_API_KEY and optional OMEGA_AI_BASE_URL only from the environment for temporary contract capture. Redact keys, signed query strings, private IDs, and user media before fixtures. The HTML does not define a complete response body, so observed responses are authoritative.
- [ ] Write response tests from redacted fixtures for submit ID forms actually observed, queued/running/succeeded/failed/expired aliases, verified result URL location, nested errors, missing URL on success, malformed JSON, bounded progress, and private-ID redaction.
- [ ] Extend private DTOs only with observed fields. Normalize statuses to model.TaskStatus, extract only the verified video URL, sanitize failure text, and retain bounded duration/usage only when actually returned. A success without URL must fail closed.
- [ ] Run gofmt and go test ./relay/channel/task/newapivideo -run 'TestOmegaAI|TestParse.*Task|TestConvert' -count=1, then commit feat(video): parse OmegaAI task responses.

## Task 3: Backend registration and Ark platform isolation

Files:
- Shared owner modifies constant/channel.go, constant/channel_test.go, relay/relay_adaptor.go, relay/seedance_task.go, relay/relay_task.go
- Modify relay/relay_task_seedance_test.go, relay/cost_accounting_adaptor_test.go, controller/channel-test.go, controller/channel_test_internal_test.go

- [ ] Add failing tests for type 208, https://omegaai.xin, display name OmegaAI, no ChannelType2APIType mapping, GetTaskAdaptor 208 implementing Ark conversion/cost accounting, Ark platform inclusion, and generic channel-test exclusion.
- [ ] Reserve ChannelTypeOmegaAI=208 and ChannelTypeFourSToken=209, move ChannelTypeDummy to 210, and add both default URLs/names in one shared constants change. At this stage return only NewOmegaAITaskAdaptor and add only OmegaAI to seedanceTaskPlatformValues, Ark converter enforcement, relay routing, cost capability tests, and generic-test exclusions. The 4stoken plan adds its adaptor/platform after its provider code exists.
- [ ] Add task fixtures with public task_omega_public and a different private upstream ID. Single/list responses must omit private ID, upstream model, channel ID, key, quota, and routing internals.
- [ ] Run gofmt and go test ./constant ./relay ./controller -run 'TestOmegaAI|TestSeedanceTask|TestSupportsGenericChannelTest' -count=1, then commit feat(omegaai): register Seedance task channel.

## Task 4: Management form and i18n

Files:
- Modify web/src/features/channels/constants.ts
- Modify web/src/features/channels/lib/channel-type-config.ts and channel-utils.ts
- Modify web/tests/channel-type-config.test.ts
- Modify web/src/i18n/locales/en.json, zh.json, zh-TW.json, fr.json, ru.json, ja.json, vi.json

- [ ] Add failing tests asserting type 208 OmegaAI, NewAPI icon, default URL https://omegaai.xin, exactly four models, task-only status, disabled generic test/model fetch, and proxy URL preservation.
- [ ] Add 208 to channel types/display order/task-only sets/key prompts/warnings/managed default URLs. Configure models [klsdpro2-720p, seedance-v2-720p, dola-seedance-2.0, lingjing-video-v1] and hints for Base URL, raw API key, and four documented models.
- [ ] Translate channel name, URL/key/model hints, and task-only warning in all seven locales. Run bun run i18n:sync, bun test tests/channel-type-config.test.ts, bun run typecheck.
- [ ] Commit only the listed OmegaAI UI files with feat(web): add OmegaAI channel configuration.

## Task 5: Ark lifecycle, billing, refunds, and no-side-effect errors

Files:
- Create e2e/omegaai_upstream_e2e_test.go
- Modify relay/relay_task_billing_test.go

- [ ] Build a deterministic mock upstream that asserts POST /v1/media/generate, Bearer auth, mapped model, prompt, aspect_ratio, duration, and media arrays. Return queued/running/succeeded fixtures from GET /v1/tasks/{escaped_private_id}. Assert Ark create returns public ID and Ark single/list returns succeeded with content.video_url only.
- [ ] Test image-only models reject video/audio with HTTP 400 InvalidParameter.content before upstream call, task creation, or quota mutation. Test frame roles, unsupported fields, ten images, and out-of-range durations with the same zero-side-effect invariant.
- [ ] Extend billing fixtures for ChannelTypeOmegaAI. Verify per-request and per-duration behavior, request-duration snapshot, exactly-once settlement, exactly-once failure refund, and bounded upstream actual duration. Do not write OtherRatios directly or use bare quota casts.
- [ ] Run gofmt, go test ./e2e -run TestOmegaAI -count=1 -v, and go test ./relay -run 'TestOmegaAI|TestTaskBilling' -count=1 -v; commit test(omegaai): cover Ark video lifecycle.

## Task 6: Real upstream acceptance and release gate

Files:
- Create after success: docs/superpowers/reports/2026-08-01-omegaai-channel-acceptance.md

- [ ] Through the gateway Ark endpoints, using only OMEGA_AI_API_KEY and optional OMEGA_AI_BASE_URL environment values, submit one low-cost request for each model: klsd with image/video/audio, seedance-v2 with image, dola with image, lingjing with two ordered images. Poll with public task IDs and verify non-empty MP4 results.
- [ ] Cross-check mapped model, statuses, public/private ID isolation, settlement/refund, media URL presence, and no leakage of keys/signed URLs/private IDs in the report.
- [ ] Run go test ./relay/channel/task/newapivideo ./constant ./relay ./controller ./e2e -count=1, go build ./..., bun test tests/channel-type-config.test.ts, bun run typecheck, bun run build, and git diff --check.
- [ ] Keep the channel disabled until all four model checks pass. Commit docs: record OmegaAI channel acceptance.

## Self-review

| Requirement | Covered |
| --- | --- |
| Ark SDK zero-code create/single/list | Scope, Tasks 3 and 5 |
| All four documented models | Tasks 1 and 4 |
| OmegaAI media submit/poll paths | Tasks 1, 2, and 5 |
| Media validation/no-side-effects | Tasks 1 and 5 |
| Public/private ID isolation | Tasks 2, 3, and 5 |
| Billing/settlement/refund safety | Task 5 |
| Real upstream contract gate | Tasks 2 and 6 |

No implementation may bypass the shared Ark task router, and no real API key may be committed.
