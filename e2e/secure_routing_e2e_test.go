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
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secureRoutingGroup = "secure-routing"

type secureRoutingRecorder struct {
	mu           sync.Mutex
	taskID       string
	responseCode int
	requests     []secureE2ERequest
}

func (r *secureRoutingRecorder) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	r.mu.Lock()
	r.requests = append(r.requests, secureE2ERequest{
		method:        request.Method,
		path:          request.URL.Path,
		authorization: request.Header.Get("Authorization"),
		contentType:   request.Header.Get("Content-Type"),
		body:          append([]byte(nil), body...),
	})
	responseCode := r.responseCode
	r.mu.Unlock()
	writer.Header().Set("Content-Type", "application/json")
	if responseCode != 0 {
		writer.WriteHeader(responseCode)
		_, _ = writer.Write([]byte(`{"error":{"code":"temporary_upstream_failure","message":"retry later"}}`))
		return
	}
	_, _ = writer.Write([]byte(`{"task_id":"` + r.taskID + `","status":"queued"}`))
}

func (r *secureRoutingRecorder) snapshot() []secureE2ERequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]secureE2ERequest(nil), r.requests...)
}

type secureRoutingEnvironment struct {
	engine     http.Handler
	recorders  map[dto.SecureVideoGroup]*secureRoutingRecorder
	channelIDs map[dto.SecureVideoGroup]int
	keys       map[dto.SecureVideoGroup]string
}

func setupSecureRoutingE2E(t *testing.T) *secureRoutingEnvironment {
	t.Helper()
	t.Cleanup(func() {
		if model.DB != nil {
			require.NoError(t, model.InitRoutingPolicyCache())
		}
	})
	setupSeedanceE2EDB(t)
	require.NoError(t, appI18n.Init())
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
	require.NoError(t, model.DB.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	require.NoError(t, model.InitRoutingPolicyCache())

	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupRatios[secureRoutingGroup] = 1
	encodedGroupRatios, err := common.Marshal(groupRatios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(encodedGroupRatios)))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios)) })

	recorders := map[dto.SecureVideoGroup]*secureRoutingRecorder{}
	servers := map[dto.SecureVideoGroup]*httptest.Server{}
	for _, group := range []dto.SecureVideoGroup{
		dto.SecureVideoGroupDiscount,
		dto.SecureVideoGroupOverseas,
		dto.SecureVideoGroupEnterprise,
	} {
		recorder := &secureRoutingRecorder{taskID: "secure-routing-" + string(group) + "-private"}
		recorders[group] = recorder
		servers[group] = httptest.NewServer(recorder)
		t.Cleanup(servers[group].Close)
	}
	seedSeedanceE2EData(t, servers[dto.SecureVideoGroupDiscount].URL)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", e2eUserID).Update("group", secureRoutingGroup).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("group", secureRoutingGroup).Error)

	channelIDs := map[dto.SecureVideoGroup]int{
		dto.SecureVideoGroupDiscount:   1,
		dto.SecureVideoGroupOverseas:   2,
		dto.SecureVideoGroupEnterprise: 3,
	}
	keys := map[dto.SecureVideoGroup]string{
		dto.SecureVideoGroupDiscount:   "secure-routing-discount-key",
		dto.SecureVideoGroupOverseas:   "secure-routing-overseas-key",
		dto.SecureVideoGroupEnterprise: "secure-routing-enterprise-key",
	}
	priorities := map[dto.SecureVideoGroup]int64{
		dto.SecureVideoGroupDiscount:   300,
		dto.SecureVideoGroupOverseas:   100,
		dto.SecureVideoGroupEnterprise: 200,
	}
	weight := uint(100)
	firstChannel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	for _, group := range []dto.SecureVideoGroup{
		dto.SecureVideoGroupDiscount,
		dto.SecureVideoGroupOverseas,
		dto.SecureVideoGroupEnterprise,
	} {
		channel := &model.Channel{
			Id:          channelIDs[group],
			Type:        constant.ChannelTypeSecure,
			Key:         keys[group],
			Status:      common.ChannelStatusEnabled,
			Name:        "secure-routing-" + string(group),
			Weight:      &weight,
			Priority:    common.GetPointer(priorities[group]),
			BaseURL:     common.GetPointer(servers[group].URL),
			Models:      modelrouting.Seedance20,
			Group:       secureRoutingGroup,
			CreatedTime: time.Now().Unix(),
		}
		channel.SetOtherSettings(dto.ChannelOtherSettings{
			DisableTaskPollingSleep: true,
			SecureVideoGroup:        group,
		})
		if group == dto.SecureVideoGroupDiscount {
			channel.CreatedTime = firstChannel.CreatedTime
			require.NoError(t, channel.Update())
			continue
		}
		require.NoError(t, channel.Insert())
	}

	ratios := ratio_setting.GetModelRatioCopy()
	ratios[modelrouting.Seedance20] = 0.1
	encodedRatios, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encodedRatios)))
	model.InvalidatePricingCache()

	minimum4, maximum15 := 4, 15
	referenceTotal12, referenceVideoAudioTotal3, noReferenceVideoDuration := 12, 3, 0
	discount := service.RouteTargetWriteRequest{
		ChannelID: channelIDs[dto.SecureVideoGroupDiscount], Name: "Secure discount", UpstreamModel: "video-2.0-pro",
		TargetPriority: 300, Enabled: true,
		Constraints: modelrouting.Constraints{
			OutputResolutions: []string{"720p", "1080p", "4k"},
			Durations:         modelrouting.DurationConstraint{Min: &minimum4, Max: &maximum15},
			AspectRatios:      []string{"16:9", "9:16"},
			InputModes: []modelrouting.InputMode{
				modelrouting.InputModeFirstFrame,
				modelrouting.InputModeOmniReference,
			},
			ReferenceMinimums:           modelrouting.ReferenceLimits{Images: 1},
			ReferenceLimits:             modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
			ReferenceTotalMax:           &referenceTotal12,
			ReferenceVideoAudioTotalMax: &referenceVideoAudioTotal3,
		},
	}
	overseas := service.RouteTargetWriteRequest{
		ChannelID: channelIDs[dto.SecureVideoGroupOverseas], Name: "Secure overseas", UpstreamModel: "video-2.0-pro",
		TargetPriority: 100, Enabled: true,
		Constraints: modelrouting.Constraints{
			OutputResolutions: []string{"720p", "1080p"},
			Durations:         modelrouting.DurationConstraint{Min: &minimum4, Max: &maximum15},
			AspectRatios:      []string{"1:1", "4:3", "3:4", "16:9", "9:16", "21:9"},
			InputModes: []modelrouting.InputMode{
				modelrouting.InputModeText,
				modelrouting.InputModeFirstFrame,
				modelrouting.InputModeFirstLastFrames,
				modelrouting.InputModeOmniReference,
			},
			ReferenceLimits:   modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
			ReferenceTotalMax: &referenceTotal12,
		},
	}
	minimum5 := 5
	enterprise := service.RouteTargetWriteRequest{
		ChannelID: channelIDs[dto.SecureVideoGroupEnterprise], Name: "Secure enterprise", UpstreamModel: "video-2.0-pro",
		TargetPriority: 200, Enabled: true,
		Constraints: modelrouting.Constraints{
			OutputResolutions: []string{"720p"},
			Durations:         modelrouting.DurationConstraint{Min: &minimum5, Max: &maximum15},
			AspectRatios:      []string{"16:9", "9:16", "1:1"},
			InputModes: []modelrouting.InputMode{
				modelrouting.InputModeText,
				modelrouting.InputModeFirstFrame,
				modelrouting.InputModeOmniReference,
			},
			ReferenceLimits:                    modelrouting.ReferenceLimits{Images: 9, Audios: 3},
			ReferenceTotalMax:                  &referenceTotal12,
			ReferenceVideoTotalDurationSeconds: &noReferenceVideoDuration,
		},
	}
	previousRouteTargetContractValidator := service.RouteTargetContractValidator
	service.RouteTargetContractValidator = relay.ValidateVideoRouteTargetContract
	t.Cleanup(func() { service.RouteTargetContractValidator = previousRouteTargetContractValidator })
	_, err = service.SaveRoutingPolicy(0, service.RoutingPolicyWriteRequest{
		GroupName: secureRoutingGroup,
		Model:     modelrouting.Seedance20,
		Enabled:   true,
		Defaults: modelrouting.Defaults{
			OutputResolution: "720p",
			DurationSeconds:  8,
			AspectRatio:      "16:9",
		},
		Targets: []service.RouteTargetWriteRequest{discount, overseas, enterprise},
	})
	require.NoError(t, err)

	service.SetVideoMetadataClient(secureE2EVideoMetadataClient{})
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() {
		service.SetVideoMetadataClient(nil)
		service.GetTaskAdaptorFunc = nil
	})
	return &secureRoutingEnvironment{
		engine: seedanceE2ERouter(), recorders: recorders, channelIDs: channelIDs, keys: keys,
	}
}

func TestSecureRoutingUsesOnlyCapabilityMatchingGroupE2E(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		resolution string
		wantGroup  dto.SecureVideoGroup
	}{
		{
			name:       "text uses enterprise and never discount",
			content:    `[{"type":"text","text":"text route"}]`,
			resolution: "720p",
			wantGroup:  dto.SecureVideoGroupEnterprise,
		},
		{
			name: "strict frames use overseas",
			content: `[
				{"type":"text","text":"strict frames"},
				{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.jpg"}},
				{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/last.jpg"}}
			]`,
			resolution: "720p",
			wantGroup:  dto.SecureVideoGroupOverseas,
		},
		{
			name: "4k image uses discount",
			content: `[
				{"type":"text","text":"4k image"},
				{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}}
			]`,
			resolution: "4k",
			wantGroup:  dto.SecureVideoGroupDiscount,
		},
		{
			name: "video-only omni uses overseas",
			content: `[
				{"type":"text","text":"video only"},
				{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}}
			]`,
			resolution: "720p",
			wantGroup:  dto.SecureVideoGroupOverseas,
		},
		{
			name: "image omni prefers discount",
			content: `[
				{"type":"text","text":"image omni"},
				{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}}
			]`,
			resolution: "720p",
			wantGroup:  dto.SecureVideoGroupDiscount,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := setupSecureRoutingE2E(t)
			body := `{"model":"` + modelrouting.Seedance20 + `","content":` + test.content + `,"duration":8,"ratio":"16:9","resolution":"` + test.resolution + `"}`
			status, response := performJSONRequest(
				t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body,
			)
			require.Equal(t, http.StatusOK, status, string(response))
			for group, recorder := range env.recorders {
				requests := recorder.snapshot()
				if group != test.wantGroup {
					assert.Empty(t, requests, "unexpected request sent to %s", group)
					continue
				}
				require.Len(t, requests, 1)
				assert.Equal(t, "Bearer "+env.keys[group], requests[0].authorization)
				expectedPath := "/api/generate-video"
				if group == dto.SecureVideoGroupEnterprise {
					expectedPath = "/v1/videos"
				}
				assert.Equal(t, expectedPath, requests[0].path)
			}

			var submit map[string]any
			require.NoError(t, common.Unmarshal(response, &submit))
			publicID, ok := submit["id"].(string)
			require.True(t, ok)
			assert.True(t, strings.HasPrefix(publicID, "task_"))
			for _, privateValue := range []string{
				string(test.wantGroup),
				env.keys[test.wantGroup],
				"video-2.0-pro",
				"channel_id",
				"secure_video_group",
			} {
				assert.NotContains(t, string(response), privateValue)
			}

			var task model.Task
			require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
			assert.Equal(t, env.channelIDs[test.wantGroup], task.ChannelId)
			require.NotNil(t, task.PrivateData.Routing)
			assert.Equal(t, "video-2.0-pro", task.PrivateData.Routing.UpstreamModel)
		})
	}
}

func TestSecureRoutingRetryStaysWithinCapabilityMatchingGroupsE2E(t *testing.T) {
	env := setupSecureRoutingE2E(t)
	enterprise := env.recorders[dto.SecureVideoGroupEnterprise]
	enterprise.mu.Lock()
	enterprise.responseCode = http.StatusInternalServerError
	enterprise.mu.Unlock()

	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = previousRetryTimes })

	body := `{"model":"` + modelrouting.Seedance20 + `","content":[{"type":"text","text":"retry text route"}],"duration":8,"ratio":"16:9","resolution":"720p"}`
	status, response := performJSONRequest(
		t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body,
	)

	require.Equal(t, http.StatusOK, status, string(response))
	assert.Len(t, env.recorders[dto.SecureVideoGroupEnterprise].snapshot(), 1)
	assert.Len(t, env.recorders[dto.SecureVideoGroupOverseas].snapshot(), 1)
	assert.Empty(t, env.recorders[dto.SecureVideoGroupDiscount].snapshot())
	for _, privateValue := range []string{
		env.keys[dto.SecureVideoGroupEnterprise],
		env.keys[dto.SecureVideoGroupOverseas],
		"temporary_upstream_failure",
		"secure_video_group",
	} {
		assert.NotContains(t, string(response), privateValue)
	}
}
