# Seedance Billing Acceptance Design

## Objective

Build a deterministic, cost-free acceptance suite for Seedance 2.0, 2.0 Fast,
2.0 Mini, and 1.5 Pro billing. The suite must enumerate every supported output
resolution and every integer output duration, cover requests with and without
video input, cover reference image presence, and exercise all Seedance 1.5 Pro
billing dimensions.

The suite uses a local mock ARK server. It never calls the real ARK endpoint and
does not download media or incur provider charges.

## Contract Boundaries

ARK requests carry reference video URLs but no reference-video duration field.
new-api does not download those URLs to inspect media. Therefore:

- new-api validates reference video count and records whether any video exists;
- the mock ARK server derives fixture durations from URL names such as
  `reference-6s-1.mp4`;
- the mock validates individual durations and the 15-second aggregate limit;
- final billing uses the mock response's `usage.completion_tokens` as the
  authoritative usage value;
- reference video count and duration do not become additional local billing
  multipliers.

The mock token formula is test-only. It proves that new-api consumes upstream
usage exactly; it does not claim to reproduce ARK's private token algorithm.

## Official Capability Matrix

| Family | Model ID | Resolutions | Explicit output durations | Smart duration |
| --- | --- | --- | --- | --- |
| Seedance 2.0 | `doubao-seedance-2-0-260128` | 480p, 720p, 1080p, 4k | 4 through 15 | `-1` |
| Seedance 2.0 Fast | `doubao-seedance-2-0-fast-260128` | 480p, 720p | 4 through 15 | `-1` |
| Seedance 2.0 Mini | `doubao-seedance-2-0-mini-260615` | 480p, 720p | 4 through 15 | `-1` |
| Seedance 1.5 Pro | `doubao-seedance-1-5-pro-251215` | 480p, 720p, 1080p | 4 through 12 | `-1` |

Seedance 2.0 families use `role=reference_image` for the image-present case.
Seedance 1.5 Pro does not support `reference_image`; its image-present case uses
the official `role=first_frame` mode.

## Pricing Matrix

Official unit prices are RMB per million completion tokens:

| Family | Resolution | No video input | Video input |
| --- | --- | ---: | ---: |
| Seedance 2.0 | 480p / 720p | 46 | 28 |
| Seedance 2.0 | 1080p | 51 | 31 |
| Seedance 2.0 | 4k | 26 | 16 |
| Seedance 2.0 Fast | 480p / 720p | 37 | 22 |
| Seedance 2.0 Mini | 480p / 720p | 23 | 14 |

Seedance 1.5 Pro has a base unit price of RMB 8 per million tokens:

- generated audio multiplies price by 2;
- `service_tier=flex` multiplies price by 0.5;
- Draft pre-consume uses `draft_estimate=0.6` with audio or `0.7` without audio;
- terminal settlement removes `draft_estimate` because authoritative completion
  tokens already represent the Draft output.

In new-api, `ModelRatio=1` represents RMB 14 per million tokens. Tests must
configure normalized model ratios, not raw RMB prices:

| Family | ModelRatio |
| --- | ---: |
| Seedance 2.0 | `46 / 14` |
| Seedance 2.0 Fast | `37 / 14` |
| Seedance 2.0 Mini | `23 / 14` |
| Seedance 1.5 Pro | `8 / 14` |

With group ratio 1, terminal quota is:

```text
quota = trunc(completion_tokens * ModelRatio * product(final_other_ratios))
```

For Seedance 2.0 families this simplifies to:

```text
quota = trunc(completion_tokens * official_unit_price_rmb / 14)
```

Output duration is not multiplied locally. Its billing effect is represented by
the authoritative `completion_tokens` returned by ARK.

## Enumeration

### Full Deterministic Matrix

The fast table-driven layer enumerates every valid explicit combination.

For reference videos, ordered duration tuples are generated with these rules:

- count is 1, 2, or 3;
- every duration is an integer from 2 through 15;
- aggregate duration is at most 15 seconds.

This produces 312 video profiles: 14 one-video profiles, 78 two-video profiles,
and 220 three-video profiles. A no-video profile is added separately.

For the three Seedance 2.0 families:

```text
96 model-resolution-output-duration cells
* 313 video states
* 2 reference-image states
= 60,096 combinations
```

For Seedance 1.5 Pro:

```text
non-Draft: 3 resolutions * 9 durations * 2 image states
           * 2 audio states * 2 service tiers = 216
Draft:     1 resolution * 9 durations * 2 image states
           * 2 audio states * 1 service tier = 36
total = 252 combinations
```

The explicit-duration matrix therefore contains 60,348 combinations. Smart
duration `-1`, omitted duration defaults, and invalid combinations are separate
contract cases rather than being multiplied across all media permutations.

### HTTP E2E Matrix

The HTTP layer covers independent billing dimensions without repeating every
order-only media permutation in the database:

- all 636 explicit model/resolution/output-duration/has-video/image/1.5-Pro
  pricing combinations;
- 120 smart-duration and omitted-duration cases covering every unique billing
  class;
- all 312 valid reference-video duration tuples through the mock ARK server;
- invalid reference media and model capability cases.

The suite therefore executes exactly 1,068 successful HTTP task lifecycles plus
the invalid-case matrix. Generated counts are asserted by tests and recorded in
the report.

## Mock ARK Behavior

Reference video URLs encode duration. The mock parses those URLs and rejects:

- a video shorter than 2 seconds;
- a video longer than 15 seconds;
- more than 3 videos;
- aggregate reference-video duration above 15 seconds.

For successful tasks, the mock returns a deterministic completion-token count:

```text
100000
+ effective_output_duration * 1000
+ reference_video_total_seconds * 100
+ reference_video_count * 10
+ reference_image_present
```

For `duration=-1`, the mock chooses a valid terminal duration deterministically
and returns it in the task response. The formula only makes every fixture
traceable and ensures settlement uses the exact upstream number.

## Assertions

Every successful HTTP billing case verifies:

- request fields and model mapping received by mock ARK;
- pre-consume quota and pre-consume `OtherRatios`;
- persisted `TaskBillingContext` request facts;
- authoritative completion tokens and terminal resolution;
- final unit-price multiplier and exact final quota;
- positive or negative settlement delta;
- user available quota and used quota conservation;
- channel and token used quota;
- `quota_data.Quota`, `quota_data.TokenUsed`, and stable request count;
- consume/settlement log billing metadata.

Paired cases additionally prove:

- reference image presence does not change the unit-price tier;
- video count and duration do not create local multipliers beyond `hasVideo`;
- `completion_tokens` is preferred over a deliberately different
  `total_tokens`;
- terminal resolution corrects the submitted estimate;
- Draft settlement removes `draft_estimate`.

Every failed case verifies the ARK error envelope, absence of a created task when
submission fails, and complete refund of any request-level pre-consume.

## Invalid Matrix

The rejection suite includes:

- unsupported resolution for each model family;
- output duration below and above each model range, zero, and invalid negative;
- reference video count above 3;
- individual and aggregate reference-video duration violations at mock ARK;
- `reference_image`, reference video, and reference audio on Seedance 1.5 Pro;
- `service_tier=flex` on Seedance 2.0 families;
- Draft on Seedance 2.0 families;
- Seedance 1.5 Pro Draft with non-480p resolution or flex tier;
- malformed `generate_audio`, `draft`, and media roles.

## Test Structure

| File | Responsibility |
| --- | --- |
| `relay/channel/task/doubao/billing_acceptance_test.go` | 60,348-case capability and price enumeration |
| `e2e/seedance_billing_matrix_e2e_test.go` | Mock ARK, HTTP lifecycles, settlement and accounting assertions |
| `docs/superpowers/acceptance/2026-07-18-seedance-billing-acceptance.md` | Human-readable acceptance checklist |
| `docs/superpowers/reports/2026-07-18-seedance-billing-e2e-acceptance.md` | Detailed matrix counts, formulas, samples, and command evidence |

Tests run sequentially because they mutate global ratio settings, task polling
hooks, and in-memory database handles. No arbitrary sleeps are used.

## Acceptance Commands

```text
go test ./relay/channel/task/doubao -run SeedanceBillingAcceptance -count=1 -v
go test ./e2e -run SeedanceBilling -count=1 -v
go test ./relay/channel/task/doubao ./service ./e2e -count=1
go test ./...
go vet ./relay/channel/task/doubao ./service ./e2e
git diff --check
```

## Reporting

The Markdown report records:

- generated case counts by model and dimension;
- every official unit-price row and internal ratio conversion;
- representative client request, mock upstream request and terminal response;
- pre-consume, final quota, delta, and account balances;
- invalid-case responses and refund facts;
- command output and residual limitations.

The report does not check in tens of thousands of repetitive rows. Every row is
reproducible from deterministic test generators and includes a case ID in any
assertion failure.
