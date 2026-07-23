package e2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	capabilityGroup      = "分组A"
	capabilityChannelA   = 1
	capabilityChannelB   = 2
	upstreamStandard1080 = "bb-seedance2.0-1080p-pro-gz-15s"
	upstreamStandard720  = "bb-seedance2.0-720p-pro-gz-15s"
	upstreamStandardMG   = "mg-seedance2.0-720p-pro"
	upstreamFast720      = "bb-seedance2.0-720p-fast-gz-15s"
	upstreamFastMG       = "mg-seedance2.0-720p-fast"
	upstreamMiniMG       = "mg-seedance2.0-720p-mini"
	upstreamUpscaled1080 = "lec-feituo-seedance-2-0-my-upscaled-1080p"
)

var capabilityUpstreamModels = []string{
	upstreamStandard1080,
	upstreamStandard720,
	upstreamStandardMG,
	upstreamFast720,
	upstreamFastMG,
	upstreamMiniMG,
	upstreamUpscaled1080,
}

type capabilityRecordingServer struct {
	mu             sync.Mutex
	requests       []mockArkRequest
	submitStatus   int
	submitResponse string
}

func (m *capabilityRecordingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method: r.Method, Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/video/generations":
		if m.submitStatus != 0 {
			w.WriteHeader(m.submitStatus)
		}
		response := m.submitResponse
		if response == "" {
			response = `{"id":"upstream-task","task_id":"upstream-task","object":"video","status":"queued","progress":0}`
		}
		_, _ = w.Write([]byte(response))
	case r.Method == http.MethodGet && r.URL.Path == "/v1/video/generations/upstream-task":
		_, _ = w.Write([]byte(newAPIVideoPollingResponse))
	default:
		http.NotFound(w, r)
	}
}

func (m *capabilityRecordingServer) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

type seedanceCapabilityE2EEnv struct {
	engine          http.Handler
	channelA        *capabilityRecordingServer
	channelB        *capabilityRecordingServer
	standardPolicy  int
	standardRequest service.RoutingPolicyWriteRequest
}

func setupSeedanceCapabilityRoutingE2E(t *testing.T) *seedanceCapabilityE2EEnv {
	t.Helper()
	// This cleanup is registered first so it runs after setupSeedanceE2EDB restores
	// the original database, leaving the global routing cache bound to that database.
	t.Cleanup(func() {
		if model.DB != nil {
			require.NoError(t, model.InitRoutingPolicyCache())
		}
	})
	setupSeedanceE2EDB(t)
	require.NoError(t, appI18n.Init())
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupRatios[capabilityGroup] = 1
	encodedGroupRatios, err := common.Marshal(groupRatios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(encodedGroupRatios)))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios)) })

	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
	require.NoError(t, model.DB.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	require.NoError(t, model.InitRoutingPolicyCache())

	channelA := &capabilityRecordingServer{}
	channelAServer := httptest.NewServer(channelA)
	t.Cleanup(channelAServer.Close)
	channelB := &capabilityRecordingServer{}
	channelBServer := httptest.NewServer(channelB)
	t.Cleanup(channelBServer.Close)

	seedSeedanceE2EData(t, channelAServer.URL)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", e2eUserID).Update("group", capabilityGroup).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("group", capabilityGroup).Error)

	allModels := strings.Join(modelrouting.CanonicalModels, ",")
	priorityA, priorityB := int64(100), int64(90)
	weight := uint(100)
	firstChannel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	firstChannel.Type = constant.ChannelTypeNewAPIVideo
	firstChannel.Key = "capability-a-key"
	firstChannel.Name = "A1"
	firstChannel.BaseURL = common.GetPointer(channelAServer.URL)
	firstChannel.Models = allModels
	firstChannel.Group = capabilityGroup
	firstChannel.Priority = &priorityA
	firstChannel.Weight = &weight
	legacyMapping := `{"doubao-seedance-2-0-260128":"must-not-be-used"}`
	firstChannel.ModelMapping = &legacyMapping
	require.NoError(t, firstChannel.Update())

	secondChannel := &model.Channel{
		Id: capabilityChannelB, Type: constant.ChannelTypeNewAPIVideo, Key: "capability-b-key",
		Status: common.ChannelStatusEnabled, Name: "A1_copy", BaseURL: common.GetPointer(channelBServer.URL),
		Models: allModels, Group: capabilityGroup, Priority: &priorityB, Weight: &weight,
		CreatedTime: time.Now().Unix(), OtherSettings: "{}",
	}
	secondChannel.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	require.NoError(t, secondChannel.Insert())

	ratios := ratio_setting.GetModelRatioCopy()
	for _, canonicalModel := range modelrouting.CanonicalModels {
		ratios[canonicalModel] = 0.1
	}
	for _, upstreamModel := range capabilityUpstreamModels {
		delete(ratios, upstreamModel)
	}
	encodedRatios, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encodedRatios)))
	model.InvalidatePricingCache()

	standardRequest := capabilityPolicyRequest(modelrouting.Seedance20, []service.RouteTargetWriteRequest{
		capabilityTarget(capabilityChannelA, upstreamStandard1080, 100, []string{"1080p"}, discreteDuration(15), []string{"9:16"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false, false, ""),
		capabilityTarget(capabilityChannelA, upstreamStandard720, 100, []string{"720p"}, discreteDuration(15), []string{"9:16"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false, false, ""),
		capabilityTarget(capabilityChannelB, upstreamStandardMG, 90, []string{"720p"}, rangeDuration(4, 15), nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, true, false, ""),
		capabilityTarget(capabilityChannelB, upstreamUpscaled1080, 110, []string{"1080p"}, rangeDuration(4, 15), nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, true, true, "720p"),
	})
	standard, err := service.SaveRoutingPolicy(0, standardRequest)
	require.NoError(t, err)
	_, err = service.SaveRoutingPolicy(0, capabilityPolicyRequest(modelrouting.Seedance20Fast, []service.RouteTargetWriteRequest{
		capabilityTarget(capabilityChannelA, upstreamFast720, 100, []string{"720p"}, discreteDuration(15), []string{"9:16"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false, false, ""),
		capabilityTarget(capabilityChannelB, upstreamFastMG, 90, []string{"720p"}, rangeDuration(4, 15), nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, true, false, ""),
	}))
	require.NoError(t, err)
	_, err = service.SaveRoutingPolicy(0, capabilityPolicyRequest(modelrouting.Seedance20Mini, []service.RouteTargetWriteRequest{
		capabilityTarget(capabilityChannelB, upstreamMiniMG, 90, []string{"720p"}, rangeDuration(4, 15), nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, true, false, ""),
	}))
	require.NoError(t, err)

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })

	return &seedanceCapabilityE2EEnv{
		engine: seedanceE2ERouter(), channelA: channelA, channelB: channelB,
		standardPolicy: standard.ID, standardRequest: standardRequest,
	}
}

func capabilityPolicyRequest(canonicalModel string, targets []service.RouteTargetWriteRequest) service.RoutingPolicyWriteRequest {
	return service.RoutingPolicyWriteRequest{
		GroupName: capabilityGroup, Model: canonicalModel, Enabled: true,
		Defaults: modelrouting.Defaults{OutputResolution: "720p", DurationSeconds: 10, AspectRatio: "16:9"},
		Targets:  targets,
	}
}

func capabilityTarget(channelID int, upstreamModel string, priority int, resolutions []string, durations modelrouting.DurationConstraint, ratios []string, references modelrouting.ReferenceLimits, supportsRealPerson, upscaled bool, generationResolution string) service.RouteTargetWriteRequest {
	return service.RouteTargetWriteRequest{
		ChannelID: channelID, Name: upstreamModel, UpstreamModel: upstreamModel, TargetPriority: priority, Enabled: true,
		Constraints: modelrouting.Constraints{
			OutputResolutions: resolutions, GenerationResolution: generationResolution, Upscaled: upscaled,
			Durations: durations, AspectRatios: ratios, ReferenceLimits: references,
			SupportsRealPerson: common.GetPointer(supportsRealPerson),
		},
	}
}

func discreteDuration(values ...int) modelrouting.DurationConstraint {
	return modelrouting.DurationConstraint{Values: values}
}

func rangeDuration(minimum, maximum int) modelrouting.DurationConstraint {
	return modelrouting.DurationConstraint{Min: common.GetPointer(minimum), Max: common.GetPointer(maximum)}
}

func capabilityRequestBody(t *testing.T, canonicalModel, resolution string, duration any, ratio string, references modelrouting.ReferenceLimits, requireRealPerson bool) string {
	t.Helper()
	content := []map[string]any{{"type": "text", "text": "capability routing acceptance"}}
	for index := 0; index < references.Images; index++ {
		content = append(content, map[string]any{
			"type": "image_url", "role": "reference_image",
			"image_url": map[string]any{"url": "https://mock.example/image-" + string(rune('a'+index)) + ".png"},
		})
	}
	for index := 0; index < references.Videos; index++ {
		content = append(content, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": "https://mock.example/video-" + string(rune('a'+index)) + ".mp4"},
		})
	}
	for index := 0; index < references.Audios; index++ {
		content = append(content, map[string]any{
			"type": "audio_url", "role": "reference_audio",
			"audio_url": map[string]any{"url": "https://mock.example/audio-" + string(rune('a'+index)) + ".mp3"},
		})
	}
	body := map[string]any{
		"model": canonicalModel, "content": content, "resolution": resolution, "duration": duration, "ratio": ratio,
	}
	if requireRealPerson {
		body["routing"] = map[string]any{"require_real_person": true}
	}
	encoded, err := common.Marshal(body)
	require.NoError(t, err)
	return string(encoded)
}

func TestSeedanceCapabilityRoutingMatrixE2E(t *testing.T) {
	largeReferences := modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}
	mediumReferences := modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}
	tests := []struct {
		name, canonicalModel, resolution, ratio string
		duration                                any
		references                              modelrouting.ReferenceLimits
		requireRealPerson                       bool
		wantChannel                             int
		wantUpstream                            string
		wantStatus                              int
		wantCode                                string
		disableChannels                         bool
	}{
		{name: "standard native 1080", canonicalModel: modelrouting.Seedance20, resolution: "1080p", duration: 15, ratio: "9:16", wantChannel: capabilityChannelA, wantUpstream: upstreamStandard1080, wantStatus: http.StatusOK},
		{name: "standard upscaled 1080", canonicalModel: modelrouting.Seedance20, resolution: "1080p", duration: 10, ratio: "16:9", references: mediumReferences, requireRealPerson: true, wantChannel: capabilityChannelB, wantUpstream: upstreamUpscaled1080, wantStatus: http.StatusOK},
		{name: "standard native 720 with 933", canonicalModel: modelrouting.Seedance20, resolution: "720p", duration: 15, ratio: "9:16", references: largeReferences, wantChannel: capabilityChannelA, wantUpstream: upstreamStandard720, wantStatus: http.StatusOK},
		{name: "standard flexible 720 with 431", canonicalModel: modelrouting.Seedance20, resolution: "720p", duration: 10, ratio: "16:9", references: mediumReferences, requireRealPerson: true, wantChannel: capabilityChannelB, wantUpstream: upstreamStandardMG, wantStatus: http.StatusOK},
		{name: "fast native 720", canonicalModel: modelrouting.Seedance20Fast, resolution: "720p", duration: 15, ratio: "9:16", wantChannel: capabilityChannelA, wantUpstream: upstreamFast720, wantStatus: http.StatusOK},
		{name: "fast flexible 720", canonicalModel: modelrouting.Seedance20Fast, resolution: "720p", duration: 10, ratio: "16:9", requireRealPerson: true, wantChannel: capabilityChannelB, wantUpstream: upstreamFastMG, wantStatus: http.StatusOK},
		{name: "mini flexible 720", canonicalModel: modelrouting.Seedance20Mini, resolution: "720p", duration: 10, ratio: "16:9", requireRealPerson: true, wantChannel: capabilityChannelB, wantUpstream: upstreamMiniMG, wantStatus: http.StatusOK},
		{name: "unsupported 4k", canonicalModel: modelrouting.Seedance20, resolution: "4k", duration: 10, ratio: "16:9", wantStatus: http.StatusBadRequest, wantCode: "no_compatible_route"},
		{name: "smart duration does not match capability target", canonicalModel: modelrouting.Seedance20, resolution: "720p", duration: -1, ratio: "16:9", wantStatus: http.StatusBadRequest, wantCode: "no_compatible_route"},
		{name: "five images exceed only compatible 431 target", canonicalModel: modelrouting.Seedance20, resolution: "720p", duration: 10, ratio: "16:9", references: modelrouting.ReferenceLimits{Images: 5}, requireRealPerson: true, wantStatus: http.StatusBadRequest, wantCode: "no_compatible_route"},
		{name: "malformed duration", canonicalModel: modelrouting.Seedance20, resolution: "720p", duration: "10", ratio: "16:9", wantStatus: http.StatusBadRequest, wantCode: "InvalidParameter.duration"},
		{name: "ten images violate request boundary", canonicalModel: modelrouting.Seedance20, resolution: "720p", duration: 10, ratio: "16:9", references: modelrouting.ReferenceLimits{Images: 10}, wantStatus: http.StatusBadRequest, wantCode: "InvalidParameter.content"},
		{name: "compatible channels disabled", canonicalModel: modelrouting.Seedance20, resolution: "1080p", duration: 15, ratio: "9:16", wantStatus: http.StatusServiceUnavailable, wantCode: "compatible_channel_unavailable", disableChannels: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := setupSeedanceCapabilityRoutingE2E(t)
			if test.disableChannels {
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id IN ?", []int{capabilityChannelA, capabilityChannelB}).Update("status", common.ChannelStatusManuallyDisabled).Error)
				require.NoError(t, model.UpdateAbilityStatus(capabilityChannelA, false))
				require.NoError(t, model.UpdateAbilityStatus(capabilityChannelB, false))
			}

			body := capabilityRequestBody(t, test.canonicalModel, test.resolution, test.duration, test.ratio, test.references, test.requireRealPerson)
			status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)
			require.Equal(t, test.wantStatus, status, string(response))
			if test.wantCode != "" {
				assert.Contains(t, string(response), `"code":"`+test.wantCode+`"`)
				assert.Empty(t, env.channelA.snapshot())
				assert.Empty(t, env.channelB.snapshot())
				return
			}

			var selected []mockArkRequest
			switch test.wantChannel {
			case capabilityChannelA:
				selected = env.channelA.snapshot()
				assert.Empty(t, env.channelB.snapshot())
			case capabilityChannelB:
				selected = env.channelB.snapshot()
				assert.Empty(t, env.channelA.snapshot())
			default:
				t.Fatalf("unexpected channel %d", test.wantChannel)
			}
			require.Len(t, selected, 1)
			assert.Equal(t, http.MethodPost, selected[0].Method)
			assert.Equal(t, "/v1/video/generations", selected[0].Path)
			var upstreamBody map[string]any
			require.NoError(t, common.Unmarshal(selected[0].Body, &upstreamBody))
			assert.Equal(t, test.wantUpstream, upstreamBody["model"])
			assert.NotContains(t, upstreamBody, "routing")
			assert.NotContains(t, string(response), test.wantUpstream)

			var task model.Task
			require.NoError(t, model.DB.Order("id DESC").First(&task).Error)
			assert.Equal(t, test.canonicalModel, task.Properties.OriginModelName)
			assert.Empty(t, task.Properties.UpstreamModelName)
			require.NotNil(t, task.PrivateData.Routing)
			assert.Equal(t, test.wantUpstream, task.PrivateData.Routing.UpstreamModel)
		})
	}
}

func TestSeedanceCapabilityRoutingRetryExcludesFailedChannelE2E(t *testing.T) {
	env := setupSeedanceCapabilityRoutingE2E(t)
	backup := capabilityTarget(capabilityChannelA, "bb-seedance2.0-1080p-pro-backup", 50, []string{"1080p"}, discreteDuration(15), []string{"9:16"}, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, false, false, "")
	env.standardRequest.Targets = append(env.standardRequest.Targets, backup)
	_, err := service.SaveRoutingPolicy(env.standardPolicy, env.standardRequest)
	require.NoError(t, err)
	env.channelA.submitStatus = http.StatusInternalServerError
	env.channelA.submitResponse = `{"code":"upstream_failure","message":"A1 failed"}`

	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })
	body := capabilityRequestBody(t, modelrouting.Seedance20, "1080p", 15, "9:16", modelrouting.ReferenceLimits{}, false)
	status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)
	require.Equal(t, http.StatusOK, status, string(response))
	require.Len(t, env.channelA.snapshot(), 1)
	require.Len(t, env.channelB.snapshot(), 1)

	var task model.Task
	require.NoError(t, model.DB.Order("id DESC").First(&task).Error)
	assert.Equal(t, capabilityChannelB, task.ChannelId)
	assert.Empty(t, task.Properties.UpstreamModelName)
	require.NotNil(t, task.PrivateData.Routing)
	assert.Equal(t, upstreamUpscaled1080, task.PrivateData.Routing.UpstreamModel)
}

func TestSeedanceCapabilityRoutingPrivacyAndBillingE2E(t *testing.T) {
	env := setupSeedanceCapabilityRoutingE2E(t)
	body := capabilityRequestBody(t, modelrouting.Seedance20, "1080p", 10, "16:9", modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, true)
	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitted map[string]any
	require.NoError(t, common.Unmarshal(submit, &submitted))
	publicID, ok := submitted["id"].(string)
	require.True(t, ok)
	assertCapabilityPublicBody(t, submit)

	task := pollNewAPIVideoTask(t, publicID)
	assert.Equal(t, modelrouting.Seedance20, task.Properties.OriginModelName)
	assert.Empty(t, task.Properties.UpstreamModelName)
	require.NotNil(t, task.PrivateData.Routing)
	assert.Equal(t, upstreamUpscaled1080, task.PrivateData.Routing.UpstreamModel)
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Equal(t, modelrouting.Seedance20, task.PrivateData.BillingContext.OriginModelName)
	assert.Equal(t, 0.1, task.PrivateData.BillingContext.ModelRatio)
	assert.Positive(t, task.Quota)

	status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assert.Contains(t, string(single), modelrouting.Seedance20)
	assertCapabilityPublicBody(t, single)
	status, list := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?page_size=20", "Bearer e2e", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assert.Contains(t, string(list), modelrouting.Seedance20)
	assertCapabilityPublicBody(t, list)

	status, modelsBody := performJSONRequest(t, env.engine, http.MethodGet, "/v1/models", "Bearer e2e", "")
	require.Equal(t, http.StatusOK, status, string(modelsBody))
	for _, canonicalModel := range modelrouting.CanonicalModels {
		assert.Contains(t, string(modelsBody), canonicalModel)
	}
	assertCapabilityPublicBody(t, modelsBody)

	userLogs, _, err := model.GetUserLogs(e2eUserID, model.LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, userLogs)
	for _, log := range userLogs {
		assert.Equal(t, modelrouting.Seedance20, log.ModelName)
		assertCapabilityPublicBody(t, []byte(log.Other))
	}

	adminLogs, _, err := model.GetAllLogs(model.LogTypeConsume, 0, 0, modelrouting.Seedance20, "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, adminLogs)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(adminLogs[0].Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]any)
	require.True(t, ok)
	routing, ok := adminInfo["routing"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(task.PrivateData.Routing.PolicyID), routing["policy_id"])
	assert.Equal(t, float64(task.PrivateData.Routing.TargetID), routing["target_id"])
	assert.Equal(t, upstreamUpscaled1080, routing["upstream_model"])
	assert.NotNil(t, routing["facts"])
}

func assertCapabilityPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range capabilityUpstreamModels {
		assert.NotContains(t, string(body), privateValue)
	}
	for _, privateValue := range []string{"policy_id", "target_id", "must-not-be-used"} {
		assert.NotContains(t, string(body), privateValue)
	}
}
