# Seedance Billing Acceptance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a deterministic, cost-free acceptance suite that exhaustively validates Seedance 2.0, 2.0 Fast, 2.0 Mini, and 1.5 Pro pricing, settlement, accounting, and invalid combinations.

**Architecture:** A fast adaptor-level matrix enumerates all 60,348 explicit request combinations. A local mock ARK server and SQLite-backed HTTP suite execute exactly 1,068 successful task lifecycles plus invalid cases, using duration-labelled media URLs and authoritative mock `completion_tokens`. The suite verifies request conversion, pre-consume, terminal settlement, refunds, and every accounting surface without calling real ARK.

**Tech Stack:** Go 1.22+, Gin, GORM v2, SQLite in-memory, `httptest.Server`, testify `require`/`assert`, project quota helpers.

**Design:** `docs/superpowers/specs/2026-07-18-seedance-billing-acceptance-design.md`

**Billing rule:** `pkg/billingexpr/expr.md` has been reviewed. This acceptance suite intentionally exercises the existing adaptor `OtherRatios` path because terminal resolution and authoritative completion tokens arrive after asynchronous polling.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `relay/channel/task/doubao/billing_acceptance_test.go` | Model specs, 312 reference-video profiles, 60,348-case price/capability enumeration |
| `e2e/seedance_billing_matrix_e2e_test.go` | Duration-aware mock ARK, HTTP task lifecycles, exact quota/accounting assertions |
| `docs/superpowers/acceptance/2026-07-18-seedance-billing-acceptance.md` | Acceptance checklist |
| `docs/superpowers/reports/2026-07-18-seedance-billing-e2e-acceptance.md` | Detailed results, formulas, request/response samples, command evidence |

Production files are not changed unless an acceptance test exposes a verified defect. Any such defect must follow `systematic-debugging` and `test-driven-development`: retain the failing contract test, identify the root cause, then apply the smallest production fix.

Do not modify or stage the unrelated dirty file `docs/superpowers/plans/2026-07-18-dimensio-translator.md`.

---

### Task 1: Deterministic Model and Media Generators

**Files:**
- Create: `relay/channel/task/doubao/billing_acceptance_test.go`

- [ ] **Step 1: Add failing generator-count tests**

Create the test file with these contracts:

```go
package doubao

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSeedanceBillingAcceptanceGeneratorCounts(t *testing.T) {
    profiles := seedanceAcceptanceVideoProfiles()
    require.Len(t, profiles, 312)

    counts := map[int]int{}
    for _, profile := range profiles {
        counts[len(profile.Durations)]++
    }
    assert.Equal(t, map[int]int{1: 14, 2: 78, 3: 220}, counts)
    assert.Equal(t, 60348, seedanceAcceptanceExplicitCaseCount())
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```text
go test ./relay/channel/task/doubao -run TestSeedanceBillingAcceptanceGeneratorCounts -count=1
```

Expected: build failure because the test-only generator symbols do not exist.

- [ ] **Step 3: Implement model specifications and ordered video profiles**

Add test-only types and generators:

```go
type seedanceAcceptanceModel struct {
    ID            string
    Resolutions   []string
    Durations     []int
    BaseRMB       float64
    SupportsVideo bool
    ImageRole     string
}

type seedanceAcceptanceVideoProfile struct {
    Durations []int
}

func seedanceAcceptanceModels() []seedanceAcceptanceModel {
    return []seedanceAcceptanceModel{
        {"doubao-seedance-2-0-260128", []string{"480p", "720p", "1080p", "4k"}, integerRange(4, 15), 46, true, "reference_image"},
        {"doubao-seedance-2-0-fast-260128", []string{"480p", "720p"}, integerRange(4, 15), 37, true, "reference_image"},
        {"doubao-seedance-2-0-mini-260615", []string{"480p", "720p"}, integerRange(4, 15), 23, true, "reference_image"},
        {"doubao-seedance-1-5-pro-251215", []string{"480p", "720p", "1080p"}, integerRange(4, 12), 8, false, "first_frame"},
    }
}

func integerRange(first, last int) []int {
    values := make([]int, 0, last-first+1)
    for value := first; value <= last; value++ {
        values = append(values, value)
    }
    return values
}

func seedanceAcceptanceVideoProfiles() []seedanceAcceptanceVideoProfile {
    profiles := make([]seedanceAcceptanceVideoProfile, 0, 312)
    var build func([]int, int)
    build = func(prefix []int, remaining int) {
        if len(prefix) > 0 {
            durations := append([]int(nil), prefix...)
            profiles = append(profiles, seedanceAcceptanceVideoProfile{Durations: durations})
        }
        if len(prefix) == 3 {
            return
        }
        for duration := 2; duration <= 15 && duration <= remaining; duration++ {
            build(append(prefix, duration), remaining-duration)
        }
    }
    build(nil, 15)
    return profiles
}

func seedanceAcceptanceExplicitCaseCount() int {
    profilesWithNoVideo := len(seedanceAcceptanceVideoProfiles()) + 1
    seedance20Cells := (4*12 + 2*12 + 2*12) * profilesWithNoVideo * 2
    seedance15Cells := 3*9*2*2*2 + 1*9*2*2
    return seedance20Cells + seedance15Cells
}
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run the Step 2 command again. Expected: PASS with 312 profiles and 60,348 explicit cases.

---

### Task 2: Exhaustive 60,348-Case Pricing Contract

**Files:**
- Modify: `relay/channel/task/doubao/billing_acceptance_test.go`

- [ ] **Step 1: Add the failing full-matrix test**

Add `TestSeedanceBillingAcceptanceExplicitMatrix`. It must:

1. Iterate all 96 Seedance 2.0 model/resolution/output-duration cells.
2. Cross each cell with no-video plus all 312 video profiles.
3. Cross each result with reference image absent/present.
4. Build `seedanceNativeRequest` and `seedanceContentFacts` using the real package types.
5. Call `validateSeedanceNativeFields` and `GetVideoBillingRatio`.
6. Assert the actual unit-price ratio and a stable case ID in every failure message.
7. Enumerate all 252 valid Seedance 1.5 Pro combinations and call `GetSeedance15ProRatios`.
8. Assert the final executed count is exactly 60,348.

Use this official-price helper inside the test file:

```go
func seedanceAcceptanceUnitPrice(model, resolution string, hasVideo bool) float64 {
    family := seedancePricingFamily(model)
    key := videoPriceKey{
        is1080p:  strings.EqualFold(resolution, "1080p"),
        is4k:     strings.EqualFold(resolution, "4k"),
        hasVideo: hasVideo,
    }
    return videoPriceTable[family][key]
}
```

For 1.5 Pro, enumerate only valid combinations:

```go
for _, resolution := range []string{"480p", "720p", "1080p"} {
    for duration := 4; duration <= 12; duration++ {
        for _, image := range []bool{false, true} {
            for _, audio := range []bool{false, true} {
                for _, tier := range []string{"default", "flex"} {
                    // non-Draft
                }
            }
        }
    }
}
for duration := 4; duration <= 12; duration++ {
    for _, image := range []bool{false, true} {
        for _, audio := range []bool{false, true} {
            // Draft: 480p + default only
        }
    }
}
```

- [ ] **Step 2: Run the matrix test and verify its first failure**

Run:

```text
go test ./relay/channel/task/doubao -run TestSeedanceBillingAcceptanceExplicitMatrix -count=1 -v
```

Expected before completing the assertions: FAIL with a case ID naming the missing or mismatched contract.

- [ ] **Step 3: Complete exact ratio assertions**

For Seedance 2.0 families assert:

```go
ratio, ok := GetVideoBillingRatio(model.ID, resolution, hasVideo)
require.True(t, ok, caseID)
wantRatio := seedanceAcceptanceUnitPrice(model.ID, resolution, hasVideo) / model.BaseRMB
assert.InDelta(t, wantRatio, ratio, 1e-12, caseID)
```

For 1.5 Pro assert maps exactly:

```go
want := map[string]float64{}
if audio {
    want["audio"] = 2
}
if tier == "flex" {
    want["service_tier"] = 0.5
}
if draft {
    if audio {
        want["draft_estimate"] = 0.6
    } else {
        want["draft_estimate"] = 0.7
    }
}
assert.Equal(t, want, got, caseID)
```

- [ ] **Step 4: Run the matrix and existing adaptor tests**

Run:

```text
go test ./relay/channel/task/doubao -run 'SeedanceBillingAcceptance|SeedancePricing|EstimateBilling|AdjustBilling' -count=1
```

Expected: PASS.

---

### Task 3: Duration-Aware Mock ARK

**Files:**
- Create: `e2e/seedance_billing_matrix_e2e_test.go`
- Reuse test fixtures from: `e2e/seedance_native_e2e_test.go`

- [ ] **Step 1: Add failing URL-duration parser tests**

Create table tests for URLs such as:

```text
https://mock.example/reference-2s-1.mp4  -> 2
https://mock.example/reference-15s-3.mp4 -> 15
https://mock.example/reference.mp4       -> error
https://mock.example/reference-0s.mp4     -> error
```

Test name: `TestSeedanceBillingMockReferenceDuration`.

- [ ] **Step 2: Run parser test and verify RED**

Run:

```text
go test ./e2e -run TestSeedanceBillingMockReferenceDuration -count=1
```

Expected: build failure because `seedanceBillingReferenceDuration` is undefined.

- [ ] **Step 3: Implement structured mock case types and parser**

Use test-only types:

```go
type seedanceBillingCase struct {
    ID                 string
    Model              string
    Resolution         string
    RequestDuration    int
    TerminalDuration   int
    VideoDurations     []int
    HasReferenceImage  bool
    GenerateAudio      bool
    ServiceTier        string
    Draft              bool
    CompletionTokens   int
    ExpectedUnitRMB    float64
}

type seedanceBillingMock struct {
    mu      sync.Mutex
    nextID  int
    tasks   map[string]seedanceBillingCase
    requests []mockArkRequest
}
```

Parse only the final URL path segment with a compiled regular expression:

```go
var seedanceBillingDurationPattern = regexp.MustCompile(`(?:^|-)reference-(\d+)s(?:-|\.)`)
```

Return an error unless the parsed duration is 2 through 15.

- [ ] **Step 4: Implement mock submission validation**

On `POST /api/v3/contents/generations/tasks`:

1. Decode with `common.DecodeJson` into `doubao`-compatible local test structs or a structured map.
2. Read every `video_url.url` duration.
3. Reject more than 3 videos with ARK `InvalidParameter.content`.
4. Reject aggregate duration over 15 with ARK `InvalidParameter.content`.
5. Calculate deterministic completion tokens:

```go
completionTokens := 100000 + effectiveDuration*1000 + videoTotal*100 + len(videoDurations)*10
if hasReferenceImage {
    completionTokens++
}
```

6. Save the case under a unique upstream `cgt-billing-*` ID.
7. Return `{"id":"cgt-billing-*"}`.

On `GET`, return a succeeded ARK object containing model, resolution, duration,
`usage.completion_tokens`, a deliberately different `usage.total_tokens`, and a
mock video URL.

- [ ] **Step 5: Run parser and mock handler tests**

Run:

```text
go test ./e2e -run 'TestSeedanceBillingMock' -count=1
```

Expected: PASS.

---

### Task 4: E2E Billing Environment and Exact Formula Helpers

**Files:**
- Modify: `e2e/seedance_billing_matrix_e2e_test.go`

- [ ] **Step 1: Add a failing model-ratio normalization test**

Add this test-only configuration function:

```go
func seedanceBillingModelRatios() map[string]float64 {
    return map[string]float64{
        "doubao-seedance-2-0-260128":      46.0 / 14.0,
        "doubao-seedance-2-0-fast-260128": 37.0 / 14.0,
        "doubao-seedance-2-0-mini-260615": 23.0 / 14.0,
        "doubao-seedance-1-5-pro-251215":  8.0 / 14.0,
    }
}
```

Test name: `TestSeedanceBillingModelRatioNormalization`. Assert:

```go
assert.InDelta(t, 46.0/14.0, ratios["doubao-seedance-2-0-260128"], 1e-12)
assert.InDelta(t, 37.0/14.0, ratios["doubao-seedance-2-0-fast-260128"], 1e-12)
assert.InDelta(t, 23.0/14.0, ratios["doubao-seedance-2-0-mini-260615"], 1e-12)
assert.InDelta(t, 8.0/14.0, ratios["doubao-seedance-1-5-pro-251215"], 1e-12)
```

- [ ] **Step 2: Implement the test environment**

Add `setupSeedanceBillingE2E` that:

- calls the existing SQLite setup pattern without `t.Parallel()`;
- seeds one high-quota user and token;
- seeds one Doubao Video channel supporting all four model IDs;
- inserts an `Ability` row for each model;
- installs all four normalized model ratios and restores the original ratio map in `t.Cleanup`;
- sets retry count to zero and installs the real task polling adaptor;
- enables quota-data collection;
- starts `httptest.NewServer(seedanceBillingMock)`.

- [ ] **Step 3: Add exact quota helpers**

Use production quota conversion rather than bare casts:

```go
func seedanceBillingExpectedQuota(tokens int, modelRatio, finalMultiplier float64) int {
    return common.QuotaFromFloat(float64(tokens) * modelRatio * finalMultiplier)
}

func seedanceBillingExpectedPreConsume(modelRatio, estimatedMultiplier float64) int {
    base := modelRatio / 2 * common.QuotaPerUnit
    return common.QuotaFromFloat(base * estimatedMultiplier)
}
```

Create a stable domain assertion helper that reads the task, user, channel,
token, quota-data totals, and logs for one case. It must accept before/after
snapshots so cumulative execution remains deterministic.

- [ ] **Step 4: Run environment tests**

Run:

```text
go test ./e2e -run 'TestSeedanceBillingModelRatioNormalization|TestSeedanceBillingEnvironment' -count=1
```

Expected: PASS.

---

### Task 5: 636 Explicit HTTP Settlement Cases

**Files:**
- Modify: `e2e/seedance_billing_matrix_e2e_test.go`

- [ ] **Step 1: Add a failing generated-case count test**

Generate HTTP pricing cases using only the `hasVideo` boolean equivalence class,
not all 312 media tuples:

- Seedance 2.0 families: 384 cases.
- Seedance 1.5 Pro: 252 cases.
- Total: 636 cases.

Assert `require.Len(t, cases, 636)`.

- [ ] **Step 2: Run and verify RED**

Run:

```text
go test ./e2e -run TestSeedanceBillingExplicitCaseCount -count=1
```

Expected: build failure until the E2E case generator exists.

- [ ] **Step 3: Implement request construction**

Build ARK Native JSON with structured maps and `common.Marshal`. Content rules:

- always include a text item;
- for Seedance 2.0 image cases add one `reference_image`;
- for Seedance 1.5 Pro image cases add one `first_frame`;
- for video cases add one `reference-2s-1.mp4` with `reference_video` role;
- include explicit resolution and output duration;
- include 1.5 Pro `generate_audio`, `service_tier`, and `draft` values.

Do not assemble JSON through string concatenation.

- [ ] **Step 4: Implement sequential HTTP execution**

For every generated case:

1. Snapshot user/channel/token/quota-data totals.
2. POST `/api/v3/contents/generations/tasks` with the real router.
3. Assert official create response and public `task_*` ID.
4. Load the persisted task and assert pre-consume quota and BillingContext.
5. Run one real task-polling cycle.
6. GET the public task and assert authoritative usage and terminal fields.
7. Calculate expected final multiplier and quota.
8. Assert task quota, settlement delta, all account deltas, quota-data token delta,
   stable request count, and billing log fields.

For 2.0 prices use `expectedUnitRMB / baseRMB`. For 1.5 Pro final multiplier:

```go
multiplier := 1.0
if generateAudio {
    multiplier *= 2
}
if serviceTier == "flex" {
    multiplier *= 0.5
}
```

Draft estimate must be present in pre-consume and absent from terminal billing.

- [ ] **Step 5: Run the 636-case E2E test**

Run:

```text
go test ./e2e -run TestSeedanceBillingExplicitMatrixE2E -count=1 -v
```

Expected: PASS and log `explicit_cases=636`.

---

### Task 6: Smart and Omitted Duration Cases

**Files:**
- Modify: `e2e/seedance_billing_matrix_e2e_test.go`

- [ ] **Step 1: Add 120 generated cases**

For each unique billing class, generate one omitted-duration case and one
`duration=-1` case:

- Seedance 2.0 families: 32 unique classes per duration mode, or 64 across the
  two modes.
- Seedance 1.5 Pro: 28 unique classes per duration mode, or 56 across the two
  modes.
- Total: `64 + 56 = 120`.

The mock uses duration 5 for omitted requests and chooses a deterministic valid
terminal duration for `-1`.

- [ ] **Step 2: Assert smart-duration terminal facts**

Verify:

- `-1` is forwarded without becoming a negative ratio;
- the task response contains the mock-selected terminal duration;
- final quota uses completion tokens, not request duration;
- omitted duration uses ARK default 5 in the mock response;
- account and quota-data deltas remain exact.

- [ ] **Step 3: Run the duration-mode E2E test**

Run:

```text
go test ./e2e -run TestSeedanceBillingDurationModesE2E -count=1 -v
```

Expected: PASS and log `duration_mode_cases=120`.

---

### Task 7: All 312 Reference-Video Duration Profiles

**Files:**
- Modify: `e2e/seedance_billing_matrix_e2e_test.go`

- [ ] **Step 1: Generate duration-labelled media requests**

Reuse the same ordered profile algorithm in the E2E package. For each profile,
build URLs `reference-<seconds>s-<index>.mp4`. Use Seedance 2.0, 720p, output
duration 5, and alternate reference-image presence by case index.

- [ ] **Step 2: Execute every profile through HTTP and polling**

Assert for every profile:

- mock ARK receives the same ordered duration list;
- BillingContext only records `HasVideoInput=true` and does not add count or
  input-duration multipliers;
- the `video_input` multiplier remains `28/46`;
- completion tokens exactly match the mock formula;
- final quota and account deltas match the formula.

- [ ] **Step 3: Run the media-profile E2E test**

Run:

```text
go test ./e2e -run TestSeedanceBillingReferenceVideoProfilesE2E -count=1 -v
```

Expected: PASS and log `reference_video_profiles=312`.

---

### Task 8: Invalid Combinations and Refunds

**Files:**
- Modify: `relay/channel/task/doubao/billing_acceptance_test.go`
- Modify: `e2e/seedance_billing_matrix_e2e_test.go`

- [ ] **Step 1: Add local validation table**

Cover these exact invalid contracts:

- 2.0 Fast/Mini with 1080p or 4k;
- all families with `duration=0`, lower-bound minus one, upper-bound plus one,
  and `duration=-2`;
- 4 reference videos;
- Seedance 1.5 Pro with `reference_image`, reference video, or reference audio;
- Seedance 2.0 families with flex tier or Draft;
- Seedance 1.5 Pro Draft with 720p, 1080p, or flex tier;
- malformed boolean controls and incorrect media roles.

Assert ARK-compatible HTTP 400 envelopes and zero mock requests for local
validation failures.

- [ ] **Step 2: Add mock-upstream duration rejection cases**

Use duration-labelled URLs for:

- one 1-second video;
- one 16-second video;
- two videos totalling 16 seconds;
- three videos totalling 16 seconds.

The local gateway forwards these because duration is not part of the protocol
body. Mock ARK returns HTTP 400 `InvalidParameter.content`.

- [ ] **Step 3: Assert request-level refunds**

For every mock-upstream rejection assert:

- no task row is created;
- user available quota, user used quota, token used quota, channel used quota,
  request count, and quota-data totals equal their pre-request snapshots;
- the client receives the mock ARK error code/message without internal details.

- [ ] **Step 4: Run invalid and refund tests**

Run:

```text
go test ./relay/channel/task/doubao ./e2e -run 'SeedanceBilling.*Invalid|SeedanceBilling.*Refund' -count=1 -v
```

Expected: PASS.

---

### Task 9: Acceptance Checklist and Detailed Report

**Files:**
- Create: `docs/superpowers/acceptance/2026-07-18-seedance-billing-acceptance.md`
- Create: `docs/superpowers/reports/2026-07-18-seedance-billing-e2e-acceptance.md`

- [ ] **Step 1: Write the acceptance checklist**

Include checkbox sections for:

- all four models;
- supported and unsupported resolutions;
- every explicit output duration plus `-1` and omitted duration;
- no-video and all 312 reference-video profiles;
- reference image absent/present with model-correct role;
- 1.5 Pro audio/tier/Draft matrix;
- completion-token authority;
- pre-consume and settlement formula;
- user/channel/token/quota-data conservation;
- local validation, upstream rejection, and refund.

- [ ] **Step 2: Write the detailed report from observed outputs**

The report must contain:

- exact generated counts: 60,348, 636, 120, 312, and invalid-case count;
- official RMB prices and normalized `ModelRatio` values;
- mock token formula with an explicit non-official disclaimer;
- representative requests and responses for each model family;
- conversion before/after and BillingContext snapshots;
- pre-consume, final quota, delta, and balance tables;
- invalid response bodies and refund balances;
- actual commands, durations, and pass/fail results;
- residual risks: mock upstream, SQLite only, no media download, no verification of
  ARK's private token algorithm.

- [ ] **Step 3: Validate every JSON code block**

Use PowerShell `ConvertFrom-Json` over fenced `json` blocks, matching the prior
ARK Native acceptance report validation. Expected: every block parses.

---

### Task 10: Full Verification

- [ ] **Step 1: Run focused exhaustive tests**

```text
go test ./relay/channel/task/doubao -run SeedanceBillingAcceptance -count=1 -v
go test ./e2e -run SeedanceBilling -count=1 -v
```

Expected: PASS with the exact generated counts in test logs.

- [ ] **Step 2: Run related package regression**

```text
go test ./relay/channel/task/doubao ./service ./e2e -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full backend regression**

```text
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Run static checks**

```text
go vet ./relay/channel/task/doubao ./service ./e2e
git diff --check
```

Expected: exit code 0 for both commands.

- [ ] **Step 5: Reconcile report evidence**

Compare the report's case counts, formula examples, command results, and residual
risks against the fresh command output. Do not claim completion if any generated
count differs or any test is skipped.

No commit is created unless the user explicitly requests one. When staging is
requested, use explicit paths and exclude the unrelated dirty Dimensio plan.
