package e2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

const (
	h3Upstream720  = "vendor-h3-720p"
	h3Upstream768  = "vendor-h3-768p-pro"
	h3Upstream2K   = "lec-h3video-2k"
	h3UnknownModel = "vendor-h3-unlisted"
)

type h3CapabilityE2EEnv struct {
	engine   http.Handler
	channelA *capabilityRecordingServer
	channelB *capabilityRecordingServer
}

// setupMiniMaxH3CapabilityE2E reuses the Seedance capability harness so H3 runs
// through the same routing policy storage, channel selection and adaptor wiring.
func setupMiniMaxH3CapabilityE2E(t *testing.T) *h3CapabilityE2EEnv {
	t.Helper()
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

	// Only the canonical MiniMax-H3 identity appears in channel model lists. The
	// provider upstream IDs stay inside route targets.
	allModels := modelrouting.MiniMaxH3
	priorityA, priorityB := int64(100), int64(90)
	weight := uint(100)
	firstChannel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	firstChannel.Type = constant.ChannelTypeNewAPIVideo
	firstChannel.Key = "h3-a-key"
	firstChannel.Name = "H3A"
	firstChannel.BaseURL = common.GetPointer(channelAServer.URL)
	firstChannel.Models = allModels
	firstChannel.Group = capabilityGroup
	firstChannel.Priority = &priorityA
	firstChannel.Weight = &weight
	require.NoError(t, firstChannel.Update())

	secondChannel := &model.Channel{
		Id: capabilityChannelB, Type: constant.ChannelTypeNewAPIVideo, Key: "h3-b-key",
		Status: common.ChannelStatusEnabled, Name: "H3B", BaseURL: common.GetPointer(channelBServer.URL),
		Models: allModels, Group: capabilityGroup, Priority: &priorityB, Weight: &weight,
		CreatedTime: time.Now().Unix(), OtherSettings: "{}",
	}
	secondChannel.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	require.NoError(t, secondChannel.Insert())

	ratios := ratio_setting.GetModelRatioCopy()
	// The documented client spelling is MiniMax-H3 while routing stores the
	// canonical lowercase identity; register both so either spelling is billable.
	ratios[modelrouting.MiniMaxH3] = 0.1
	ratios["MiniMax-H3"] = 0.1
	for _, upstreamModel := range []string{h3Upstream720, h3Upstream768, h3Upstream2K, h3UnknownModel} {
		delete(ratios, upstreamModel)
	}
	encodedRatios, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encodedRatios)))
	model.InvalidatePricingCache()

	// Two channels expose genuinely different provider upstream IDs and different
	// capabilities, so the selected channel proves capability filtering happened
	// before the existing weight/priority selection.
	_, err = service.SaveRoutingPolicy(0, capabilityPolicyRequest(modelrouting.MiniMaxH3, []service.RouteTargetWriteRequest{
		h3CapabilityTarget(capabilityChannelA, h3Upstream720, 100, []string{"720p"}, rangeDuration(4, 15), modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		h3CapabilityTarget(capabilityChannelA, h3Upstream2K, 100, []string{"2k"}, rangeDuration(4, 15), modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		h3CapabilityTarget(capabilityChannelB, h3Upstream768, 90, []string{"768p"}, rangeDuration(4, 15), modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 3}),
		h3CapabilityTarget(capabilityChannelB, h3UnknownModel, 110, []string{"2k"}, rangeDuration(4, 15), modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 3}),
	}))
	require.NoError(t, err)

	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })

	return &h3CapabilityE2EEnv{engine: seedanceE2ERouter(), channelA: channelA, channelB: channelB}
}

func h3CapabilityTarget(channelID int, upstreamModel string, priority int, resolutions []string, durations modelrouting.DurationConstraint, references modelrouting.ReferenceLimits) service.RouteTargetWriteRequest {
	total := references.Images + references.Videos + references.Audios
	return service.RouteTargetWriteRequest{
		ChannelID: channelID, Name: upstreamModel, UpstreamModel: upstreamModel, TargetPriority: priority, Enabled: true,
		CostVariantKey: resolutions[0],
		Constraints: modelrouting.Constraints{
			OutputResolutions: resolutions,
			Durations:         durations,
			AspectRatios:      []string{"auto", "16:9"},
			ReferenceLimits:   references,
			ReferenceTotalMax: common.GetPointer(total),
		},
	}
}

// h3CapabilityRequestBody builds an H3 request with publicly addressable media
// URLs, because the H3 protocol rejects private or unresolvable hosts.
func h3CapabilityRequestBody(t *testing.T, clientModel, resolution string, references modelrouting.ReferenceLimits, ratio string) string {
	t.Helper()
	content := []map[string]any{{"type": "text", "text": "capability routing acceptance"}}
	for index := 0; index < references.Images; index++ {
		content = append(content, map[string]any{
			"type": "image_url", "role": "reference_image",
			"image_url": map[string]any{"url": "https://8.8.8.8/image-" + string(rune('a'+index)) + ".png"},
		})
	}
	for index := 0; index < references.Videos; index++ {
		content = append(content, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": "https://8.8.4.4/video-" + string(rune('a'+index)) + ".mp4"},
		})
	}
	for index := 0; index < references.Audios; index++ {
		content = append(content, map[string]any{
			"type": "audio_url", "role": "reference_audio",
			"audio_url": map[string]any{"url": "https://1.1.1.1/audio-" + string(rune('a'+index)) + ".mp3"},
		})
	}
	body := map[string]any{
		"model": clientModel, "content": content, "resolution": resolution, "duration": 10, "ratio": ratio,
	}
	encoded, err := common.Marshal(body)
	require.NoError(t, err)
	return string(encoded)
}

// TestMiniMaxH3CapabilityRoutingMatrixE2E proves the client always sends the
// MiniMax-H3 identity on the new-api video entry, capability routing narrows the
// candidate set to providers that satisfy the request, and the existing channel
// priority/weight selection still decides the final channel.
func TestMiniMaxH3CapabilityRoutingMatrixE2E(t *testing.T) {
	tests := []struct {
		name         string
		clientModel  string
		resolution   string
		ratio        string
		references   modelrouting.ReferenceLimits
		wantChannel  int
		wantUpstream string
		wantStatus   int
		wantCode     string
	}{
		{name: "client spelling MiniMax-H3 reaches the 720p provider", clientModel: "MiniMax-H3", resolution: "720p", ratio: "16:9", wantChannel: capabilityChannelA, wantUpstream: h3Upstream720, wantStatus: http.StatusOK},
		{name: "768p has exactly one compatible provider", clientModel: modelrouting.MiniMaxH3, resolution: "768p", ratio: "16:9", wantChannel: capabilityChannelB, wantUpstream: h3Upstream768, wantStatus: http.StatusOK},
		// Both channels serve 2k, so the second stage keeps the higher channel
		// priority even though the other target has a higher target priority.
		{name: "channel priority decides among compatible providers", clientModel: modelrouting.MiniMaxH3, resolution: "2k", ratio: "16:9", wantChannel: capabilityChannelA, wantUpstream: h3Upstream2K, wantStatus: http.StatusOK},
		// Five images exceed the 4-image provider, so only the larger provider
		// remains and the request must still succeed through it.
		{name: "reference limits narrow the candidate set", clientModel: modelrouting.MiniMaxH3, resolution: "2k", ratio: "16:9", references: modelrouting.ReferenceLimits{Images: 5}, wantChannel: capabilityChannelA, wantUpstream: h3Upstream2K, wantStatus: http.StatusOK},
		{name: "request ratio outside every target has no compatible route", clientModel: modelrouting.MiniMaxH3, resolution: "2k", ratio: "9:16", wantStatus: http.StatusBadRequest, wantCode: "no_compatible_route"},
		{name: "unsupported resolution is rejected by the H3 contract", clientModel: modelrouting.MiniMaxH3, resolution: "1080p", ratio: "16:9", wantStatus: http.StatusBadRequest, wantCode: "InvalidParameter.resolution"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := setupMiniMaxH3CapabilityE2E(t)
			body := h3CapabilityRequestBody(t, test.clientModel, test.resolution, test.references, test.ratio)
			status, response := performJSONRequest(t, env.engine, http.MethodPost, "/v1/video/generations", "Bearer e2e", body)
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
			// The H3 dialect submits to /v1/videos regardless of channel.
			assert.Equal(t, "/v1/videos", selected[0].Path)
			var upstreamBody map[string]any
			require.NoError(t, common.Unmarshal(selected[0].Body, &upstreamBody))
			assert.Equal(t, test.wantUpstream, upstreamBody["model"])
			assert.Equal(t, test.resolution, upstreamBody["resolution"])
			assert.NotContains(t, string(response), test.wantUpstream)

			var task model.Task
			require.NoError(t, model.DB.Order("id DESC").First(&task).Error)
			assert.Equal(t, test.clientModel, task.Properties.OriginModelName)
			require.NotNil(t, task.PrivateData.Routing)
			assert.Equal(t, test.wantUpstream, task.PrivateData.Routing.UpstreamModel)
		})
	}
}


func TestMiniMaxH3RejectsArkNativeEntryE2E(t *testing.T) {
	env := setupMiniMaxH3CapabilityE2E(t)
	body := h3CapabilityRequestBody(t, modelrouting.MiniMaxH3, "2k", modelrouting.ReferenceLimits{}, "16:9")

	status, response := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", body)
	require.Equal(t, http.StatusBadRequest, status, string(response))
	assert.Contains(t, string(response), "only serves Seedance models")
	assert.Empty(t, env.channelA.snapshot())
	assert.Empty(t, env.channelB.snapshot())
}

func TestMiniMaxH3ProviderUpstreamIDsStayPrivateE2E(t *testing.T) {
	env := setupMiniMaxH3CapabilityE2E(t)
	// A client addressing a provider upstream ID directly must not be routed:
	// those IDs are hidden from the public model list and rejected as requests.
	body := h3CapabilityRequestBody(t, h3Upstream2K, "2k", modelrouting.ReferenceLimits{}, "16:9")
	status, response := performJSONRequest(t, env.engine, http.MethodPost, "/v1/video/generations", "Bearer e2e", body)
	require.Equal(t, http.StatusNotFound, status, string(response))
	assert.Contains(t, strings.ToLower(string(response)), "model")
	assert.Empty(t, env.channelA.snapshot())
	assert.Empty(t, env.channelB.snapshot())
}
