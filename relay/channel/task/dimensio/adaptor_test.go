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
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRequestDefaultsAndRejectsUnsupportedValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func(body string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.KeySeedanceOfficialAPI, true)
		return c
	}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	valid := `{"model":"m","content":[{"type":"text","text":"x"}]}`
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(newContext(valid), info))
	cases := []string{
		`{"model":"m","content":[{"type":"text","text":"x"}],"duration":0}`,
		`{"model":"m","content":[{"type":"text","text":"x"}],"duration":-1}`,
		`{"model":"m","content":[{"type":"text","text":"x"}],"duration":16}`,
		`{"model":"m","content":[{"type":"text","text":"x"}],"duration":3600}`,
		`{"model":"m","content":[{"type":"text","text":"x"}],"ratio":"adaptive"}`,
		`{"model":"m","content":[{"type":"text","text":"x"}],"seed":0}`,
		`{"model":"m","content":[{"type":"text","text":"x"}],"unknown":0}`,
	}
	for _, body := range cases {
		require.NotNil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(newContext(body), &relaycommon.RelayInfo{}))
	}
}

func TestDoResponseReturnsPublicID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"created":1,"task_id":"dim-upstream","status":"pending"}`))}
	taskID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}})
	require.Nil(t, taskErr)
	assert.Equal(t, "dim-upstream", taskID)
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
}

func TestParseTaskErrorPreservesDimensioCode(t *testing.T) {
	err := (&TaskAdaptor{}).ParseTaskError([]byte(`{"code":-2000,"message":"duration invalid"}`), http.StatusBadGateway)
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.Equal(t, "-2000", err.Code)
}

func TestDoResponseMapsPositiveProviderErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"code":1006,"message":"积分不足"}`))}
	taskID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}})
	assert.Empty(t, taskID)
	require.NotNil(t, taskErr)
	assert.Equal(t, "1006", taskErr.Code)
	assert.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
}

func TestParseTaskResultDoesNotAdjustRequestBasedBilling(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{"status":"completed","duration":10,"result":{"url":"https://x/v.mp4"}}`))
	require.NoError(t, err)

	// The provider contract does not define duration on task-query responses.
	// Billing remains based on the validated duration captured at submission.
	task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{ModelRatio: 1, GroupRatio: 1, OtherRatios: map[string]float64{"seconds": 5, "resolution": 1}}}}
	assert.Zero(t, adaptor.AdjustBillingOnComplete(task, result))
	assert.Equal(t, 5.0, task.PrivateData.BillingContext.OtherRatios["seconds"])
}

func TestParseTaskResultMapsProviderErrorEnvelopeToFailure(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"code":-2011,"message":"task expired"}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	assert.Equal(t, "task expired", result.Reason)
	assert.Equal(t, "-2011", result.ErrorCode)
}

func TestParseTaskResultKeepsRateLimitErrorsRetryable(t *testing.T) {
	for _, code := range []int{1057, 121101} {
		result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(fmt.Sprintf(`{"code":%d,"message":"request too frequent"}`, code)))
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "request too frequent")
	}
}

func TestConvertToArkVideoTaskMapsProviderErrorEnvelope(t *testing.T) {
	for _, test := range []struct {
		name, body, code, message string
	}{
		{name: "negative code", body: `{"code":-2011,"message":"task expired"}`, code: "-2011", message: "task expired"},
		{name: "positive code", body: `{"code":1006,"message":"积分不足"}`, code: "1006", message: "积分不足"},
	} {
		t.Run(test.name, func(t *testing.T) {
			task := &model.Task{
				TaskID:     "task_public",
				SubmitTime: 111,
				UpdatedAt:  222,
				Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
				Data:       []byte(test.body),
			}
			data, err := (&TaskAdaptor{}).ConvertToArkVideoTask(task)
			require.NoError(t, err)
			var response ArkTaskResponse
			require.NoError(t, common.Unmarshal(data, &response))
			assert.Equal(t, "task_public", response.ID)
			assert.Equal(t, "failed", response.Status)
			require.NotNil(t, response.Error)
			assert.Equal(t, test.code, response.Error.Code)
			assert.Equal(t, test.message, response.Error.Message)
		})
	}
}

func TestConvertToOpenAIVideoMapsProviderErrorEnvelope(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public",
		Status:     model.TaskStatusFailure,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		Data:       []byte(`{"code":-2011,"message":"task expired"}`),
	}
	data, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(data, &response))
	assert.Equal(t, "-2011", response.Error.Code)
	assert.Equal(t, "task expired", response.Error.Message)
}

func TestDurationBillingUsesValidatedRequestDuration(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := ArkRequest{Duration: common.GetPointer(7), Resolution: "1080p"}
	c.Set("dimensio_ark_request", req)
	c.Set("task_resolution", "1080p")

	requested, taskErr := (&TaskAdaptor{}).EstimateDurationSeconds(c, nil)
	require.Nil(t, taskErr)
	assert.Equal(t, 7, requested)

	ratio := (&TaskAdaptor{}).EstimateBilling(c, nil)
	assert.Equal(t, map[string]float64{"resolution": 2.5}, ratio)
	assert.NotContains(t, ratio, "seconds")
}

func TestEstimateDurationSecondsRejectsInvalidContext(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		set   bool
	}{
		{name: "missing"},
		{name: "wrong type", value: "not-an-ark-request", set: true},
		{name: "missing duration", value: ArkRequest{}, set: true},
		{name: "below provider minimum", value: ArkRequest{Duration: common.GetPointer(3)}, set: true},
		{name: "above provider maximum", value: ArkRequest{Duration: common.GetPointer(16)}, set: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			if test.set {
				c.Set("dimensio_ark_request", test.value)
			}

			requested, taskErr := (&TaskAdaptor{}).EstimateDurationSeconds(c, nil)

			assert.Zero(t, requested)
			require.NotNil(t, taskErr)
			assert.Equal(t, "invalid_duration", taskErr.Code)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		})
	}
}

func TestValidateBillingRequestEnforcesDocumentedModelResolutionMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		model      string
		resolution string
		wantError  bool
	}{
		{name: "fast vip 720p", model: "jimeng-video-seedance-2.0-fast-vip", resolution: "720p"},
		{name: "mini 720p", model: "jimeng-video-seedance-2.0-mini", resolution: "720p"},
		{name: "vip 720p", model: "jimeng-video-seedance-2.0-vip", resolution: "720p"},
		{name: "vip 1080p", model: "jimeng-video-seedance-2.0-vip", resolution: "1080p"},
		{name: "fast vip 1080p", model: "jimeng-video-seedance-2.0-fast-vip", resolution: "1080p", wantError: true},
		{name: "mini 1080p", model: "jimeng-video-seedance-2.0-mini", resolution: "1080p", wantError: true},
		{name: "unknown model", model: "jimeng-video-unknown", resolution: "720p", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_resolution", test.resolution)
			err := (&TaskAdaptor{}).ValidateBillingRequest(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: test.model}})
			if test.wantError {
				require.NotNil(t, err)
				assert.Equal(t, http.StatusBadRequest, err.StatusCode)
				return
			}
			require.Nil(t, err)
		})
	}
}
