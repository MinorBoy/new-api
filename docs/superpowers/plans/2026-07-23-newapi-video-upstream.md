# NewAPI Video Upstream Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a task-only `NewAPIVideo` channel that submits and polls an upstream new-api `/v1/video/generations` service while exposing safe OpenAI Video and ARK-compatible local APIs, including ARK token usage.

**Architecture:** Register channel type 60 without a generic API type. A focused task adaptor preserves OpenAI JSON request semantics, translates the verified ARK request into the upstream's required string `prompt` plus preserved `content` array, parses both detailed `TaskResponse<TaskDto>` and direct `OpenAIVideo` polling responses, and builds allowlisted public projections. The shared polling service remains responsible for persistence, CAS transitions, settlement, and refunds, but gains deterministic HTTP classification and a `result_url`-aware wrapper projection.

**Tech Stack:** Go 1.22+, Gin, GORM v2, `common.*` JSON wrappers, `shopspring/decimal`, `testify`, React 19, TypeScript, Bun.

---

## Frozen Contracts

- Upstream submit: `POST {baseURL}/v1/video/generations`.
- Upstream poll: `GET {baseURL}/v1/video/generations/{upstream_task_id}`.
- The detailed polling endpoint is intentional: the standard `GET /v1/videos/{task_id}` response is useful for client polling, but does not contain the complete nested task data or token usage needed by the gateway.
- Local OpenAI submit/query return direct `dto.OpenAIVideo`, with local public IDs and origin model names.
- Local ARK submit returns `{"id":"task_local_public_id"}`; query/list return an allowlisted ARK task object.
- OpenAI submission accepts only `application/json`, preserves every top-level JSON value, and replaces only `model`.
- ARK input requires text in a top-level string `prompt` for the upstream request. The adaptor derives that prompt from the ARK text item. Text-only, first-frame, and first+last-frame modes retain their documented `prompt`/`image`/`image_with_roles` translations. Reference mode preserves the complete `content` array, including the verified combination of two reference images, one reference video, and one reference audio. Mixing first/last-frame roles with reference-mode media remains unverified and returns 400, as do `draft_task`, `draft:true`, non-empty `tools`, and other unverified semantic fields.
- Upstream `generateAudio` is camelCase; the local ARK spelling remains `generate_audio` and is translated to `generateAudio`.
- Upstream `duration` is accepted but does not control this route. Effective non-default duration uses the string field `seconds`, for example `"seconds":"10"`; numeric `seconds` is rejected by the upstream. OpenAI `duration` remains passthrough-only and cannot satisfy per-duration billing by itself.
- Detailed polling data is stored intact in `Task.Data`, subject only to the existing Base64 redaction policy.
- ARK output exposes nested `usage.completion_tokens` and `usage.total_tokens`, including explicit zero, but never exposes upstream IDs, provider model, user/channel/group/platform/quota fields, or local `Task.Quota`.
- Fixed-price and per-duration tasks retain their configured charge. Ratio/token tasks settle from authoritative polling usage.

### Verified upstream facts (2026-07-23 test report)

- `prompt` is required and must be a string. A top-level `content` array cannot replace it, and an array in `prompt` returns a JSON type error.
- A request containing one text item, two `reference_image` items, one `reference_video`, and one `reference_audio` completed successfully when the same text was also supplied as the top-level `prompt`.
- The upstream accepts `generateAudio:true` and returns `generate_audio:true` in the detailed nested result. The resulting media contained an AAC audio track.
- `duration:10` was accepted but generated about five seconds. `seconds:10` was rejected as a type error; `seconds:"10"` generated about ten seconds.
- The detailed response reports `data.data.duration` and token usage, while the standard response reports progress and `metadata.url`. `completed_at` may be non-zero while status is still `in_progress`.

## File Map

**Create**

- `relay/channel/task/newapivideo/constants.go`: channel name and empty model list.
- `relay/channel/task/newapivideo/dto.go`: request, polling, error, and public ARK projections with presence-preserving pointer fields.
- `relay/channel/task/newapivideo/request.go`: OpenAI JSON validation, billing facts, and model-only rewrite.
- `relay/channel/task/newapivideo/native.go`: verified ARK-to-new-api translation.
- `relay/channel/task/newapivideo/response.go`: dual polling parser, status/error/URL extraction, token saturation, and public converters.
- `relay/channel/task/newapivideo/adaptor.go`: task adaptor lifecycle and HTTP transport.
- `relay/channel/task/newapivideo/request_test.go`
- `relay/channel/task/newapivideo/native_test.go`
- `relay/channel/task/newapivideo/response_test.go`
- `relay/channel/task/newapivideo/adaptor_test.go`
- `e2e/newapi_video_upstream_e2e_test.go`

**Modify**

- `constant/channel.go`, `constant/channel_test.go`: type 60 registration and Dummy move to 61.
- `relay/relay_adaptor.go`: return the new task adaptor.
- `controller/channel-test.go`, `controller/channel_test_internal_test.go`: reject generic channel tests.
- `relay/relay_task.go`, `relay/relay_task_seedance_test.go`: recognize both OpenAI Video query paths.
- `relay/common/relay_info.go`: track explicit `total_tokens` presence for zero-safe settlement.
- `service/task_polling.go`, `service/task_polling_test.go`, `service/task_billing_tokens_test.go`: result-aware wrapper parsing, HTTP classification, raw persistence, and clamp propagation.
- `relay/seedance_task.go`, `relay/relay_task_seedance_test.go`: ARK whitelist, converter enforcement, nested filtering, and raw-data isolation.
- `web/default/src/features/channels/constants.ts`: type, order, task-only warning, and key prompt.
- `web/default/src/features/channels/lib/channel-type-config.ts`: empty defaults and NewAPI icon metadata.
- `web/default/src/features/channels/lib/channel-utils.ts`: type 60 icon mapping.
- `web/default/tests/channel-type-config.test.ts`: frontend contract tests.
- `web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`: real translations for new guidance.

No database field or migration is required.

---

### Task 1: Register the Task-Only Channel

**Files:**
- Create: `relay/channel/task/newapivideo/constants.go`
- Modify: `constant/channel.go`
- Modify: `constant/channel_test.go`
- Modify: `controller/channel-test.go`
- Modify: `controller/channel_test_internal_test.go`

- [ ] **Step 1: Write failing registration tests**

Replace the narrow constants test with assertions for both task-only channels:

```go
func TestTaskOnlyVideoChannelConstants(t *testing.T) {
	require.Equal(t, 59, constant.ChannelTypeDimensio)
	require.Equal(t, 60, constant.ChannelTypeNewAPIVideo)
	require.Equal(t, 61, constant.ChannelTypeDummy)
	require.Equal(t, "", constant.ChannelBaseURLs[constant.ChannelTypeNewAPIVideo])
	require.Equal(t, "NewAPIVideo", constant.GetChannelTypeName(constant.ChannelTypeNewAPIVideo))
	_, success := common.ChannelType2APIType(constant.ChannelTypeNewAPIVideo)
	require.False(t, success)
}
```

Extend `TestSupportsGenericChannelTestRejectsDimensio`:

```go
require.False(t, supportsGenericChannelTest(constant.ChannelTypeNewAPIVideo))
```

- [ ] **Step 2: Run the tests and confirm the missing symbols fail**

```powershell
go test ./constant ./controller -run 'TestTaskOnlyVideoChannelConstants|TestSupportsGenericChannelTest' -count=1
```

Expected: FAIL because `ChannelTypeNewAPIVideo` is undefined.

- [ ] **Step 3: Add constants and the adaptor scaffold**

Use these exact constants:

```go
ChannelTypeDimensio       = 59
ChannelTypeNewAPIVideo    = 60 // new-api /v1/video/generations task protocol
ChannelTypeDummy          = 61 // this one is only for count, do not add any channel after this
```

Append an empty index 60 entry to `ChannelBaseURLs`, add
`ChannelTypeNewAPIVideo: "NewAPIVideo"` to `ChannelTypeNames`, and create:

```go
package newapivideo

const ChannelName = "NewAPIVideo"

var ModelList = []string{}
```

Add `constant.ChannelTypeNewAPIVideo` to `unsupportedChannelTypes` in
`supportsGenericChannelTest`. Do not add an entry to `ChannelType2APIType`.

- [ ] **Step 4: Run the focused tests**

```powershell
go test ./constant ./controller -run 'TestTaskOnlyVideoChannelConstants|TestSupportsGenericChannelTest' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit channel registration**

```powershell
git add constant/channel.go constant/channel_test.go relay/channel/task/newapivideo/constants.go controller/channel-test.go controller/channel_test_internal_test.go
git commit -m "feat(video): register new-api video task channel"
```

---

### Task 2: Validate and Preserve OpenAI Video JSON

**Files:**
- Create: `relay/channel/task/newapivideo/dto.go`
- Create: `relay/channel/task/newapivideo/request.go`
- Create: `relay/channel/task/newapivideo/request_test.go`

- [ ] **Step 1: Write table tests for media type, JSON shape, and billing bounds**

Create Gin contexts with the stated body and assert the exact status/code from
`ValidateRequestAndSetAction`:

| Case | Body / Content-Type | Expected |
|---|---|---|
| JSON charset | valid object / `application/json; charset=utf-8` | success |
| multipart | valid fields / `multipart/form-data` | 415 |
| damaged | `{bad` | 400 `invalid_json` |
| array root | `[]` | 400 `invalid_request` |
| missing model | `{"prompt":"x"}` | 400 `missing_model` |
| missing prompt | `{"model":"m"}` | 400 `invalid_request` |
| empty prompt | `{"model":"m","prompt":" "}` | 400 `invalid_request` |
| duration zero | `duration:0` | 400 `invalid_duration` |
| duration overflow | `duration:3601` | 400 `invalid_duration` |
| seconds string | `seconds:"5"` | success and retained |
| seconds number | `seconds:5` | 400 `invalid_seconds` |
| conflicting duration | `duration:5,seconds:"6"` | 400 `invalid_duration` |
| n one | `n:1` | success |
| n huge | `n:18446744073686646784` | 400 `invalid_n` |
| metadata bypass | top-level `duration:5`, metadata `duration:3601` | 400 `invalid_duration` |

Add a semantic-preservation assertion:

```go
body := `{"model":"client","prompt":"x","watermark":false,"seed":0,"duration":5.5,"unknown":{"zero":0,"flag":false}}`
require.Nil(t, validateOpenAIRequest(c, info, []byte(body)))
out, err := buildOpenAIRequestBody(c, "provider-model")
require.NoError(t, err)
assert.JSONEq(t, `{"model":"provider-model","prompt":"x","watermark":false,"seed":0,"duration":5.5,"unknown":{"zero":0,"flag":false}}`, string(out))
```

Add a report-derived passthrough case containing a string `prompt`, the full
text/image/video/audio `content` array, `generateAudio:true`, and
`seconds:"10"`. Assert every value and content item is unchanged and only
`model` is replaced. A corresponding `seconds:10` request must fail locally
before upstream traffic.

- [ ] **Step 2: Run the request tests and confirm they fail**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestValidateOpenAIRequest|TestBuildOpenAIRequestBody' -count=1
```

Expected: FAIL because the request functions do not exist.

- [ ] **Step 3: Define presence-preserving request state**

Use `encoding/json` only for `json.RawMessage` and `json.Number` types; all
marshal/unmarshal calls go through `common.*`:

```go
const requestStateContextKey = "newapi_video_request_state"

type requestState struct {
	OpenAIFields map[string]json.RawMessage
	Duration     *decimal.Decimal
	Seconds      *decimal.Decimal
}

func getRequestState(c *gin.Context) (requestState, error) {
	value, exists := c.Get(requestStateContextKey)
	if !exists {
		return requestState{}, fmt.Errorf("new-api video request state is missing")
	}
	state, ok := value.(requestState)
	if !ok {
		return requestState{}, fmt.Errorf("invalid new-api video request state")
	}
	return state, nil
}
```

Decode the root into `map[string]json.RawMessage`; reject a nil map. Parse
numeric `duration` values with
`decimal.NewFromString(common.JsonRawMessageToString(raw))`. Parse `seconds`
only after requiring its JSON type to be string, matching the verified
upstream contract. Require finite positive values up to
`relaycommon.MaxTaskDurationSeconds`; `seconds` must be an integer string.
Require `n` and `seed` to be integers, and require `n == 1`.

Require a non-empty top-level string `prompt`. Do not treat `messages` as a
prompt source: the tested upstream endpoint ignores Chat Completions-shaped
`messages`. Preserve `content` and every other top-level value unchanged for
the upstream request.

For `metadata`, decode another raw-message map and enforce this rule for each
of `duration`, `seconds`, and `n`: the top-level field must exist and its
normalized numeric value must equal the metadata value. The top-level value
is the only value stored as the billing duration; when `seconds` is present,
it is authoritative for billing and known upstream behavior.

- [ ] **Step 4: Implement model-only rewriting**

Use a fresh map so retries cannot mutate cached state:

```go
func buildOpenAIRequestBody(c *gin.Context, upstreamModel string) ([]byte, error) {
	state, err := getRequestState(c)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]json.RawMessage, len(state.OpenAIFields))
	for key, value := range state.OpenAIFields {
		fields[key] = append(json.RawMessage(nil), value...)
	}
	modelJSON, err := common.Marshal(upstreamModel)
	if err != nil {
		return nil, err
	}
	fields["model"] = modelJSON
	return common.Marshal(fields)
}
```

`validateOpenAIRequest` stores `requestState`, then calls:

```go
relaycommon.StoreTaskRequest(c, info, constant.TaskActionGenerate, relaycommon.TaskSubmitReq{
	Model: clientModel,
	Prompt: prompt,
})
```

- [ ] **Step 5: Run and commit the OpenAI request contract**

```powershell
gofmt -w relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/request.go relay/channel/task/newapivideo/request_test.go
go test ./relay/channel/task/newapivideo -run 'TestValidateOpenAIRequest|TestBuildOpenAIRequestBody' -count=1
git add relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/request.go relay/channel/task/newapivideo/request_test.go
git commit -m "feat(video): validate new-api video JSON passthrough"
```

Expected: tests PASS; explicit `false`, `0`, fractional duration, nested
objects, arrays, and unknown top-level fields survive unchanged.

---

### Task 3: Translate the Verified ARK Request

**Files:**
- Modify: `relay/channel/task/newapivideo/dto.go`
- Create: `relay/channel/task/newapivideo/native.go`
- Create: `relay/channel/task/newapivideo/native_test.go`

- [ ] **Step 1: Define ARK request and upstream request DTOs**

Use pointer optional scalars so explicit false and zero remain distinguishable:

```go
type arkRequest struct {
	Model                 string       `json:"model"`
	Content               []arkContent `json:"content"`
	Ratio                 string       `json:"ratio,omitempty"`
	Resolution            string       `json:"resolution,omitempty"`
	Duration              *int         `json:"duration,omitempty"`
	Watermark             *bool        `json:"watermark,omitempty"`
	GenerateAudio         *bool        `json:"generate_audio,omitempty"`
	ServiceTier           *string      `json:"service_tier,omitempty"`
	Draft                 *bool        `json:"draft,omitempty"`
	Tools                 *[]arkTool   `json:"tools,omitempty"`
}

type arkTool struct {
	Type string `json:"type,omitempty"`
}

type arkMedia struct {
	URL string `json:"url"`
}

type arkContent struct {
	Type      string    `json:"type"`
	Text      string    `json:"text,omitempty"`
	ImageURL  *arkMedia `json:"image_url,omitempty"`
	VideoURL  *arkMedia `json:"video_url,omitempty"`
	AudioURL  *arkMedia `json:"audio_url,omitempty"`
	DraftTask any       `json:"draft_task,omitempty"`
	Role      string    `json:"role,omitempty"`
}

type upstreamRequest struct {
	Model          string              `json:"model"`
	Prompt         string              `json:"prompt"`
	Image          string              `json:"image,omitempty"`
	ImageWithRoles []upstreamRoleImage `json:"image_with_roles,omitempty"`
	Content        []arkContent        `json:"content,omitempty"`
	GenerateAudio  *bool               `json:"generateAudio,omitempty"`
	Ratio          string              `json:"ratio,omitempty"`
	Seconds        *string             `json:"seconds,omitempty"`
	Watermark      *bool               `json:"watermark,omitempty"`
}

type upstreamRoleImage struct {
	URL  string `json:"url"`
	Role string `json:"role"`
}
```

Add `ARK *arkRequest` to the `requestState` created in Task 2. ARK parsing
stores the typed request there; OpenAI parsing continues to leave it nil.

- [ ] **Step 2: Write exact translation tests**

Cover these outputs with `assert.JSONEq`:

```json
{"model":"provider-720p","prompt":"text"}
```

```json
{"model":"provider-720p","prompt":"text","image":"https://x/first.png"}
```

```json
{"model":"provider-720p","prompt":"text","image_with_roles":[{"url":"https://x/first.png","role":"first_frame"},{"url":"https://x/last.png","role":"last_frame"}]}
```

Add the report-derived mixed reference case and assert that the complete
content order and roles are preserved:

```json
{"model":"provider-720p","prompt":"text","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://x/a.png"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"https://x/b.png"},"role":"reference_image"},{"type":"video_url","video_url":{"url":"https://x/a.mp4"},"role":"reference_video"},{"type":"audio_url","audio_url":{"url":"https://x/a.mp3"},"role":"reference_audio"}],"generateAudio":true,"seconds":"10"}
```

Assert explicit local `generate_audio:false` becomes upstream
`generateAudio:false`, `duration:10` becomes `seconds:"10"` and no upstream
`duration` field is emitted. Reject `draft_task`, `draft:true`, non-empty
`tools`, two text items, missing text, malformed media URLs, unsupported roles,
reference audio combined with explicit `generate_audio:false`, first/last-frame
content mixed with reference-mode media, and top-level `seed`, `frames`,
`camera_fixed`, `return_last_frame`, `priority`,
`execution_expires_after`, or `safety_identifier`.

- [ ] **Step 3: Run the tests and confirm translation is absent**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestARKToUpstream|TestARKRejects' -count=1
```

Expected: FAIL because `arkToUpstream` is undefined.

- [ ] **Step 4: Implement strict parsing and translation**

Before decoding `arkRequest`, decode a top-level raw map. The accepted keys are
exactly:

```go
var acceptedARKFields = map[string]struct{}{
	"model": {}, "content": {}, "ratio": {}, "resolution": {},
	"duration": {}, "watermark": {}, "generate_audio": {},
	"service_tier": {}, "draft": {}, "tools": {},
}
```

Reject every other key with
`fmt.Sprintf("InvalidParameter.%s", field)`. Accept only absent or
`"default"` service tier. Accept `draft:false` and an empty tools array but do
not send either upstream. Require integer ARK duration from 1 through
`relaycommon.MaxTaskDurationSeconds`, then serialize it as the upstream string
`seconds`. Never send ARK `duration` upstream because the report proves that
field is accepted but ignored for this route.

Require at least one non-empty text item and derive the upstream string
`prompt` from it. For a single first frame, emit `image`; for one first plus
one last frame, emit `image_with_roles`. For reference mode, preserve every
supported `content` item, its URL, type, role, and relative order. The
verified initial reference combination is text plus two `reference_image`
items, one `reference_video`, and one `reference_audio`; do not downgrade
those roles or reject that cross-media combination. The upstream
`generateAudio` field is emitted in camelCase. When reference audio is present,
emit `generateAudio:true` unless the client explicitly supplied false, which
is rejected as contradictory. Do not infer support for
`draft` or `tools` from the successful media test; their unsupported values
remain explicit 400 responses.

Apply the existing Seedance 2.0 media safety limits before serialization:
at most 9 images, 3 videos, and 3 audios; audio requires at least one image
or video; first/last-frame mode cannot mix with reference media, accepts at
most one first and one last image, and requires first before last. These
limits protect request size and match the project's existing Seedance
validation; they are not billing multipliers.

Validate `resolution` after model mapping:

```go
func validateMappedResolution(requested, upstreamModel string) error {
	if requested == "" {
		return nil
	}
	normalized := strings.ToLower(upstreamModel)
	for _, candidate := range []string{"480p", "720p", "1080p"} {
		if strings.Contains(normalized, candidate) {
			if !strings.EqualFold(requested, candidate) {
				return fmt.Errorf("resolution %s does not match mapped model %s", requested, upstreamModel)
			}
			return nil
		}
	}
	return fmt.Errorf("mapped model %s does not declare a resolution tier", upstreamModel)
}
```

- [ ] **Step 5: Run and commit ARK translation**

```powershell
gofmt -w relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/native_test.go
go test ./relay/channel/task/newapivideo -run 'TestARKToUpstream|TestARKRejects|TestValidateMappedResolution' -count=1
git add relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/native_test.go
git commit -m "feat(video): translate verified ARK inputs"
```

---

### Task 4: Build Submit and Poll HTTP Contracts

**Files:**
- Create: `relay/channel/task/newapivideo/adaptor.go`
- Create: `relay/channel/task/newapivideo/adaptor_test.go`

- [ ] **Step 1: Write mock-server transport tests**

Assert `BuildRequestURL` returns `/v1/video/generations`; a request passed to
`BuildRequestHeader` receives `Authorization: Bearer test-key`,
`Accept: application/json`, and `Content-Type: application/json`. Through a
mock server, assert `FetchTask` sends
`GET /v1/video/generations/upstream%2Ftask` with Bearer authorization and the
Accept header. The report also confirms that `GET /v1/videos/{task_id}` is the
standard client-facing polling shape; do not substitute it for the detailed
backend polling request because it omits the nested usage-bearing task data.

Test submit responses:

- matching `id` and `task_id` succeeds;
- only `id` succeeds;
- only `task_id` succeeds;
- conflicting IDs returns `invalid_response` and writes no client body;
- both IDs absent returns `invalid_response`;
- OpenAI output rewrites both IDs and model, fixes object to `video`, and keeps status/progress/created time;
- ARK output contains only the public `id`.

- [ ] **Step 2: Run the transport tests and confirm failure**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestTaskAdaptorSubmit|TestTaskAdaptorFetch|TestTaskAdaptorDoResponse' -count=1
```

Expected: FAIL because `TaskAdaptor` methods are incomplete.

- [ ] **Step 3: Implement the adaptor lifecycle**

Embed the existing no-op billing implementation:

```go
type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey string
	baseURL string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
}

func (a *TaskAdaptor) BuildRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/v1/video/generations", nil
}
```

`ValidateRequestAndSetAction` parses `Content-Type` with `mime.ParseMediaType`,
returns 415 unless the media type is exactly `application/json`, reads the
cached body with `common.GetBodyStorage`, and dispatches to OpenAI or ARK
validation based on `common.KeySeedanceOfficialAPI`.

`BuildRequestBody` calls `arkToUpstream` for ARK state and
`buildOpenAIRequestBody` otherwise. `EstimateDurationSeconds` requires a
stored duration and uses `seconds` when present, otherwise `duration`. The
`seconds` value is always an integer string; fractional `duration` remains
valid for fixed/token billing but invalid for `per_duration`. OpenAI JSON is
still forwarded unchanged, so a client that sends `duration` rather than the
upstream-specific `seconds` field must not be promised that the requested
duration will take effect; reject duration-only requests in `per_duration`
mode. `EstimateBilling` remains nil so duration is never duplicated in
`OtherRatios`.

Implement `GetModelList` as the empty `ModelList` and `GetChannelName` as
`ChannelName`. The interface-dependent `DoRequest` is added in Task 5 after
`ParseTaskResult` exists, so this intermediate commit remains compilable
without a partial polling implementation.

Create submit response logic around:

```go
upstreamID := response.TaskID
if upstreamID == "" {
	upstreamID = response.ID
}
if response.ID != "" && response.TaskID != "" && response.ID != response.TaskID {
	return "", body, service.TaskErrorWrapperLocal(
		fmt.Errorf("upstream id and task_id do not match"),
		"invalid_response", http.StatusBadGateway,
	)
}
```

Use `url.PathEscape(taskID)` in `FetchTask`; never concatenate an unescaped ID.
Decode JSON normally so escaped URL separators such as `\u0026` become `&`;
never perform a string-level replacement on the signed URL.

- [ ] **Step 4: Parse both upstream error envelopes**

`ParseTaskError` recognizes:

```go
type upstreamErrorEnvelope struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Error   *upstreamError `json:"error,omitempty"`
}

type upstreamError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
```

Prefer nested `error.code/message`, then top-level `code/message`. Preserve
the upstream HTTP status. Invalid 2xx submit bodies use local
`invalid_response` with HTTP 502; 429/5xx submission errors remain non-local
so the existing retry loop can retry another channel.

- [ ] **Step 5: Run and commit transport behavior**

```powershell
gofmt -w relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/adaptor_test.go
go test ./relay/channel/task/newapivideo -run 'TestTaskAdaptorSubmit|TestTaskAdaptorFetch|TestTaskAdaptorDoResponse|TestParseTaskError' -count=1
git add relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/adaptor_test.go
git commit -m "feat(video): relay new-api video task requests"
```

---

### Task 5: Parse Detailed Polling Data and Build Safe Public Responses

**Files:**
- Modify: `relay/channel/task/newapivideo/dto.go`
- Create: `relay/channel/task/newapivideo/response.go`
- Create: `relay/channel/task/newapivideo/response_test.go`
- Modify: `relay/common/relay_info.go`
- Modify: `relay/relay_adaptor.go`
- Modify: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: Define detailed, direct, and public response projections**

Use `*json.Number` for usage and zero-capable numeric ARK fields:

```go
type tokenUsage struct {
	CompletionTokens *json.Number `json:"completion_tokens,omitempty"`
	TotalTokens      *json.Number `json:"total_tokens,omitempty"`
}

type arkVideoContent struct {
	VideoURL string `json:"video_url,omitempty"`
}

type arkTaskData struct {
	Content               *arkVideoContent `json:"content,omitempty"`
	CreatedAt             *int64           `json:"created_at,omitempty"`
	UpdatedAt             *int64           `json:"updated_at,omitempty"`
	Draft                 *bool            `json:"draft,omitempty"`
	Duration              *json.Number     `json:"duration,omitempty"`
	ExecutionExpiresAfter *json.Number     `json:"execution_expires_after,omitempty"`
	FramesPerSecond       *json.Number     `json:"framespersecond,omitempty"`
	GenerateAudio         *bool            `json:"generate_audio,omitempty"`
	Priority              *json.Number     `json:"priority,omitempty"`
	Ratio                 string           `json:"ratio,omitempty"`
	Resolution            string           `json:"resolution,omitempty"`
	Seed                   *json.Number     `json:"seed,omitempty"`
	ServiceTier            string           `json:"service_tier,omitempty"`
	Status                 string           `json:"status,omitempty"`
	Usage                  *tokenUsage      `json:"usage,omitempty"`
	Error                  *upstreamError   `json:"error,omitempty"`
}

type detailedTask struct {
	TaskID     string          `json:"task_id"`
	Status     string          `json:"status"`
	FailReason string          `json:"fail_reason"`
	ResultURL  string          `json:"result_url"`
	SubmitTime int64           `json:"submit_time"`
	StartTime  int64           `json:"start_time"`
	FinishTime int64           `json:"finish_time"`
	Progress   string          `json:"progress"`
	Data       json.RawMessage `json:"data"`
}

type detailedEnvelope struct {
	Code    *string       `json:"code"`
	Message string        `json:"message"`
	Data    *detailedTask `json:"data"`
}
```

Define the public response with only the allowlisted fields:

```go
type arkTaskResponse struct {
	ID                    string           `json:"id"`
	Model                 string           `json:"model"`
	Status                string           `json:"status"`
	Content               *arkVideoContent `json:"content,omitempty"`
	CreatedAt             *int64           `json:"created_at,omitempty"`
	UpdatedAt             *int64           `json:"updated_at,omitempty"`
	Draft                 *bool            `json:"draft,omitempty"`
	Duration              *json.Number     `json:"duration,omitempty"`
	ExecutionExpiresAfter *json.Number     `json:"execution_expires_after,omitempty"`
	FramesPerSecond       *json.Number     `json:"framespersecond,omitempty"`
	GenerateAudio         *bool            `json:"generate_audio,omitempty"`
	Priority              *json.Number     `json:"priority,omitempty"`
	Ratio                 string           `json:"ratio,omitempty"`
	Resolution            string           `json:"resolution,omitempty"`
	Seed                   *json.Number     `json:"seed,omitempty"`
	ServiceTier            string           `json:"service_tier,omitempty"`
	Usage                  *tokenUsage      `json:"usage,omitempty"`
	Error                  *upstreamError   `json:"error,omitempty"`
}
```

Add this field beside `CompletionTokensPresent` in `relaycommon.TaskInfo`:

```go
TotalTokensPresent bool `json:"-"`
```

Existing adaptors leave it false, preserving their current settlement
semantics; NewAPIVideo sets it from the `*json.Number` presence.

For the direct form, decode both transport fields and the top-level ARK-safe
projection:

```go
type directTask struct {
	ID          string            `json:"id"`
	TaskID      string            `json:"task_id"`
	Status      string            `json:"status"`
	Progress    int               `json:"progress"`
	CreatedAt   int64             `json:"created_at"`
	CompletedAt int64             `json:"completed_at"`
	Metadata    *struct {
		URL string `json:"url,omitempty"`
	} `json:"metadata,omitempty"`
	Content     *arkVideoContent  `json:"content,omitempty"`
	Data        *struct {
		URL string `json:"url,omitempty"`
	} `json:"data,omitempty"`
	Usage *tokenUsage    `json:"usage,omitempty"`
	Error *upstreamError `json:"error,omitempty"`
}
```

Unmarshal the same direct body into `arkTaskData` to capture its allowlisted
top-level `draft`, duration, resolution, seed, service tier, usage, and error
fields without admitting unknown fields to public output.

- [ ] **Step 2: Write parsing and converter tests from the real report**

Use the report's complete wrapper, including `draft:false`, `priority:0`,
`seed`, `framespersecond:24`, `generate_audio:true`,
`execution_expires_after:172800`, unknown nested field
`"future_field":{"keep":true}`, and usage 108900. Assert:

```go
assert.Equal(t, model.TaskStatusSuccess, result.Status)
assert.Equal(t, "https://example.com/video.mp4", result.Url)
assert.Equal(t, 108900, result.CompletionTokens)
assert.True(t, result.CompletionTokensPresent)
assert.Equal(t, 108900, result.TotalTokens)
assert.True(t, result.TotalTokensPresent)
```

Add status tables for wrapper `NOT_START`, `SUBMITTED`, `QUEUED`,
`IN_PROGRESS`, `SUCCESS`, `FAILURE` and direct `queued`, `in_progress`,
`running`, `completed`, `succeeded`, `failed`, `cancelled`. Unknown status and
terminal successful status without any known URL must return an error. A
non-terminal direct `in_progress` result with `progress:50`, an empty
`metadata.url`, and a non-zero `completed_at` must remain in progress; never
use `completed_at` alone as a completion signal.

Assert URL precedence in this order: outer `result_url`, nested
`content.video_url`, direct `metadata.url`, direct `content.video_url`, direct
`data.url`. Assert failure precedence: outer `fail_reason`, nested error,
direct error, envelope message, `task failed`.

For public converters assert no serialized body contains any of:
`upstream-secret`, `provider-model`, `user_id`, `channel_id`, `group`,
`platform`, or `quota`. Assert ARK usage preserves explicit zero and the
OpenAI response uses the public ID in both ID fields.

- [ ] **Step 3: Run the response tests and confirm failure**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestParseTaskResult|TestConvertToOpenAIVideo|TestConvertToArkVideoTask' -count=1
```

Expected: FAIL because response parsing and converters are undefined.

- [ ] **Step 4: Implement one shared response projection parser**

Create a stable domain result used by polling and both converters:

```go
type parsedTask struct {
	Status       model.TaskStatus
	Progress     string
	URL          string
	Reason       string
	ErrorCode    string
	CreatedAt    int64
	UpdatedAt    int64
	Nested       *arkTaskData
	Usage        *tokenUsage
	BillingClamp *common.QuotaClamp
}
```

Treat a non-nil top-level `Code` as the wrapper discriminator. Require
`code == "success"` and non-nil data. Otherwise parse the body as a direct
response projection. Unknown status returns an error rather than defaulting
to processing or success.

Convert usage to billing integers through `decimal.NewFromString` and
`common.QuotaFromDecimalChecked`; require an integral numeric value. Store the
first clamp in `TaskInfo.BillingClamp`. The ARK response continues to marshal
the original `json.Number`, so public usage is not replaced by the saturated
billing integer.

- [ ] **Step 5: Implement both allowlisted public converters**

OpenAI conversion always starts from local task facts:

```go
video := dto.NewOpenAIVideo()
video.ID = task.TaskID
video.TaskID = task.TaskID
video.Model = task.Properties.OriginModelName
video.Status = task.Status.ToVideoStatus()
video.CreatedAt = task.SubmitTime
if video.CreatedAt == 0 {
	video.CreatedAt = task.CreatedAt
}
video.SetProgressStr(task.Progress)
if video.Progress < 0 {
	video.Progress = 0
}
if video.Progress > 100 {
	video.Progress = 100
}
video.SetMetadata("url", "")
```

Only a successful local task receives a URL. Only terminal tasks receive
`CompletedAt`, using `FinishTime` then `UpdatedAt`. A failed task receives the
parsed error, then `task.FailReason`, then `task failed`.

ARK conversion copies only the `arkTaskData` fields declared above, then
forcibly overwrites ID, model, and status from the local task. It falls back
to `Task.PrivateData.ResultURL` only for `content.video_url`, and never derives
usage from outer `quota` or local `Task.Quota`.

With `ParseTaskResult` now implemented, import the package in
`relay/relay_adaptor.go` and register the complete task adaptor:

```go
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, body)
}

case constant.ChannelTypeNewAPIVideo:
	return &newapivideo.TaskAdaptor{}
```

Add the task-only registry assertion:

```go
func TestNewAPIVideoTaskAdaptorIsTaskOnly(t *testing.T) {
	require.NotNil(t, GetTaskAdaptor(constant.TaskPlatform("60")))
	_, success := common.ChannelType2APIType(constant.ChannelTypeNewAPIVideo)
	require.False(t, success)
}
```

- [ ] **Step 6: Run and commit polling projections**

```powershell
gofmt -w relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/response_test.go relay/common/relay_info.go
go test ./relay/channel/task/newapivideo ./relay -run 'TestParseTaskResult|TestConvertToOpenAIVideo|TestConvertToArkVideoTask|TestNewAPIVideoTaskAdaptorIsTaskOnly' -count=1
git add relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/response_test.go relay/common/relay_info.go relay/relay_adaptor.go relay/relay_task_seedance_test.go
git commit -m "feat(video): parse detailed new-api video tasks"
```

---

### Task 6: Make Shared Polling Result-Aware and HTTP-Aware

**Files:**
- Modify: `service/task_polling.go`
- Modify: `service/task_polling_test.go`
- Modify: `service/task_billing_tokens_test.go`
- Modify: `relay/channel/task/newapivideo/response.go`
- Modify: `relay/channel/task/newapivideo/response_test.go`

- [ ] **Step 1: Add failing polling behavior tests**

Extend `taskPollingFetchAdaptor` with configurable status/body/error and add an
optional parser implementation. Assert:

- network error, 429, 500, and 503 leave status/data/quota unchanged;
- 404 and 410 become failure with `task not found or expired`;
- other 4xx become failure with parsed upstream code/message;
- 2xx malformed JSON and unknown status leave the task unchanged;
- a detailed success wrapper stores the entire wrapper in `Task.Data`, reads
  outer `result_url`, and does not create a proxy placeholder URL;
- a direct `in_progress` response with `progress:50`, empty `metadata.url`,
  and non-zero `completed_at` leaves the task processing;
- the stored body still contains `draft:false`, `seed`, `usage`, and
  `future_field`;
- the fetch call receives `Task.PrivateData.UpstreamTaskID`.

Add this clamp propagation assertion:

```go
existing := &common.QuotaClamp{Op: "QuotaFromDecimal", Kind: common.QuotaClampOverflow, Clamped: common.MaxQuota}
tokens, present, clamp := taskBillingTokensChecked(&relaycommon.TaskInfo{
	CompletionTokens: common.MaxQuota,
	CompletionTokensPresent: true,
	BillingClamp: existing,
})
assert.Equal(t, common.MaxQuota, tokens)
assert.True(t, present)
assert.Same(t, existing, clamp)
```

Also assert `TotalTokensPresent:true, TotalTokens:0` returns
`tokens=0, present=true`; a missing total field must continue to return
`present=false`.

- [ ] **Step 2: Run the tests and confirm current regressions**

```powershell
go test ./service -run 'TestUpdateVideoSingleTaskHTTP|TestUpdateVideoSingleTaskDetailedWrapper|TestTaskBillingTokens' -count=1
```

Expected: FAIL because non-2xx responses are not classified and the generic
wrapper projection does not have `result_url`.

- [ ] **Step 3: Add the optional HTTP error interface and explicit wrapper DTO**

Define locally in `service/task_polling.go` to avoid a service-to-relay cycle:

```go
type TaskPollingHTTPErrorParser interface {
	ParseTaskPollingHTTPError(body []byte, statusCode int) *relaycommon.TaskInfo
}

type taskPollingResponseData struct {
	TaskID     string          `json:"task_id"`
	Status     model.TaskStatus `json:"status"`
	FailReason string          `json:"fail_reason"`
	ResultURL  string          `json:"result_url"`
	Progress   string          `json:"progress"`
	Data       json.RawMessage `json:"data"`
}
```

The new adaptor implements `ParseTaskPollingHTTPError`: parse either upstream
error envelope; 404/410 use the stable expiration message; all other 4xx use
the upstream message and code with an HTTP-code fallback.

- [ ] **Step 4: Reorder parsing and classify HTTP responses**

After reading the body:

```go
if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
	return fmt.Errorf("retryable polling HTTP status %d for task %s", resp.StatusCode, taskId)
}

var taskResult *relaycommon.TaskInfo
if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
	if parser, ok := adaptor.(TaskPollingHTTPErrorParser); ok {
		taskResult = parser.ParseTaskPollingHTTPError(responseBody, resp.StatusCode)
	}
	if taskResult == nil {
		taskResult = relaycommon.FailTaskInfo(fmt.Sprintf("upstream returned HTTP %d", resp.StatusCode))
		taskResult.ErrorCode = strconv.Itoa(resp.StatusCode)
	}
} else {
	taskResult, err = adaptor.ParseTaskResult(responseBody)
	if err != nil || taskResult == nil || taskResult.Status == "" {
		parseErr := err
		if parseErr == nil {
			parseErr = fmt.Errorf("upstream returned empty task status")
		}
		var wrapper dto.TaskResponse[taskPollingResponseData]
		if wrapperErr := common.Unmarshal(responseBody, &wrapper); wrapperErr != nil || !wrapper.IsSuccess() {
			return fmt.Errorf("parseTaskResult failed for task %s: %w", taskId, parseErr)
		}
		taskResult = &relaycommon.TaskInfo{
			TaskID: wrapper.Data.TaskID, Status: string(wrapper.Data.Status),
			Url: wrapper.Data.ResultURL, Progress: wrapper.Data.Progress,
			Reason: wrapper.Data.FailReason,
		}
	}
}
```

Assign `task.Data = redactVideoResponseBody(responseBody)` only after a valid
task result has been obtained. Preserve existing CAS, settlement, refund, and
Base64 redaction behavior.

- [ ] **Step 5: Propagate an adaptor-originated billing clamp**

Choose tokens in this order: present completion, present total, legacy
non-zero total, legacy positive completion, then absent. This preserves old
adaptor behavior while making the new adaptor's explicit total zero
authoritative:

```go
tokens := taskResult.TotalTokens
switch {
case taskResult.CompletionTokensPresent:
	tokens = taskResult.CompletionTokens
case taskResult.TotalTokensPresent:
case taskResult.TotalTokens != 0:
case taskResult.CompletionTokens > 0:
	tokens = taskResult.CompletionTokens
default:
	return 0, false, nil
}
```

After the negative and `> common.MaxQuota` checks in
`taskBillingTokensChecked`, return:

```go
return tokens, true, taskResult.BillingClamp
```

This records overflow detected while decoding a number that was already
saturated to `common.MaxQuota`.

- [ ] **Step 6: Run and commit polling safety**

```powershell
gofmt -w service/task_polling.go service/task_polling_test.go service/task_billing_tokens_test.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/response_test.go
go test ./service ./relay/channel/task/newapivideo -run 'TestUpdateVideoSingleTask|TestTaskBillingTokens|TestParseTaskPollingHTTPError' -count=1
git add service/task_polling.go service/task_polling_test.go service/task_billing_tokens_test.go relay/channel/task/newapivideo/response.go relay/channel/task/newapivideo/response_test.go
git commit -m "fix(video): preserve detailed polling results"
```

---

### Task 7: Enforce OpenAI and ARK Public Query Boundaries

**Files:**
- Modify: `relay/relay_task.go`
- Modify: `relay/seedance_task.go`
- Modify: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: Write route and privacy regression tests**

Add a type 60 task with a full detailed wrapper. Assert:

- `GET /v1/video/generations/task_public` calls `OpenAIVideoConverter` and
  returns direct OpenAI JSON, not `{"code":"success","data":...}`;
- `/v1/videos/task_public` remains supported;
- ARK single query and list both include the type 60 task and its usage;
- ARK explicit zeros (`draft:false`, `priority:0`, zero token usage) remain;
- an unsupported platform queried through ARK returns 404 instead of raw
  `Task.Data`;
- neither query path contains upstream task ID, provider model, user ID,
  channel ID, group, platform, or quota;
- ARK `filter.service_tier=default` finds the value inside
  `data.data.service_tier`.

- [ ] **Step 2: Run the tests and confirm current routing fails**

```powershell
go test ./relay -run 'TestNewAPIVideoOpenAIQuery|TestNewAPIVideoARKQuery|TestSeedanceTaskFetchRejectsUnsupportedPlatform' -count=1
```

Expected: FAIL because `/v1/video/generations/` is not classified as OpenAI
Video and ARK single lookup does not enforce the platform whitelist.

- [ ] **Step 3: Recognize both OpenAI Video query paths**

Replace the single prefix check in `videoFetchByIDRespBodyBuilder` with:

```go
path := c.Request.URL.Path
isOpenAIVideoAPI := strings.HasPrefix(path, "/v1/videos/") ||
	strings.HasPrefix(path, "/v1/video/generations/")
```

Keep adaptor conversion mandatory; do not add a raw or generic fallback for
type 60.

- [ ] **Step 4: Centralize and enforce the ARK platform whitelist**

Add:

```go
func isSeedanceTaskPlatform(platform constant.TaskPlatform) bool {
	switch platform {
	case constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVolcEngine)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDimensio)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)):
		return true
	default:
		return false
	}
}
```

Use it in both the list SQL platform set and single-task lookup. For type 60,
require `channel.ArkVideoTaskConverter`; if conversion fails, return an error.
Retain raw parsing only for the existing official ARK platforms whose stored
data is already their public response shape. Do not replace other ARK-native
platforms when adding type 60; if the CLMM Mall channel is registered under
the reconciled type 61, it must be added to this same whitelist and use its
own converter.

When filtering service tier, inspect top-level `service_tier`, then wrapper
`data.data.service_tier`, then billing context, then default to `default`.

- [ ] **Step 5: Run and commit public query isolation**

```powershell
gofmt -w relay/relay_task.go relay/seedance_task.go relay/relay_task_seedance_test.go
go test ./relay -run 'TestNewAPIVideo|TestSeedanceTaskFetch|TestSeedanceTaskList' -count=1
git add relay/relay_task.go relay/seedance_task.go relay/relay_task_seedance_test.go
git commit -m "feat(video): expose safe OpenAI and ARK task queries"
```

---

### Task 8: Prove Billing and the Full Local Lifecycle

**Files:**
- Create: `e2e/newapi_video_upstream_e2e_test.go`
- Modify: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: Add focused duration-mode relay tests**

Use a mock upstream and `RelayTaskSubmit` to assert:

- fixed/token mode accepts `duration:5.5` and forwards it unchanged;
- per-duration mode rejects `duration:5.5` before upstream traffic;
- per-duration mode accepts integer `seconds:"5"`, records requested and
  billable seconds as 5, and does not add a duplicate duration ratio;
- per-duration mode rejects numeric `seconds:5` and accepts string
  `seconds:"10"`, forwarding the string unchanged;
- a request using `duration:10` remains a valid passthrough but is not claimed
  to produce ten seconds; a request using `seconds:"10"` is the effective
  ten-second upstream form;
- per-duration mode requires `seconds` (a duration-only request is rejected);
- oversized duration and metadata bypass are rejected before pre-consume and
  before upstream traffic.

- [ ] **Step 2: Create a mock full-lifecycle E2E**

Reuse the existing `setupSeedanceE2EDB`, `seedSeedanceE2EData`,
`seedanceE2ERouter`, and `performJSONRequest` helpers from the same `e2e`
package. After seeding, update channel 1 to type 60, key
`mock-newapi-video-key`, models `client-video`, and mapping
`{"client-video":"seedance-720p-token"}`. Call `channel.Update()` after these
assignments so its ability row is rebuilt for `client-video`.

The mock server must record and serve exactly:

```text
POST /v1/video/generations
GET  /v1/video/generations/upstream-task
```

Submit response:

```json
{"id":"upstream-task","task_id":"upstream-task","object":"video","model":"seedance-720p-token","status":"queued","progress":0,"created_at":1784728184}
```

Polling response:

```json
{"code":"success","message":"","data":{"task_id":"upstream-task","status":"SUCCESS","result_url":"https://example.com/video.mp4","submit_time":1784728184,"finish_time":1784728390,"progress":"100%","user_id":59,"channel_id":14,"group":"secret","quota":2000000,"platform":"54","data":{"content":{"video_url":"https://example.com/video.mp4"},"created_at":1784728184,"updated_at":1784728390,"draft":false,"duration":10,"execution_expires_after":172800,"framespersecond":24,"generate_audio":true,"id":"provider-secret","model":"doubao-seedance-2.0","priority":0,"ratio":"16:9","resolution":"720p","seed":92859,"service_tier":"default","status":"succeeded","usage":{"completion_tokens":216900,"total_tokens":216900},"future_field":{"keep":true}}}}
```

The fixture should also include the report-observed `start_time` and
`properties.origin_model_name`/`properties.upstream_model_name` fields in the
stored-body assertion. The converter must ignore those provider metadata
fields in public output while retaining them in redacted `Task.Data`.

- [ ] **Step 3: Assert both client protocols against one stored task**

Submit OpenAI JSON with `watermark:false`, `seed:0`, and an unknown nested
object. Assert the mock receives all fields with only model changed. Assert
the submit response has public IDs and origin model and contains no upstream
ID.

Call `service.UpdateVideoTasks` with `service.GetTaskAdaptorFunc` returning
`relay.GetTaskAdaptor(platform)`. Reload the task and assert:

```go
assert.Equal(t, model.TaskStatusSuccess, task.Status)
assert.Equal(t, "upstream-task", task.PrivateData.UpstreamTaskID)
assert.Equal(t, "https://example.com/video.mp4", task.PrivateData.ResultURL)
assert.Contains(t, string(task.Data), `"future_field":{"keep":true}`)
assert.Equal(t, 216900, task.PrivateData.BillingContext.BillingTokens)
```

Query both local endpoints. OpenAI must return `completed`, public IDs, origin
model, and `metadata.url`. ARK must return `succeeded`, safe ARK fields, and
usage 216900. Assert the detailed wrapper's private fields and both upstream
IDs are absent from each client body.

- [ ] **Step 4: Add an ARK submit lifecycle case**

Submit the report-derived ARK request through
`POST /api/v3/contents/generations/tasks`: one text item, two
`reference_image` items, one `reference_video`, one `reference_audio`, and
`generate_audio:true`, with `duration:10`. Assert the mock body contains the
top-level string prompt, mapped model, the complete ordered content array,
`generateAudio:true`, and `seconds:"10"` with no upstream `duration` field.
The local response contains only the public ID. Reuse the same polling/query
assertions and add a negative case proving that ARK content without a
non-empty text item cannot be translated into the required upstream prompt.

- [ ] **Step 5: Run and commit lifecycle coverage**

```powershell
gofmt -w e2e/newapi_video_upstream_e2e_test.go relay/relay_task_seedance_test.go
go test ./relay -run 'TestNewAPIVideoDuration' -count=1
go test ./e2e -run 'TestNewAPIVideoOpenAILifecycleE2E|TestNewAPIVideoARKLifecycleE2E' -count=1 -v
git add e2e/newapi_video_upstream_e2e_test.go relay/relay_task_seedance_test.go
git commit -m "test(video): cover new-api video lifecycle"
```

---

### Task 9: Add Default Frontend Channel Configuration and i18n

**Files:**
- Modify: `web/default/src/features/channels/constants.ts`
- Modify: `web/default/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/default/src/features/channels/lib/channel-utils.ts`
- Modify: `web/default/tests/channel-type-config.test.ts`
- Modify: `web/default/src/i18n/locales/en.json`
- Modify: `web/default/src/i18n/locales/zh.json`
- Modify: `web/default/src/i18n/locales/fr.json`
- Modify: `web/default/src/i18n/locales/ru.json`
- Modify: `web/default/src/i18n/locales/ja.json`
- Modify: `web/default/src/i18n/locales/vi.json`

- [ ] **Step 1: Load required frontend instructions before editing**

Read `web/default/AGENTS.md` and the `i18n-translate`, `shadcn-ui`, and
`vercel-react-best-practices` skills. This task changes channel form behavior
and all supported locales.

- [ ] **Step 2: Add failing channel configuration tests**

Extend `web/default/tests/channel-type-config.test.ts`:

```ts
describe('NewAPIVideo channel configuration', () => {
  test('registers task-only type 60 without fake defaults', () => {
    expect(CHANNEL_TYPES[60]).toBe('NewAPIVideo')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 60, label: 'NewAPIVideo' })
    expect(getChannelTypeIcon(60)).toBe('NewAPI')
    expect(getDefaultBaseUrl(60)).toBe('')
    expect(getChannelTypeConfig(60).supportedModels).toEqual([])
    expect(MODEL_FETCHABLE_TYPES.has(60)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(60)).toBe(true)
  })

  test('provides protocol-specific guidance', () => {
    expect(TYPE_TO_KEY_PROMPT[60]).toBe('Enter the upstream NewAPI video API key')
    expect(CHANNEL_TYPE_WARNINGS[60]).toBe(
      'NewAPIVideo is task-only. Call it through /v1/video/generations or the ARK /api/v3 task API.'
    )
    expect(getChannelTypeHints(60)).toEqual({
      baseUrl: 'Enter the upstream NewAPI base URL',
      key: 'Enter the upstream NewAPI video API key',
      models: 'Add client model names and map them to upstream video models',
    })
  })
})
```

- [ ] **Step 3: Run the frontend test and confirm failure**

```powershell
cd web/default
bun test tests/channel-type-config.test.ts
```

Expected: FAIL because type 60 is absent.

- [ ] **Step 4: Register type 60 in frontend constants**

Add `60: 'NewAPIVideo'`, place 60 after 59 in display order, add 60 to
`GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES`, and add the exact key prompt and
warning from the test. Do not add 60 to `MODEL_FETCHABLE_TYPES`.

Add this configuration:

```ts
60: {
  id: 60,
  name: CHANNEL_TYPES[60],
  icon: 'NewAPI',
  supportedModels: [],
  hints: {
    baseUrl: 'Enter the upstream NewAPI base URL',
    key: 'Enter the upstream NewAPI video API key',
    models: 'Add client model names and map them to upstream video models',
  },
},
```

Map `60: 'NewAPI'` in `getChannelTypeIcon`.

- [ ] **Step 5: Add all locale values**

Use the English source text as each flat JSON key:

| Key | zh | fr | ru | ja | vi |
|---|---|---|---|---|---|
| `NewAPIVideo` | NewAPIVideo | NewAPIVideo | NewAPIVideo | NewAPIVideo | NewAPIVideo |
| `Enter the upstream NewAPI base URL` | 输入上游 NewAPI 的 Base URL | Saisissez l'URL de base du NewAPI en amont | Введите базовый URL вышестоящего NewAPI | 上流 NewAPI のベース URL を入力してください | Nhập URL cơ sở của NewAPI thượng nguồn |
| `Enter the upstream NewAPI video API key` | 输入上游 NewAPI 视频 API 密钥 | Saisissez la clé API vidéo du NewAPI en amont | Введите API-ключ видео вышестоящего NewAPI | 上流 NewAPI の動画 API キーを入力してください | Nhập khóa API video của NewAPI thượng nguồn |
| `Add client model names and map them to upstream video models` | 添加客户端模型名称，并将其映射到上游视频模型 | Ajoutez les noms de modèles clients et associez-les aux modèles vidéo en amont | Добавьте имена клиентских моделей и сопоставьте их с вышестоящими видеомоделями | クライアントモデル名を追加し、上流の動画モデルにマッピングしてください | Thêm tên mô hình phía máy khách và ánh xạ tới mô hình video thượng nguồn |
| `NewAPIVideo is task-only. Call it through /v1/video/generations or the ARK /api/v3 task API.` | NewAPIVideo 仅支持任务接口，请通过 /v1/video/generations 或 ARK /api/v3 任务 API 调用。 | NewAPIVideo prend uniquement en charge les tâches. Appelez-le via /v1/video/generations ou l'API de tâches ARK /api/v3. | NewAPIVideo поддерживает только задачи. Вызывайте его через /v1/video/generations или API задач ARK /api/v3. | NewAPIVideo はタスク専用です。/v1/video/generations または ARK /api/v3 タスク API から呼び出してください。 | NewAPIVideo chỉ hỗ trợ tác vụ. Hãy gọi qua /v1/video/generations hoặc API tác vụ ARK /api/v3. |

The English locale maps every key to itself.

- [ ] **Step 6: Run and commit frontend configuration**

```powershell
cd web/default
bun test tests/channel-type-config.test.ts
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
bun run build
cd ../..
git add web/default/src/features/channels/constants.ts web/default/src/features/channels/lib/channel-type-config.ts web/default/src/features/channels/lib/channel-utils.ts web/default/tests/channel-type-config.test.ts web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/vi.json
git commit -m "feat(web): configure new-api video channels"
```

Expected: all commands exit 0; generated untranslated reports may change but
must not be included in this commit unless `i18n:sync` requires tracked report
updates for consistency.

---

### Task 10: Run Full Verification and Review the Contract

**Files:**
- Modify only files owned by Tasks 1-9 when a verification failure proves a defect.

- [ ] **Step 1: Format and run focused backend tests**

```powershell
$files = git diff --name-only -- '*.go'
if ($files) { gofmt -w $files }
go test ./relay/channel/task/newapivideo ./relay ./service ./constant ./controller -count=1
go test ./e2e -run 'TestNewAPIVideo' -count=1 -v
```

Expected: PASS.

- [ ] **Step 2: Run the complete backend suite and build**

```powershell
go test ./... -count=1
go vet ./...
go build ./...
```

Expected: every command exits 0.

- [ ] **Step 3: Run complete frontend verification**

```powershell
cd web/default
bun test tests/channel-type-config.test.ts
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
bun run build
cd ../..
```

Expected: every command exits 0 and all six required locales have translated
values rather than English fallback, except the product name `NewAPIVideo`.

- [ ] **Step 4: Audit privacy, protocol paths, and billing invariants**

```powershell
rg -n 'upstream_task_id|channel_id|user_id|"quota"|provider-model' relay/channel/task/newapivideo e2e/newapi_video_upstream_e2e_test.go
rg -n '/v1/videos/' relay/channel/task/newapivideo
rg -n 'int\(.*quota|int\(math\.|OtherRatios\[' relay/channel/task/newapivideo service/task_polling.go
```

Inspect every match. The first command may match internal DTOs and negative
test fixtures, but no public response DTO or converter assignment may expose
those values. The second command must not show an upstream poll URL. The third
must show no bare quota conversion and no direct `OtherRatios` assignment.

- [ ] **Step 5: Check the diff without touching unrelated worktree files**

```powershell
git diff --check
git status --short
git diff -- constant/channel.go relay/channel/task/newapivideo relay/relay_adaptor.go relay/relay_task.go relay/seedance_task.go service/task_polling.go controller/channel-test.go e2e/newapi_video_upstream_e2e_test.go web/default/src/features/channels web/default/tests/channel-type-config.test.ts web/default/src/i18n/locales
```

Confirm `_sync-report.json`, `docs/api/image-generation.md`,
`docs/api/video-generation.md`, and existing untranslated reports remain
outside feature commits unless the user separately asks to include them.

- [ ] **Step 6: Perform a final spec coverage review**

Check each frozen contract at the top of this plan against an automated test.
Specifically verify the report's full `data.data` sample remains in stored
`Task.Data`, ARK usage includes explicit zero, `GET /v1/video/generations/:id`
is direct OpenAI JSON, ARK single/list share the same whitelist and converter,
fixed/per-duration billing does not get overwritten by token usage, a
non-zero `completed_at` cannot finish an `in_progress` task, mixed ARK
image/video/audio content is retained, and ARK duration is serialized upstream
as string `seconds` rather than ineffective `duration`.

If verification required a correction, return to the task that owns the
affected files, stage the explicit file list in that task, rerun its focused
tests, and commit with
`git commit -m "fix(video): address new-api video verification findings"`.

Do not create an empty verification commit.

---

## Manual Upstream Acceptance After Merge

Use a temporary API key supplied through the shell environment, never a file
or command-line literal committed to history. Configure a type 60 channel with
the upstream root URL and a model mapping in the same group available to the
test token. Model availability is a function of the token group, channel, and
mapping; a `model_not_found` or `No available channel` response must be
diagnosed as configuration failure rather than retried as a payload failure.
Then perform these acceptance cases:

1. OpenAI JSON text-only generation with `seconds:"10"`.
2. ARK text-only generation with `duration:10`, verifying the adaptor sends
   `seconds:"10"` upstream.
3. ARK mixed-reference generation with two images, one video, one audio, and
   `generate_audio:true`.

For each task, exercise:

```http
POST /v1/video/generations
GET /v1/video/generations/{public_task_id}
POST /api/v3/contents/generations/tasks
GET /api/v3/contents/generations/tasks/{public_task_id}
GET /api/v3/contents/generations/tasks
```

Record only HTTP statuses, local public task ID, local model, state
transitions, final URL presence, and token usage. Do not record the API key,
full signed video URL, upstream task ID, group, channel ID, or upstream wrapper
identity fields. Confirm the detailed upstream poll is
`/v1/video/generations/{upstream_task_id}`, the completed ten-second task
reports `data.data.duration:10`, and the mixed-reference task returns
`generate_audio:true` and non-zero usage. Media-content adherence still
requires visual/audio acceptance; a successful response alone proves request
acceptance, not that every reference was followed.
