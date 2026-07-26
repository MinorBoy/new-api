# Lucen Seedance Channel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dedicated Lucen Seedance task channel that preserves Ark SDK create/get/list calls, supports image/video/audio references, and lets the existing routing and cost systems choose between separately configured fixed-duration and token-billing Lucen channels.

**Architecture:** Register Lucen as a task-only channel backed by a provider profile in the existing `newapivideo` adapter. Keep model constraints in the current routing-policy system, use a shared structured media-URL parser for HTTP(S), `data:`, and `asset://`, and fail closed for token-cost prediction when reference-video duration cannot be resolved. The administrator creates two ordinary Lucen channels and selects the corresponding model subset for each API key.

**Tech Stack:** Go 1.22+, Gin, GORM v2, testify, React 19, TypeScript, Base UI, i18next, Bun.

**Design:** `docs/superpowers/specs/2026-07-26-lucen-channel-design.md`

---

## File Structure

### New files

- `relay/common/task_media_url.go`: shared parser and classification for task media URLs.
- `relay/common/task_media_url_test.go`: URL scheme and validation contract tests.
- `relay/channel/task/newapivideo/profile.go`: generic and Lucen protocol profiles plus the 12-model Lucen catalog.
- `e2e/lucen_upstream_e2e_test.go`: Ark-facing Lucen lifecycle and privacy coverage.

### Modified backend files

- `middleware/model_routing.go`, `middleware/model_routing_test.go`: retain `data:` and `asset://` media while exposing only fetchable HTTP(S) video URLs to metadata lookup.
- `pkg/modelrouting/types.go`: document that `ReferenceVideoURLs` contains only fetchable HTTP(S) URLs; `ReferenceVideos` remains the total request count.
- `service/video_metadata_client.go`, `service/video_metadata_client_test.go`: distinguish “no reference video” from “reference video cannot be inspected”.
- `service/channel_select.go`, `service/profit_routing.go`, `service/profit_routing_test.go`: pass the total reference-video count into the shared request state and exclude token-cost candidates when duration is unknowable.
- `relay/channel/task/newapivideo/adaptor.go`, `dto.go`, `native.go`, `native_test.go`, `adaptor_test.go`: select the Lucen profile, ignore unsupported optional Ark fields only for Lucen, accept Lucen media schemes, preserve zero values, and use routing facts for omitted-duration billing.
- `relay/relay_task_billing_test.go`: run the existing fixed-duration cost settlement fixture with the Lucen task platform.
- `constant/channel.go`, `constant/channel_test.go`: reserve channel type 62 for Lucen and move the dummy sentinel to 63.
- `relay/relay_adaptor.go`, `relay/seedance_task.go`, `relay/relay_task.go`, `relay/relay_task_seedance_test.go`: register Lucen task submit/poll and Ark/OpenAI query conversion.
- `relay/cost_accounting_adaptor_test.go`: protect Lucen cost-meter registration.
- `controller/channel-test.go`, `controller/channel_test_internal_test.go`: keep Lucen out of the generic chat channel test.
- `e2e/profit_routing_e2e_test.go`: prove cost/profit routing can select either Lucen channel without a hard-coded billing-group preference.

### Modified frontend files

- `web/src/features/channels/constants.ts`: Lucen type, ordering, task-only behavior, key prompt, and warning.
- `web/src/features/channels/lib/channel-type-config.ts`: default URL, 12 models, and ordinary channel-form hints.
- `web/src/features/channels/lib/channel-utils.ts`: Lucen icon mapping.
- `web/tests/channel-type-config.test.ts`: form/configuration regression tests.
- `web/src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`: Lucen UI translations.

---

### Task 1: Parse and Preserve Lucen Media URL Schemes

**Files:**
- Create: `relay/common/task_media_url.go`
- Create: `relay/common/task_media_url_test.go`
- Modify: `middleware/model_routing.go`
- Modify: `middleware/model_routing_test.go`
- Modify: `pkg/modelrouting/types.go`

- [ ] **Step 1: Write failing shared parser tests**

Create `relay/common/task_media_url_test.go`:

```go
package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskMediaURL(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		kind      TaskMediaURLKind
		fetchable bool
	}{
		{name: "https", value: "https://assets.example/video.mp4?sig=secret", kind: TaskMediaURLHTTP, fetchable: true},
		{name: "data base64", value: "data:image/png;base64,QUJDRA==", kind: TaskMediaURLData},
		{name: "asset", value: "asset://video-reference-1", kind: TaskMediaURLAsset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseTaskMediaURL(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.value, parsed.Value)
			assert.Equal(t, tt.kind, parsed.Kind)
			assert.Equal(t, tt.fetchable, parsed.FetchableHTTP())
		})
	}
}

func TestParseTaskMediaURLRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{
		"", "ftp://assets.example/video.mp4", "https:///video.mp4",
		"data:image/png,not-base64", "data:image/png;base64,", "asset://",
	} {
		_, err := ParseTaskMediaURL(value)
		require.Error(t, err, value)
	}
}
```

- [ ] **Step 2: Run the parser test and verify it fails**

```powershell
go test ./relay/common -run 'TestParseTaskMediaURL' -count=1
```

Expected: FAIL because `TaskMediaURLKind` and `ParseTaskMediaURL` do not exist.

- [ ] **Step 3: Implement the structured parser**

Create `relay/common/task_media_url.go`:

```go
package common

import (
	"fmt"
	"net/url"
	"strings"
)

type TaskMediaURLKind string

const (
	TaskMediaURLHTTP  TaskMediaURLKind = "http"
	TaskMediaURLData  TaskMediaURLKind = "data"
	TaskMediaURLAsset TaskMediaURLKind = "asset"
)

type TaskMediaURL struct {
	Value string
	Kind  TaskMediaURLKind
}

func (u TaskMediaURL) FetchableHTTP() bool {
	return u.Kind == TaskMediaURLHTTP
}

func ParseTaskMediaURL(raw string) (TaskMediaURL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return TaskMediaURL{}, fmt.Errorf("media URL is empty")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return TaskMediaURL{}, fmt.Errorf("invalid media URL: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return TaskMediaURL{}, fmt.Errorf("HTTP media URL requires a host")
		}
		return TaskMediaURL{Value: value, Kind: TaskMediaURLHTTP}, nil
	case "data":
		comma := strings.IndexByte(value, ',')
		if comma < 0 || !strings.Contains(strings.ToLower(value[:comma]), ";base64") || comma == len(value)-1 {
			return TaskMediaURL{}, fmt.Errorf("data media URL must contain a base64 payload")
		}
		return TaskMediaURL{Value: value, Kind: TaskMediaURLData}, nil
	case "asset":
		if parsed.Host == "" {
			return TaskMediaURL{}, fmt.Errorf("asset media URL requires an identifier")
		}
		return TaskMediaURL{Value: value, Kind: TaskMediaURLAsset}, nil
	default:
		return TaskMediaURL{}, fmt.Errorf("unsupported media URL scheme")
	}
}
```

- [ ] **Step 4: Add failing routing-fact coverage**

Add to `middleware/model_routing_test.go`:

```go
func TestExtractSeedanceRoutingInputAcceptsEmbeddedMediaWithoutMetadataURLs(t *testing.T) {
	body := `{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"make a video"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,QUJDRA=="}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"asset://video-reference-1"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}
		],
		"resolution":"720p","duration":10,"ratio":"16:9"
	}`
	c := seedanceRoutingContext(t, http.MethodPost, "/v1/video/generations", body, true)

	input, routeErr := extractSeedanceRoutingInput(c, modelrouting.Seedance20)
	require.Nil(t, routeErr)
	require.NotNil(t, input)
	assert.Equal(t, 1, input.ReferenceImages)
	assert.Equal(t, 1, input.ReferenceVideos)
	assert.Equal(t, 1, input.ReferenceAudios)
	assert.Empty(t, input.ReferenceVideoURLs)
}
```

- [ ] **Step 5: Route through the shared parser without exposing non-HTTP video data**

Change `validateRoutingMedia` in `middleware/model_routing.go` to return `relaycommon.TaskMediaURL`. For `video_url`, append `media.Value` to `facts.videoURLs` only when `media.FetchableHTTP()` is true. Image and audio callers validate and discard the parsed result. Keep the raw request body unchanged.

```go
func validateRoutingMedia(raw json.RawMessage) (relaycommon.TaskMediaURL, *routingInputError) {
	// Keep the existing object/url-string extraction.
	parsed, err := relaycommon.ParseTaskMediaURL(value)
	if err != nil {
		return relaycommon.TaskMediaURL{}, newRoutingInputError("InvalidParameter.content", "media URL is invalid")
	}
	return parsed, nil
}
```

The `video_url` branch must use:

```go
media, routeErr := validateRoutingMedia(item["video_url"])
if routeErr != nil {
	return facts, routeErr
}
// role validation remains unchanged
facts.videos++
if media.FetchableHTTP() {
	facts.videoURLs = append(facts.videoURLs, media.Value)
}
```

Update the `FactsInput.ReferenceVideoURLs` comment in `pkg/modelrouting/types.go` to say it contains only normalized fetchable HTTP(S) URLs, while `ReferenceVideos` remains the total count from the request.

- [ ] **Step 6: Format, test, and commit media URL support**

```powershell
gofmt -w relay/common/task_media_url.go relay/common/task_media_url_test.go middleware/model_routing.go middleware/model_routing_test.go
go test ./relay/common ./middleware -run 'TestParseTaskMediaURL|TestExtractSeedanceRoutingInput' -count=1
git add relay/common/task_media_url.go relay/common/task_media_url_test.go middleware/model_routing.go middleware/model_routing_test.go
git commit -m "feat(video): preserve embedded task media URLs"
```

Expected: PASS. Existing malformed HTTP URL and `adaptive` routing tests must remain green.

---

### Task 2: Fail Closed When Reference-Video Cost Metadata Is Unavailable

**Files:**
- Modify: `service/video_metadata_client.go`
- Modify: `service/video_metadata_client_test.go`
- Modify: `service/channel_select.go`
- Modify: `service/profit_routing.go`
- Modify: `service/profit_routing_test.go`

- [ ] **Step 1: Write a failing unresolved-reference test**

Add to `service/video_metadata_client_test.go`:

```go
func TestProfitRoutingRequestStateRejectsUnfetchableReferenceVideo(t *testing.T) {
	calls := atomic.Int32{}
	state := NewProfitRoutingRequestState(&fakeMetadataClient{Calls: &calls}, nil, 1)

	assert.True(t, state.HasReferenceVideos())
	result, err := state.Metadata(context.Background())
	require.Error(t, err)
	assert.True(t, result.HasReferenceVideos)
	assert.Zero(t, result.TotalDurationMS)
	assert.Zero(t, calls.Load())
	var metadataErr *VideoMetadataError
	require.ErrorAs(t, err, &metadataErr)
	assert.Equal(t, VideoMetadataUnavailable, metadataErr.Kind)
}
```

Update existing constructor calls in this test file to pass `len(urls)` as the third argument; the no-reference case passes `0`.

- [ ] **Step 2: Run the state tests and verify they fail**

```powershell
go test ./service -run 'TestProfitRoutingRequestState' -count=1
```

Expected: FAIL because the constructor has only two arguments and cannot represent an unresolvable reference video.

- [ ] **Step 3: Carry total reference count in request state**

Change the state and constructor in `service/video_metadata_client.go`:

```go
type ProfitRoutingRequestState struct {
	client              VideoMetadataClient
	urls                []string
	referenceVideoCount int

	once   sync.Once
	result videoMetadataResult
	err    error
}

func NewProfitRoutingRequestState(client VideoMetadataClient, urls []string, referenceVideoCount int) *ProfitRoutingRequestState {
	return &ProfitRoutingRequestState{
		client: client, urls: urls, referenceVideoCount: referenceVideoCount,
	}
}

func (s *ProfitRoutingRequestState) HasReferenceVideos() bool {
	return s != nil && s.referenceVideoCount > 0
}
```

Start `resolve` with:

```go
if s.referenceVideoCount == 0 {
	return videoMetadataResult{HasReferenceVideos: false}, nil
}
if len(s.urls) != s.referenceVideoCount {
	return videoMetadataResult{HasReferenceVideos: true}, &VideoMetadataError{Kind: VideoMetadataUnavailable}
}
```

- [ ] **Step 4: Update production call sites**

In `service/channel_select.go`, create state when `RoutingInput.ReferenceVideos > 0`, not only when the URL slice is non-empty:

```go
p.profitRoutingState = NewProfitRoutingRequestState(
	currentVideoMetadataClient(),
	p.RoutingInput.ReferenceVideoURLs,
	p.RoutingInput.ReferenceVideos,
)
```

In the authoritative recheck in `service/profit_routing.go`, pass the same count and URLs from `modelrouting.FactsInput`. Update remaining tests and helper call sites with the explicit count.

- [ ] **Step 5: Protect candidate filtering**

Add a table case to `service/profit_routing_test.go` using `NewProfitRoutingRequestState(client, nil, 1)`. Assert a per-token candidate is excluded with `ProfitReasonMetadataUnavailable`, while a per-request or per-duration candidate remains eligible. This protects full Lucen dispatch through a fixed-duration channel without underestimating an unknowable token-priced input-video cost.

- [ ] **Step 6: Format, test, and commit metadata safety**

```powershell
gofmt -w service/video_metadata_client.go service/video_metadata_client_test.go service/channel_select.go service/profit_routing.go service/profit_routing_test.go
go test ./service -run 'TestProfitRoutingRequestState|TestFilterProfitEligibleChannels' -count=1
git add service/video_metadata_client.go service/video_metadata_client_test.go service/channel_select.go service/profit_routing.go service/profit_routing_test.go
git commit -m "fix(routing): fail closed on unresolved video metadata"
```

Expected: PASS. HTTP(S) reference videos still resolve once per request; `data:` and `asset://` reference videos never reach the metadata HTTP client.

---

### Task 3: Add a Lucen Profile to the Shared NewAPIVideo Adapter

**Files:**
- Create: `relay/channel/task/newapivideo/profile.go`
- Modify: `relay/channel/task/newapivideo/adaptor.go`
- Modify: `relay/channel/task/newapivideo/dto.go`
- Modify: `relay/channel/task/newapivideo/native.go`
- Modify: `relay/channel/task/newapivideo/native_test.go`
- Modify: `relay/channel/task/newapivideo/adaptor_test.go`

- [ ] **Step 1: Write failing Lucen profile tests**

Add tests that protect the provider-specific behavior while keeping the generic type strict:

```go
func TestLucenTaskAdaptorProfile(t *testing.T) {
	adaptor := NewLucenTaskAdaptor()
	assert.Equal(t, "Lucen", adaptor.GetChannelName())
	assert.Equal(t, []string{
		"seedance-480p-5s", "seedance-480p-10s", "seedance-480p-15s",
		"seedance-720p-5s", "seedance-720p-10s", "seedance-720p-15s",
		"seedance-1080p-5s", "seedance-1080p-10s", "seedance-1080p-15s",
		"seedance-480p-token", "seedance-720p-token", "seedance-1080p-token",
	}, adaptor.GetModelList())
}

func TestLucenARKProfileIgnoresOptionalFieldsAndPreservesMedia(t *testing.T) {
	body := []byte(`{
		"model":"client-model",
		"content":[
			{"type":"text","text":"text"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,QUJDRA=="}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"asset://video-reference-1"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}
		],
		"duration":10,
		"generate_audio":true,
		"watermark":false,
		"callback_url":"https://client.example/callback",
		"return_last_frame":true,
		"priority":7,
		"execution_expires_after":3600,
		"service_tier":"flex",
		"draft":true,
		"tools":[{"type":"web_search"}]
	}`)

	request, err := parseARKRequest(body, lucenProtocolProfile())
	require.NoError(t, err)
	converted, err := arkToUpstream(request, "seedance-720p-token", false, lucenProtocolProfile())
	require.NoError(t, err)
	encoded, err := marshalUpstreamRequest(converted)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"seedance-720p-token","prompt":"text","content":[
			{"type":"text","text":"text"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,QUJDRA=="}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"asset://video-reference-1"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}
		],"generateAudio":true,"seconds":"10","watermark":false
	}`, string(encoded))
}

func TestGenericARKProfileRejectsEmbeddedMedia(t *testing.T) {
	_, err := parseARKRequest([]byte(`{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,QUJDRA=="}}]}`), genericProtocolProfile())
	require.Error(t, err)
}
```

Keep `TestARKRejects` on the generic profile and retain its assertions for `priority`, `execution_expires_after`, and other unknown fields.

- [ ] **Step 2: Run the profile tests and verify they fail**

```powershell
go test ./relay/channel/task/newapivideo -run 'TestLucen|TestARKRejects' -count=1
```

Expected: FAIL because the Lucen profile and constructor do not exist.

- [ ] **Step 3: Add immutable protocol profiles**

Create `relay/channel/task/newapivideo/profile.go` with an unexported configuration type and a public constructor:

```go
package newapivideo

type protocolProfile struct {
	channelName                       string
	modelList                         []string
	ignoreUnsupportedOptionalARKFields bool
	allowEmbeddedMedia                bool
	useRoutingDurationDefault         bool
}

func genericProtocolProfile() protocolProfile {
	return protocolProfile{channelName: "NewAPIVideo"}
}

func lucenProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName: "Lucen",
		modelList: []string{
			"seedance-480p-5s", "seedance-480p-10s", "seedance-480p-15s",
			"seedance-720p-5s", "seedance-720p-10s", "seedance-720p-15s",
			"seedance-1080p-5s", "seedance-1080p-10s", "seedance-1080p-15s",
			"seedance-480p-token", "seedance-720p-token", "seedance-1080p-token",
		},
		ignoreUnsupportedOptionalARKFields: true,
		allowEmbeddedMedia:                true,
		useRoutingDurationDefault:         true,
	}
}

func NewLucenTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: lucenProtocolProfile()}
}

func (a *TaskAdaptor) activeProfile() protocolProfile {
	if a == nil || a.profile.channelName == "" {
		return genericProtocolProfile()
	}
	return a.profile
}
```

Add `profile protocolProfile` to `TaskAdaptor`. A zero-valued `TaskAdaptor` must continue to use `genericProtocolProfile()` so existing type 60 behavior and tests do not change. Implement the accessors without returning the profile's backing slice:

```go
func (a *TaskAdaptor) GetModelList() []string {
	return append([]string(nil), a.activeProfile().modelList...)
}

func (a *TaskAdaptor) GetChannelName() string {
	return a.activeProfile().channelName
}
```

- [ ] **Step 4: Make Ark parsing profile-aware**

Thread `protocolProfile` through `validateARKRequest`, `parseARKRequest`, `buildARKRequestBody`, `arkToUpstream`, and `validateARKSemantics`.

Update every existing generic-package test call from `parseARKRequest(body)` to `parseARKRequest(body, genericProtocolProfile())`, and from `arkToUpstream(request, model, prevalidated)` to `arkToUpstream(request, model, prevalidated, genericProtocolProfile())`; this keeps the existing strict assertions explicit.

Use these rules:

```go
for field := range fields {
	if _, accepted := acceptedARKFields[field]; !accepted && !profile.ignoreUnsupportedOptionalARKFields {
		return arkRequest{}, &arkRequestError{
			Code: "InvalidParameter." + field,
			Message: "field is not supported by this channel",
		}
	}
}
```

Apply the existing `service_tier`, `draft`, and `tools` rejection block only when `ignoreUnsupportedOptionalARKFields` is false. Model/content/media-role/count/duration validation remains mandatory for every profile.

Replace `validMediaURL` with profile-aware parsing through `relaycommon.ParseTaskMediaURL`:

```go
func validMediaURL(value string, profile protocolProfile) bool {
	media, err := relaycommon.ParseTaskMediaURL(value)
	if err != nil {
		return false
	}
	return media.FetchableHTTP() || profile.allowEmbeddedMedia
}
```

The upstream DTO remains pointer-based for `GenerateAudio` and `Watermark`; do not replace those pointers with non-pointer scalars.

- [ ] **Step 5: Use routing facts for omitted Lucen duration billing**

Keep generic type 60's existing duration-only rejection. For Lucen only, when the request has no explicit `seconds`, read the already-resolved routing fact:

```go
value := state.Seconds
profile := a.activeProfile()
if value == nil && profile.useRoutingDurationDefault && info != nil && info.Routing != nil {
	routedSeconds := info.Routing.Facts.DurationSeconds
	if routedSeconds > 0 {
		seconds := decimal.NewFromInt(int64(routedSeconds))
		value = &seconds
	}
}
```

Add `TestLucenDurationEstimatorUsesRoutingDefault` with `modelrouting.Audit{Facts: modelrouting.Facts{DurationSeconds: 5}}`. Assert the estimator returns 5. Preserve `TestTaskAdaptorRejectsDurationOnlyForDurationEstimator` for generic type 60.

- [ ] **Step 6: Format, test, and commit the profile**

```powershell
gofmt -w relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/native_test.go relay/channel/task/newapivideo/adaptor_test.go
go test ./relay/channel/task/newapivideo -count=1
git add relay/channel/task/newapivideo/profile.go relay/channel/task/newapivideo/adaptor.go relay/channel/task/newapivideo/dto.go relay/channel/task/newapivideo/native.go relay/channel/task/newapivideo/native_test.go relay/channel/task/newapivideo/adaptor_test.go
git commit -m "feat(video): add Lucen protocol profile"
```

Expected: PASS. Generic NewAPIVideo still rejects unknown Ark fields and embedded media; Lucen silently drops unsupported optional fields and preserves supported media.

---

### Task 4: Register Lucen as a Task-Only Backend Channel

**Files:**
- Modify: `constant/channel.go`
- Modify: `constant/channel_test.go`
- Modify: `relay/relay_adaptor.go`
- Modify: `relay/seedance_task.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_seedance_test.go`
- Modify: `relay/relay_task_billing_test.go`
- Modify: `relay/cost_accounting_adaptor_test.go`
- Modify: `controller/channel-test.go`
- Modify: `controller/channel_test_internal_test.go`

- [ ] **Step 1: Write failing channel identity and task registration tests**

Add to `constant/channel_test.go`:

```go
func TestLucenChannelConstants(t *testing.T) {
	require.Equal(t, 62, constant.ChannelTypeLucen)
	require.Equal(t, 63, constant.ChannelTypeDummy)
	require.Equal(t, "https://lucen.asia", constant.ChannelBaseURLs[constant.ChannelTypeLucen])
	require.Equal(t, "Lucen", constant.GetChannelTypeName(constant.ChannelTypeLucen))
	_, success := common.ChannelType2APIType(constant.ChannelTypeLucen)
	require.False(t, success)
}
```

Add to `relay/relay_task_seedance_test.go`:

```go
func TestLucenTaskAdaptorIsTaskOnly(t *testing.T) {
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeLucen)))
	require.NotNil(t, adaptor)
	assert.Equal(t, "Lucen", adaptor.GetChannelName())
	_, success := common.ChannelType2APIType(constant.ChannelTypeLucen)
	require.False(t, success)
}
```

Extend `TestTaskCostCapabilitiesAreRegisteredPerPlatform` with Lucen and assert the same three meter sources as NewAPIVideo.

Extend the existing fixed-duration settlement fixture in `relay/relay_task_billing_test.go` to run once with `ChannelTypeLucen` (the fixture currently uses `ChannelTypeNewAPIVideo`). Keep the same integer `seconds:"5"` request and assert the final quota and billing context are unchanged; this proves the Lucen platform uses the shared fixed-duration billing path.

- [ ] **Step 2: Run registration tests and verify they fail**

```powershell
go test ./constant ./relay -run 'TestLucen|TestTaskCostCapabilitiesAreRegisteredPerPlatform' -count=1
```

Expected: FAIL because channel type 62 is not registered.

- [ ] **Step 3: Reserve channel type 62**

In `constant/channel.go`:

```go
ChannelTypeClmmMall = 61
ChannelTypeLucen    = 62 // Lucen Seedance via shared NewAPIVideo protocol
ChannelTypeDummy    = 63 // sentinel; keep last
```

Append `"https://lucen.asia"` at index 62 in `ChannelBaseURLs` and map `ChannelTypeLucen: "Lucen"` in `ChannelTypeNames`. Update constant tests that currently expect `ChannelTypeDummy == 62`.

- [ ] **Step 4: Register submit, polling, and public task queries**

In `relay/relay_adaptor.go`:

```go
case constant.ChannelTypeLucen:
	return newapivideo.NewLucenTaskAdaptor()
```

Add Lucen to both `isSeedanceTaskPlatform` and `seedanceTaskPlatformValues` in `relay/seedance_task.go`. Add Lucen beside NewAPIVideo in the Ark conversion branch of `relay/relay_task.go` so both `/v1/video/generations/{id}` and Ark single-task responses use the shared public converter.

Extend the existing query test fixture to create a Lucen task and assert:

- Ark single query returns it;
- Ark list returns it;
- OpenAI video query returns the public task ID and standard model;
- Lucen upstream task ID, upstream model, channel ID, group, quota, and API key are absent.

- [ ] **Step 5: Keep Lucen out of generic channel tests**

Add `constant.ChannelTypeLucen` to `unsupportedChannelTypes` in `controller/channel-test.go`, and add this assertion to `controller/channel_test_internal_test.go`:

```go
require.False(t, supportsGenericChannelTest(constant.ChannelTypeLucen))
```

- [ ] **Step 6: Format, test, and commit backend registration**

```powershell
gofmt -w constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/relay_task_billing_test.go relay/cost_accounting_adaptor_test.go controller/channel-test.go controller/channel_test_internal_test.go
go test ./constant ./relay ./controller -run 'TestLucen|TestTaskCostCapabilitiesAreRegisteredPerPlatform|TestNewAPIVideo|TestSeedanceTask' -count=1
git add constant/channel.go constant/channel_test.go relay/relay_adaptor.go relay/seedance_task.go relay/relay_task.go relay/relay_task_seedance_test.go relay/relay_task_billing_test.go relay/cost_accounting_adaptor_test.go controller/channel-test.go controller/channel_test_internal_test.go
git commit -m "feat(channel): register Lucen Seedance tasks"
```

Expected: PASS. Lucen remains absent from `ChannelType2APIType`, proving it is task-only.

---

### Task 5: Add the Ordinary Lucen Channel Form and Model Catalog

**Files:**
- Modify: `web/src/features/channels/constants.ts`
- Modify: `web/src/features/channels/lib/channel-type-config.ts`
- Modify: `web/src/features/channels/lib/channel-utils.ts`
- Modify: `web/tests/channel-type-config.test.ts`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/zh.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/vi.json`

- [ ] **Step 1: Write failing frontend configuration tests**

Add to `web/tests/channel-type-config.test.ts`:

```ts
describe('Lucen channel configuration', () => {
  test('uses one ordinary task-only channel type with all Lucen models', () => {
    expect(CHANNEL_TYPES[62]).toBe('Lucen')
    expect(CHANNEL_TYPE_OPTIONS).toContainEqual({ value: 62, label: 'Lucen' })
    expect(getChannelTypeIcon(62)).toBe('NewAPI')
    expect(getDefaultBaseUrl(62)).toBe('https://lucen.asia')
    expect(getChannelTypeConfig(62).supportedModels).toEqual([
      'seedance-480p-5s', 'seedance-480p-10s', 'seedance-480p-15s',
      'seedance-720p-5s', 'seedance-720p-10s', 'seedance-720p-15s',
      'seedance-1080p-5s', 'seedance-1080p-10s', 'seedance-1080p-15s',
      'seedance-480p-token', 'seedance-720p-token', 'seedance-1080p-token',
    ])
    expect(MODEL_FETCHABLE_TYPES.has(62)).toBe(false)
    expect(GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES.has(62)).toBe(true)
    expect(TASK_ONLY_CHANNEL_TYPES.has(62)).toBe(true)
  })

  test('explains the two ordinary API-key channels without a group field', () => {
    expect(TYPE_TO_KEY_PROMPT[62]).toBe('Enter the API key issued by Lucen')
    expect(CHANNEL_TYPE_WARNINGS[62]).toBe(
      'Lucen is task-only. Create separate channels for the fixed-duration key and token-billing key.'
    )
    expect(getChannelTypeHints(62)).toEqual({
      baseUrl: 'Default: https://lucen.asia',
      key: 'Enter the API key issued by Lucen',
      models: "Select Lucen models matching this channel's fixed-duration or token-billing API key",
    })
  })
})
```

- [ ] **Step 2: Run the frontend test and verify it fails**

```powershell
Set-Location web
bun test tests/channel-type-config.test.ts
Set-Location ..
```

Expected: FAIL because type 62 is absent.

- [ ] **Step 3: Register the Lucen channel form**

In `constants.ts`:

- add `62: 'Lucen'` after CLMM Mall;
- add 62 to display order, `GENERIC_CHANNEL_TEST_UNSUPPORTED_TYPES`, and `TASK_ONLY_CHANNEL_TYPES`;
- do not add 62 to `MODEL_FETCHABLE_TYPES`;
- add the exact key prompt and warning asserted above.

In `channel-type-config.ts`, add:

```ts
62: {
  id: 62,
  name: CHANNEL_TYPES[62],
  icon: 'NewAPI',
  defaultBaseUrl: 'https://lucen.asia',
  supportedModels: [
    'seedance-480p-5s', 'seedance-480p-10s', 'seedance-480p-15s',
    'seedance-720p-5s', 'seedance-720p-10s', 'seedance-720p-15s',
    'seedance-1080p-5s', 'seedance-1080p-10s', 'seedance-1080p-15s',
    'seedance-480p-token', 'seedance-720p-token', 'seedance-1080p-token',
  ],
  hints: {
    baseUrl: 'Default: https://lucen.asia',
    key: 'Enter the API key issued by Lucen',
    models: "Select Lucen models matching this channel's fixed-duration or token-billing API key",
  },
},
```

Add 62 to `MANAGED_DEFAULT_BASE_URL_TYPES`, and map `62: 'NewAPI'` in `channel-utils.ts`. Do not add a Lucen-specific group selector, key mode, or conditional drawer section.

- [ ] **Step 4: Add all locale values**

Use these flat i18n keys and translations:

| English key | zh | zh-TW | fr | ru | ja | vi |
| --- | --- | --- | --- | --- | --- | --- |
| `Lucen` | Lucen | Lucen | Lucen | Lucen | Lucen | Lucen |
| `Enter the API key issued by Lucen` | 输入 Lucen 签发的 API Key | 輸入 Lucen 簽發的 API Key | Saisissez la clé API fournie par Lucen | Введите API-ключ, выданный Lucen | Lucen が発行した API キーを入力してください | Nhập khóa API do Lucen cấp |
| `Default: https://lucen.asia` | 默认：https://lucen.asia | 預設：https://lucen.asia | Par défaut : https://lucen.asia | По умолчанию: https://lucen.asia | デフォルト: https://lucen.asia | Mặc định: https://lucen.asia |
| `Select Lucen models matching this channel's fixed-duration or token-billing API key` | 选择与本渠道固定秒或 Token 计费 API Key 匹配的 Lucen 模型 | 選擇與此渠道固定秒或 Token 計費 API Key 相符的 Lucen 模型 | Sélectionnez les modèles Lucen correspondant à la clé API à durée fixe ou à facturation par jetons de ce canal | Выберите модели Lucen, соответствующие ключу API этого канала с фиксированной длительностью или оплатой по токенам | このチャネルの固定時間またはトークン課金 API キーに一致する Lucen モデルを選択してください | Chọn các mô hình Lucen khớp với khóa API thời lượng cố định hoặc tính phí theo token của kênh này |
| `Lucen is task-only. Create separate channels for the fixed-duration key and token-billing key.` | Lucen 仅支持任务接口。请分别为固定秒 Key 和 Token 计费 Key 创建渠道。 | Lucen 僅支援任務介面。請分別為固定秒 Key 與 Token 計費 Key 建立渠道。 | Lucen prend uniquement en charge les tâches. Créez des canaux séparés pour la clé à durée fixe et la clé facturée par jetons. | Lucen поддерживает только задачи. Создайте отдельные каналы для ключа с фиксированной длительностью и ключа с оплатой по токенам. | Lucen はタスク専用です。固定時間キーとトークン課金キーには別々のチャネルを作成してください。 | Lucen chỉ hỗ trợ tác vụ. Hãy tạo các kênh riêng cho khóa thời lượng cố định và khóa tính phí theo token. |

The English locale maps every key to itself.

- [ ] **Step 5: Format, test, and commit frontend configuration**

```powershell
Set-Location web
bun test tests/channel-type-config.test.ts
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
bun run build
Set-Location ..
git add web/src/features/channels/constants.ts web/src/features/channels/lib/channel-type-config.ts web/src/features/channels/lib/channel-utils.ts web/tests/channel-type-config.test.ts web/src/i18n/locales/en.json web/src/i18n/locales/zh.json web/src/i18n/locales/zh-TW.json web/src/i18n/locales/fr.json web/src/i18n/locales/ru.json web/src/i18n/locales/ja.json web/src/i18n/locales/vi.json
git commit -m "feat(web): configure Lucen video channels"
```

Expected: all commands exit 0. Do not stage generated untranslated reports unless the repository's i18n tool requires a tracked report update.

---

### Task 6: Prove the Ark Lucen Lifecycle and Optional-Field Behavior

**Files:**
- Create: `e2e/lucen_upstream_e2e_test.go`

- [ ] **Step 1: Create a mock Lucen lifecycle test**

Reuse the same-package helpers from `e2e/newapi_video_upstream_e2e_test.go` and `e2e/seedance_native_e2e_test.go`. The setup must update channel 1 to:

```go
channel.Type = constant.ChannelTypeLucen
channel.Key = "mock-lucen-token-key"
channel.Models = "client-video"
mapping := `{"client-video":"seedance-720p-token"}`
channel.ModelMapping = &mapping
channel.BaseURL = common.GetPointer(server.URL)
require.NoError(t, channel.Update())
```

Submit this exact Ark request through `/api/v3/contents/generations/tasks`:

```json
{
  "model": "client-video",
  "content": [
    {"type":"text","text":"multimodal Lucen acceptance"},
    {"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,QUJDRA=="}},
    {"type":"video_url","role":"reference_video","video_url":{"url":"asset://video-reference-1"}},
    {"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}
  ],
  "resolution": "720p",
  "ratio": "16:9",
  "duration": 10,
  "generate_audio": true,
  "watermark": false,
  "callback_url": "https://client.example/callback",
  "return_last_frame": true,
  "priority": 7,
  "execution_expires_after": 3600
}
```

- [ ] **Step 2: Assert the exact Lucen request contract**

The mock must receive:

- `POST /v1/video/generations`;
- `Authorization: Bearer mock-lucen-token-key`;
- upstream model `seedance-720p-token`;
- top-level prompt `multimodal Lucen acceptance`;
- the full ordered media array with the original `data:` and `asset://` values;
- `"seconds":"10"`, `"generateAudio":true`, and `"watermark":false`;
- no `duration`, `callback_url`, `return_last_frame`, `priority`, or `execution_expires_after` fields.

Use the existing detailed polling fixture with non-zero usage. Assert the task reaches success and stores `BillingTokens == 216900`.

- [ ] **Step 3: Assert Ark single and list queries**

After polling, call:

```text
GET /api/v3/contents/generations/tasks/{public_id}
GET /api/v3/contents/generations/tasks
```

Assert both responses include the public task ID, `model: client-video`, successful status, video URL, and usage. Assert neither response includes `upstream-task`, `seedance-720p-token`, `mock-lucen-token-key`, `channel_id`, `group`, `quota`, or provider-private metadata.

- [ ] **Step 4: Run and commit the Lucen lifecycle test**

```powershell
gofmt -w e2e/lucen_upstream_e2e_test.go
go test ./e2e -run 'TestLucenARKLifecycleE2E' -count=1 -v
git add e2e/lucen_upstream_e2e_test.go
git commit -m "test(video): cover Lucen Ark lifecycle"
```

Expected: PASS with one upstream POST and one detailed polling GET. No live Lucen key is used.

---

### Task 7: Prove Fixed-Duration and Token Lucen Channels Use Existing Profit Routing

**Files:**
- Modify: `e2e/profit_routing_e2e_test.go`

- [ ] **Step 1: Add a two-channel Lucen routing test**

Add `TestLucenProfitRoutingChoosesEligibleFixedOrTokenChannelE2E`. Each table case uses `setupStrictProfitRoutingE2E` with a revenue preview of `1_000_000_000` nano-USD, updates both seeded channels to `ChannelTypeLucen`, replaces the standard policy with one fixed-duration and one Token target, seeds both cost rules, and submits a 720p/10s/16:9 Ark request.

Create these two cases with fresh setup per case:

| Case | Fixed per-request cost | Token cost per 1M | Expected channel/model |
| --- | ---: | ---: | --- |
| token eligible | `2` USD | `1` USD | channel B / `seedance-720p-token` |
| fixed eligible | `0.1` USD | `10` USD | channel A / `seedance-720p-10s` |

Represent the cases in the test as:

```go
tests := []struct {
	name            string
	fixedCost       string
	tokenPerMillion string
	wantChannelID   int
	wantModel       string
}{
	{name: "token eligible", fixedCost: "2", tokenPerMillion: "1", wantChannelID: capabilityChannelB, wantModel: "seedance-720p-token"},
	{name: "fixed eligible", fixedCost: "0.1", tokenPerMillion: "10", wantChannelID: capabilityChannelA, wantModel: "seedance-720p-10s"},
}
for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) {
		env := setupStrictProfitRoutingE2E(t, func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
			return 1_000_000_000, "1000", nil
		})
		require.NoError(t, model.DB.Model(&model.Channel{}).
			Where("id IN ?", []int{capabilityChannelA, capabilityChannelB}).
			Update("type", constant.ChannelTypeLucen).Error)
		request := capabilityPolicyRequest(modelrouting.Seedance20, []service.RouteTargetWriteRequest{
			capabilityTarget(capabilityChannelA, "seedance-720p-10s", 100, []string{"720p"}, discreteDuration(10), []string{"16:9"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false),
			capabilityTarget(capabilityChannelB, "seedance-720p-token", 90, []string{"720p"}, discreteDuration(10), []string{"16:9"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false),
		})
		_, err := service.SaveRoutingPolicy(env.standardPolicy, request)
		require.NoError(t, err)
		fixedPrice := tt.fixedCost
		seedProfitRoutingRuleE2E(t, capabilityChannelA, "seedance-720p-10s", types.CostModePerRequest, types.CostRuleConfigV1{
			Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1", RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1",
			UnitPrice: &fixedPrice, ChargeEvent: types.CostChargeSubmitAccepted,
		})
		seedProfitRoutingTokenRuleE2E(t, capabilityChannelB, "seedance-720p-token", tt.tokenPerMillion)
		body := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 10, "16:9", modelrouting.ReferenceLimits{}, false)
		status, response := performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", body)
		require.Equal(t, http.StatusOK, status, string(response))
		selectedA, selectedB := env.channelA.snapshot(), env.channelB.snapshot()
		if tt.wantChannelID == capabilityChannelA {
			require.Len(t, selectedA, 1)
			assert.Empty(t, selectedB)
			var upstream map[string]interface{}
			require.NoError(t, common.Unmarshal(selectedA[0].Body, &upstream))
			assert.Equal(t, tt.wantModel, upstream["model"])
		} else {
			require.Len(t, selectedB, 1)
			assert.Empty(t, selectedA)
			var upstream map[string]interface{}
			require.NoError(t, common.Unmarshal(selectedB[0].Body, &upstream))
			assert.Equal(t, tt.wantModel, upstream["model"])
		}
		assert.NotContains(t, string(response), tt.wantModel)
		beforeA, beforeB := len(selectedA), len(selectedB)
		mismatchBody := capabilityRequestBody(t, modelrouting.Seedance20, "720p", 5, "16:9", modelrouting.ReferenceLimits{}, false)
		status, response = performProfitRoutingRequest(t, env.engine, "Bearer e2e", "", mismatchBody)
		require.Equal(t, http.StatusServiceUnavailable, status, string(response))
		assert.Len(t, env.channelA.snapshot(), beforeA)
		assert.Len(t, env.channelB.snapshot(), beforeB)
	})
}
```

- [ ] **Step 2: Assert there is no hard-coded billing-group preference**
The table-driven body in Step 1 submits a 720p, 10-second, 16:9 Ark request for each fresh environment. Inspect the two recording servers and assert exactly one received a POST. Decode the body and assert its `model` equals the expected upstream model. Assert the task's private routing snapshot records the selected channel and upstream model while public submit JSON contains neither value. The same body then submits a 5-second request and asserts no additional upstream request; this proves the existing routing policy, rather than the Lucen adapter, filters a `seedance-720p-10s` fixed target.

- [ ] **Step 3: Run and commit profit-routing coverage**

```powershell
gofmt -w e2e/profit_routing_e2e_test.go
go test ./e2e -run 'TestLucenProfitRoutingChoosesEligibleFixedOrTokenChannelE2E|TestProfitRoutingMetadataUnavailableExcludesTokenAndDispatchesNonTokenE2E' -count=1 -v
git add e2e/profit_routing_e2e_test.go
git commit -m "test(routing): cover Lucen billing channel choice"
```

Expected: PASS. The selected Lucen channel changes only because the configured cost makes the other candidate ineligible, not because one key category has a built-in priority.

---

### Task 8: Run Full Verification and Real-Upstream Acceptance

**Files:**
- Modify only files owned by Tasks 1-7 if verification exposes a defect.

- [ ] **Step 1: Run focused backend tests**

```powershell
go test ./relay/common ./middleware ./service ./relay/channel/task/newapivideo ./constant ./relay ./controller -count=1
go test ./e2e -run 'TestLucen|TestNewAPIVideo|TestProfitRouting' -count=1 -v
```

Expected: PASS.

- [ ] **Step 2: Run complete backend verification**

```powershell
go test ./... -count=1
go vet ./...
go build ./...
```

Expected: every command exits 0.

- [ ] **Step 3: Run complete frontend verification**

```powershell
Set-Location web
bun test tests/channel-type-config.test.ts
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
bun run build
Set-Location ..
```

Expected: every command exits 0 and all seven locales contain translated Lucen UI values.

- [ ] **Step 4: Audit channel identity, privacy, and billing boundaries**

```powershell
rg -n 'ChannelTypeLucen|Lucen' constant relay controller service web/src/features/channels web/tests e2e
rg -n 'upstream_task_id|api.?key|channel_id|user_id|"quota"|provider-secret' relay/channel/task/newapivideo e2e/lucen_upstream_e2e_test.go
rg -n 'int\(.*quota|int\(math\.|OtherRatios\[' relay/channel/task/newapivideo service
git diff --check
git status --short
```

Inspect every match. Public response DTOs and assertions must not expose private values. No new bare quota cast or direct `OtherRatios` assignment is permitted. Unrelated pre-existing worktree changes remain untouched.

- [ ] **Step 5: Perform real Lucen acceptance with two temporary keys**

Create two local administrator channels without committing credentials:

1. Lucen fixed-duration channel using the fixed-duration key and the 9 `*-5s`, `*-10s`, `*-15s` models.
2. Lucen token-billing channel using the token key and the 3 `*-token` models.

Configure routing targets for a standard Ark Seedance model. Run:

```text
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{public_id}
GET  /api/v3/contents/generations/tasks
```

Acceptance cases:

- fixed-duration 720p/10s text-to-video reaches `seedance-720p-10s` and produces about 10 seconds;
- token-billing 720p/10s text-to-video reaches `seedance-720p-token`, sends `"seconds":"10"`, and returns non-zero usage;
- mixed reference image + video + audio produces a video with audio and returns the standard Ark model name;
- optional `callback_url`, `return_last_frame`, `priority`, and `execution_expires_after` do not appear in the Lucen upstream body;
- no request uses `ratio: adaptive`.

Record only public task IDs, status transitions, model, final URL presence, duration, and usage. Never record API keys or full signed media URLs in the repository.

- [ ] **Step 6: Review the design contract and avoid an empty commit**

Confirm every requirement in `docs/superpowers/specs/2026-07-26-lucen-channel-design.md` has an automated test or the explicit real-upstream acceptance above. If verification requires a correction, return to the owning task, rerun its focused commands, and commit only that correction. Do not create a verification-only empty commit.
