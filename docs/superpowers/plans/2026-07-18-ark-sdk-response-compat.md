# ARK SDK Response Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Seedance Native create, query-failure, and HTTP error responses match the official ARK SDK contracts while retaining new-api public task ID isolation and billing refunds.

**Architecture:** Keep the upstream task response as the source of truth for all official response fields. Replace only the upstream task ID with the public `task_*` ID, normalize the public status, and fill `model` or timestamps only when the upstream response omitted them. Create-task responses use the official `ContentGenerationTaskID` shape instead of the OpenAI Video shape.

**Tech Stack:** Go 1.22+, Gin, GORM, testify, SQLite in-memory E2E, official `volcenginesdkarkruntime` response models as the contract reference.

---

### Task 1: Official create response contract

**Files:**
- Modify: `relay/channel/task/doubao/adaptor_test.go`
- Modify: `relay/channel/task/doubao/adaptor.go`

- [x] Add a test that passes upstream `{"id":"cgt-secret"}` through `TaskAdaptor.DoResponse` with public ID `task_public` and asserts the exact client JSON object is `{"id":"task_public"}`.
- [x] Run `go test ./relay/channel/task/doubao -run TestDoResponseUsesARKTaskIDShape -count=1` and confirm it fails because the response currently contains OpenAI Video fields.
- [x] Replace `dto.NewOpenAIVideo()` response construction with `c.JSON(http.StatusOK, responsePayload{ID: info.PublicTaskID})`.
- [x] Re-run the focused test and confirm it passes.

### Task 2: Official failed-task query contract

**Files:**
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `relay/seedance_task.go`

- [x] Add the real ARK failed-task fixture containing `error`, `created_at`, `updated_at`, `service_tier`, `execution_expires_after`, `generate_audio`, `draft`, and `priority`.
- [x] Assert all fixture fields survive public conversion, `id` becomes `task_public`, and upstream timestamps remain unchanged.
- [x] Run `go test ./relay -run TestSeedanceTaskFetchPreservesOfficialFailedTaskFields -count=1` and confirm it fails because local timestamps currently overwrite upstream timestamps.
- [x] Change `seedanceTaskResponse` to replace `id` and normalized `status`, but only fill `model`, `created_at`, and `updated_at` when those fields are absent from upstream data.
- [x] Re-run the focused test and confirm it passes.

### Task 3: HTTP E2E failure and refund acceptance

**Files:**
- Modify: `e2e/seedance_native_e2e_test.go`

- [x] Add an HTTP E2E mock that returns `{"id":"cgt-mock-failed"}` on submit and the real failed-task fixture on polling.
- [x] Assert submit response is exactly the official one-field object, public query returns HTTP 200 with the complete official failure object, and the upstream ID is never exposed.
- [x] Assert failed polling refunds the pre-consumed quota once: user/channel/token usage and `quota_data.Quota` return to their pre-submit values while `quota_data.Count` remains one.
- [x] Run `go test ./e2e -run TestSeedanceNativeFailedTaskResponseAndRefundE2E -count=1 -v` and confirm it passes.

### Task 4: Official SDK and report verification

**Files:**
- Modify: `docs/superpowers/reports/2026-07-18-ark-native-compat-e2e-acceptance.md`

- [x] Correct all create-response examples to `{"id":"task_<public>"}`.
- [x] Replace the synthetic failure example with the real ARK `OutputVideoSensitiveContentDetected.PolicyViolation` fixture and document the exact allowed ID substitution.
- [x] Document HTTP transport/upstream errors separately from HTTP 200 task failure objects.
- [x] Run focused Go tests, `go test ./e2e ./relay ./relay/channel/task/doubao ./controller -count=1`, official SDK model parsing against captured response JSON, and `git diff --check`.
