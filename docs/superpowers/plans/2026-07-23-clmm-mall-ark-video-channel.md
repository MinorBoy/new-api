# CLMM Mall Ark Video Channel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a task-only CLMM Mall channel that accepts the existing Ark video-task API, translates it to CLMM Mall `/v1/videos`, polls safely, and preserves local billing and public task-ID isolation.

**Architecture:** Register channel type 61 and a focused `TaskAdaptor` under `relay/channel/task/clmmmall`. The adaptor validates a typed Ark request, parses the mapped CLMM channel prefix and documented control suffixes, converts Ark duration into `seconds`/`mySeconds`, and implements submit, polling, billing duration, and Ark response conversion. Existing routes, task persistence, settlement, and refunds remain shared.

**Tech Stack:** Go 1.22+, Gin, GORM v2, `common` JSON wrappers, `testify`, React 19, TypeScript, Bun, i18next.

---

## Task 1: Register the Task-Only Channel

**Files:**
- Create: `relay/channel/task/clmmmall/constants.go`
- Modify: `constant/channel.go`
- Modify: `constant/channel_test.go`
- Modify: `relay/relay_adaptor.go`
- Modify: `relay/seedance_task.go`
- Modify: `controller/channel-test.go`
- Modify: `controller/channel_test_internal_test.go`

- [ ] **Step 1: Write failing registration tests.** Add `TestClmmMallChannelConstants` asserting type 61, Dummy 62, default URL `https://clmm-mall.top`, name `CLMM Mall`, and no `ChannelType2APIType` mapping. Extend the generic channel-test test to reject type 61.

- [ ] **Step 2: Run the focused tests and confirm missing symbols fail.**

```powershell
go test ./constant ./controller -run 'TestClmmMallChannelConstants|TestSupportsGenericChannelTest' -count=1
```

- [ ] **Step 3: Implement registration.** Add:

```go
ChannelTypeClmmMall = 61 // CLMM Mall video generation (Ark task gateway)
ChannelTypeDummy    = 62 // this one is only for count, do not add any channel after this
```

Append the index-61 base URL and channel name. Do not add an API type. Create `constants.go` with `ChannelName = "CLMM Mall"` and an empty `ModelList`. Register `TaskAdaptor` in `GetTaskAdaptor`, add type 61 to the Ark list platform set, and disable generic channel testing.

- [ ] **Step 4: Format, verify, and commit.**

```powershell
gofmt -w constant/channel.go constant/channel_test.go relay/channel/task/clmmmall/constants.go relay/relay_adaptor.go relay/seedance_task.go controller/channel-test.go controller/channel_test_internal_test.go
go test ./constant ./controller -run 'TestClmmMallChannelConstants|TestSupportsGenericChannelTest' -count=1
git add constant/channel.go constant/channel_test.go relay/channel/task/clmmmall/constants.go relay/relay_adaptor.go relay/seedance_task.go controller/channel-test.go controller/channel_test_internal_test.go
git commit -m "feat(video): register CLMM Mall Ark channel"
```

## Task 2: Translate Ark Requests to CLMM Mall

**Files:**
- Create: `relay/channel/task/clmmmall/dto.go`
- Create: `relay/channel/task/clmmmall/translate.go`
- Create: `relay/channel/task/clmmmall/translate_test.go`

- [ ] **Step 1: Write failing exact-output tests.** Cover text-only with default `480p`, `9:16` size derivation, all image roles degrading in original order, three reference videos, joined text items, ordinary `-Ns` with `mySeconds`, and fixed `-gz` with `seconds:"1"`. Add rejection tables for missing prompt/media URL, audio, invalid ratio/resolution/duration, more than 9 images, more than 3 videos, unknown fields, non-default `service_tier`, `-gz` without `-Ns`, insufficient `-Nimg`, and duration above an `-Ns` limit. Cover `-nv` dropping reference videos and model resolution suffix precedence.

- [ ] **Step 2: Run tests and confirm translation functions are absent.**

```powershell
go test ./relay/channel/task/clmmmall -run 'TestArkToClmm|TestClmmValidation' -count=1
```

- [ ] **Step 3: Define private DTOs.** Optional Ark scalars use pointers. The outbound DTO is:

```go
type ClmmRequest struct {
	Model              string   `json:"model"`
	Prompt             string   `json:"prompt"`
	AspectRatio        string   `json:"aspect_ratio"`
	Resolution         string   `json:"resolution"`
	Size               string   `json:"size"`
	Seconds            string   `json:"seconds"`
	MySeconds          string   `json:"mySeconds,omitempty"`
	ReferenceImageURLs []string `json:"reference_image_urls,omitempty"`
	ReferenceVideos    []string `json:"reference_videos,omitempty"`
}
```

Define Ark content/media/tool DTOs, CLMM submit/task response DTOs, and a public Ark task projection. Use `common.Marshal` and `common.Unmarshal` for operations; `encoding/json` types are allowed only as types.

- [ ] **Step 4: Implement deterministic validation and conversion.** Validate allowed top-level fields before typed unmarshal. Join non-empty text with newline. Degrade empty, `first_frame`, `last_frame`, and `reference_image` roles into `reference_image_urls`. Require `reference_video`; reject audio and unsupported Ark fields. Default ratio/resolution/duration to `16:9`, `480p`, and 5. Enforce ratio set, `480p`/`720p`, duration bounds and `MaxTaskDurationSeconds`, image limit 9, video limit 3, and total media limit 12. Require a documented channel prefix without enumerating concrete base models. Parse only documented resolution, `-{n}s`, `-gz`, `-{n}img`, and `-nv` control suffixes. No `-Ns` sends Ark duration as `seconds`; ordinary `-Ns` sends `seconds:"1"` and actual duration as `mySeconds`; `-gz` requires `-Ns` and fixes `mySeconds` to n. `-Nimg` enforces a minimum and `-nv` drops reference videos.

- [ ] **Step 5: Format, verify, and commit.**

```powershell
gofmt -w relay/channel/task/clmmmall/dto.go relay/channel/task/clmmmall/translate.go relay/channel/task/clmmmall/translate_test.go
go test ./relay/channel/task/clmmmall -run 'TestArkToClmm|TestClmmValidation' -count=1
git add relay/channel/task/clmmmall/dto.go relay/channel/task/clmmmall/translate.go relay/channel/task/clmmmall/translate_test.go
git commit -m "feat(video): translate Ark requests for CLMM Mall"
```

## Task 3: Implement the CLMM Task Adaptor

**Files:**
- Create: `relay/channel/task/clmmmall/adaptor.go`
- Create: `relay/channel/task/clmmmall/adaptor_test.go`
- Create: `relay/channel/task/clmmmall/response_test.go`

- [ ] **Step 1: Write failing adaptor tests.** With `httptest.Server`, assert POST `/v1/videos`, Bearer header, exact body including `mySeconds`, `task_id` then `id` fallback, GET `/v1/videos/{escaped-id}`, all documented status aliases, progress clamping, URL priority `video_url` then `url` then `result_url` then `metadata.url`, unknown-status retry error, and public Ark output without upstream ID.

- [ ] **Step 2: Run tests and confirm the adaptor is absent.**

```powershell
go test ./relay/channel/task/clmmmall -run 'TestTaskAdaptor|TestParseTaskResult|TestConvertToArk' -count=1
```

- [ ] **Step 3: Implement validation and submit.** Embed `taskcommon.BaseBilling`. Require `common.KeySeedanceOfficialAPI`, reject non-Ark input, detect unknown top-level JSON fields before typed decoding, store the typed request in context, and call `StoreTaskRequest`. In `ValidateBillingRequest`, use mapped `info.UpstreamModelName`; require a documented channel prefix and validate the parsed control suffix contract without enumerating the concrete base model. Build JSON/Bearer request and use `channel.DoTaskApiRequest`.

- [ ] **Step 4: Implement response and errors.** `DoResponse` reads `task_id` then `id`, requires a non-empty upstream ID, persists upstream JSON, and writes only `{"id": publicID}`. `ParseTaskError` maps 400/422 to client error, 429 to rate limit, and upstream 401/403/other failures to stable gateway errors without raw secrets.

- [ ] **Step 5: Implement polling and Ark conversion.** Fetch with GET, `url.PathEscape`, Bearer key, and configured proxy. Parse status aliases case-insensitively, clamp progress, and extract URL priority. Unknown/empty status returns an error. `ConvertToArkVideoTask` allowlists public fields and always uses local public ID plus origin model.

- [ ] **Step 6: Format, verify, and commit.**

```powershell
gofmt -w relay/channel/task/clmmmall/adaptor.go relay/channel/task/clmmmall/adaptor_test.go relay/channel/task/clmmmall/response_test.go
go test ./relay/channel/task/clmmmall -run 'TestTaskAdaptor|TestParseTaskResult|TestConvertToArk' -count=1
git add relay/channel/task/clmmmall/adaptor.go relay/channel/task/clmmmall/adaptor_test.go relay/channel/task/clmmmall/response_test.go
git commit -m "feat(video): add CLMM Mall task adaptor"
```

## Task 4: Integrate Duration Billing and Ark Queries

**Files:**
- Modify: `relay/channel/task/clmmmall/adaptor.go`
- Create: `relay/channel/task/clmmmall/billing_test.go`
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `relay/seedance_task.go` only when a failing query test proves a missing integration

- [ ] **Step 1: Write failing billing/query tests.** Assert ordinary duration, ordinary `-Ns` explicit/default duration, fixed `-gz` actual duration rather than placeholder 1, Ark success/failure query shape, list inclusion and filtering, user ownership, and absence of upstream IDs from single/list responses.

- [ ] **Step 2: Run tests and confirm missing behavior.**

```powershell
go test ./relay ./relay/channel/task/clmmmall -run 'TestClmmMallDuration|TestSeedanceTaskFetch|TestSeedanceTaskList' -count=1
```

- [ ] **Step 3: Implement `TaskDurationEstimator`.** Models without `-Ns` return validated Ark duration. Ordinary `-Ns` returns explicit Ark duration when present, otherwise suffix n. `-gz` returns suffix n regardless of Ark duration. Keep `EstimateBilling`, submit adjustment, and completion adjustment as no-ops. Do not add `seconds`/`duration` other ratios. Central `taskDurationQuota` remains responsible for decimal calculation and quota saturation auditing.

- [ ] **Step 4: Complete Ark query integration.** Ensure type 61 is in the shared platform list and that `ArkVideoTaskConverter` is mandatory for type 61. Never fall through to raw `Task.Data`. Do not add CLMM to OpenAI video routes.

- [ ] **Step 5: Format, verify, and commit.**

```powershell
gofmt -w relay/channel/task/clmmmall/adaptor.go relay/channel/task/clmmmall/billing_test.go relay/relay_task_seedance_test.go relay/seedance_task.go
go test ./relay ./relay/channel/task/clmmmall -run 'TestClmmMallDuration|TestSeedanceTaskFetch|TestSeedanceTaskList' -count=1
git add relay/channel/task/clmmmall/adaptor.go relay/channel/task/clmmmall/billing_test.go relay/relay_task_seedance_test.go relay/seedance_task.go
git commit -m "feat(video): integrate CLMM Mall billing and Ark queries"
```

## Task 5: Configure the Default Frontend and i18n

**Files:**
- Modify: `web/default/src/features/channels/constants.ts`
- Modify: `web/default/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/default/src/features/channels/lib/channel-utils.ts`
- Modify: `web/default/tests/channel-type-config.test.ts`
- Modify: `web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`

- [ ] **Step 1: Read required frontend instructions.** Read `web/default/AGENTS.md` and the complete `i18n-translate`, `shadcn-ui`, and `vercel-react-best-practices` skills before editing.

- [ ] **Step 2: Write failing frontend tests.** Assert type/name 61, option presence, `Jimeng` icon, default URL, raw-Key hint, empty supported models, and generic-test disablement.

- [ ] **Step 3: Run the focused test and confirm failure.**

```powershell
Set-Location web/default
bun test tests/channel-type-config.test.ts
Set-Location ../..
```

- [ ] **Step 4: Implement metadata and translations.** Add type 61 after the existing type 60, default URL `https://clmm-mall.top`, icon `Jimeng`, raw-Key prompt, task-only warning, and model-mapping guidance. Use English source keys and add real values to all six required locales. Do not stage generated reports unless the i18n command proves they are required.

- [ ] **Step 5: Verify and commit frontend changes.**

```powershell
Set-Location web/default
bun test tests/channel-type-config.test.ts
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
bun run build
Set-Location ../..
git add web/default/src/features/channels/constants.ts web/default/src/features/channels/lib/channel-type-config.ts web/default/src/features/channels/lib/channel-utils.ts web/default/tests/channel-type-config.test.ts web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/vi.json
git commit -m "feat(web): configure CLMM Mall channel"
```

## Task 6: Full Verification and Review

**Files:** Only files owned by Tasks 1-5 may change during fixes.

- [ ] **Step 1: Run focused backend verification.**

```powershell
go test ./constant ./controller ./relay/channel/task/clmmmall ./relay -count=1
```

- [ ] **Step 2: Run complete backend verification.**

```powershell
go test ./... -count=1
go vet ./...
go build ./...
```

- [ ] **Step 3: Run complete frontend verification.**

```powershell
Set-Location web/default
bun test tests/channel-type-config.test.ts
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
bun run build
Set-Location ../..
```

- [ ] **Step 4: Audit protocol and billing invariants.**

```powershell
rg -n '/v1/videos|seconds|reference_image_urls|reference_videos' relay/channel/task/clmmmall
rg -n 'OtherRatios\[|int\(.*quota|math\.Round' relay/channel/task/clmmmall relay/relay_task.go
git diff --check
git status --short
```

Inspect every match. There must be no direct other-ratio map writes, bare quota conversion, upstream task ID in public converters, or unrelated parent-worktree files.

- [ ] **Step 5: Review every approved-spec section against a test.** Confirm request mapping, image-role degradation, model mapping, `-Ns`/`-gz` duration behavior, control suffixes, status aliases, error mapping, public ID isolation, refunds, admin configuration, and i18n each have automated coverage. Fix only proven gaps, rerun the owning task tests, and commit the explicit files.
