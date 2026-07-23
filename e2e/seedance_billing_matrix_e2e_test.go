package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var seedanceBillingReferenceDurationPattern = regexp.MustCompile(`(?:^|-)reference-(\d+)s(?:-|\.)`)

func silenceSeedanceBillingLogs(t *testing.T) {
	t.Helper()
	common.LogWriterMu.Lock()
	originalWriter := gin.DefaultWriter
	originalErrorWriter := gin.DefaultErrorWriter
	gin.DefaultWriter = io.Discard
	gin.DefaultErrorWriter = io.Discard
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = originalWriter
		gin.DefaultErrorWriter = originalErrorWriter
		common.LogWriterMu.Unlock()
	})
}

type seedanceBillingCase struct {
	ID                string
	Model             string
	Resolution        string
	RequestDuration   *int
	TerminalDuration  int
	VideoDurations    []int
	HasReferenceImage bool
	HasReferenceVideo bool
	GenerateAudio     bool
	ServiceTier       string
	Draft             bool
	CompletionTokens  int
	ExpectedUnitRMB   float64
}

type seedanceBillingMock struct {
	mu             sync.Mutex
	nextID         int
	tasks          map[string]seedanceBillingCase
	requests       []mockArkRequest
	submitObserver func()
}

type seedanceBillingE2EEnv struct {
	Router  http.Handler
	Mock    *seedanceBillingMock
	Server  *httptest.Server
	User    *model.User
	Token   *model.Token
	Channel *model.Channel
}

type seedanceBillingDomainSnapshot struct {
	TaskCount                         int64
	TaskQuota                         int
	UserQuota                         int
	UserUsedQuota                     int
	UserRequestCount                  int
	ChannelUsedQuota                  int64
	TokenRemainQuota                  int
	TokenUsedQuota                    int
	QuotaDataCount                    int
	QuotaDataQuota                    int
	QuotaDataTokenUsed                int
	LogCount                          int64
	ConsumeLogCount                   int
	ConsumeLogQuota                   int
	RefundLogCount                    int
	RefundLogQuota                    int
	SignedLogQuota                    int
	TaskBillingTokens                 int
	SettlementLogCount                int
	SettlementConsumeLogCount         int
	SettlementConsumeLogQuota         int
	SettlementRefundLogCount          int
	SettlementRefundLogQuota          int
	SettlementSignedLogQuota          int
	SettlementLogBillingTokens        int
	SettlementLogBillingTokensPresent bool
	LastLogID                         int
}

type seedanceBillingLogSnapshot struct {
	Type    int
	Quota   int
	Content string
	Other   map[string]any
}

func requireSeedanceBillingARKError(t *testing.T, responseBody []byte, wantCode, wantMessage string) {
	t.Helper()
	for _, internalDetail := range []string{
		"debug", "stack", "trace", "goroutine ", "runtime.", "relay/", "new-api/", "C:\\",
		"json:", "cannot unmarshal", "Go value",
	} {
		require.NotContains(t, string(responseBody), internalDetail)
	}

	var envelope map[string]any
	require.NoError(t, common.Unmarshal(responseBody, &envelope))
	require.Len(t, envelope, 1)
	errorValue, exists := envelope["error"]
	require.True(t, exists)
	errorObject, ok := errorValue.(map[string]any)
	require.True(t, ok)
	require.Len(t, errorObject, 2)

	code, ok := errorObject["code"].(string)
	require.True(t, ok)
	message, ok := errorObject["message"].(string)
	require.True(t, ok)
	require.Equal(t, wantCode, code)
	require.NotEmpty(t, message)
	if wantMessage != "" {
		require.Equal(t, wantMessage, message)
	}
}

func newSeedanceBillingMock() *seedanceBillingMock {
	return &seedanceBillingMock{
		nextID: 1,
		tasks:  make(map[string]seedanceBillingCase),
	}
}

func seedanceBillingReferenceDuration(rawURL string) (int, bool) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" || strings.HasSuffix(parsedURL.Path, "/") {
		return 0, false
	}
	match := seedanceBillingReferenceDurationPattern.FindStringSubmatch(path.Base(parsedURL.Path))
	if len(match) != 2 {
		return 0, false
	}
	duration, err := strconv.Atoi(match[1])
	if err != nil || duration < 2 || duration > 15 {
		return 0, false
	}
	return duration, true
}

func seedanceBillingModelRatios() map[string]float64 {
	return map[string]float64{
		"doubao-seedance-2-0-260128":      46.0 / 14.0,
		"doubao-seedance-2-0-fast-260128": 37.0 / 14.0,
		"doubao-seedance-2-0-mini-260615": 23.0 / 14.0,
		"doubao-seedance-1-5-pro-251215":  8.0 / 14.0,
	}
}

func seedanceBillingOfficialUnitRMB(modelID, resolution string, hasVideo bool) (float64, bool) {
	switch modelID {
	case "doubao-seedance-2-0-260128":
		switch resolution {
		case "480p", "720p":
			if hasVideo {
				return 28, true
			}
			return 46, true
		case "1080p":
			if hasVideo {
				return 31, true
			}
			return 51, true
		case "4k":
			if hasVideo {
				return 16, true
			}
			return 26, true
		}
	case "doubao-seedance-2-0-fast-260128":
		if resolution == "480p" || resolution == "720p" {
			if hasVideo {
				return 22, true
			}
			return 37, true
		}
	case "doubao-seedance-2-0-mini-260615":
		if resolution == "480p" || resolution == "720p" {
			if hasVideo {
				return 14, true
			}
			return 23, true
		}
	case "doubao-seedance-1-5-pro-251215":
		if resolution == "480p" || resolution == "720p" || resolution == "1080p" {
			return 8, true
		}
	}
	return 0, false
}

func seedanceBillingExplicitCases() []seedanceBillingCase {
	testCases := make([]seedanceBillingCase, 0, 636)
	seedance20Models := []struct {
		model       string
		resolutions []string
	}{
		{model: "doubao-seedance-2-0-260128", resolutions: []string{"480p", "720p", "1080p", "4k"}},
		{model: "doubao-seedance-2-0-fast-260128", resolutions: []string{"480p", "720p"}},
		{model: "doubao-seedance-2-0-mini-260615", resolutions: []string{"480p", "720p"}},
	}
	for _, modelConfig := range seedance20Models {
		for _, resolution := range modelConfig.resolutions {
			for duration := 4; duration <= 15; duration++ {
				for _, hasVideo := range []bool{false, true} {
					for _, hasImage := range []bool{false, true} {
						unitRMB, ok := seedanceBillingOfficialUnitRMB(modelConfig.model, resolution, hasVideo)
						if !ok {
							panic("missing explicit Seedance billing oracle")
						}
						testCases = append(testCases, seedanceBillingCase{
							ID:    fmt.Sprintf("%s/%s/duration-%02d/video-%t/image-%t", modelConfig.model, resolution, duration, hasVideo, hasImage),
							Model: modelConfig.model, Resolution: resolution,
							RequestDuration: common.GetPointer(duration), TerminalDuration: duration,
							HasReferenceImage: hasImage, HasReferenceVideo: hasVideo,
							ExpectedUnitRMB: unitRMB,
						})
					}
				}
			}
		}
	}

	const seedance15Model = "doubao-seedance-1-5-pro-251215"
	for _, resolution := range []string{"480p", "720p", "1080p"} {
		for duration := 4; duration <= 12; duration++ {
			for _, hasImage := range []bool{false, true} {
				for _, generateAudio := range []bool{false, true} {
					for _, serviceTier := range []string{"default", "flex"} {
						testCases = append(testCases, seedanceBillingCase{
							ID:    fmt.Sprintf("%s/%s/duration-%02d/image-%t/audio-%t/tier-%s/draft-false", seedance15Model, resolution, duration, hasImage, generateAudio, serviceTier),
							Model: seedance15Model, Resolution: resolution,
							RequestDuration: common.GetPointer(duration), TerminalDuration: duration,
							HasReferenceImage: hasImage, GenerateAudio: generateAudio, ServiceTier: serviceTier,
							ExpectedUnitRMB: 8,
						})
					}
				}
			}
		}
	}
	for duration := 4; duration <= 12; duration++ {
		for _, hasImage := range []bool{false, true} {
			for _, generateAudio := range []bool{false, true} {
				testCases = append(testCases, seedanceBillingCase{
					ID:    fmt.Sprintf("%s/480p/duration-%02d/image-%t/audio-%t/tier-default/draft-true", seedance15Model, duration, hasImage, generateAudio),
					Model: seedance15Model, Resolution: "480p",
					RequestDuration: common.GetPointer(duration), TerminalDuration: duration,
					HasReferenceImage: hasImage, GenerateAudio: generateAudio, ServiceTier: "default", Draft: true,
					ExpectedUnitRMB: 8,
				})
			}
		}
	}
	return testCases
}

func seedanceBillingDurationModeCases() []seedanceBillingCase {
	testCases := make([]seedanceBillingCase, 0, 120)
	appendModeCases := func(base seedanceBillingCase) {
		for _, mode := range []string{"omitted", "smart"} {
			caseCopy := base
			caseCopy.ID = base.ID + "/mode-" + mode
			if mode == "smart" {
				caseCopy.RequestDuration = common.GetPointer(-1)
				caseCopy.TerminalDuration = 7
			} else {
				caseCopy.RequestDuration = nil
				caseCopy.TerminalDuration = 5
			}
			testCases = append(testCases, caseCopy)
		}
	}
	seedance20Models := []struct {
		model       string
		resolutions []string
	}{
		{model: "doubao-seedance-2-0-260128", resolutions: []string{"480p", "720p", "1080p", "4k"}},
		{model: "doubao-seedance-2-0-fast-260128", resolutions: []string{"480p", "720p"}},
		{model: "doubao-seedance-2-0-mini-260615", resolutions: []string{"480p", "720p"}},
	}
	for _, modelConfig := range seedance20Models {
		for _, resolution := range modelConfig.resolutions {
			for _, hasImage := range []bool{false, true} {
				for _, hasVideo := range []bool{false, true} {
					unitRMB, ok := seedanceBillingOfficialUnitRMB(modelConfig.model, resolution, hasVideo)
					if !ok {
						panic("missing duration-mode Seedance billing oracle")
					}
					appendModeCases(seedanceBillingCase{
						ID:    fmt.Sprintf("%s/%s/image-%t/video-%t", modelConfig.model, resolution, hasImage, hasVideo),
						Model: modelConfig.model, Resolution: resolution,
						HasReferenceImage: hasImage, HasReferenceVideo: hasVideo, ExpectedUnitRMB: unitRMB,
					})
				}
			}
		}
	}
	const seedance15Model = "doubao-seedance-1-5-pro-251215"
	for _, resolution := range []string{"480p", "720p", "1080p"} {
		for _, hasImage := range []bool{false, true} {
			for _, generateAudio := range []bool{false, true} {
				for _, serviceTier := range []string{"default", "flex"} {
					appendModeCases(seedanceBillingCase{
						ID:    fmt.Sprintf("%s/%s/image-%t/audio-%t/tier-%s/draft-false", seedance15Model, resolution, hasImage, generateAudio, serviceTier),
						Model: seedance15Model, Resolution: resolution,
						HasReferenceImage: hasImage, GenerateAudio: generateAudio, ServiceTier: serviceTier,
						ExpectedUnitRMB: 8,
					})
				}
			}
		}
	}
	for _, hasImage := range []bool{false, true} {
		for _, generateAudio := range []bool{false, true} {
			appendModeCases(seedanceBillingCase{
				ID:    fmt.Sprintf("%s/480p/image-%t/audio-%t/tier-default/draft-true", seedance15Model, hasImage, generateAudio),
				Model: seedance15Model, Resolution: "480p", HasReferenceImage: hasImage,
				GenerateAudio: generateAudio, ServiceTier: "default", Draft: true, ExpectedUnitRMB: 8,
			})
		}
	}
	return testCases
}

func seedanceBillingReferenceVideoProfiles() [][]int {
	profiles := make([][]int, 0, 312)
	for first := 2; first <= 15; first++ {
		profiles = append(profiles, []int{first})
	}
	for first := 2; first <= 13; first++ {
		for second := 2; first+second <= 15; second++ {
			profiles = append(profiles, []int{first, second})
		}
	}
	for first := 2; first <= 11; first++ {
		for second := 2; first+second <= 13; second++ {
			for third := 2; first+second+third <= 15; third++ {
				profiles = append(profiles, []int{first, second, third})
			}
		}
	}
	return profiles
}

func setupSeedanceBillingE2E(t *testing.T) *seedanceBillingE2EEnv {
	t.Helper()
	setupSeedanceE2EDB(t)

	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 0
	t.Cleanup(func() { common.RetryTimes = originalRetryTimes })

	originalGetTaskAdaptorFunc := service.GetTaskAdaptorFunc
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = originalGetTaskAdaptorFunc })

	originalRatios := ratio_setting.ModelRatio2JSONString()
	ratios := ratio_setting.GetModelRatioCopy()
	for modelID, ratio := range seedanceBillingModelRatios() {
		ratios[modelID] = ratio
	}
	encodedRatios, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encodedRatios)))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalRatios))
	})

	mock := newSeedanceBillingMock()
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)

	user := &model.User{
		Id:       e2eUserID,
		Username: "seedance_billing_e2e_user",
		Password: "e2e-password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Quota:    2_000_000_000,
		Group:    "default",
		AffCode:  "seedance-billing-e2e",
	}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{
		Id:             1,
		UserId:         user.Id,
		Key:            e2eToken,
		Status:         common.TokenStatusEnabled,
		Name:           "seedance-billing-e2e-token",
		RemainQuota:    2_000_000_000,
		UnlimitedQuota: true,
		Group:          "default",
	}
	require.NoError(t, model.DB.Create(token).Error)

	modelIDs := []string{
		"doubao-seedance-2-0-260128",
		"doubao-seedance-2-0-fast-260128",
		"doubao-seedance-2-0-mini-260615",
		"doubao-seedance-1-5-pro-251215",
	}
	channel := &model.Channel{
		Id:            e2eChannelID,
		Type:          constant.ChannelTypeDoubaoVideo,
		Key:           "mock-ark-key",
		Status:        common.ChannelStatusEnabled,
		Name:          "seedance-billing-e2e-mock",
		Weight:        common.GetPointer[uint](1),
		BaseURL:       common.GetPointer(server.URL),
		Models:        strings.Join(modelIDs, ","),
		Group:         "default",
		CreatedTime:   time.Now().Unix(),
		OtherSettings: "{}",
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	require.NoError(t, channel.Insert())

	return &seedanceBillingE2EEnv{
		Router:  seedanceE2ERouter(),
		Mock:    mock,
		Server:  server,
		User:    user,
		Token:   token,
		Channel: channel,
	}
}

func seedanceBillingExpectedQuota(tokens int, modelRatio, finalMultiplier float64) int {
	return common.QuotaFromFloat(float64(tokens) * modelRatio * finalMultiplier)
}

func seedanceBillingExpectedPreConsume(modelRatio, estimatedMultiplier float64) int {
	base := modelRatio / 2 * common.QuotaPerUnit
	return common.QuotaFromFloat(base * estimatedMultiplier)
}

func seedanceBillingDomainSnapshotFor(t *testing.T, env *seedanceBillingE2EEnv, targetTaskIDs ...string) seedanceBillingDomainSnapshot {
	t.Helper()
	var user model.User
	var channel model.Channel
	var token model.Token
	require.NoError(t, model.DB.First(&user, env.User.Id).Error)
	require.NoError(t, model.DB.First(&channel, env.Channel.Id).Error)
	require.NoError(t, model.DB.First(&token, env.Token.Id).Error)

	var tasks []model.Task
	require.NoError(t, model.DB.Where("user_id = ?", env.User.Id).Find(&tasks).Error)
	var quotaDataRows []model.QuotaData
	require.NoError(t, model.DB.Where("user_id = ?", env.User.Id).Find(&quotaDataRows).Error)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ?", env.User.Id).Order("id").Find(&logs).Error)
	targetTaskID := ""
	if len(targetTaskIDs) > 0 && targetTaskIDs[0] != "" {
		targetTaskID = targetTaskIDs[0]
	}

	snapshot := seedanceBillingDomainSnapshot{
		TaskCount:        int64(len(tasks)),
		UserQuota:        user.Quota,
		UserUsedQuota:    user.UsedQuota,
		UserRequestCount: user.RequestCount,
		ChannelUsedQuota: channel.UsedQuota,
		TokenRemainQuota: token.RemainQuota,
		TokenUsedQuota:   token.UsedQuota,
		LogCount:         int64(len(logs)),
	}
	for _, task := range tasks {
		snapshot.TaskQuota += task.Quota
		if task.TaskID == targetTaskID && task.PrivateData.BillingContext != nil {
			snapshot.TaskBillingTokens = task.PrivateData.BillingContext.BillingTokens
		}
	}
	for _, quotaData := range quotaDataRows {
		snapshot.QuotaDataCount += quotaData.Count
		snapshot.QuotaDataQuota += quotaData.Quota
		snapshot.QuotaDataTokenUsed += quotaData.TokenUsed
	}
	model.CacheQuotaDataLock.Lock()
	for _, quotaData := range model.CacheQuotaData {
		if quotaData.UserID != env.User.Id {
			continue
		}
		snapshot.QuotaDataCount += quotaData.Count
		snapshot.QuotaDataQuota += quotaData.Quota
		snapshot.QuotaDataTokenUsed += quotaData.TokenUsed
	}
	model.CacheQuotaDataLock.Unlock()
	for _, log := range logs {
		if log.Id > snapshot.LastLogID {
			snapshot.LastLogID = log.Id
		}
		switch log.Type {
		case model.LogTypeConsume:
			snapshot.ConsumeLogCount++
			snapshot.ConsumeLogQuota += log.Quota
		case model.LogTypeRefund:
			snapshot.RefundLogCount++
			snapshot.RefundLogQuota += log.Quota
		}
		if targetTaskID == "" || log.Other == "" {
			continue
		}
		var other struct {
			TaskID        string `json:"task_id"`
			BillingTokens *int   `json:"billing_tokens"`
		}
		require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
		if other.TaskID != targetTaskID {
			continue
		}
		snapshot.SettlementLogCount++
		switch log.Type {
		case model.LogTypeConsume:
			snapshot.SettlementConsumeLogCount++
			snapshot.SettlementConsumeLogQuota += log.Quota
		case model.LogTypeRefund:
			snapshot.SettlementRefundLogCount++
			snapshot.SettlementRefundLogQuota += log.Quota
		}
		if other.BillingTokens != nil {
			snapshot.SettlementLogBillingTokens = *other.BillingTokens
			snapshot.SettlementLogBillingTokensPresent = true
		}
	}
	snapshot.SignedLogQuota = snapshot.ConsumeLogQuota - snapshot.RefundLogQuota
	snapshot.SettlementSignedLogQuota = snapshot.SettlementConsumeLogQuota - snapshot.SettlementRefundLogQuota
	return snapshot
}

func seedanceBillingLogsAfter(t *testing.T, env *seedanceBillingE2EEnv, lastLogID int) []seedanceBillingLogSnapshot {
	t.Helper()
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND id > ?", env.User.Id, lastLogID).Order("id").Find(&logs).Error)
	result := make([]seedanceBillingLogSnapshot, 0, len(logs))
	for _, log := range logs {
		var other map[string]any
		if log.Other != "" {
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
		}
		result = append(result, seedanceBillingLogSnapshot{Type: log.Type, Quota: log.Quota, Content: log.Content, Other: other})
	}
	return result
}

func seedanceBillingExplicitRequest(t *testing.T, testCase seedanceBillingCase) (map[string]any, []byte) {
	t.Helper()
	content := []any{map[string]any{"type": "text", "text": "Seedance explicit billing acceptance " + testCase.ID}}
	if testCase.HasReferenceImage {
		role := "reference_image"
		if testCase.Model == "doubao-seedance-1-5-pro-251215" {
			role = "first_frame"
		}
		content = append(content, map[string]any{
			"type": "image_url", "role": role,
			"image_url": map[string]any{"url": "https://mock.example/reference.png"},
		})
	}
	if testCase.HasReferenceVideo {
		content = append(content, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": "https://mock.example/reference-2s-1.mp4"},
		})
	}
	request := map[string]any{
		"model": testCase.Model, "content": content, "resolution": testCase.Resolution,
		"duration": *testCase.RequestDuration,
	}
	if testCase.Model == "doubao-seedance-1-5-pro-251215" {
		request["generate_audio"] = testCase.GenerateAudio
		request["service_tier"] = testCase.ServiceTier
		request["draft"] = testCase.Draft
	}
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	return request, encoded
}

func seedanceBillingDurationModeRequest(t *testing.T, testCase seedanceBillingCase) (map[string]any, []byte) {
	t.Helper()
	content := []any{map[string]any{"type": "text", "text": "Seedance duration mode billing acceptance " + testCase.ID}}
	if testCase.HasReferenceImage {
		role := "reference_image"
		if testCase.Model == "doubao-seedance-1-5-pro-251215" {
			role = "first_frame"
		}
		content = append(content, map[string]any{
			"type": "image_url", "role": role,
			"image_url": map[string]any{"url": "https://mock.example/reference.png"},
		})
	}
	if testCase.HasReferenceVideo {
		content = append(content, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": "https://mock.example/reference-2s-1.mp4"},
		})
	}
	request := map[string]any{
		"model": testCase.Model, "content": content, "resolution": testCase.Resolution,
	}
	if testCase.RequestDuration != nil {
		request["duration"] = *testCase.RequestDuration
	}
	if testCase.Model == "doubao-seedance-1-5-pro-251215" {
		request["generate_audio"] = testCase.GenerateAudio
		request["service_tier"] = testCase.ServiceTier
		request["draft"] = testCase.Draft
	}
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	return request, encoded
}

func seedanceBillingReferenceVideoRequest(t *testing.T, profile []int, hasReferenceImage bool, caseID string) (map[string]any, []byte) {
	t.Helper()
	content := []any{map[string]any{"type": "text", "text": "Seedance reference video profile billing acceptance " + caseID}}
	for index, duration := range profile {
		content = append(content, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": fmt.Sprintf("https://mock.example/reference-%ds-%d.mp4", duration, index+1)},
		})
	}
	if hasReferenceImage {
		content = append(content, map[string]any{
			"type": "image_url", "role": "reference_image",
			"image_url": map[string]any{"url": "https://mock.example/reference.png"},
		})
	}
	request := map[string]any{
		"model": "doubao-seedance-2-0-260128", "content": content,
		"resolution": "720p", "duration": 5,
	}
	encoded, err := common.Marshal(request)
	require.NoError(t, err, caseID)
	var normalized map[string]any
	require.NoError(t, common.Unmarshal(encoded, &normalized), caseID)
	return normalized, encoded
}

func seedanceBillingExpectedRatios(testCase seedanceBillingCase, includeDraftEstimate bool) (float64, map[string]float64) {
	if testCase.Model != "doubao-seedance-1-5-pro-251215" {
		baseRMB := map[string]float64{
			"doubao-seedance-2-0-260128": 46, "doubao-seedance-2-0-fast-260128": 37,
			"doubao-seedance-2-0-mini-260615": 23,
		}[testCase.Model]
		multiplier := testCase.ExpectedUnitRMB / baseRMB
		if multiplier == 1 {
			return multiplier, map[string]float64{}
		}
		return multiplier, map[string]float64{"video_input": multiplier}
	}

	multiplier := 1.0
	ratios := map[string]float64{}
	if testCase.GenerateAudio {
		multiplier *= 2
		ratios["audio"] = 2
	}
	if testCase.ServiceTier == "flex" {
		multiplier *= 0.5
		ratios["service_tier"] = 0.5
	}
	if testCase.Draft && includeDraftEstimate {
		draftEstimate := 0.7
		if testCase.GenerateAudio {
			draftEstimate = 0.6
		}
		multiplier *= draftEstimate
		ratios["draft_estimate"] = draftEstimate
	}
	return multiplier, ratios
}

func (after seedanceBillingDomainSnapshot) delta(before seedanceBillingDomainSnapshot) seedanceBillingDomainSnapshot {
	return seedanceBillingDomainSnapshot{
		TaskCount:                         after.TaskCount - before.TaskCount,
		TaskQuota:                         after.TaskQuota - before.TaskQuota,
		UserQuota:                         after.UserQuota - before.UserQuota,
		UserUsedQuota:                     after.UserUsedQuota - before.UserUsedQuota,
		UserRequestCount:                  after.UserRequestCount - before.UserRequestCount,
		ChannelUsedQuota:                  after.ChannelUsedQuota - before.ChannelUsedQuota,
		TokenRemainQuota:                  after.TokenRemainQuota - before.TokenRemainQuota,
		TokenUsedQuota:                    after.TokenUsedQuota - before.TokenUsedQuota,
		QuotaDataCount:                    after.QuotaDataCount - before.QuotaDataCount,
		QuotaDataQuota:                    after.QuotaDataQuota - before.QuotaDataQuota,
		QuotaDataTokenUsed:                after.QuotaDataTokenUsed - before.QuotaDataTokenUsed,
		LogCount:                          after.LogCount - before.LogCount,
		ConsumeLogCount:                   after.ConsumeLogCount - before.ConsumeLogCount,
		ConsumeLogQuota:                   after.ConsumeLogQuota - before.ConsumeLogQuota,
		RefundLogCount:                    after.RefundLogCount - before.RefundLogCount,
		RefundLogQuota:                    after.RefundLogQuota - before.RefundLogQuota,
		SignedLogQuota:                    after.SignedLogQuota - before.SignedLogQuota,
		TaskBillingTokens:                 after.TaskBillingTokens - before.TaskBillingTokens,
		SettlementLogCount:                after.SettlementLogCount - before.SettlementLogCount,
		SettlementConsumeLogCount:         after.SettlementConsumeLogCount - before.SettlementConsumeLogCount,
		SettlementConsumeLogQuota:         after.SettlementConsumeLogQuota - before.SettlementConsumeLogQuota,
		SettlementRefundLogCount:          after.SettlementRefundLogCount - before.SettlementRefundLogCount,
		SettlementRefundLogQuota:          after.SettlementRefundLogQuota - before.SettlementRefundLogQuota,
		SettlementSignedLogQuota:          after.SettlementSignedLogQuota - before.SettlementSignedLogQuota,
		SettlementLogBillingTokens:        after.SettlementLogBillingTokens - before.SettlementLogBillingTokens,
		SettlementLogBillingTokensPresent: after.SettlementLogBillingTokensPresent && !before.SettlementLogBillingTokensPresent,
	}
}

func seedanceBillingAssertDomainDelta(t *testing.T, expected, before, after seedanceBillingDomainSnapshot) {
	t.Helper()
	require.Equal(t, expected, after.delta(before))
}

func seedanceBillingExpectedDomainDelta(preConsume, finalQuota, completionTokens int) seedanceBillingDomainSnapshot {
	expected := seedanceBillingDomainSnapshot{
		TaskCount: 1, TaskQuota: finalQuota,
		UserQuota: -finalQuota, UserUsedQuota: finalQuota, UserRequestCount: 1,
		ChannelUsedQuota: int64(finalQuota), TokenRemainQuota: -finalQuota, TokenUsedQuota: finalQuota,
		QuotaDataCount: 1, QuotaDataQuota: finalQuota, QuotaDataTokenUsed: completionTokens,
		TaskBillingTokens: completionTokens, SignedLogQuota: finalQuota,
	}
	quotaDelta := finalQuota - preConsume
	switch {
	case quotaDelta > 0:
		expected.LogCount = 2
		expected.ConsumeLogCount = 2
		expected.ConsumeLogQuota = finalQuota
		expected.SettlementLogCount = 1
		expected.SettlementConsumeLogCount = 1
		expected.SettlementConsumeLogQuota = quotaDelta
		expected.SettlementSignedLogQuota = quotaDelta
		expected.SettlementLogBillingTokens = completionTokens
		expected.SettlementLogBillingTokensPresent = true
	case quotaDelta < 0:
		expected.LogCount = 2
		expected.ConsumeLogCount = 1
		expected.ConsumeLogQuota = preConsume
		expected.RefundLogCount = 1
		expected.RefundLogQuota = -quotaDelta
		expected.SettlementLogCount = 1
		expected.SettlementRefundLogCount = 1
		expected.SettlementRefundLogQuota = -quotaDelta
		expected.SettlementSignedLogQuota = quotaDelta
		expected.SettlementLogBillingTokens = completionTokens
		expected.SettlementLogBillingTokensPresent = true
	default:
		expected.LogCount = 1
		expected.ConsumeLogCount = 1
		expected.ConsumeLogQuota = preConsume
	}
	return expected
}

func (m *seedanceBillingMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"InvalidParameter.content","message":"failed to read request content"}}`))
		return
	}
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method:        r.Method,
		Path:          r.URL.Path,
		Authorization: r.Header.Get("Authorization"),
		Body:          append([]byte(nil), body...),
	})
	submitObserver := m.submitObserver
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks" {
		if submitObserver != nil {
			submitObserver()
		}
		var upstreamRequest struct {
			Model   string `json:"model"`
			Content []struct {
				Type     string `json:"type"`
				Role     string `json:"role"`
				ImageURL *struct {
					URL string `json:"url"`
				} `json:"image_url"`
				VideoURL *struct {
					URL string `json:"url"`
				} `json:"video_url"`
			} `json:"content"`
			Resolution    string `json:"resolution"`
			Ratio         string `json:"ratio"`
			Duration      *int   `json:"duration"`
			GenerateAudio *bool  `json:"generate_audio"`
			ServiceTier   string `json:"service_tier"`
			Draft         bool   `json:"draft"`
		}
		if err := common.DecodeJson(bytes.NewReader(body), &upstreamRequest); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"InvalidParameter.content","message":"request content must be valid JSON"}}`))
			return
		}

		videoCount := 0
		videoTotal := 0
		videoDurations := make([]int, 0, 3)
		hasReferenceImage := false
		invalidContent := ""
		for _, item := range upstreamRequest.Content {
			if item.ImageURL != nil && (item.Role == "reference_image" || item.Role == "first_frame") {
				hasReferenceImage = true
			}
			if item.VideoURL == nil {
				continue
			}
			videoCount++
			duration, ok := seedanceBillingReferenceDuration(item.VideoURL.URL)
			if !ok {
				invalidContent = "each video_url.url must encode a duration from 2 to 15 seconds"
				break
			}
			videoTotal += duration
			videoDurations = append(videoDurations, duration)
		}
		if invalidContent == "" && videoCount > 3 {
			invalidContent = "content supports at most three video_url items"
		}
		if invalidContent == "" && videoTotal > 15 {
			invalidContent = "total reference video duration must not exceed 15 seconds"
		}
		if invalidContent != "" {
			encodedError, marshalErr := common.Marshal(map[string]any{
				"error": map[string]string{"code": "InvalidParameter.content", "message": invalidContent},
			})
			if marshalErr != nil {
				http.Error(w, marshalErr.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(encodedError)
			return
		}

		effectiveDuration := 5
		if upstreamRequest.Duration != nil {
			switch {
			case *upstreamRequest.Duration == -1:
				effectiveDuration = 7
			case *upstreamRequest.Duration > 0:
				effectiveDuration = *upstreamRequest.Duration
			}
		}
		completionTokens := 100000 + effectiveDuration*1000 + videoTotal*100 + videoCount*10
		if hasReferenceImage {
			completionTokens++
		}
		resolution := upstreamRequest.Resolution
		if resolution == "" {
			resolution = "720p"
		}
		serviceTier := upstreamRequest.ServiceTier
		if serviceTier == "" {
			serviceTier = "default"
		}
		generateAudio := true
		if upstreamRequest.GenerateAudio != nil {
			generateAudio = *upstreamRequest.GenerateAudio
		}

		m.mu.Lock()
		taskID := fmt.Sprintf("cgt-billing-%d", m.nextID)
		m.nextID++
		var requestDuration *int
		if upstreamRequest.Duration != nil {
			requestDuration = common.GetPointer(*upstreamRequest.Duration)
		}
		m.tasks[taskID] = seedanceBillingCase{
			ID:                taskID,
			Model:             upstreamRequest.Model,
			Resolution:        resolution,
			RequestDuration:   requestDuration,
			TerminalDuration:  effectiveDuration,
			VideoDurations:    append([]int(nil), videoDurations...),
			HasReferenceImage: hasReferenceImage,
			GenerateAudio:     generateAudio,
			ServiceTier:       serviceTier,
			Draft:             upstreamRequest.Draft,
			CompletionTokens:  completionTokens,
		}
		m.mu.Unlock()
		encodedResponse, err := common.Marshal(map[string]string{"id": taskID})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(encodedResponse)
		return
	}

	const taskPathPrefix = "/api/v3/contents/generations/tasks/"
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, taskPathPrefix) {
		taskID := strings.TrimPrefix(r.URL.Path, taskPathPrefix)
		m.mu.Lock()
		task, ok := m.tasks[taskID]
		m.mu.Unlock()
		if !ok {
			encodedError, marshalErr := common.Marshal(map[string]any{
				"error": map[string]string{"code": "InvalidParameter.TaskNotFound", "message": "task does not exist"},
			})
			if marshalErr != nil {
				http.Error(w, marshalErr.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(encodedError)
			return
		}
		const createdAt = int64(1780000000)
		encodedResponse, err := common.Marshal(map[string]any{
			"id":                      task.ID,
			"model":                   task.Model,
			"status":                  "succeeded",
			"content":                 map[string]string{"video_url": "http://" + r.Host + "/videos/" + task.ID + ".mp4"},
			"usage":                   map[string]int{"completion_tokens": task.CompletionTokens, "total_tokens": task.CompletionTokens + 97},
			"created_at":              createdAt,
			"updated_at":              createdAt + 1,
			"seed":                    int64(900000 + len(task.ID)),
			"resolution":              task.Resolution,
			"ratio":                   "16:9",
			"duration":                task.TerminalDuration,
			"framespersecond":         24,
			"service_tier":            task.ServiceTier,
			"execution_expires_after": 172800,
			"generate_audio":          task.GenerateAudio,
			"draft":                   task.Draft,
			"priority":                0,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(encodedResponse)
		return
	}

	http.NotFound(w, r)
}

func (m *seedanceBillingMock) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	for index, request := range m.requests {
		requests[index] = request
		requests[index].Body = append([]byte(nil), request.Body...)
	}
	return requests
}

func (m *seedanceBillingMock) taskSnapshot(taskID string) (seedanceBillingCase, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[taskID]
	task.VideoDurations = append([]int(nil), task.VideoDurations...)
	if task.RequestDuration != nil {
		task.RequestDuration = common.GetPointer(*task.RequestDuration)
	}
	return task, ok
}

func (m *seedanceBillingMock) taskCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tasks)
}

func (m *seedanceBillingMock) setSubmitObserver(observer func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submitObserver = observer
}

func TestSeedanceBillingMockReferenceDuration(t *testing.T) {
	testCases := []struct {
		name     string
		rawURL   string
		expected int
		valid    bool
	}{
		{name: "two seconds", rawURL: "https://mock.example/media/reference-2s-1.mp4", expected: 2, valid: true},
		{name: "fifteen seconds", rawURL: "https://mock.example/media/reference-15s-3.mp4", expected: 15, valid: true},
		{name: "missing duration", rawURL: "https://mock.example/media/reference.mp4"},
		{name: "zero seconds", rawURL: "https://mock.example/media/reference-0s.mp4"},
		{name: "missing extension delimiter", rawURL: "https://mock.example/media/reference-1s"},
		{name: "one second", rawURL: "https://mock.example/media/reference-1s.mp4"},
		{name: "sixteen seconds", rawURL: "https://mock.example/media/reference-16s.mp4"},
		{name: "query spoof", rawURL: "https://mock.example/media/reference.mp4?file=reference-8s-1.mp4"},
		{name: "parent path spoof", rawURL: "https://mock.example/reference-8s-1.mp4/media/reference.mp4"},
		{name: "invalid URL", rawURL: "://mock.example/reference-8s-1.mp4"},
		{name: "relative URL", rawURL: "media/reference-8s-1.mp4"},
		{name: "non HTTP URL", rawURL: "file:///media/reference-8s-1.mp4"},
		{name: "empty final segment", rawURL: "https://mock.example/media/reference-8s-1.mp4/"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			duration, ok := seedanceBillingReferenceDuration(testCase.rawURL)

			assert.Equal(t, testCase.valid, ok)
			assert.Equal(t, testCase.expected, duration)
		})
	}
}

func TestSeedanceBillingMockCreateAndGet(t *testing.T) {
	mock := newSeedanceBillingMock()
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)

	requestBody := map[string]any{
		"model": "doubao-seedance-2-0-260128",
		"content": []any{
			map[string]any{"type": "text", "text": "billing mock"},
			map[string]any{"type": "image_url", "role": "first_frame", "image_url": map[string]any{"url": "https://mock.example/reference.jpg"}},
			map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://mock.example/reference-2s-1.mp4"}},
			map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://mock.example/reference-3s-2.mp4"}},
		},
		"resolution":     "1080p",
		"ratio":          "16:9",
		"duration":       -1,
		"generate_audio": true,
		"service_tier":   "flex",
		"draft":          true,
	}
	encodedRequest, err := common.Marshal(requestBody)
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v3/contents/generations/tasks", bytes.NewReader(encodedRequest))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer mock-ark-key")
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(responseBody))

	var createResponse struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(responseBody, &createResponse))
	assert.Equal(t, "cgt-billing-1", createResponse.ID)

	requests := mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/api/v3/contents/generations/tasks", requests[0].Path)
	assert.Equal(t, "Bearer mock-ark-key", requests[0].Authorization)
	assert.Equal(t, encodedRequest, requests[0].Body)
	requests[0].Body[0] = '!'
	assert.Equal(t, encodedRequest, mock.snapshot()[0].Body)
	storedCase, ok := mock.taskSnapshot(createResponse.ID)
	require.True(t, ok)
	assert.Equal(t, createResponse.ID, storedCase.ID)
	assert.Equal(t, "doubao-seedance-2-0-260128", storedCase.Model)
	assert.Equal(t, "1080p", storedCase.Resolution)
	assert.Equal(t, common.GetPointer(-1), storedCase.RequestDuration)
	assert.Equal(t, 7, storedCase.TerminalDuration)
	assert.Equal(t, []int{2, 3}, storedCase.VideoDurations)
	assert.True(t, storedCase.HasReferenceImage)
	assert.True(t, storedCase.GenerateAudio)
	assert.Equal(t, "flex", storedCase.ServiceTier)
	assert.True(t, storedCase.Draft)
	assert.Equal(t, 107521, storedCase.CompletionTokens)
	*storedCase.RequestDuration = 99
	storedAgain, ok := mock.taskSnapshot(createResponse.ID)
	require.True(t, ok)
	assert.Equal(t, common.GetPointer(-1), storedAgain.RequestDuration)

	response, err = server.Client().Get(server.URL + "/api/v3/contents/generations/tasks/" + createResponse.ID)
	require.NoError(t, err)
	defer response.Body.Close()
	responseBody, err = io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(responseBody))

	var taskResponse struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		CreatedAt             int64  `json:"created_at"`
		UpdatedAt             int64  `json:"updated_at"`
		Seed                  int64  `json:"seed"`
		Resolution            string `json:"resolution"`
		Ratio                 string `json:"ratio"`
		Duration              int    `json:"duration"`
		FramesPerSecond       int    `json:"framespersecond"`
		ServiceTier           string `json:"service_tier"`
		ExecutionExpiresAfter int    `json:"execution_expires_after"`
		GenerateAudio         bool   `json:"generate_audio"`
		Draft                 bool   `json:"draft"`
		Priority              int    `json:"priority"`
	}
	require.NoError(t, common.Unmarshal(responseBody, &taskResponse))
	assert.Equal(t, createResponse.ID, taskResponse.ID)
	assert.Equal(t, "doubao-seedance-2-0-260128", taskResponse.Model)
	assert.Equal(t, "succeeded", taskResponse.Status)
	assert.Equal(t, server.URL+"/videos/"+createResponse.ID+".mp4", taskResponse.Content.VideoURL)
	assert.Equal(t, 107521, taskResponse.Usage.CompletionTokens)
	assert.NotEqual(t, taskResponse.Usage.CompletionTokens, taskResponse.Usage.TotalTokens)
	assert.Positive(t, taskResponse.CreatedAt)
	assert.GreaterOrEqual(t, taskResponse.UpdatedAt, taskResponse.CreatedAt)
	assert.Positive(t, taskResponse.Seed)
	assert.Equal(t, "1080p", taskResponse.Resolution)
	assert.Equal(t, "16:9", taskResponse.Ratio)
	assert.Equal(t, 7, taskResponse.Duration)
	assert.Equal(t, 24, taskResponse.FramesPerSecond)
	assert.Equal(t, "flex", taskResponse.ServiceTier)
	assert.Equal(t, 172800, taskResponse.ExecutionExpiresAfter)
	assert.True(t, taskResponse.GenerateAudio)
	assert.True(t, taskResponse.Draft)
	assert.Zero(t, taskResponse.Priority)

	requests = mock.snapshot()
	require.Len(t, requests, 2)
	assert.Equal(t, http.MethodGet, requests[1].Method)
	assert.Equal(t, "/api/v3/contents/generations/tasks/"+createResponse.ID, requests[1].Path)
}

func TestSeedanceBillingMockRejectsInvalidContent(t *testing.T) {
	testCases := []struct {
		name   string
		videos []string
	}{
		{name: "more than three videos", videos: []string{"reference-2s-1.mp4", "reference-2s-2.mp4", "reference-2s-3.mp4", "reference-2s-4.mp4"}},
		{name: "total duration over fifteen", videos: []string{"reference-8s-1.mp4", "reference-8s-2.mp4"}},
		{name: "duration omitted", videos: []string{"reference.mp4"}},
		{name: "duration below range", videos: []string{"reference-1s-1.mp4"}},
		{name: "duration above range", videos: []string{"reference-16s-1.mp4"}},
		{name: "query spoof", videos: []string{"reference.mp4?name=reference-8s-1.mp4"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mock := newSeedanceBillingMock()
			server := httptest.NewServer(mock)
			t.Cleanup(server.Close)
			content := []any{map[string]any{"type": "text", "text": "invalid content"}}
			for _, video := range testCase.videos {
				content = append(content, map[string]any{
					"type":      "video_url",
					"role":      "reference_video",
					"video_url": map[string]any{"url": "https://mock.example/media/" + video},
				})
			}
			encodedRequest, err := common.Marshal(map[string]any{"model": "doubao-seedance-2-0-260128", "content": content})
			require.NoError(t, err)
			response, err := server.Client().Post(server.URL+"/api/v3/contents/generations/tasks", "application/json", bytes.NewReader(encodedRequest))
			require.NoError(t, err)
			defer response.Body.Close()
			responseBody, err := io.ReadAll(response.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, response.StatusCode, string(responseBody))
			var envelope struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(responseBody, &envelope))
			assert.Equal(t, "InvalidParameter.content", envelope.Error.Code)
			assert.NotEmpty(t, envelope.Error.Message)
			assert.Len(t, mock.snapshot(), 1)
		})
	}
}

func TestSeedanceBillingMockUnknownTask(t *testing.T) {
	mock := newSeedanceBillingMock()
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)

	response, err := server.Client().Get(server.URL + "/api/v3/contents/generations/tasks/cgt-billing-unknown")
	require.NoError(t, err)
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, response.StatusCode, string(responseBody))
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(responseBody, &envelope))
	assert.Equal(t, "InvalidParameter.TaskNotFound", envelope.Error.Code)
}

func TestSeedanceBillingMockControlPresence(t *testing.T) {
	models := []string{
		"doubao-seedance-2-0-260128",
		"doubao-seedance-2-0-fast-260128",
		"doubao-seedance-2-0-mini-260615",
		"doubao-seedance-1-5-pro-251215",
	}
	testCases := []struct {
		name                     string
		generateAudio            *bool
		requestDuration          *int
		expectedGenerateAudio    bool
		expectedTerminalDuration int
	}{
		{name: "omitted", expectedGenerateAudio: true, expectedTerminalDuration: 5},
		{name: "explicit false and smart duration", generateAudio: common.GetPointer(false), requestDuration: common.GetPointer(-1), expectedTerminalDuration: 7},
		{name: "explicit true and positive duration", generateAudio: common.GetPointer(true), requestDuration: common.GetPointer(9), expectedGenerateAudio: true, expectedTerminalDuration: 9},
	}

	for _, modelID := range models {
		for _, testCase := range testCases {
			t.Run(modelID+"/"+testCase.name, func(t *testing.T) {
				mock := newSeedanceBillingMock()
				server := httptest.NewServer(mock)
				t.Cleanup(server.Close)
				requestBody := map[string]any{
					"model":   modelID,
					"content": []any{map[string]any{"type": "text", "text": "control presence"}},
				}
				if testCase.generateAudio != nil {
					requestBody["generate_audio"] = *testCase.generateAudio
				}
				if testCase.requestDuration != nil {
					requestBody["duration"] = *testCase.requestDuration
				}
				encodedRequest, err := common.Marshal(requestBody)
				require.NoError(t, err)
				response, err := server.Client().Post(server.URL+"/api/v3/contents/generations/tasks", "application/json", bytes.NewReader(encodedRequest))
				require.NoError(t, err)
				responseBody, err := io.ReadAll(response.Body)
				response.Body.Close()
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, response.StatusCode, string(responseBody))
				var created struct {
					ID string `json:"id"`
				}
				require.NoError(t, common.Unmarshal(responseBody, &created))

				storedCase, ok := mock.taskSnapshot(created.ID)
				require.True(t, ok)
				assert.Equal(t, testCase.requestDuration, storedCase.RequestDuration)
				assert.Equal(t, testCase.expectedGenerateAudio, storedCase.GenerateAudio)
				assert.Equal(t, testCase.expectedTerminalDuration, storedCase.TerminalDuration)

				response, err = server.Client().Get(server.URL + "/api/v3/contents/generations/tasks/" + created.ID)
				require.NoError(t, err)
				responseBody, err = io.ReadAll(response.Body)
				response.Body.Close()
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, response.StatusCode, string(responseBody))
				var terminal struct {
					Duration      int  `json:"duration"`
					GenerateAudio bool `json:"generate_audio"`
				}
				require.NoError(t, common.Unmarshal(responseBody, &terminal))
				assert.Equal(t, testCase.expectedTerminalDuration, terminal.Duration)
				assert.Equal(t, testCase.expectedGenerateAudio, terminal.GenerateAudio)
			})
		}
	}
}

func TestSeedanceBillingInvalidCombinationsE2E(t *testing.T) {
	silenceSeedanceBillingLogs(t)
	env := setupSeedanceBillingE2E(t)
	type invalidHTTPCase struct {
		id       string
		request  map[string]any
		wantCode string
	}

	textContent := []any{map[string]any{"type": "text", "text": "local invalid billing acceptance"}}
	models := []struct {
		id          string
		maxDuration int
	}{
		{id: "doubao-seedance-2-0-260128", maxDuration: 15},
		{id: "doubao-seedance-2-0-fast-260128", maxDuration: 15},
		{id: "doubao-seedance-2-0-mini-260615", maxDuration: 15},
		{id: "doubao-seedance-1-5-pro-251215", maxDuration: 12},
	}
	invalidCases := make([]invalidHTTPCase, 0, 38)
	for _, modelID := range []string{"doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615"} {
		for _, resolution := range []string{"1080p", "4k"} {
			invalidCases = append(invalidCases, invalidHTTPCase{
				id: fmt.Sprintf("model=%s/resolution=%s", modelID, resolution),
				request: map[string]any{
					"model": modelID, "content": textContent, "resolution": resolution,
				},
				wantCode: "InvalidParameter",
			})
		}
	}
	for _, modelConfig := range models {
		for _, duration := range []int{0, 3, modelConfig.maxDuration + 1, -2} {
			wantDurationCode := "InvalidParameter"
			if strings.HasPrefix(modelConfig.id, "doubao-seedance-2-0-") && (duration == 0 || duration < -1) {
				wantDurationCode = "InvalidParameter.duration"
			}
			invalidCases = append(invalidCases, invalidHTTPCase{
				id: fmt.Sprintf("model=%s/duration=%d", modelConfig.id, duration),
				request: map[string]any{
					"model": modelConfig.id, "content": textContent, "duration": duration,
				},
				wantCode: wantDurationCode,
			})
		}
	}

	fourVideos := []any{map[string]any{"type": "text", "text": "four reference videos must remain local"}}
	for index := 0; index < 4; index++ {
		fourVideos = append(fourVideos, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": fmt.Sprintf("https://mock.example/reference-2s-%d.mp4", index+1)},
		})
	}
	invalidCases = append(invalidCases, invalidHTTPCase{
		id: "model=doubao-seedance-2-0-260128/reference_videos=4",
		request: map[string]any{
			"model": "doubao-seedance-2-0-260128", "content": fourVideos,
		},
		wantCode: "InvalidParameter.content",
	})

	invalidCases = append(invalidCases,
		invalidHTTPCase{
			id: "model=doubao-seedance-1-5-pro-251215/content=reference_image",
			request: map[string]any{
				"model": "doubao-seedance-1-5-pro-251215",
				"content": []any{
					map[string]any{"type": "text", "text": "unsupported reference image"},
					map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://mock.example/reference.png"}},
				},
			},
			wantCode: "InvalidParameter",
		},
		invalidHTTPCase{
			id: "model=doubao-seedance-1-5-pro-251215/content=reference_video",
			request: map[string]any{
				"model": "doubao-seedance-1-5-pro-251215",
				"content": []any{
					map[string]any{"type": "text", "text": "unsupported reference video"},
					map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "https://mock.example/reference-2s.mp4"}},
				},
			},
			wantCode: "InvalidParameter",
		},
		invalidHTTPCase{
			id: "model=doubao-seedance-1-5-pro-251215/content=reference_audio",
			request: map[string]any{
				"model": "doubao-seedance-1-5-pro-251215",
				"content": []any{
					map[string]any{"type": "text", "text": "unsupported reference audio"},
					map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://mock.example/reference.png"}},
					map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "https://mock.example/reference.wav"}},
				},
			},
			wantCode: "InvalidParameter",
		},
	)

	for _, modelConfig := range models[:3] {
		invalidCases = append(invalidCases,
			invalidHTTPCase{
				id: fmt.Sprintf("model=%s/service_tier=flex", modelConfig.id),
				request: map[string]any{
					"model": modelConfig.id, "content": textContent, "service_tier": "flex",
				},
				wantCode: "InvalidParameter",
			},
			invalidHTTPCase{
				id: fmt.Sprintf("model=%s/draft=true", modelConfig.id),
				request: map[string]any{
					"model": modelConfig.id, "content": textContent, "draft": true,
				},
				wantCode: "InvalidParameter",
			},
		)
	}
	invalidCases = append(invalidCases,
		invalidHTTPCase{
			id: "model=doubao-seedance-1-5-pro-251215/draft=true/resolution=720p",
			request: map[string]any{
				"model": "doubao-seedance-1-5-pro-251215", "content": textContent, "draft": true, "resolution": "720p",
			},
			wantCode: "InvalidParameter",
		},
		invalidHTTPCase{
			id: "model=doubao-seedance-1-5-pro-251215/draft=true/resolution=1080p",
			request: map[string]any{
				"model": "doubao-seedance-1-5-pro-251215", "content": textContent, "draft": true, "resolution": "1080p",
			},
			wantCode: "InvalidParameter",
		},
		invalidHTTPCase{
			id: "model=doubao-seedance-1-5-pro-251215/draft=true/service_tier=flex",
			request: map[string]any{
				"model": "doubao-seedance-1-5-pro-251215", "content": textContent, "draft": true, "service_tier": "flex",
			},
			wantCode: "InvalidParameter",
		},
		invalidHTTPCase{
			id: "content=video_url/role=first_frame",
			request: map[string]any{
				"model":   "doubao-seedance-2-0-260128",
				"content": []any{map[string]any{"type": "video_url", "role": "first_frame", "video_url": map[string]any{"url": "https://mock.example/reference-2s.mp4"}}},
			},
			wantCode: "InvalidParameter.content",
		},
		invalidHTTPCase{
			id: "content=audio_url/role=reference_video",
			request: map[string]any{
				"model":   "doubao-seedance-2-0-260128",
				"content": []any{map[string]any{"type": "audio_url", "role": "reference_video", "audio_url": map[string]any{"url": "https://mock.example/reference.wav"}}},
			},
			wantCode: "InvalidParameter.content",
		},
		invalidHTTPCase{
			id: "content=image_url/role=reference_video",
			request: map[string]any{
				"model":   "doubao-seedance-2-0-260128",
				"content": []any{map[string]any{"type": "image_url", "role": "reference_video", "image_url": map[string]any{"url": "https://mock.example/reference.png"}}},
			},
			wantCode: "InvalidParameter.content",
		},
		invalidHTTPCase{
			id: "generate_audio=malformed_string",
			request: map[string]any{
				"model": "doubao-seedance-1-5-pro-251215", "content": textContent, "generate_audio": "not-a-bool",
			},
			wantCode: "InvalidParameter",
		},
		invalidHTTPCase{
			id: "draft=malformed_object",
			request: map[string]any{
				"model": "doubao-seedance-1-5-pro-251215", "content": textContent, "draft": map[string]any{"value": true},
			},
			wantCode: "InvalidParameter",
		},
	)

	require.Len(t, invalidCases, 38)
	seen := make(map[string]struct{}, len(invalidCases))
	executed := 0
	for _, testCase := range invalidCases {
		caseID := testCase.id
		require.NotEmpty(t, caseID)
		_, duplicate := seen[caseID]
		require.False(t, duplicate, caseID)
		seen[caseID] = struct{}{}

		before := seedanceBillingDomainSnapshotFor(t, env)
		requestsBefore := env.Mock.snapshot()
		mockTasksBefore := env.Mock.taskCount()
		requestBody, err := common.Marshal(testCase.request)
		require.NoError(t, err, caseID)
		status, responseBody := performJSONRequest(t, env.Router, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", string(requestBody))
		require.Equal(t, http.StatusBadRequest, status, "%s: %s", caseID, responseBody)
		requireSeedanceBillingARKError(t, responseBody, testCase.wantCode, "")
		assert.Len(t, env.Mock.snapshot(), len(requestsBefore), caseID)
		assert.Equal(t, mockTasksBefore, env.Mock.taskCount(), caseID)
		after := seedanceBillingDomainSnapshotFor(t, env)
		require.Equal(t, before, after, caseID)
		if t.Failed() {
			t.FailNow()
		}
		executed++
	}
	require.Len(t, seen, 38)
	require.Equal(t, 38, executed)
	t.Logf("local_invalid_cases=%d", executed)
}

func TestSeedanceBillingUpstreamDurationRefundE2E(t *testing.T) {
	silenceSeedanceBillingLogs(t)
	env := setupSeedanceBillingE2E(t)
	type submitObservation struct {
		calls            int
		queryErr         error
		userQuota        int
		userUsedQuota    int
		userRequestCount int
		tokenRemainQuota int
		tokenUsedQuota   int
		channelUsedQuota int64
	}
	testCases := []struct {
		id          string
		videoURLs   []string
		wantMessage string
	}{
		{
			id:          "one-1s",
			videoURLs:   []string{"https://mock.example/reference-1s.mp4"},
			wantMessage: "each video_url.url must encode a duration from 2 to 15 seconds",
		},
		{
			id:          "one-16s",
			videoURLs:   []string{"https://mock.example/reference-16s.mp4"},
			wantMessage: "each video_url.url must encode a duration from 2 to 15 seconds",
		},
		{
			id:          "two-8s-plus-8s",
			videoURLs:   []string{"https://mock.example/reference-8s-1.mp4", "https://mock.example/reference-8s-2.mp4"},
			wantMessage: "total reference video duration must not exceed 15 seconds",
		},
		{
			id:          "three-6s-plus-5s-plus-5s",
			videoURLs:   []string{"https://mock.example/reference-6s-1.mp4", "https://mock.example/reference-5s-2.mp4", "https://mock.example/reference-5s-3.mp4"},
			wantMessage: "total reference video duration must not exceed 15 seconds",
		},
	}

	require.Len(t, testCases, 4)
	expectedPreConsume := seedanceBillingExpectedPreConsume(46.0/14.0, 28.0/46.0)
	require.Equal(t, 500000, expectedPreConsume)
	seen := make(map[string]struct{}, len(testCases))
	executed := 0
	for _, testCase := range testCases {
		caseID := testCase.id
		_, duplicate := seen[caseID]
		require.False(t, duplicate, caseID)
		seen[caseID] = struct{}{}

		content := []any{map[string]any{"type": "text", "text": "upstream duration refund " + caseID}}
		for _, videoURL := range testCase.videoURLs {
			content = append(content, map[string]any{
				"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": videoURL},
			})
		}
		requestBody, err := common.Marshal(map[string]any{
			"model": "doubao-seedance-2-0-260128", "content": content, "resolution": "720p", "duration": 5,
		})
		require.NoError(t, err, caseID)
		before := seedanceBillingDomainSnapshotFor(t, env)
		requestsBefore := env.Mock.snapshot()
		mockTasksBefore := env.Mock.taskCount()
		var observationMu sync.Mutex
		observed := submitObservation{}
		env.Mock.setSubmitObserver(func() {
			current := submitObservation{calls: 1}
			var user model.User
			var token model.Token
			var channel model.Channel
			if err := model.DB.First(&user, env.User.Id).Error; err != nil {
				current.queryErr = fmt.Errorf("query pre-refund user: %w", err)
			} else if err := model.DB.First(&token, env.Token.Id).Error; err != nil {
				current.queryErr = fmt.Errorf("query pre-refund token: %w", err)
			} else if err := model.DB.First(&channel, env.Channel.Id).Error; err != nil {
				current.queryErr = fmt.Errorf("query pre-refund channel: %w", err)
			} else {
				current.userQuota = user.Quota
				current.userUsedQuota = user.UsedQuota
				current.userRequestCount = user.RequestCount
				current.tokenRemainQuota = token.RemainQuota
				current.tokenUsedQuota = token.UsedQuota
				current.channelUsedQuota = channel.UsedQuota
			}
			observationMu.Lock()
			observed = current
			observationMu.Unlock()
		})

		status, responseBody := performJSONRequest(t, env.Router, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", string(requestBody))
		env.Mock.setSubmitObserver(nil)
		require.Equal(t, http.StatusBadRequest, status, "%s: %s", caseID, responseBody)
		observationMu.Lock()
		observation := observed
		observationMu.Unlock()
		require.NoError(t, observation.queryErr, caseID)
		require.Equal(t, 1, observation.calls, caseID)
		assert.Equal(t, before.UserQuota-expectedPreConsume, observation.userQuota, caseID)
		assert.Equal(t, before.TokenRemainQuota-expectedPreConsume, observation.tokenRemainQuota, caseID)
		assert.Equal(t, before.UserUsedQuota, observation.userUsedQuota, caseID)
		assert.Equal(t, before.UserRequestCount, observation.userRequestCount, caseID)
		assert.Equal(t, before.TokenUsedQuota+expectedPreConsume, observation.tokenUsedQuota, caseID)
		assert.Equal(t, before.ChannelUsedQuota, observation.channelUsedQuota, caseID)
		requireSeedanceBillingARKError(t, responseBody, "InvalidParameter.content", testCase.wantMessage)

		requestsAfter := env.Mock.snapshot()
		require.Len(t, requestsAfter, len(requestsBefore)+1, caseID)
		submitRequest := requestsAfter[len(requestsBefore)]
		assert.Equal(t, http.MethodPost, submitRequest.Method, caseID)
		assert.Equal(t, "/api/v3/contents/generations/tasks", submitRequest.Path, caseID)
		assert.Equal(t, "Bearer mock-ark-key", submitRequest.Authorization, caseID)
		assert.Equal(t, mockTasksBefore, env.Mock.taskCount(), caseID)
		after := seedanceBillingDomainSnapshotFor(t, env)
		require.Equal(t, before, after, caseID)
		if t.Failed() {
			t.FailNow()
		}
		executed++
	}
	require.Len(t, seen, 4)
	require.Equal(t, 4, executed)
	t.Logf("upstream_refund_cases=%d", executed)
}

func TestSeedanceBillingModelRatioNormalization(t *testing.T) {
	ratios := seedanceBillingModelRatios()

	require.Len(t, ratios, 4)
	assert.InDelta(t, 46.0/14.0, ratios["doubao-seedance-2-0-260128"], 1e-12)
	assert.InDelta(t, 37.0/14.0, ratios["doubao-seedance-2-0-fast-260128"], 1e-12)
	assert.InDelta(t, 23.0/14.0, ratios["doubao-seedance-2-0-mini-260615"], 1e-12)
	assert.InDelta(t, 8.0/14.0, ratios["doubao-seedance-1-5-pro-251215"], 1e-12)
}

func TestSeedanceBillingExplicitCaseCount(t *testing.T) {
	testCases := seedanceBillingExplicitCases()
	require.Len(t, testCases, 636)

	countsByModel := make(map[string]int)
	countsByFamily := map[string]int{"2.0": 0, "1.5_non_draft": 0, "1.5_draft": 0}
	ids := make(map[string]struct{}, len(testCases))
	draftCount := 0
	for _, testCase := range testCases {
		countsByModel[testCase.Model]++
		_, duplicate := ids[testCase.ID]
		assert.False(t, duplicate, testCase.ID)
		ids[testCase.ID] = struct{}{}
		if testCase.Draft {
			draftCount++
			countsByFamily["1.5_draft"]++
		} else if testCase.Model == "doubao-seedance-1-5-pro-251215" {
			countsByFamily["1.5_non_draft"]++
		} else {
			countsByFamily["2.0"]++
		}
	}
	assert.Equal(t, map[string]int{
		"doubao-seedance-2-0-260128":      192,
		"doubao-seedance-2-0-fast-260128": 96,
		"doubao-seedance-2-0-mini-260615": 96,
		"doubao-seedance-1-5-pro-251215":  252,
	}, countsByModel)
	assert.Equal(t, 36, draftCount)
	assert.Equal(t, map[string]int{"2.0": 384, "1.5_non_draft": 216, "1.5_draft": 36}, countsByFamily)
	assert.Len(t, ids, 636)
}

func TestSeedanceBillingDurationModeCaseCount(t *testing.T) {
	testCases := seedanceBillingDurationModeCases()
	require.Len(t, testCases, 120)
	ids := make(map[string]struct{}, len(testCases))
	countsByFamily := map[string]int{"2.0": 0, "1.5_non_draft": 0, "1.5_draft": 0}
	for _, testCase := range testCases {
		_, duplicate := ids[testCase.ID]
		assert.False(t, duplicate, testCase.ID)
		ids[testCase.ID] = struct{}{}
		if testCase.Draft {
			countsByFamily["1.5_draft"]++
		} else if testCase.Model == "doubao-seedance-1-5-pro-251215" {
			countsByFamily["1.5_non_draft"]++
		} else {
			countsByFamily["2.0"]++
		}
	}
	assert.Equal(t, map[string]int{"2.0": 64, "1.5_non_draft": 48, "1.5_draft": 8}, countsByFamily)
	assert.Len(t, ids, 120)
}

func TestSeedanceBillingReferenceVideoProfileCount(t *testing.T) {
	profiles := seedanceBillingReferenceVideoProfiles()
	require.Len(t, profiles, 312)
	assert.Equal(t, []int{2}, profiles[0])
	assert.Equal(t, []int{15}, profiles[13])
	assert.Equal(t, []int{2, 2}, profiles[14])
	assert.Equal(t, []int{13, 2}, profiles[91])
	assert.Equal(t, []int{2, 2, 2}, profiles[92])
	assert.Equal(t, []int{11, 2, 2}, profiles[311])

	countsByVideoCount := map[int]int{}
	seen := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		countsByVideoCount[len(profile)]++
		profileID := fmt.Sprint(profile)
		_, duplicate := seen[profileID]
		assert.False(t, duplicate, profileID)
		seen[profileID] = struct{}{}
		for _, duration := range profile {
			assert.GreaterOrEqual(t, duration, 2, profileID)
			assert.LessOrEqual(t, duration, 15, profileID)
		}
		totalDuration := 0
		for _, duration := range profile {
			totalDuration += duration
		}
		assert.LessOrEqual(t, totalDuration, 15, profileID)
	}
	assert.Equal(t, map[int]int{1: 14, 2: 78, 3: 220}, countsByVideoCount)
	assert.Len(t, seen, 312)
}

func TestSeedanceBillingDurationModesE2E(t *testing.T) {
	silenceSeedanceBillingLogs(t)
	env := setupSeedanceBillingE2E(t)
	testCases := seedanceBillingDurationModeCases()
	executed := 0
	type durationModeResult struct {
		preConsume int
		quota      int
		tokens     int
		duration   int
	}
	resultsByClass := make(map[string]durationModeResult, len(testCases)/2)
	for _, testCase := range testCases {
		caseID := testCase.ID
		before := seedanceBillingDomainSnapshotFor(t, env)
		requestsBefore := env.Mock.snapshot()
		_, requestBody := seedanceBillingDurationModeRequest(t, testCase)
		var expectedRequest map[string]any
		require.NoError(t, common.Unmarshal(requestBody, &expectedRequest), caseID)
		status, createBody := performJSONRequest(t, env.Router, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", string(requestBody))
		require.Equal(t, http.StatusOK, status, "%s: %s", caseID, createBody)
		var created struct {
			ID string `json:"id"`
		}
		require.NoError(t, common.Unmarshal(createBody, &created), caseID)
		require.NotEmpty(t, created.ID, caseID)
		assert.True(t, strings.HasPrefix(created.ID, "task_"), caseID)

		requests := env.Mock.snapshot()
		require.Len(t, requests, len(requestsBefore)+1, caseID)
		submitRequest := requests[len(requestsBefore)]
		assert.Equal(t, http.MethodPost, submitRequest.Method, caseID)
		assert.Equal(t, "/api/v3/contents/generations/tasks", submitRequest.Path, caseID)
		assert.Equal(t, "Bearer mock-ark-key", submitRequest.Authorization, caseID)
		var actualRequest map[string]any
		require.NoError(t, common.Unmarshal(submitRequest.Body, &actualRequest), caseID)
		require.Equal(t, expectedRequest, actualRequest, caseID)
		if testCase.RequestDuration == nil {
			assert.NotContains(t, actualRequest, "duration", caseID)
		} else {
			assert.Equal(t, float64(-1), actualRequest["duration"], caseID)
		}

		var task model.Task
		require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error, caseID)
		upstreamTaskID := task.PrivateData.UpstreamTaskID
		require.NotEmpty(t, upstreamTaskID, caseID)
		mockTask, ok := env.Mock.taskSnapshot(upstreamTaskID)
		require.True(t, ok, caseID)
		assert.Equal(t, testCase.RequestDuration, mockTask.RequestDuration, caseID)
		assert.Equal(t, testCase.TerminalDuration, mockTask.TerminalDuration, caseID)
		assert.Equal(t, testCase.HasReferenceImage, mockTask.HasReferenceImage, caseID)
		if testCase.HasReferenceVideo {
			assert.Equal(t, []int{2}, mockTask.VideoDurations, caseID)
		} else {
			assert.Empty(t, mockTask.VideoDurations, caseID)
		}

		completionTokens := 100000 + testCase.TerminalDuration*1000
		if testCase.HasReferenceVideo {
			completionTokens += 210
		}
		if testCase.HasReferenceImage {
			completionTokens++
		}
		require.Equal(t, completionTokens, mockTask.CompletionTokens, caseID)
		modelRatio := seedanceBillingModelRatios()[testCase.Model]
		estimatedMultiplier, estimatedRatios := seedanceBillingExpectedRatios(testCase, true)
		finalMultiplier, finalRatios := seedanceBillingExpectedRatios(testCase, false)
		for ratioName, ratio := range estimatedRatios {
			assert.Positive(t, ratio, "%s estimated ratio %s", caseID, ratioName)
		}
		for ratioName, ratio := range finalRatios {
			assert.Positive(t, ratio, "%s final ratio %s", caseID, ratioName)
		}
		expectedPreConsume := seedanceBillingExpectedPreConsume(modelRatio, estimatedMultiplier)
		expectedFinalQuota := seedanceBillingExpectedQuota(completionTokens, modelRatio, finalMultiplier)
		require.Equal(t, expectedPreConsume, task.Quota, caseID)
		require.NotNil(t, task.PrivateData.BillingContext, caseID)
		actualEstimatedRatios := task.PrivateData.BillingContext.OtherRatios
		if actualEstimatedRatios == nil {
			actualEstimatedRatios = map[string]float64{}
		}
		assert.Equal(t, estimatedRatios, actualEstimatedRatios, caseID)
		assert.Zero(t, task.PrivateData.BillingContext.BillingTokens, caseID)

		summary := service.RunTaskPollingOnce(context.Background(), nil)
		require.Equal(t, 1, summary.UnfinishedTasks, caseID)
		require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error, caseID)
		assert.Equal(t, string(model.TaskStatusSuccess), string(task.Status), caseID)
		require.Equal(t, expectedFinalQuota, task.Quota, caseID)
		require.NotNil(t, task.PrivateData.BillingContext, caseID)
		assert.Equal(t, completionTokens, task.PrivateData.BillingContext.BillingTokens, caseID)
		actualFinalRatios := task.PrivateData.BillingContext.OtherRatios
		if actualFinalRatios == nil {
			actualFinalRatios = map[string]float64{}
		}
		assert.Equal(t, finalRatios, actualFinalRatios, caseID)
		assert.NotContains(t, finalRatios, "draft_estimate", caseID)

		status, terminalBody := performJSONRequest(t, env.Router, http.MethodGet, "/api/v3/contents/generations/tasks/"+created.ID, "Bearer e2e-1", "")
		require.Equal(t, http.StatusOK, status, "%s: %s", caseID, terminalBody)
		var terminal struct {
			Usage struct {
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
			Duration int `json:"duration"`
		}
		require.NoError(t, common.Unmarshal(terminalBody, &terminal), caseID)
		assert.Equal(t, completionTokens, terminal.Usage.CompletionTokens, caseID)
		assert.Equal(t, completionTokens+97, terminal.Usage.TotalTokens, caseID)
		assert.Equal(t, testCase.TerminalDuration, terminal.Duration, caseID)

		after := seedanceBillingDomainSnapshotFor(t, env, created.ID)
		expectedDelta := seedanceBillingExpectedDomainDelta(expectedPreConsume, expectedFinalQuota, completionTokens)
		require.Equal(t, expectedDelta, after.delta(before), caseID)
		classID := strings.TrimSuffix(strings.TrimSuffix(caseID, "/mode-omitted"), "/mode-smart")
		result := durationModeResult{preConsume: expectedPreConsume, quota: expectedFinalQuota, tokens: completionTokens, duration: testCase.TerminalDuration}
		if previous, ok := resultsByClass[classID]; ok {
			omitted, smart := previous, result
			if testCase.RequestDuration != nil {
				omitted, smart = previous, result
			}
			assert.Equal(t, omitted.preConsume, smart.preConsume, caseID)
			assert.Equal(t, 5, omitted.duration, caseID)
			assert.Equal(t, 7, smart.duration, caseID)
			assert.Equal(t, omitted.tokens+2000, smart.tokens, caseID)
			assert.NotEqual(t, omitted.quota, smart.quota, caseID)
		} else {
			resultsByClass[classID] = result
		}
		if t.Failed() {
			t.FailNow()
		}
		executed++
	}
	t.Logf("duration_mode_cases=%d", executed)
}

func TestSeedanceBillingExplicitMatrixE2E(t *testing.T) {
	silenceSeedanceBillingLogs(t)
	env := setupSeedanceBillingE2E(t)
	testCases := seedanceBillingExplicitCases()
	executed := 0

	for _, testCase := range testCases {
		caseID := testCase.ID
		before := seedanceBillingDomainSnapshotFor(t, env)
		requestsBefore := env.Mock.snapshot()
		_, requestBody := seedanceBillingExplicitRequest(t, testCase)

		status, createBody := performJSONRequest(t, env.Router, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", string(requestBody))
		require.Equal(t, http.StatusOK, status, "%s: %s", caseID, createBody)
		var created map[string]any
		require.NoError(t, common.Unmarshal(createBody, &created), caseID)
		require.Len(t, created, 1, caseID)
		publicTaskID, ok := created["id"].(string)
		require.True(t, ok, caseID)
		assert.True(t, strings.HasPrefix(publicTaskID, "task_"), caseID)

		requests := env.Mock.snapshot()
		require.Len(t, requests, len(requestsBefore)+1, caseID)
		submitRequest := requests[len(requestsBefore)]
		assert.Equal(t, http.MethodPost, submitRequest.Method, caseID)
		assert.Equal(t, "/api/v3/contents/generations/tasks", submitRequest.Path, caseID)
		assert.Equal(t, "Bearer mock-ark-key", submitRequest.Authorization, caseID)
		var expectedUpstreamRequest map[string]any
		var actualUpstreamRequest map[string]any
		require.NoError(t, common.Unmarshal(requestBody, &expectedUpstreamRequest), caseID)
		require.NoError(t, common.Unmarshal(submitRequest.Body, &actualUpstreamRequest), caseID)
		require.Equal(t, expectedUpstreamRequest, actualUpstreamRequest, caseID)

		var task model.Task
		require.NoError(t, model.DB.Where("task_id = ?", publicTaskID).First(&task).Error, caseID)
		upstreamTaskID := task.PrivateData.UpstreamTaskID
		require.NotEmpty(t, upstreamTaskID, caseID)
		assert.NotEqual(t, publicTaskID, upstreamTaskID, caseID)
		assert.NotContains(t, string(createBody), upstreamTaskID, caseID)
		mockTask, ok := env.Mock.taskSnapshot(upstreamTaskID)
		require.True(t, ok, caseID)
		assert.Equal(t, testCase.Model, mockTask.Model, caseID)
		assert.Equal(t, testCase.Resolution, mockTask.Resolution, caseID)
		assert.Equal(t, testCase.RequestDuration, mockTask.RequestDuration, caseID)
		assert.Equal(t, testCase.TerminalDuration, mockTask.TerminalDuration, caseID)
		assert.Equal(t, testCase.HasReferenceImage, mockTask.HasReferenceImage, caseID)
		assert.Equal(t, testCase.HasReferenceVideo, len(mockTask.VideoDurations) == 1, caseID)
		if testCase.HasReferenceVideo {
			assert.Equal(t, []int{2}, mockTask.VideoDurations, caseID)
		} else {
			assert.Empty(t, mockTask.VideoDurations, caseID)
		}

		completionTokens := 100000 + testCase.TerminalDuration*1000
		if testCase.HasReferenceVideo {
			completionTokens += 210
		}
		if testCase.HasReferenceImage {
			completionTokens++
		}
		require.Equal(t, completionTokens, mockTask.CompletionTokens, caseID)

		modelRatio := seedanceBillingModelRatios()[testCase.Model]
		estimatedMultiplier, estimatedRatios := seedanceBillingExpectedRatios(testCase, true)
		finalMultiplier, finalRatios := seedanceBillingExpectedRatios(testCase, false)
		expectedPreConsume := seedanceBillingExpectedPreConsume(modelRatio, estimatedMultiplier)
		expectedFinalQuota := seedanceBillingExpectedQuota(completionTokens, modelRatio, finalMultiplier)
		require.Equal(t, expectedPreConsume, task.Quota, caseID)
		require.NotNil(t, task.PrivateData.BillingContext, caseID)
		billingContext := task.PrivateData.BillingContext
		require.InDelta(t, modelRatio, billingContext.ModelRatio, 1e-12, caseID)
		assert.Equal(t, testCase.Model, billingContext.OriginModelName, caseID)
		assert.Equal(t, testCase.Model, billingContext.UpstreamModelName, caseID)
		assert.Equal(t, testCase.HasReferenceVideo, billingContext.HasVideoInput, caseID)
		require.NotNil(t, billingContext.GenerateAudio, caseID)
		expectedGenerateAudio := true
		if testCase.Model == "doubao-seedance-1-5-pro-251215" {
			expectedGenerateAudio = testCase.GenerateAudio
		}
		require.Equal(t, expectedGenerateAudio, *billingContext.GenerateAudio, caseID)
		assert.Equal(t, testCase.Draft, billingContext.Draft, caseID)
		expectedTier := testCase.ServiceTier
		if expectedTier == "" {
			expectedTier = "default"
		}
		require.Equal(t, expectedTier, billingContext.ServiceTier, caseID)
		require.Equal(t, testCase.Resolution, billingContext.Resolution, caseID)
		actualEstimatedRatios := billingContext.OtherRatios
		if actualEstimatedRatios == nil {
			actualEstimatedRatios = map[string]float64{}
		}
		require.Equal(t, estimatedRatios, actualEstimatedRatios, caseID)
		assert.Zero(t, billingContext.BillingTokens, caseID)

		summary := service.RunTaskPollingOnce(context.Background(), nil)
		require.Equal(t, 1, summary.UnfinishedTasks, caseID)
		requests = env.Mock.snapshot()
		require.Len(t, requests, len(requestsBefore)+2, caseID)
		pollRequest := requests[len(requestsBefore)+1]
		assert.Equal(t, http.MethodGet, pollRequest.Method, caseID)
		assert.Equal(t, "/api/v3/contents/generations/tasks/"+upstreamTaskID, pollRequest.Path, caseID)
		assert.Equal(t, "Bearer mock-ark-key", pollRequest.Authorization, caseID)

		require.NoError(t, model.DB.Where("task_id = ?", publicTaskID).First(&task).Error, caseID)
		assert.Equal(t, string(model.TaskStatusSuccess), string(task.Status), caseID)
		assert.Equal(t, "100%", task.Progress, caseID)
		require.Equal(t, expectedFinalQuota, task.Quota, caseID)
		require.NotNil(t, task.PrivateData.BillingContext, caseID)
		billingContext = task.PrivateData.BillingContext
		require.Equal(t, completionTokens, billingContext.BillingTokens, caseID)
		actualFinalRatios := billingContext.OtherRatios
		if actualFinalRatios == nil {
			actualFinalRatios = map[string]float64{}
		}
		require.Equal(t, finalRatios, actualFinalRatios, caseID)
		assert.NotContains(t, actualFinalRatios, "draft_estimate", caseID)

		status, terminalBody := performJSONRequest(t, env.Router, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicTaskID, "Bearer e2e-1", "")
		require.Equal(t, http.StatusOK, status, "%s: %s", caseID, terminalBody)
		var terminalFields map[string]any
		require.NoError(t, common.Unmarshal(terminalBody, &terminalFields), caseID)
		assert.Len(t, terminalFields, 17, caseID)
		var terminal struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Status  string `json:"status"`
			Content struct {
				VideoURL string `json:"video_url"`
			} `json:"content"`
			Usage struct {
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
			CreatedAt             int64  `json:"created_at"`
			UpdatedAt             int64  `json:"updated_at"`
			Seed                  int64  `json:"seed"`
			Resolution            string `json:"resolution"`
			Ratio                 string `json:"ratio"`
			Duration              int    `json:"duration"`
			FramesPerSecond       int    `json:"framespersecond"`
			ServiceTier           string `json:"service_tier"`
			ExecutionExpiresAfter int    `json:"execution_expires_after"`
			GenerateAudio         bool   `json:"generate_audio"`
			Draft                 bool   `json:"draft"`
			Priority              int    `json:"priority"`
		}
		require.NoError(t, common.Unmarshal(terminalBody, &terminal), caseID)
		assert.Equal(t, publicTaskID, terminal.ID, caseID)
		assert.Equal(t, testCase.Model, terminal.Model, caseID)
		assert.Equal(t, "succeeded", terminal.Status, caseID)
		assert.Equal(t, env.Server.URL+"/videos/"+upstreamTaskID+".mp4", terminal.Content.VideoURL, caseID)
		assert.Equal(t, completionTokens, terminal.Usage.CompletionTokens, caseID)
		assert.Equal(t, completionTokens+97, terminal.Usage.TotalTokens, caseID)
		assert.Equal(t, int64(1780000000), terminal.CreatedAt, caseID)
		assert.Equal(t, int64(1780000001), terminal.UpdatedAt, caseID)
		assert.Positive(t, terminal.Seed, caseID)
		assert.Equal(t, testCase.Resolution, terminal.Resolution, caseID)
		assert.Equal(t, "16:9", terminal.Ratio, caseID)
		assert.Equal(t, testCase.TerminalDuration, terminal.Duration, caseID)
		assert.Equal(t, 24, terminal.FramesPerSecond, caseID)
		assert.Equal(t, expectedTier, terminal.ServiceTier, caseID)
		assert.Equal(t, 172800, terminal.ExecutionExpiresAfter, caseID)
		assert.Equal(t, expectedGenerateAudio, terminal.GenerateAudio, caseID)
		assert.Equal(t, testCase.Draft, terminal.Draft, caseID)
		assert.Zero(t, terminal.Priority, caseID)

		after := seedanceBillingDomainSnapshotFor(t, env, publicTaskID)
		quotaDelta := expectedFinalQuota - expectedPreConsume
		expectedDelta := seedanceBillingExpectedDomainDelta(expectedPreConsume, expectedFinalQuota, completionTokens)
		seedanceBillingAssertDomainDelta(t, expectedDelta, before, after)

		logs := seedanceBillingLogsAfter(t, env, before.LastLogID)
		require.Len(t, logs, int(expectedDelta.LogCount), caseID)
		assert.Equal(t, model.LogTypeConsume, logs[0].Type, caseID)
		assert.Equal(t, expectedPreConsume, logs[0].Quota, caseID)
		assert.Equal(t, true, logs[0].Other["is_task"], caseID)
		assert.Equal(t, "/v1/video/generations", logs[0].Other["request_path"], caseID)
		assert.Equal(t, modelRatio, logs[0].Other["model_ratio"], caseID)
		assert.Equal(t, float64(1), logs[0].Other["group_ratio"], caseID)
		if quotaDelta != 0 {
			settlement := logs[1]
			assert.Equal(t, float64(completionTokens), settlement.Other["billing_tokens"], caseID)
			assert.Equal(t, publicTaskID, settlement.Other["task_id"], caseID)
			assert.Equal(t, modelRatio, settlement.Other["model_ratio"], caseID)
			assert.Equal(t, float64(1), settlement.Other["group_ratio"], caseID)
			assert.Equal(t, testCase.HasReferenceVideo, settlement.Other["has_video_input"], caseID)
			assert.Equal(t, testCase.Resolution, settlement.Other["resolution"], caseID)
			assert.Equal(t, expectedTier, settlement.Other["service_tier_value"], caseID)
			if serviceTierRatio, ok := finalRatios["service_tier"]; ok {
				assert.Equal(t, serviceTierRatio, settlement.Other["service_tier"], caseID)
			} else {
				assert.NotContains(t, settlement.Other, "service_tier", caseID)
			}
			assert.Equal(t, expectedGenerateAudio, settlement.Other["generate_audio"], caseID)
			assert.Equal(t, testCase.Draft, settlement.Other["draft"], caseID)
			assert.NotContains(t, settlement.Other, "draft_estimate", caseID)
			for ratioName, ratio := range finalRatios {
				if ratioName == "service_tier" {
					continue
				}
				assert.Equal(t, ratio, settlement.Other[ratioName], caseID)
			}
		}
		if t.Failed() {
			t.FailNow()
		}
		executed++
	}

	t.Logf("explicit_cases=%d", executed)
}

func TestSeedanceBillingReferenceVideoProfilesE2E(t *testing.T) {
	silenceSeedanceBillingLogs(t)
	env := setupSeedanceBillingE2E(t)
	profiles := seedanceBillingReferenceVideoProfiles()
	const modelID = "doubao-seedance-2-0-260128"
	const modelRatio = 46.0 / 14.0
	const videoInputMultiplier = 28.0 / 46.0
	expectedRatios := map[string]float64{"video_input": videoInputMultiplier}
	executed := 0

	for profileIndex, profile := range profiles {
		hasReferenceImage := profileIndex%2 == 1
		caseID := fmt.Sprintf("profile-%03d/durations-%v/image-%t", profileIndex+1, profile, hasReferenceImage)
		totalVideoDuration := 0
		for _, duration := range profile {
			totalVideoDuration += duration
		}
		completionTokens := 100000 + 5000 + totalVideoDuration*100 + len(profile)*10
		if hasReferenceImage {
			completionTokens++
		}
		expectedPreConsume := seedanceBillingExpectedPreConsume(modelRatio, videoInputMultiplier)
		expectedFinalQuota := common.QuotaFromFloat(float64(completionTokens) * modelRatio * videoInputMultiplier)

		before := seedanceBillingDomainSnapshotFor(t, env)
		requestsBefore := env.Mock.snapshot()
		expectedRequest, requestBody := seedanceBillingReferenceVideoRequest(t, profile, hasReferenceImage, caseID)
		status, createBody := performJSONRequest(t, env.Router, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", string(requestBody))
		require.Equal(t, http.StatusOK, status, "%s: %s", caseID, createBody)
		var created struct {
			ID string `json:"id"`
		}
		require.NoError(t, common.Unmarshal(createBody, &created), caseID)
		require.NotEmpty(t, created.ID, caseID)
		assert.True(t, strings.HasPrefix(created.ID, "task_"), caseID)

		requests := env.Mock.snapshot()
		require.Len(t, requests, len(requestsBefore)+1, caseID)
		submitRequest := requests[len(requestsBefore)]
		assert.Equal(t, http.MethodPost, submitRequest.Method, caseID)
		assert.Equal(t, "/api/v3/contents/generations/tasks", submitRequest.Path, caseID)
		assert.Equal(t, "Bearer mock-ark-key", submitRequest.Authorization, caseID)
		var actualRequest map[string]any
		require.NoError(t, common.Unmarshal(submitRequest.Body, &actualRequest), caseID)
		require.Equal(t, expectedRequest, actualRequest, caseID)
		assert.Equal(t, expectedRequest["content"], actualRequest["content"], caseID)

		var task model.Task
		require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error, caseID)
		upstreamTaskID := task.PrivateData.UpstreamTaskID
		require.NotEmpty(t, upstreamTaskID, caseID)
		assert.NotEqual(t, created.ID, upstreamTaskID, caseID)
		assert.NotContains(t, string(createBody), upstreamTaskID, caseID)
		mockTask, ok := env.Mock.taskSnapshot(upstreamTaskID)
		require.True(t, ok, caseID)
		assert.Equal(t, modelID, mockTask.Model, caseID)
		assert.Equal(t, "720p", mockTask.Resolution, caseID)
		assert.Equal(t, common.GetPointer(5), mockTask.RequestDuration, caseID)
		assert.Equal(t, 5, mockTask.TerminalDuration, caseID)
		assert.Equal(t, hasReferenceImage, mockTask.HasReferenceImage, caseID)
		assert.Equal(t, profile, mockTask.VideoDurations, caseID)
		require.Equal(t, completionTokens, mockTask.CompletionTokens, caseID)

		require.Equal(t, expectedPreConsume, task.Quota, caseID)
		require.NotNil(t, task.PrivateData.BillingContext, caseID)
		billingContext := task.PrivateData.BillingContext
		require.InDelta(t, modelRatio, billingContext.ModelRatio, 1e-12, caseID)
		assert.Equal(t, modelID, billingContext.OriginModelName, caseID)
		assert.Equal(t, modelID, billingContext.UpstreamModelName, caseID)
		assert.True(t, billingContext.HasVideoInput, caseID)
		require.Equal(t, expectedRatios, billingContext.OtherRatios, caseID)
		assert.NotContains(t, billingContext.OtherRatios, "video_count", caseID)
		assert.NotContains(t, billingContext.OtherRatios, "video_duration", caseID)
		assert.NotContains(t, billingContext.OtherRatios, "reference_image", caseID)
		assert.Zero(t, billingContext.BillingTokens, caseID)

		summary := service.RunTaskPollingOnce(context.Background(), nil)
		require.Equal(t, 1, summary.UnfinishedTasks, caseID)
		requests = env.Mock.snapshot()
		require.Len(t, requests, len(requestsBefore)+2, caseID)
		pollRequest := requests[len(requestsBefore)+1]
		assert.Equal(t, http.MethodGet, pollRequest.Method, caseID)
		assert.Equal(t, "/api/v3/contents/generations/tasks/"+upstreamTaskID, pollRequest.Path, caseID)
		assert.Equal(t, "Bearer mock-ark-key", pollRequest.Authorization, caseID)

		require.NoError(t, model.DB.Where("task_id = ?", created.ID).First(&task).Error, caseID)
		assert.Equal(t, string(model.TaskStatusSuccess), string(task.Status), caseID)
		require.Equal(t, expectedFinalQuota, task.Quota, caseID)
		require.NotNil(t, task.PrivateData.BillingContext, caseID)
		billingContext = task.PrivateData.BillingContext
		assert.True(t, billingContext.HasVideoInput, caseID)
		require.Equal(t, completionTokens, billingContext.BillingTokens, caseID)
		require.Equal(t, expectedRatios, billingContext.OtherRatios, caseID)
		assert.NotContains(t, billingContext.OtherRatios, "video_count", caseID)
		assert.NotContains(t, billingContext.OtherRatios, "video_duration", caseID)
		assert.NotContains(t, billingContext.OtherRatios, "reference_image", caseID)

		status, terminalBody := performJSONRequest(t, env.Router, http.MethodGet, "/api/v3/contents/generations/tasks/"+created.ID, "Bearer e2e-1", "")
		require.Equal(t, http.StatusOK, status, "%s: %s", caseID, terminalBody)
		var terminal struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage struct {
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		require.NoError(t, common.Unmarshal(terminalBody, &terminal), caseID)
		assert.Equal(t, created.ID, terminal.ID, caseID)
		assert.Equal(t, modelID, terminal.Model, caseID)
		assert.Equal(t, completionTokens, terminal.Usage.CompletionTokens, caseID)
		assert.NotEqual(t, completionTokens, terminal.Usage.TotalTokens, caseID)

		after := seedanceBillingDomainSnapshotFor(t, env, created.ID)
		expectedDelta := seedanceBillingExpectedDomainDelta(expectedPreConsume, expectedFinalQuota, completionTokens)
		seedanceBillingAssertDomainDelta(t, expectedDelta, before, after)
		logs := seedanceBillingLogsAfter(t, env, before.LastLogID)
		require.Len(t, logs, int(expectedDelta.LogCount), caseID)
		assert.Equal(t, model.LogTypeConsume, logs[0].Type, caseID)
		assert.Equal(t, expectedPreConsume, logs[0].Quota, caseID)
		assert.Equal(t, true, logs[0].Other["is_task"], caseID)
		assert.Equal(t, "/v1/video/generations", logs[0].Other["request_path"], caseID)
		assert.Equal(t, modelRatio, logs[0].Other["model_ratio"], caseID)
		assert.Equal(t, float64(1), logs[0].Other["group_ratio"], caseID)

		require.Len(t, logs, 2, caseID)
		settlementLog := logs[1]
		assert.Equal(t, model.LogTypeRefund, settlementLog.Type, caseID)
		assert.Equal(t, expectedPreConsume-expectedFinalQuota, settlementLog.Quota, caseID)
		assert.Equal(t, created.ID, settlementLog.Other["task_id"], caseID)
		assert.Equal(t, float64(completionTokens), settlementLog.Other["billing_tokens"], caseID)
		assert.Equal(t, modelRatio, settlementLog.Other["model_ratio"], caseID)
		assert.Equal(t, float64(1), settlementLog.Other["group_ratio"], caseID)
		assert.Equal(t, true, settlementLog.Other["has_video_input"], caseID)
		assert.Equal(t, "720p", settlementLog.Other["resolution"], caseID)
		assert.Equal(t, videoInputMultiplier, settlementLog.Other["video_input"], caseID)
		assert.NotContains(t, settlementLog.Other, "video_count", caseID)
		assert.NotContains(t, settlementLog.Other, "video_duration", caseID)
		assert.NotContains(t, settlementLog.Other, "reference_image", caseID)

		if t.Failed() {
			t.FailNow()
		}
		executed++
	}

	t.Logf("reference_video_profiles=%d", executed)
}

func TestSeedanceBillingEnvironment(t *testing.T) {
	originalRatios := ratio_setting.GetModelRatioCopy()
	originalRetryTimes := common.RetryTimes

	t.Run("installed environment", func(t *testing.T) {
		env := setupSeedanceBillingE2E(t)
		require.NotNil(t, env.Router)
		require.NotNil(t, env.Mock)
		require.NotNil(t, env.Server)
		assert.NotEmpty(t, env.Server.URL)
		assert.True(t, common.DataExportEnabled)
		assert.Zero(t, common.RetryTimes)
		require.NotNil(t, service.GetTaskAdaptorFunc)
		assert.NotNil(t, service.GetTaskAdaptorFunc(constant.TaskPlatform("54")))

		var seededUser model.User
		var seededToken model.Token
		var seededChannel model.Channel
		require.NoError(t, model.DB.First(&seededUser, env.User.Id).Error)
		require.NoError(t, model.DB.First(&seededToken, env.Token.Id).Error)
		require.NoError(t, model.DB.First(&seededChannel, env.Channel.Id).Error)
		assert.Equal(t, env.User.Username, seededUser.Username)
		assert.Equal(t, 2_000_000_000, seededUser.Quota)
		assert.Equal(t, env.Token.Key, seededToken.Key)
		assert.Equal(t, env.Server.URL, *seededChannel.BaseURL)

		var abilities []model.Ability
		require.NoError(t, model.DB.Where("channel_id = ?", env.Channel.Id).Order("model").Find(&abilities).Error)
		require.Len(t, abilities, 4)
		for modelID, expectedRatio := range seedanceBillingModelRatios() {
			assert.Contains(t, seededChannel.Models, modelID)
			actualRatio, configured, _ := ratio_setting.GetModelRatio(modelID)
			assert.True(t, configured, modelID)
			assert.InDelta(t, expectedRatio, actualRatio, 1e-12, modelID)
		}

		before := seedanceBillingDomainSnapshotFor(t, env)
		seedanceBillingAssertDomainDelta(t, seedanceBillingDomainSnapshot{}, before, before)
		requestBody, err := common.Marshal(map[string]any{
			"model":      "doubao-seedance-2-0-260128",
			"content":    []any{map[string]any{"type": "text", "text": "environment probe"}},
			"resolution": "720p",
			"duration":   5,
		})
		require.NoError(t, err)
		status, responseBody := performJSONRequest(t, env.Router, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", string(requestBody))
		require.Equal(t, http.StatusOK, status, string(responseBody))
		var createResponse struct {
			ID string `json:"id"`
		}
		require.NoError(t, common.Unmarshal(responseBody, &createResponse))
		assert.True(t, strings.HasPrefix(createResponse.ID, "task_"))
		require.Len(t, env.Mock.snapshot(), 1)

		var task model.Task
		require.NoError(t, model.DB.Where("task_id = ?", createResponse.ID).First(&task).Error)
		after := seedanceBillingDomainSnapshotFor(t, env, createResponse.ID)
		delta := after.delta(before)
		assert.Equal(t, int64(1), delta.TaskCount)
		assert.Equal(t, task.Quota, delta.TaskQuota)
		assert.Equal(t, -task.Quota, delta.UserQuota)
		assert.Equal(t, task.Quota, delta.UserUsedQuota)
		assert.Equal(t, 1, delta.UserRequestCount)
		assert.Equal(t, int64(task.Quota), delta.ChannelUsedQuota)
		assert.Equal(t, task.Quota, delta.TokenUsedQuota)
		assert.Equal(t, 1, delta.QuotaDataCount)
		assert.Equal(t, task.Quota, delta.QuotaDataQuota)
		assert.Zero(t, delta.QuotaDataTokenUsed)
		assert.Equal(t, int64(1), delta.LogCount)
		assert.Equal(t, 1, delta.ConsumeLogCount)
		assert.Equal(t, task.Quota, delta.ConsumeLogQuota)
		assert.Zero(t, delta.RefundLogCount)
		assert.Zero(t, delta.RefundLogQuota)
		assert.Equal(t, task.Quota, delta.SignedLogQuota)
		assert.Zero(t, delta.TaskBillingTokens)
		assert.Zero(t, delta.SettlementLogCount)
		assert.Zero(t, delta.SettlementConsumeLogCount)
		assert.Zero(t, delta.SettlementConsumeLogQuota)
		assert.Zero(t, delta.SettlementRefundLogCount)
		assert.Zero(t, delta.SettlementRefundLogQuota)
		assert.Zero(t, delta.SettlementSignedLogQuota)
		assert.Zero(t, delta.SettlementLogBillingTokens)
		assert.False(t, delta.SettlementLogBillingTokensPresent)

		modelRatio := 46.0 / 14.0
		assert.Equal(t,
			common.QuotaFromFloat(float64(107521)*modelRatio*(31.0/46.0)),
			seedanceBillingExpectedQuota(107521, modelRatio, 31.0/46.0),
		)
		assert.Equal(t,
			common.QuotaFromFloat(modelRatio/2*common.QuotaPerUnit*(51.0/46.0)),
			seedanceBillingExpectedPreConsume(modelRatio, 51.0/46.0),
		)
	})

	assert.Equal(t, originalRatios, ratio_setting.GetModelRatioCopy())
	assert.Equal(t, originalRetryTimes, common.RetryTimes)
}

func TestSeedanceBillingDomainSnapshotLogSemantics(t *testing.T) {
	env := setupSeedanceBillingE2E(t)
	targetTaskID := "task_snapshot_target"
	before := seedanceBillingDomainSnapshotFor(t, env, targetTaskID)
	task := &model.Task{
		TaskID:    targetTaskID,
		UserId:    env.User.Id,
		ChannelId: env.Channel.Id,
		Quota:     70,
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			BillingTokens: 4321,
		}},
	}
	require.NoError(t, model.DB.Create(task).Error)
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: env.User.Id, LogType: model.LogTypeConsume, ChannelId: env.Channel.Id, Quota: 100,
	})
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: env.User.Id, LogType: model.LogTypeRefund, ChannelId: env.Channel.Id, Quota: 30,
		Other: map[string]any{"task_id": targetTaskID, "billing_tokens": 4321},
	})
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: env.User.Id, LogType: model.LogTypeConsume, ChannelId: env.Channel.Id, Quota: 20,
		Other: map[string]any{"task_id": "task_snapshot_other", "billing_tokens": 999},
	})

	after := seedanceBillingDomainSnapshotFor(t, env, targetTaskID)
	expectedDelta := seedanceBillingDomainSnapshot{
		TaskCount:                         1,
		TaskQuota:                         70,
		ConsumeLogCount:                   2,
		ConsumeLogQuota:                   120,
		RefundLogCount:                    1,
		RefundLogQuota:                    30,
		SignedLogQuota:                    90,
		LogCount:                          3,
		TaskBillingTokens:                 4321,
		SettlementLogCount:                1,
		SettlementConsumeLogCount:         0,
		SettlementConsumeLogQuota:         0,
		SettlementRefundLogCount:          1,
		SettlementRefundLogQuota:          30,
		SettlementSignedLogQuota:          -30,
		SettlementLogBillingTokens:        4321,
		SettlementLogBillingTokensPresent: true,
	}
	seedanceBillingAssertDomainDelta(t, expectedDelta, before, after)
}

func TestSeedanceBillingDomainSnapshotDoesNotInferTargetAcrossCases(t *testing.T) {
	env := setupSeedanceBillingE2E(t)
	firstTaskID := "task_snapshot_first"
	require.NoError(t, model.DB.Create(&model.Task{
		TaskID: firstTaskID, UserId: env.User.Id, ChannelId: env.Channel.Id, Quota: 10,
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{BillingTokens: 111}},
	}).Error)
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: env.User.Id, LogType: model.LogTypeRefund, ChannelId: env.Channel.Id, Quota: 3,
		Other: map[string]any{"task_id": firstTaskID, "billing_tokens": 111},
	})

	before := seedanceBillingDomainSnapshotFor(t, env)
	assert.Zero(t, before.TaskBillingTokens)
	assert.Zero(t, before.SettlementLogCount)
	assert.Zero(t, before.SettlementSignedLogQuota)
	assert.Zero(t, before.SettlementLogBillingTokens)
	assert.False(t, before.SettlementLogBillingTokensPresent)
	assert.Equal(t, before, seedanceBillingDomainSnapshotFor(t, env, ""))

	secondTaskID := "task_snapshot_second"
	require.NoError(t, model.DB.Create(&model.Task{
		TaskID: secondTaskID, UserId: env.User.Id, ChannelId: env.Channel.Id, Quota: 20,
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{BillingTokens: 222}},
	}).Error)
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId: env.User.Id, LogType: model.LogTypeConsume, ChannelId: env.Channel.Id, Quota: 5,
		Other: map[string]any{"task_id": secondTaskID, "billing_tokens": 222},
	})

	after := seedanceBillingDomainSnapshotFor(t, env, secondTaskID)
	expectedDelta := seedanceBillingDomainSnapshot{
		TaskCount:                         1,
		TaskQuota:                         20,
		LogCount:                          1,
		ConsumeLogCount:                   1,
		ConsumeLogQuota:                   5,
		SignedLogQuota:                    5,
		TaskBillingTokens:                 222,
		SettlementLogCount:                1,
		SettlementConsumeLogCount:         1,
		SettlementConsumeLogQuota:         5,
		SettlementSignedLogQuota:          5,
		SettlementLogBillingTokens:        222,
		SettlementLogBillingTokensPresent: true,
	}
	seedanceBillingAssertDomainDelta(t, expectedDelta, before, after)
}
