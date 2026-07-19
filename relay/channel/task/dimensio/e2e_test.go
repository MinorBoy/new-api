package dimensio

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDimensioSeedance20ProtocolE2E(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	models := []struct {
		name            string
		upstreamModel   string
		resolution      string
		resolutionRatio float64
	}{
		{name: "fast_vip_720p", upstreamModel: "jimeng-video-seedance-2.0-fast-vip", resolution: "720p", resolutionRatio: 1},
		{name: "mini_720p", upstreamModel: "jimeng-video-seedance-2.0-mini", resolution: "720p", resolutionRatio: 1},
		{name: "vip_1080p", upstreamModel: "jimeng-video-seedance-2.0-vip", resolution: "1080p", resolutionRatio: 2.5},
	}
	terminalStates := []struct {
		name          string
		dimensioBody  string
		expectedState string
	}{
		{
			name:          "success",
			dimensioBody:  `{"task_id":"dim-upstream","status":"completed","progress":100,"result":{"url":"https://mock.dimensio/video.mp4"}}`,
			expectedState: "succeeded",
		},
		{
			name:          "failure",
			dimensioBody:  `{"task_id":"dim-upstream","status":"failed","error":"视频安全审核不通过，请重试","error_code":"2043"}`,
			expectedState: "failed",
		},
	}

	for _, modelCase := range models {
		for _, terminalCase := range terminalStates {
			t.Run(modelCase.name+"/"+terminalCase.name, func(t *testing.T) {
				const originModel = "doubao-seedance-2-0-260128"
				const publicTaskID = "task_public"
				const upstreamTaskID = "dim-upstream"
				const prompt = "@image_file_1作为主体，参考@video_file_1的动作和@audio_file_1的节奏，镜头缓慢推进"

				arkRequest := fmt.Sprintf(`{
					"model":%q,
					"content":[
						{"type":"image_url","role":"reference_image","image_url":{"url":"https://assets.example/reference.jpg"}},
						{"type":"video_url","role":"reference_video","video_url":{"url":"https://assets.example/reference.mp4"}},
						{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://assets.example/reference.mp3"}},
						{"type":"text","text":%q}
					],
					"duration":6,
					"resolution":%q,
					"ratio":"16:9",
					"intelligent_ratio":false,
					"face_grid":true
				}`, originModel, prompt, modelCase.resolution)

				var capturedSubmitBody []byte
				mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "Bearer mock-dimensio-key", r.Header.Get("Authorization"))
					assert.Equal(t, "application/json", r.Header.Get("Accept"))
					switch {
					case r.Method == http.MethodPost && r.URL.Path == "/v1/videos/generations":
						var err error
						capturedSubmitBody, err = io.ReadAll(r.Body)
						require.NoError(t, err)
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"created":1709123456,"task_id":"dim-upstream","status":"pending"}`))
					case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/tasks/dim-upstream":
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(terminalCase.dimensioBody))
					default:
						http.NotFound(w, r)
					}
				}))
				t.Cleanup(mockServer.Close)

				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(arkRequest))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set(common.KeySeedanceOfficialAPI, true)

				info := &relaycommon.RelayInfo{
					OriginModelName: originModel,
					ChannelMeta: &relaycommon.ChannelMeta{
						ChannelType:    constant.ChannelTypeDimensio,
						ChannelBaseUrl: mockServer.URL,
						ApiKey:         "mock-dimensio-key",
					},
					TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: publicTaskID},
				}
				adaptor := &TaskAdaptor{}
				adaptor.Init(info)

				require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
				info.UpstreamModelName = modelCase.upstreamModel
				require.Nil(t, adaptor.ValidateBillingRequest(c, info))
				requested, durationErr := adaptor.EstimateDurationSeconds(c, info)
				require.Nil(t, durationErr)
				assert.Equal(t, 6, requested)
				ratios := adaptor.EstimateBilling(c, info)
				assert.Equal(t, modelCase.resolutionRatio, ratios["resolution"])
				assert.NotContains(t, ratios, "seconds")

				requestBody, err := adaptor.BuildRequestBody(c, info)
				require.NoError(t, err)
				submitResponse, err := adaptor.DoRequest(c, info, requestBody)
				require.NoError(t, err)
				returnedUpstreamID, submitData, taskErr := adaptor.DoResponse(c, submitResponse, info)
				require.Nil(t, taskErr)
				assert.Equal(t, upstreamTaskID, returnedUpstreamID)
				assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
				assert.NotContains(t, recorder.Body.String(), upstreamTaskID)

				var upstreamRequest map[string]interface{}
				require.NoError(t, common.Unmarshal(capturedSubmitBody, &upstreamRequest))
				assert.Equal(t, modelCase.upstreamModel, upstreamRequest["model"])
				assert.Equal(t, prompt, upstreamRequest["prompt"])
				assert.Equal(t, "omni_reference", upstreamRequest["functionMode"])
				assert.Equal(t, "https://assets.example/reference.jpg", upstreamRequest["image_file_1"])
				assert.Equal(t, "https://assets.example/reference.mp4", upstreamRequest["video_file_1"])
				assert.Equal(t, "https://assets.example/reference.mp3", upstreamRequest["audio_file_1"])
				assert.Equal(t, float64(6), upstreamRequest["duration"])
				assert.Equal(t, modelCase.resolution, upstreamRequest["resolution"])
				assert.Equal(t, "16:9", upstreamRequest["ratio"])
				assert.Equal(t, false, upstreamRequest["intelligent_ratio"])
				assert.Equal(t, true, upstreamRequest["face_grid"])
				assert.NotContains(t, upstreamRequest, "file_paths")

				fetchResponse, err := adaptor.FetchTask(mockServer.URL, "mock-dimensio-key", map[string]any{"task_id": returnedUpstreamID}, "")
				require.NoError(t, err)
				queryData, err := io.ReadAll(fetchResponse.Body)
				require.NoError(t, err)
				require.NoError(t, fetchResponse.Body.Close())
				parsedResult, err := adaptor.ParseTaskResult(queryData)
				require.NoError(t, err)

				task := &model.Task{
					TaskID: publicTaskID, SubmitTime: 1709123456, UpdatedAt: 1709123556,
					Properties:  model.Properties{OriginModelName: originModel, UpstreamModelName: modelCase.upstreamModel},
					PrivateData: model.TaskPrivateData{UpstreamTaskID: upstreamTaskID},
				}
				task.Data = queryData
				arkResponseData, err := adaptor.ConvertToArkVideoTask(task)
				require.NoError(t, err)
				assert.NotContains(t, string(arkResponseData), upstreamTaskID)
				var arkResponse ArkTaskResponse
				require.NoError(t, common.Unmarshal(arkResponseData, &arkResponse))
				assert.Equal(t, publicTaskID, arkResponse.ID)
				assert.Equal(t, originModel, arkResponse.Model)
				assert.Equal(t, terminalCase.expectedState, arkResponse.Status)
				assert.Zero(t, adaptor.AdjustBillingOnComplete(task, parsedResult))

				if terminalCase.name == "success" {
					assert.Equal(t, model.TaskStatusSuccess, parsedResult.Status)
					assert.Equal(t, "https://mock.dimensio/video.mp4", arkResponse.Content.VideoURL)
					assert.Nil(t, arkResponse.Error)
				} else {
					assert.Equal(t, model.TaskStatusFailure, parsedResult.Status)
					require.NotNil(t, arkResponse.Error)
					assert.Equal(t, "2043", arkResponse.Error.Code)
					assert.Equal(t, "视频安全审核不通过，请重试", arkResponse.Error.Message)
				}

				var normalizedSubmit map[string]interface{}
				require.NoError(t, common.Unmarshal(submitData, &normalizedSubmit))
				assert.Equal(t, upstreamTaskID, normalizedSubmit["task_id"])
				t.Logf("ARK request: %s", strings.TrimSpace(arkRequest))
				t.Logf("Dimensio submit request: %s", string(capturedSubmitBody))
				t.Logf("Dimensio terminal response: %s", string(queryData))
				t.Logf("ARK terminal response: %s", string(arkResponseData))
			})
		}
	}
}
