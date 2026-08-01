# Seedance Terminal Usage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Return Ark-compatible `completion_tokens` and `total_tokens` for every new successful Seedance task, calculating them locally when per-request or per-duration upstreams omit usage.

**Architecture:** Resolve reference-video duration once before upstream dispatch and persist only its aggregate milliseconds in the task billing snapshot. Keep upstream polling/audit payloads untouched; augment only the public Ark projection with the existing Seedance token estimator when authoritative usage is missing or all-zero.

**Tech Stack:** Go 1.22+, Gin, GORM v2, shopspring/decimal, testify

---

### Task 1: Persist Usage Calculation Inputs

**Files:**
- Modify: `relay/common/relay_info.go`
- Modify: `model/task.go`
- Modify: `controller/relay.go`
- Test: `controller/cost_task_relay_test.go`
- Test: `model/task_private_data_test.go`

- [ ] **Step 1: Write failing persistence tests**

Add assertions that `persistSubmittedTask` copies the aggregate reference-video duration and selected upstream cost mode into `TaskBillingContext`, and that JSON round-tripping preserves both fields:

```go
relayInfo.TaskRelayInfo.InputVideoDurationMS = 2500
relayInfo.CostAttempt = &types.CostAttemptHandle{CostMode: types.CostModePerDuration}

assert.Equal(t, int64(2500), stored.PrivateData.BillingContext.InputVideoDurationMS)
assert.Equal(t, string(types.CostModePerDuration), stored.PrivateData.BillingContext.UpstreamCostMode)
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./controller ./model -run 'Test.*(SubmittedTask|TaskPrivateData)' -count=1`

Expected: FAIL because the new snapshot fields do not exist.

- [ ] **Step 3: Add the minimal snapshot fields and persistence**

Add:

```go
type TaskRelayInfo struct {
    // existing fields...
    InputVideoDurationMS int64
}

type TaskBillingContext struct {
    // existing fields...
    InputVideoDurationMS int64  `json:"input_video_duration_ms,omitempty"`
    UpstreamCostMode     string `json:"upstream_cost_mode,omitempty"`
}
```

In `persistSubmittedTask`, copy `relayInfo.TaskRelayInfo.InputVideoDurationMS`. Copy `relayInfo.CostAttempt.CostMode` when the handle is present.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./controller ./model -run 'Test.*(SubmittedTask|TaskPrivateData)' -count=1`

Expected: PASS.

### Task 2: Resolve Reference Video Duration Before Dispatch

**Files:**
- Modify: `controller/relay.go`
- Test: `controller/relay_task_usage_test.go`

- [ ] **Step 1: Write failing request-preparation tests**

Cover these observable contracts with the existing injectable metadata client:

```go
// Reference-video duration is resolved once and stored on TaskRelayInfo.
assert.Equal(t, int64(2500), relayInfo.TaskRelayInfo.InputVideoDurationMS)

// Metadata failure returns before adaptor.DoRequest can dispatch upstream.
assert.False(t, upstreamCalled)
```

Test invalid media as HTTP 400 and unavailable metadata service as HTTP 503.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./controller -run TestPrepareSeedanceUsageInputs -count=1`

Expected: FAIL because task submission does not currently resolve the duration for public usage.

- [ ] **Step 3: Implement pre-dispatch resolution**

After building `RetryParam` and before channel selection/dispatch:

```go
if state := retryParam.ProfitRoutingState(); state != nil {
    metadata, metadataErr := state.Metadata(c.Request.Context())
    if metadataErr != nil {
        // Map invalid media to 400 and service unavailability to 503.
        taskErr = seedanceMetadataTaskError(metadataErr)
    } else {
        relayInfo.TaskRelayInfo.InputVideoDurationMS = metadata.TotalDurationMS
    }
}
```

Return before the retry loop when `taskErr != nil`. Do not copy URLs or detailed metadata into `RelayInfo`, tasks, logs, or diagnostics.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./controller -run TestPrepareSeedanceUsageInputs -count=1`

Expected: PASS.

### Task 3: Add Ark Usage Fallback to the Public Projection

**Files:**
- Modify: `relay/seedance_task.go`
- Test: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: Write failing public-response tests**

Add deterministic cases for:

```go
// 720p, 5 seconds, 24 fps, no input video.
assert.EqualValues(t, 108000, usage["completion_tokens"])
assert.EqualValues(t, 108000, usage["total_tokens"])

// 720p, 3 seconds input + 5 seconds output.
assert.EqualValues(t, 108000, usage["completion_tokens"])
assert.EqualValues(t, 172800, usage["total_tokens"])
```

Also assert that a valid upstream `usage` object is preserved byte-for-value; per-token upstreams with unusable usage are not silently reclassified; queued/failed tasks receive no synthetic usage; and historical reference-video tasks with no stored aggregate duration are not under-counted by guessing zero.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./relay -run 'TestSeedanceTaskResponse.*Usage' -count=1`

Expected: FAIL because the public projection currently passes missing/all-zero usage through.

- [ ] **Step 3: Implement the projection fallback**

In `seedanceTaskResponse`, after the adaptor conversion and standard public fields are set:

```go
if task.Status == model.TaskStatusSuccess && shouldCalculateSeedanceUsage(task, response) {
    facts, err := service.EstimateProfitRoutingFacts(
        resolvedResolution,
        resolvedOutputDuration,
        task.PrivateData.BillingContext.InputVideoDurationMS,
    )
    if err == nil {
        response["usage"] = map[string]interface{}{
            "completion_tokens": facts.OutputTokens,
            "total_tokens":      facts.TotalTokens,
        }
    }
}
```

Use provider response values first, then `TaskBillingContext.Resolution` and `RequestedDurationSeconds`. The local fallback is allowed only for `per_request` and `per_duration` upstream cost modes. Preserve valid upstream usage without normalization. If `HasVideoInput` is true but `InputVideoDurationMS` is absent, do not fabricate usage.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./relay -run 'TestSeedanceTaskResponse.*Usage' -count=1`

Expected: PASS.

### Task 4: Verify Cross-Layer Contracts

**Files:**
- No production changes expected

- [ ] **Step 1: Run focused task suites**

Run: `go test ./relay ./controller ./service ./model -count=1`

Expected: PASS.

- [ ] **Step 2: Run the complete backend suite**

Run: `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 3: Run formatting and diff validation**

Run: `gofmt -w controller/relay.go controller/relay_task_usage_test.go model/task.go model/task_private_data_test.go relay/common/relay_info.go relay/seedance_task.go relay/relay_task_seedance_test.go`

Run: `git diff --check`

Expected: no output and exit code 0.

- [ ] **Step 4: Inspect the final diff against the design**

Confirm the final code:

- returns only Ark's `completion_tokens` and `total_tokens`;
- preserves authoritative upstream usage;
- does not modify administrator upstream-response audit data;
- stores only aggregate input duration;
- never dispatches a reference-video task when mandatory duration metadata cannot be resolved;
- does not change unrelated frontend or billing-expression behavior.
