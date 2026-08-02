# Video Metadata Load Test Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and execute a reproducible load-test program that finds the safe single-instance capacity of `video-metadata`, validates normal traffic at 60% of that capacity, and separately measures behavior against controlled and authorized public video origins.

**Architecture:** Use a dedicated pre-production environment whose service resources match production or are recorded as an explicit scale factor. A `k6` load generator calls `POST /v1/metadata/video` directly, while service logs, container metrics, load-generator metrics, and origin metrics are collected on one synchronized timeline. Controlled same-region object storage determines capacity; authorized public origins provide a separate end-to-end result and never influence the capacity number.

**Tech Stack:** Go video metadata service, Docker/Kubernetes, k6, HTTP/JSON, object storage, Prometheus-compatible container and node metrics, structured service logs.

---

## Fixed Decisions

- Environment: dedicated pre-production, isolated from users and production billing.
- Goals: discover the single-instance capacity boundary and verify the derived business load.
- Unknown demand: define business load as 60% of measured safe mixed-cache capacity until production telemetry supplies a better target.
- Origins: controlled same-region object storage for capacity; authorized public origins for supplemental end-to-end testing.
- Cache modes: cold, hot, and a 70% hot / 30% cold mixed workload, each reported independently.
- Capacity discovery: `concurrency_limited` is allowed as a boundary signal, but every request counts toward success rate when declaring a safe plateau or running business acceptance.
- Timing: each complete capacity staircase lasts 2 hours; the final mixed workload soak lasts 8 hours.
- Primary SLOs: cold P95 <= 10 seconds, hot P95 <= 500 ms, success rate >= 99.5%, CPU below 75%, no sustained memory or temporary-disk growth, and no unexpected 5xx responses.
- Third-party safety: do not load-test a URL unless the owner has authorized the expected traffic. The Ark sample URL used in functional acceptance may be used for a low-rate smoke check only.

## Capacity Definitions

For each cache mode, `C_safe` is the highest complete 10-minute plateau that satisfies all of these conditions:

1. HTTP 200 success rate is at least 99.5%. All 400, 401, 413, and 5xx responses count as failures. During discovery, `concurrency_limited` identifies the boundary, but a plateau containing enough of those responses to miss 99.5% is not safe.
2. Cold requests have P95 <= 10 seconds; hot requests have P95 <= 500 ms. In the mixed test, evaluate the tagged hot and cold subsets separately.
3. One-minute service CPU averages stay below 75%; no CPU value remains above 90% for 30 seconds.
4. The service is not restarted, OOM-killed, or marked unhealthy.
5. Resident memory at the end of the plateau is no more than 5% above its start after a two-minute drain. During the 8-hour soak, RSS growth is below 1% per hour and below 10% total.
6. Temporary filesystem usage returns to within 5% of its pre-plateau baseline within two minutes, and no `video-metadata-*` files remain after drain.
7. There are no `internal_error` responses. Origin-related failures (`fetch_unavailable`, `deadline_exceeded`) are counted as failures and must be correlated with origin metrics before attributing them to the service.
8. The load generator remains below 60% CPU and 70% of available network throughput; otherwise the result is invalid and must be rerun with more generators.

Derived values:

```text
normal_business_qps = floor(0.60 * C_safe_mixed)
cold_contingency_qps = floor(0.60 * C_safe_cold)
recommended_instance_concurrency = the tested VIDEO_METADATA_MAX_CONCURRENCY value
                                   with the highest C_safe_mixed that still meets every SLO
```

If a staircase never produces an unsafe plateau, report only `C_safe >= highest_tested_rate`; do not claim a maximum. Run another staircase with doubled rates.

## Abort Conditions

Stop new traffic immediately, preserve evidence, and allow in-flight requests up to 30 seconds to drain when any condition occurs:

- health check fails three times consecutively;
- service restarts or is OOM-killed;
- CPU remains above 90% for two minutes;
- memory or temporary filesystem exceeds 85% of its configured limit;
- `internal_error` exceeds 0.1% in any one-minute window;
- all non-`concurrency_limited` 5xx responses exceed 0.5% in any one-minute window;
- controlled origin 5xx exceeds 0.5% or origin egress exceeds 70% of provisioned capacity;
- load generator CPU exceeds 80% or its network exceeds 80% for two minutes.

After an abort, do not restart the service until logs, container events, `/tmp` state, and the final resource samples have been captured.

## Test Matrix

| ID | Purpose | Cache | Origin | Duration | Offered load |
|---|---|---|---|---:|---|
| S0 | Contract smoke | enabled | controlled + Ark sample | 5 min | 1 request every 10 seconds |
| C1 | Cold capacity | disabled | controlled | 120 min | 12 x 10-minute plateaus |
| H1 | Hot capacity | enabled and pre-warmed | controlled | 120 min | 12 x 10-minute plateaus |
| M1 | Mixed capacity | enabled | controlled | 120 min | 70% hot / 30% unique cold URLs |
| T1 | Concurrency tuning | cold + mixed | controlled | 40 min per candidate | 80%, 100%, 120%, 100% of baseline boundary |
| B1 | Business acceptance | mixed | controlled | 30 min | `normal_business_qps` |
| R1 | Recovery burst | mixed | controlled | 12 min | 2 min at 150%, then 10 min at 60% of `C_safe_mixed` |
| P1 | Public E2E | cold + mixed | authorized public | 30 min | 10%, 30%, 60% of `C_safe_mixed`, capped by owner authorization |
| L1 | Stability soak | mixed | controlled | 8 hours | `normal_business_qps` |

Suggested first-pass staircase rates are deliberately conservative and may be doubled only when no boundary is found:

```text
cold:  1, 2, 3, 4, 5, 6, 8, 10, 12, 16, 20, 24 requests/second
hot:   5, 10, 20, 40, 80, 120, 160, 240, 320, 480, 640, 800 requests/second
mixed: 2, 4, 6, 8, 10, 12, 16, 20, 24, 32, 48, 64 requests/second
```

Run `C1`, `H1`, and `M1` with the production candidate default `VIDEO_METADATA_MAX_CONCURRENCY=16`. For `T1`, test `8`, `16`, and `32` with identical CPU, memory, filesystem, origin, and load-generator resources. A candidate is rejected if it improves request rate by consuming unsafe memory, disk, or origin bandwidth.

## Controlled Corpus

Create public-read test objects in a same-region bucket dedicated to load testing. Arbitrary query parameters must not change the object body, ETag, Last-Modified, or Content-Length; verify this before relying on unique query strings for cold requests.

| Weight | Container | Resolution | Duration | Target size | Purpose |
|---:|---|---|---:|---:|---|
| 40% | MP4 | 1280x720 | 5 s | 3-5 MiB | Common short input |
| 25% | MP4 | 1920x1080 | 15 s | 20-30 MiB | Medium input |
| 15% | MOV | 1280x720 | 10 s | 8-15 MiB | Supported alternate container |
| 15% | MP4 | 1920x1080 | 60 s | 80-100 MiB | Large input and disk pressure |
| 5% | MP4 | 1920x1080 | 60-90 s | 120-127 MiB | Near the 128 MiB service limit |

Every asset must have a known expected duration, dimensions, frame rate, container, content length, ETag, and Last-Modified value. Keep at least 100 stable URLs for hot traffic. Cold traffic uses the same controlled objects with unique `load_run` and `request` query values; the manifest preflight must prove those variants return identical validators.

Public-origin testing uses at least three separately owned or authorized origins: one same-country CDN, one cross-region CDN, and one ordinary HTTPS object host. Keep their results separate by origin. Do not place secrets, signed query strings, Authorization headers, or complete URLs in k6 summaries, service logs, or reports.

## Metrics and Evidence

Use synchronized UTC time on service, load generators, origin, and monitoring nodes. Record:

- k6: offered rate, completed rate, active VUs, dropped iterations, HTTP status, error code, P50/P95/P99 latency, connect/TLS/wait/receive timings, and tagged hot/cold/origin metrics;
- service logs: `request_id`, `result_code`, `elapsed_ms`, `bytes`, and `cache_hit` only;
- service container: CPU, RSS/working set, filesystem used/inodes, network RX/TX, open file descriptors, health, restarts, and OOM events;
- host: CPU steal, disk latency, filesystem capacity, and network saturation;
- controlled origin: HEAD/GET rate, 2xx/4xx/5xx, P95 TTFB, bytes served, and egress utilization;
- load generator: CPU, memory, network throughput, socket errors, and clock drift.

Do not enable application profiling during the first capacity run because it can change the result. If a bottleneck is repeatable, run a separate diagnostic reproduction with profiling and label that run non-comparable.

---

### Task 1: Freeze the Environment and Run Contract Smoke

**Files:**
- Create: `tests/video-metadata-load/README.md`
- Create: `tests/video-metadata-load/results/.gitkeep`
- Modify: `docs/deployment/video-metadata-service.md`

- [ ] **Step 1: Record the immutable run inventory**

Add a checklist to `tests/video-metadata-load/README.md` requiring the service image digest, Git commit, instance count, CPU and memory limits, temporary filesystem size, node type, region, kernel, `VIDEO_METADATA_MAX_*`, cache settings, origin bucket, k6 version, load-generator shape, and NTP status. State that a changed field creates a new comparison series.

- [ ] **Step 2: Pin the load generator**

Use the same pinned k6 image for every run:

```bash
docker pull grafana/k6:0.57.0
docker image inspect grafana/k6:0.57.0 --format '{{index .RepoDigests 0}}'
```

Expected: the second command prints one immutable digest; paste it into the run inventory.

- [ ] **Step 3: Verify service and origin prerequisites**

Run from the load-generator network:

```bash
curl --fail --silent --show-error "$VIDEO_METADATA_BASE_URL/healthz"
curl --fail --silent --show-error --head "$CONTROLLED_SMOKE_URL"
```

Expected: health returns `{"status":"ok"}`; the asset returns HTTP 200 with positive Content-Length, ETag, and Last-Modified headers.

- [ ] **Step 4: Verify the known Ark sample at smoke rate only**

Submit the URL `https://ark-project.tos-cn-beijing.volces.com/doc_video/r2v_tea_video1.mp4` once and assert:

```json
{
  "duration_ms": 5042,
  "width": 1280,
  "height": 720,
  "frame_rate_num": 24,
  "frame_rate_den": 1,
  "container": "mp4",
  "content_length": 3487655
}
```

Expected: HTTP 200 and the listed fields match. Do not use this third-party URL in `C1`, `H1`, `M1`, `T1`, `B1`, `R1`, or `L1`.

- [ ] **Step 5: Commit the operator guide**

```bash
git add tests/video-metadata-load/README.md tests/video-metadata-load/results/.gitkeep docs/deployment/video-metadata-service.md
git commit -m "docs: add video metadata load test prerequisites"
```

### Task 2: Build and Validate the Test Corpus Manifest

**Files:**
- Create: `tests/video-metadata-load/config/controlled.example.json`
- Create: `tests/video-metadata-load/config/public.example.json`
- Create: `tests/video-metadata-load/scripts/preflight.ps1`

- [ ] **Step 1: Define the controlled manifest contract**

Use this shape, with real deployment values kept in an untracked copy named `controlled.local.json`:

```json
{
  "assets": [
    {
      "id": "mp4-720p-5s-a",
      "url_env": "CONTROLLED_URL_MP4_720P_5S_A",
      "weight": 40,
      "duration_ms": 5000,
      "width": 1280,
      "height": 720,
      "frame_rate_num": 24,
      "frame_rate_den": 1,
      "container": "mp4",
      "min_bytes": 3145728,
      "max_bytes": 5242880
    }
  ]
}
```

Include all five corpus rows and make weights total exactly 100. The checked-in example contains environment-variable names, never signed URLs.

- [ ] **Step 2: Define the authorized public manifest contract**

Use this shape and keep actual URLs in `public.local.json`, which must be ignored by Git:

```json
{
  "origins": [
    {"id":"same-country-cdn","url_env":"PUBLIC_URL_SAME_COUNTRY","authorized_max_rps":10},
    {"id":"cross-region-cdn","url_env":"PUBLIC_URL_CROSS_REGION","authorized_max_rps":5},
    {"id":"ordinary-object-host","url_env":"PUBLIC_URL_OBJECT_HOST","authorized_max_rps":2}
  ]
}
```

- [ ] **Step 3: Implement preflight validation**

`preflight.ps1` must fail unless every URL is HTTPS, HEAD returns 2xx, Content-Length is positive and within the manifest bounds, validators are present, a query variant returns the same validators, and one metadata POST matches the expected fields. It must print only asset IDs and status, never URLs or tokens.

- [ ] **Step 4: Run preflight**

```powershell
pwsh tests/video-metadata-load/scripts/preflight.ps1 `
  -Manifest tests/video-metadata-load/config/controlled.local.json `
  -BaseUrl $env:VIDEO_METADATA_BASE_URL
```

Expected: one `PASS` line per asset and exit code 0. Any mismatch blocks all capacity testing.

- [ ] **Step 5: Commit the manifest contract and preflight**

```bash
git add tests/video-metadata-load/config tests/video-metadata-load/scripts/preflight.ps1 .gitignore
git commit -m "test: add video metadata load corpus preflight"
```

### Task 3: Implement the k6 Workload

**Files:**
- Create: `tests/video-metadata-load/k6/video_metadata.js`
- Create: `tests/video-metadata-load/scripts/smoke.ps1`

- [ ] **Step 1: Implement request generation and validation**

The k6 script must accept `MODE=cold|hot|mixed`, `RATE`, `DURATION`, `RUN_ID`, `BASE_URL`, `TOKEN`, and a manifest path. Use `constant-arrival-rate`, set `timeUnit: '1s'`, set `preAllocatedVUs` high enough to avoid startup scaling, and report dropped iterations. For mixed mode, choose hot traffic when `__ITER % 10 < 7`; otherwise append unique `load_run` and `request` query values. Send a safe `X-Request-ID` and validate HTTP 200 plus all metadata fields.

Core request logic:

```javascript
const requestId = `lt-${runId}-${mode}-${__VU}-${__ITER}`;
const cold = mode === 'cold' || (mode === 'mixed' && __ITER % 10 >= 7);
const target = cold
  ? `${asset.url}?load_run=${encodeURIComponent(runId)}&request=${__VU}-${__ITER}`
  : asset.url;

const response = http.post(`${baseUrl}/v1/metadata/video`, JSON.stringify({
  url: target,
  media_type: 'video',
  max_bytes: 134217728,
  deadline_ms: 30000,
}), {
  headers: {
    Authorization: `Bearer ${token}`,
    'Content-Type': 'application/json',
    'X-Request-ID': requestId,
  },
  tags: { cache_expectation: cold ? 'cold' : 'hot', asset_id: asset.id },
  timeout: '35s',
});
```

Do not log the request body, URL, token, or response body. Parse known error envelopes into bounded `result_code` counters.

- [ ] **Step 2: Add threshold profiles**

Set these k6 thresholds:

```javascript
const latencyThreshold = mode === 'hot'
  ? 'p(95)<500'
  : mode === 'cold'
    ? 'p(95)<10000'
    : 'p(95)<10000';

export const options = {
  thresholds: {
    checks: ['rate>=0.995'],
    http_req_failed: ['rate<=0.005'],
    http_req_duration: [latencyThreshold],
    'http_req_duration{cache_expectation:hot}': ['p(95)<500'],
    'http_req_duration{cache_expectation:cold}': ['p(95)<10000'],
    dropped_iterations: ['count==0'],
  },
};
```

For discovery runs, keep thresholds non-aborting so the boundary is recorded. Abort conditions come from the external monitor, not k6 thresholds.

- [ ] **Step 3: Run a local syntax and smoke check**

```bash
docker run --rm -v "$PWD:/work" -w /work grafana/k6:0.57.0 inspect tests/video-metadata-load/k6/video_metadata.js
pwsh tests/video-metadata-load/scripts/smoke.ps1
```

Expected: `inspect` exits 0; smoke sends six requests, records six HTTP 200 responses, and exposes no URL or token in stdout.

- [ ] **Step 4: Commit the workload**

```bash
git add tests/video-metadata-load/k6/video_metadata.js tests/video-metadata-load/scripts/smoke.ps1
git commit -m "test: add video metadata k6 workload"
```

### Task 4: Automate Plateaus and Evidence Collection

**Files:**
- Create: `tests/video-metadata-load/scripts/run-plateau.ps1`
- Create: `tests/video-metadata-load/scripts/run-staircase.ps1`
- Create: `tests/video-metadata-load/scripts/monitor.ps1`
- Create: `tests/video-metadata-load/scripts/summarize.ps1`

- [ ] **Step 1: Implement one plateau runner**

`run-plateau.ps1` must require `RunId`, `Mode`, `Rate`, `Duration`, and `Manifest`; create `results/<RunId>/<Mode>-<Rate>/`; capture inventory before traffic; start monitoring; run k6 with JSON and summary outputs; stop monitoring; wait two minutes; capture logs, events, `/tmp` file count, resource state, and origin counters. Use `try/finally` so evidence is captured after failure.

- [ ] **Step 2: Implement the external abort monitor**

Poll every 10 seconds and enforce the Abort Conditions above. On abort, create `ABORTED.json` with the timestamp and bounded reason code, stop only the load-generator process, and leave the service running for evidence collection.

- [ ] **Step 3: Implement a staircase runner**

Accept a comma-separated exact rate list. Run ten-minute plateaus sequentially with a two-minute drain between them. Stop after the first unsafe plateau plus evidence collection. Example:

```powershell
pwsh tests/video-metadata-load/scripts/run-staircase.ps1 `
  -RunId cold-16-20260801 `
  -Mode cold `
  -Rates 1,2,3,4,5,6,8,10,12,16,20,24 `
  -PlateauMinutes 10 `
  -Manifest tests/video-metadata-load/config/controlled.local.json
```

- [ ] **Step 4: Implement deterministic summarization**

For each plateau, output `summary.json` and `summary.md` containing offered/completed rate, counts by HTTP status and result code, success rate, P50/P95/P99 by cache tag, dropped iterations, CPU average/max, memory start/end/slope, temporary-disk start/peak/end, origin error rate/throughput, generator utilization, and a `safe: true|false` decision with violated rule IDs.

- [ ] **Step 5: Test the automation with synthetic evidence**

Add fixtures for one safe plateau, one latency failure, one `concurrency_limited` boundary, one generator-saturated invalid run, and one memory-growth failure. Run:

```powershell
pwsh tests/video-metadata-load/scripts/summarize.ps1 -FixtureDir tests/video-metadata-load/fixtures/safe
pwsh tests/video-metadata-load/scripts/summarize.ps1 -FixtureDir tests/video-metadata-load/fixtures/concurrency-boundary
```

Expected: the first prints `safe=true`; the second prints `safe=false` and `CAPACITY_CONCURRENCY_LIMITED`.

- [ ] **Step 6: Commit automation**

```bash
git add tests/video-metadata-load/scripts tests/video-metadata-load/fixtures
git commit -m "test: automate video metadata load plateaus"
```

### Task 5: Execute Controlled Capacity Discovery

**Files:**
- Create: `tests/video-metadata-load/results/<run-id>/` (raw artifacts remain untracked)
- Create: `docs/testing/video-metadata-load-test-report.md`

- [ ] **Step 1: Establish idle baseline**

Run one service instance for 15 minutes without traffic. Record CPU, RSS, filesystem, open descriptors, health, origin counters, and generator state. Restart before the first cold test so cache and temporary state are known.

- [ ] **Step 2: Run C1 with cache disabled**

Set `VIDEO_METADATA_CACHE_ENTRIES=0`, keep `VIDEO_METADATA_MAX_CONCURRENCY=16`, deploy one instance, run preflight, then execute the cold staircase. If no unsafe plateau appears, run a new two-hour staircase with every rate doubled and report the first run only as a lower bound.

- [ ] **Step 3: Run H1 with cache enabled and pre-warmed**

Set cache entries to the production candidate value (default 10000). Warm every hot URL once, confirm service logs show `cache_hit=true` on a second pass, then run the hot staircase. Include HEAD origin traffic in origin metrics because hot requests still validate cache keys with HEAD.

- [ ] **Step 4: Run M1 with 70/30 traffic**

Restart the instance, pre-warm the stable hot pool, and run the mixed staircase. Verify the service-log cache-hit ratio is 70% +/- 3%; otherwise invalidate and rerun the test.

- [ ] **Step 5: Calculate initial capacities**

Apply the Capacity Definitions mechanically to choose `C_safe_cold`, `C_safe_hot`, and `C_safe_mixed`. Preserve the first unsafe plateau and label its dominant boundary: concurrency, CPU, memory, temporary disk, service latency, origin, or generator.

### Task 6: Tune Concurrency and Verify Recovery

**Files:**
- Modify: `docs/testing/video-metadata-load-test-report.md`

- [ ] **Step 1: Test concurrency candidates**

For `VIDEO_METADATA_MAX_CONCURRENCY` values `8`, `16`, and `32`, run cold and mixed plateaus at 80%, 100%, 120%, and again 100% of the baseline boundary. Keep all other variables identical. Do not test 32 unless worst-case temporary storage (`32 x 128 MiB` plus 20% headroom) fits the configured filesystem.

- [ ] **Step 2: Select the recommendation**

Choose the value with the highest repeatable safe mixed throughput. Require the final 100% repeat to differ by no more than 10% in P95 latency and completed rate from its first 100% plateau. If throughput gains less than 10% while peak memory, disk, or origin bandwidth grows more than 20%, retain the lower concurrency.

- [ ] **Step 3: Run R1 recovery burst**

At the selected concurrency, send 150% of `C_safe_mixed` for two minutes, then immediately send 60% for ten minutes. The burst may return `concurrency_limited`; after one minute of recovery, success rate and P95 must meet normal SLOs, health must remain green, and `/tmp` must drain.

- [ ] **Step 4: Update report evidence**

Record the selected concurrency, rejected candidates, limiting resource, recovery time, and whether `concurrency_limited` remained a clean fast-fail response rather than becoming timeout or internal errors.

### Task 7: Validate Business Load and Public Origins

**Files:**
- Modify: `docs/testing/video-metadata-load-test-report.md`

- [ ] **Step 1: Run B1 at the derived load**

Calculate `normal_business_qps = floor(0.60 * C_safe_mixed)` and run mixed traffic for 30 minutes. Count every response against success rate. This run must meet all SLOs and have zero `concurrency_limited` responses.

- [ ] **Step 2: Run P1 only within authorization**

For each public origin independently, run 10%, 30%, and 60% of `C_safe_mixed` for ten minutes each, capped at that origin's `authorized_max_rps`. Stop that origin if its own 5xx exceeds 0.5%. Report latency and errors per origin; never average them into controlled capacity.

- [ ] **Step 3: Attribute public-path latency**

Compare k6 latency, service `elapsed_ms`, and origin TTFB. Classify the observed limit as service, DNS/connect/TLS, origin TTFB, origin throughput, or inconclusive. Do not lower `C_safe` because of public-origin variance; add an operational timeout or routing recommendation only when supported by evidence.

### Task 8: Run the 8-Hour Soak and Publish the Capacity Result

**Files:**
- Modify: `docs/testing/video-metadata-load-test-report.md`
- Modify: `docs/deployment/video-metadata-service.md`

- [ ] **Step 1: Run L1**

Use the selected concurrency, production cache settings, controlled origin, 70/30 traffic, and `normal_business_qps` for eight uninterrupted hours. Do not restart or redeploy any component during the run.

- [ ] **Step 2: Evaluate soak-specific criteria**

Require success >= 99.5%, hot P95 <= 500 ms, cold P95 <= 10 seconds, CPU below 75%, RSS slope below 1% per hour and total growth below 10%, no sustained filesystem growth, no leftover temporary files, no restart/OOM, no `internal_error`, and no worsening latency trend between the first and last hour greater than 10%.

- [ ] **Step 3: Complete the report**

The report must include environment inventory, corpus distribution, each plateau, first unsafe boundary, all three `C_safe` values, selected concurrency, `normal_business_qps`, cold contingency capacity, resource cost per successful request and GiB downloaded, public-origin results, recovery result, soak trends, invalidated runs, raw artifact checksums, and a pass/fail decision.

- [ ] **Step 4: Add deployment guidance**

Update `docs/deployment/video-metadata-service.md` with the measured per-instance values tied to the exact resource shape and image digest. State that extrapolation to a different CPU, memory, disk, region, origin, or service build requires a new validation.

- [ ] **Step 5: Run final repository verification**

```bash
go test ./pkg/videometa ./cmd/video-metadata-service -count=1
docker run --rm -v "$PWD:/work" -w /work grafana/k6:0.57.0 inspect tests/video-metadata-load/k6/video_metadata.js
```

Expected: Go tests pass and k6 inspection exits 0.

- [ ] **Step 6: Commit the report and guidance**

```bash
git add docs/testing/video-metadata-load-test-report.md docs/deployment/video-metadata-service.md
git commit -m "docs: publish video metadata capacity baseline"
```

## Report Decision Table

| Result | Decision |
|---|---|
| All C/H/M safe plateaus identified, B1 and L1 pass | Approved at `normal_business_qps` per instance |
| No unsafe plateau reached | Capacity is a lower bound; extend staircase before approval |
| Controlled origin or generator saturates first | Invalid capacity result; enlarge the bottleneck and rerun |
| B1 fails while capacity plateau passed | Do not approve; investigate non-repeatability or metric attribution |
| L1 fails memory/disk trend only | Do not approve; diagnose leak/cleanup and rerun full L1 |
| Public P1 fails but controlled B1/L1 pass | Controlled capacity remains valid; public-path risk remains open |
| Recovery does not return to SLO within one minute | Do not approve at the selected concurrency |

Raw artifacts can contain infrastructure identifiers and must remain outside Git. Commit only sanitized summaries with bounded origin IDs and checksums.
