package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/QuantumNous/new-api/relay"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMockVideoServerServesMediaThroughRealMetadataParsers(t *testing.T) {
	server := httptest.NewServer(&mockVideoServer{models: make(map[string]string)})
	t.Cleanup(server.Close)
	assetClient, err := fixtureAssetClient(server)
	require.NoError(t, err)
	fetcher := videometa.NewFetcher(videometa.FetcherOptions{Client: assetClient, TempDir: t.TempDir()})
	metadata, err := fetcher.Metadata(context.Background(), videometa.Request{
		URL: seedAssetBaseURL + "/sample.mp4", MediaType: "video",
		MaxBytes: videometa.MaxVideoBytes, DeadlineMS: videometa.MaxDeadlineMS,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1_000), metadata.DurationMS)
	require.Equal(t, 320, metadata.Width)
	require.Equal(t, 180, metadata.Height)

	service.SetReferenceAudioDurationResolver(service.NewReferenceAudioDurationResolver(assetClient, t.TempDir()))
	t.Cleanup(func() { service.SetReferenceAudioDurationResolver(nil) })
	durationMS, err := service.ResolveReferenceAudioDurationMS(context.Background(), []string{seedAssetBaseURL + "/audio.wav"})
	require.NoError(t, err)
	require.Equal(t, int64(1_000), durationMS)
}

func TestSeedPasswordHashAcceptsSeedPassword(t *testing.T) {
	hashed, err := seedPasswordHash()
	require.NoError(t, err)
	require.NotEqual(t, seedPassword, hashed)
	require.True(t, common.ValidatePasswordAndHash(seedPassword, hashed))
}

func TestDisableRemovedSeedChannelsLeavesOnlyCurrentMatrixLinesEnabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	current := model.Channel{Name: seedChannelName + "current", Key: "key", Status: common.ChannelStatusEnabled}
	removed := model.Channel{Name: seedChannelName + "removed", Key: "key", Status: common.ChannelStatusEnabled}
	unrelated := model.Channel{Name: "unrelated", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&current).Error)
	require.NoError(t, db.Create(&removed).Error)
	require.NoError(t, db.Create(&unrelated).Error)
	currentRule := model.ChannelModelCostRule{
		ChannelID: current.Id, BillableUpstreamModel: "current-model", CostVariantKey: "default",
		Version: 1, Status: string(types.CostRuleActive),
	}
	removedRule := model.ChannelModelCostRule{
		ChannelID: removed.Id, BillableUpstreamModel: "removed-model", CostVariantKey: "default",
		Version: 1, Status: string(types.CostRuleActive),
	}
	unrelatedRule := model.ChannelModelCostRule{
		ChannelID: unrelated.Id, BillableUpstreamModel: "unrelated-model", CostVariantKey: "default",
		Version: 1, Status: string(types.CostRuleActive),
	}
	require.NoError(t, db.Create(&currentRule).Error)
	require.NoError(t, db.Create(&removedRule).Error)
	require.NoError(t, db.Create(&unrelatedRule).Error)

	require.NoError(t, disableRemovedSeedChannels(map[string]struct{}{current.Name: {}}))

	require.NoError(t, db.First(&current, current.Id).Error)
	require.NoError(t, db.First(&removed, removed.Id).Error)
	require.NoError(t, db.First(&unrelated, unrelated.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, current.Status)
	require.Equal(t, common.ChannelStatusManuallyDisabled, removed.Status)
	require.Equal(t, common.ChannelStatusEnabled, unrelated.Status)
	require.NoError(t, db.First(&currentRule, currentRule.ID).Error)
	require.NoError(t, db.First(&removedRule, removedRule.ID).Error)
	require.NoError(t, db.First(&unrelatedRule, unrelatedRule.ID).Error)
	require.Equal(t, string(types.CostRuleActive), currentRule.Status)
	require.Equal(t, string(types.CostRuleRetired), removedRule.Status)
	require.NotNil(t, removedRule.EffectiveTo)
	require.Equal(t, string(types.CostRuleActive), unrelatedRule.Status)
}

func TestLoadTargetsPreservesImportedMaterialMatrix(t *testing.T) {
	targets, err := loadTargets(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)
	require.Len(t, targets, 45)
	require.Equal(t, map[string]int{"431": 17, "933": 28}, materialDistribution(targets))
	enabledCosts := 0
	for _, target := range targets {
		if target.CostEnabled {
			enabledCosts++
		}
	}
	require.Equal(t, len(targets), enabledCosts)

	targetsByLine := make(map[string][]matrixTarget)
	for _, target := range targets {
		targetsByLine[target.LineRef] = append(targetsByLine[target.LineRef], target)
	}
	require.Equal(t, constant.ChannelTypeFFLink, targetsByLine["channel-fflink"][0].ChannelType)
	require.Equal(t, constant.ChannelTypeZZone, targetsByLine["channel-zzone"][0].ChannelType)
	require.Equal(t, constant.ChannelTypeMegaByAI, targetsByLine["megabyai-fast-real-person"][0].ChannelType)
	require.Equal(t, constant.ChannelTypeMikoto, targetsByLine["mikoto-sd"][0].ChannelType)
	require.Equal(t, constant.ChannelTypeMikoto, targetsByLine["mikoto-sora"][0].ChannelType)
}

func TestLoadTargetsUsesAllowedAspectRatioForPolicyAndRequest(t *testing.T) {
	targets, err := loadTargets(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)

	var target matrixTarget
	for _, candidate := range targets {
		if candidate.RouteTargetRef == "route-target/MAP-ZZONE-R217-720" {
			target = candidate
			break
		}
	}
	require.NotEmpty(t, target.RouteTargetRef)
	require.Equal(t, "16:9", target.AspectRatio)
	require.Contains(t, target.AspectRatios, target.AspectRatio)

	var body map[string]any
	require.NoError(t, common.UnmarshalJsonStr(requestBody(target, seedAssetBaseURL), &body))
	require.Equal(t, target.AspectRatio, body["ratio"])
}

func TestLoadTargetsMatchesRouteContractBlocks(t *testing.T) {
	targets, err := loadTargets(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)

	accepted := 0
	blocked := 0
	disabled := 0
	for _, target := range targets {
		if !target.CostEnabled {
			disabled++
			continue
		}
		channel := &model.Channel{Type: target.ChannelType}
		if target.ChannelType == constant.ChannelTypeSecure {
			switch target.LineRef {
			case "secure-discount":
				channel.SetOtherSettings(relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupDiscount})
			case "secure-overseas":
				channel.SetOtherSettings(relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupOverseas})
			case "secure-enterprise":
				channel.SetOtherSettings(relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupEnterprise})
			}
		}
		err := relay.ValidateVideoRouteTargetContract(channel, target.RuntimeModel, modelrouting.Target{
			UpstreamModel: target.UpstreamModel,
			Constraints: modelrouting.Constraints{
				OutputResolutions:                  []string{target.Resolution},
				Durations:                          target.Durations,
				AspectRatios:                       target.AspectRatios,
				InputModes:                         target.InputModes,
				ReferenceMinimums:                  target.Minimums,
				ReferenceLimits:                    target.References,
				ReferenceTotalMax:                  target.ReferenceTotalMax,
				ReferenceVideoAudioTotalMax:        target.ReferenceVideoAudioTotalMax,
				ReferenceVideoTotalDurationSeconds: target.ReferenceVideoTotalDurationSeconds,
				ReferenceModes:                     target.ReferenceModes,
				SupportsRealPerson:                 target.SupportsRealPerson,
			},
		})
		if err != nil {
			blocked++
			continue
		}
		accepted++
	}

	require.Equal(t, len(targets), accepted)
	require.Zero(t, blocked)
	require.Zero(t, disabled)
}

func TestSelectFailureFixtureTargetFallsBackToAcceptedProviderChannel(t *testing.T) {
	targets := []matrixTarget{
		{CaseID: "cangyuan", ChannelType: constant.ChannelTypeCangyuan},
		{CaseID: "secure", ChannelType: constant.ChannelTypeSecure},
	}

	selected := selectFailureFixtureTarget(targets)
	require.NotNil(t, selected)
	require.Equal(t, "cangyuan", selected.CaseID)

	targets = append(targets, matrixTarget{CaseID: "new-api-video", ChannelType: constant.ChannelTypeNewAPIVideo})
	selected = selectFailureFixtureTarget(targets)
	require.NotNil(t, selected)
	require.Equal(t, "new-api-video", selected.CaseID)
}

func TestMockVideoServerSupportsProviderSubmitAndPollingPaths(t *testing.T) {
	server := &mockVideoServer{models: make(map[string]string)}
	submitPaths := []string{"/v1/video/generations", "/v1/videos/generations", "/v1/videos", "/v1/media/generate", "/api/generate-video"}
	for _, path := range submitPaths {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"seedance-provider-model"}`))
		server.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code, path)
		require.Contains(t, recorder.Body.String(), "upstream-task-", path)
	}

	pollPaths := []struct {
		path   string
		status string
	}{
		{path: "/v1/video/generations/upstream-task-001", status: "SUCCESS"},
		{path: "/api/task/upstream-task-002", status: "SUCCESS"},
		{path: "/v1/videos/tasks/upstream-task-003", status: "completed"},
		{path: "/v1/videos/upstream-task-004", status: "completed"},
		{path: "/v1/tasks/upstream-task-005", status: "SUCCESS"},
	}
	for _, testCase := range pollPaths {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.path, nil))
		require.Equal(t, http.StatusOK, recorder.Code, testCase.path)
		require.Contains(t, recorder.Body.String(), "upstream-task-", testCase.path)
		require.Contains(t, recorder.Body.String(), `"status":"`+testCase.status+`"`, testCase.path)
		switch {
		case strings.HasPrefix(testCase.path, "/v1/videos/tasks/"),
			strings.HasPrefix(testCase.path, "/v1/videos/"):
			require.NotContains(t, recorder.Body.String(), `"framespersecond"`, testCase.path)
		default:
			require.Contains(t, recorder.Body.String(), `"framespersecond":24`, testCase.path)
		}
	}

	server.failNextTask()
	submit := httptest.NewRecorder()
	server.ServeHTTP(submit, httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"seedance-failure-fixture"}`)))
	require.Equal(t, http.StatusOK, submit.Code)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(submit.Body.Bytes(), &created))
	require.NotEmpty(t, created.ID)

	failed := httptest.NewRecorder()
	server.ServeHTTP(failed, httptest.NewRequest(http.MethodGet, "/v1/video/generations/"+created.ID, nil))
	require.Equal(t, http.StatusOK, failed.Code)
	require.Contains(t, failed.Body.String(), `"status":"FAILURE"`)
	require.Contains(t, failed.Body.String(), "mock content policy rejection")

	server.failNextTask()
	directSubmit := httptest.NewRecorder()
	server.ServeHTTP(directSubmit, httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"seedance-direct-failure-fixture"}`)))
	require.Equal(t, http.StatusOK, directSubmit.Code)
	var directCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(directSubmit.Body.Bytes(), &directCreated))
	require.NotEmpty(t, directCreated.ID)

	directFailed := httptest.NewRecorder()
	server.ServeHTTP(directFailed, httptest.NewRequest(http.MethodGet, "/v1/videos/"+directCreated.ID, nil))
	require.Equal(t, http.StatusOK, directFailed.Code)
	require.Contains(t, directFailed.Body.String(), `"status":"failed"`)
	require.Contains(t, directFailed.Body.String(), "mock content policy rejection")
}

func TestValidateArkTerminalResponse(t *testing.T) {
	complete := json.RawMessage(`{
		"id":"task_x",
		"model":"m",
		"status":"succeeded",
		"content":{"video_url":"https://x/video.mp4"},
		"usage":{"completion_tokens":0,"total_tokens":0},
		"created_at":1,
		"updated_at":2,
		"seed":0,
		"resolution":"720p",
		"ratio":"16:9",
		"duration":5,
		"framespersecond":24,
		"service_tier":"default",
		"execution_expires_after":172800,
		"generate_audio":true,
		"draft":false,
		"priority":0
	}`)
	require.NoError(t, validateArkTerminalResponse(complete, model.TaskStatusSuccess, "task_x", "m"))

	missingFPS := json.RawMessage(strings.Replace(string(complete), `"framespersecond":24,`, "", 1))
	require.ErrorContains(t, validateArkTerminalResponse(missingFPS, model.TaskStatusSuccess, "task_x", "m"), "framespersecond")

	failed := json.RawMessage(`{
		"id":"task_x",
		"model":"m",
		"status":"failed",
		"usage":{"completion_tokens":0,"total_tokens":0},
		"error":{"code":"task_failed","message":"task failed"},
		"created_at":1,
		"updated_at":2,
		"seed":0,
		"resolution":"720p",
		"ratio":"16:9",
		"duration":5,
		"framespersecond":24,
		"service_tier":"default",
		"execution_expires_after":172800,
		"generate_audio":true,
		"draft":false,
		"priority":0
	}`)
	require.NoError(t, validateArkTerminalResponse(failed, model.TaskStatusFailure, "task_x", "m"))
	for _, status := range []string{"expired", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			response := json.RawMessage(strings.Replace(string(failed), `"status":"failed"`, `"status":"`+status+`"`, 1))
			require.NoError(t, validateArkTerminalResponse(response, model.TaskStatusFailure, "task_x", "m"))
		})
	}

	failedWithVideo := json.RawMessage(strings.Replace(
		string(failed),
		`"status":"failed"`,
		`"status":"failed","content":{"video_url":"https://x/video.mp4"}`,
		1,
	))
	require.ErrorContains(t, validateArkTerminalResponse(failedWithVideo, model.TaskStatusFailure, "task_x", "m"), "content")

	successWithError := json.RawMessage(strings.Replace(
		string(complete),
		`"status":"succeeded"`,
		`"status":"succeeded","error":{"code":"stale","message":"private"}`,
		1,
	))
	require.ErrorContains(t, validateArkTerminalResponse(successWithError, model.TaskStatusSuccess, "task_x", "m"), "error")

	negativeDuration := json.RawMessage(strings.Replace(string(complete), `"duration":5`, `"duration":-1`, 1))
	require.ErrorContains(t, validateArkTerminalResponse(negativeDuration, model.TaskStatusSuccess, "task_x", "m"), "duration")

	unknownField := json.RawMessage(strings.Replace(
		string(complete),
		`"status":"succeeded"`,
		`"status":"succeeded","diagnostic":"private"`,
		1,
	))
	require.ErrorContains(t, validateArkTerminalResponse(unknownField, model.TaskStatusSuccess, "task_x", "m"), "diagnostic")

	require.ErrorContains(t, validateArkTerminalResponse(complete, model.TaskStatusSuccess, "other_task", "m"), "id")
	require.ErrorContains(t, validateArkTerminalResponse(complete, model.TaskStatusSuccess, "task_x", "other-model"), "model")
}

func TestMockVideoServerPollingResponseIncludesOfficialArkTaskFields(t *testing.T) {
	server := &mockVideoServer{models: map[string]string{
		"upstream-task-001": "doubao-seedance-2-0-mini-260615",
	}}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/video/generations/upstream-task-001", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, float64(78674), response.Data.Data["seed"])
	require.Equal(t, float64(24), response.Data.Data["framespersecond"])
	require.Equal(t, "default", response.Data.Data["service_tier"])
	require.Equal(t, float64(172800), response.Data.Data["execution_expires_after"])
	require.Equal(t, true, response.Data.Data["generate_audio"])
	createdAt, createdAtExists := response.Data.Data["created_at"]
	require.True(t, createdAtExists)
	updatedAt, updatedAtExists := response.Data.Data["updated_at"]
	require.True(t, updatedAtExists)
	require.GreaterOrEqual(t, updatedAt.(float64), createdAt.(float64))
	require.Contains(t, response.Data.Data, "draft")
	require.Equal(t, false, response.Data.Data["draft"])
	require.Contains(t, response.Data.Data, "priority")
	require.Equal(t, float64(0), response.Data.Data["priority"])
}

func TestMockVideoServerPollingResponseIncludesInputTokensForTokenPricedModel(t *testing.T) {
	server := &mockVideoServer{models: map[string]string{
		"upstream-task-001": "seedance-720p-token",
	}}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/video/generations/upstream-task-001", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Data struct {
			Data struct {
				Usage struct {
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
			} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Positive(t, response.Data.Data.Usage.CompletionTokens)
	require.Greater(t, response.Data.Data.Usage.TotalTokens, response.Data.Data.Usage.CompletionTokens)
}

func TestCleanupSeedDataOnlyRemovesSeedIdentityRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Token{}, &model.Channel{}, &model.Task{}, &model.Log{}, &model.QuotaData{},
		&model.CostAccountingRequest{}, &model.CostAccountingAttempt{}, &model.CostAccountingAudit{},
		&model.ChannelModelCostRule{},
	))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
	})

	seedUser := model.User{Id: 100, Username: seedUsername, Password: "hash", AffCode: "seed-aff", UsedQuota: 123, RequestCount: 4}
	otherUser := model.User{Id: 101, Username: "other-user", Password: "hash", AffCode: "other-aff", UsedQuota: 456, RequestCount: 5}
	seedTokenRow := model.Token{Id: 200, UserId: seedUser.Id, Key: seedToken, UsedQuota: 123}
	seedChannel := model.Channel{Id: 300, Name: seedChannelName + "line", Key: "key", UsedQuota: 123}
	otherChannel := model.Channel{Id: 301, Name: "other-channel", Key: "key", UsedQuota: 456}
	require.NoError(t, db.Create(&seedUser).Error)
	require.NoError(t, db.Create(&otherUser).Error)
	require.NoError(t, db.Create(&seedTokenRow).Error)
	require.NoError(t, db.Create(&seedChannel).Error)
	require.NoError(t, db.Create(&otherChannel).Error)
	require.NoError(t, db.Create(&model.Task{TaskID: "seed-task", UserId: seedUser.Id, ChannelId: seedChannel.Id}).Error)
	require.NoError(t, db.Create(&model.Task{TaskID: "other-task", UserId: otherUser.Id, ChannelId: otherChannel.Id}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: seedUser.Id, RequestId: "seed-log"}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: otherUser.Id, RequestId: "other-log"}).Error)
	require.NoError(t, db.Create(&model.QuotaData{UserID: seedUser.Id, ChannelID: seedChannel.Id}).Error)
	require.NoError(t, db.Create(&model.QuotaData{UserID: otherUser.Id, ChannelID: otherChannel.Id}).Error)
	seedRequest := model.CostAccountingRequest{ID: 400, RequestID: "seed-request", UserID: seedUser.Id}
	otherRequest := model.CostAccountingRequest{ID: 401, RequestID: "other-request", UserID: otherUser.Id}
	require.NoError(t, db.Create(&seedRequest).Error)
	require.NoError(t, db.Create(&otherRequest).Error)
	require.NoError(t, db.Create(&model.CostAccountingAttempt{ID: 500, CostRequestID: seedRequest.ID, AttemptNo: 1, ChannelID: seedChannel.Id, ChannelName: seedChannel.Name}).Error)
	require.NoError(t, db.Create(&model.CostAccountingAttempt{ID: 501, CostRequestID: otherRequest.ID, AttemptNo: 1, ChannelID: otherChannel.Id, ChannelName: otherChannel.Name}).Error)
	require.NoError(t, db.Create(&model.CostAccountingAttempt{ID: 502, CostRequestID: otherRequest.ID, AttemptNo: 2, ChannelID: seedChannel.Id, ChannelName: seedChannel.Name}).Error)
	require.NoError(t, db.Create(&model.CostAccountingAudit{ID: 600, CostRequestID: seedRequest.ID}).Error)
	require.NoError(t, db.Create(&model.CostAccountingAudit{ID: 601, CostRequestID: otherRequest.ID}).Error)
	otherAttemptID := int64(502)
	require.NoError(t, db.Create(&model.CostAccountingAudit{ID: 602, CostRequestID: otherRequest.ID, CostAttemptID: &otherAttemptID}).Error)
	require.NoError(t, db.Create(&model.ChannelModelCostRule{ID: 700, ChannelID: seedChannel.Id, BillableUpstreamModel: "seed-model", CostVariantKey: "720p", Version: 1, Status: string(types.CostRuleActive), Source: "local_seed"}).Error)
	require.NoError(t, db.Create(&model.ChannelModelCostRule{ID: 701, ChannelID: otherChannel.Id, BillableUpstreamModel: "other-model", CostVariantKey: "720p", Version: 1, Status: string(types.CostRuleActive), Source: "manual"}).Error)

	require.NoError(t, cleanupSeedData(seedUser.Id, seedTokenRow.Id, map[string]int{"line": seedChannel.Id}))

	for _, table := range []any{&model.Task{}, &model.Log{}, &model.QuotaData{}, &model.CostAccountingRequest{}} {
		var count int64
		require.NoError(t, db.Model(table).Count(&count).Error)
		require.Equal(t, int64(1), count)
	}
	var attemptCount int64
	require.NoError(t, db.Model(&model.CostAccountingAttempt{}).Count(&attemptCount).Error)
	require.Equal(t, int64(2), attemptCount)
	var auditCount int64
	require.NoError(t, db.Model(&model.CostAccountingAudit{}).Count(&auditCount).Error)
	require.Equal(t, int64(2), auditCount)
	var costRuleCount int64
	require.NoError(t, db.Model(&model.ChannelModelCostRule{}).Count(&costRuleCount).Error)
	require.Equal(t, int64(1), costRuleCount)
	require.NoError(t, db.First(&seedUser, seedUser.Id).Error)
	require.Zero(t, seedUser.UsedQuota)
	require.Zero(t, seedUser.RequestCount)
	require.NoError(t, db.First(&seedTokenRow, seedTokenRow.Id).Error)
	require.Zero(t, seedTokenRow.UsedQuota)
	require.NoError(t, db.First(&seedChannel, seedChannel.Id).Error)
	require.Equal(t, int64(123), seedChannel.UsedQuota)
	require.NoError(t, db.First(&otherUser, otherUser.Id).Error)
	require.Equal(t, 456, otherUser.UsedQuota)
}

func TestSeedRuntimeSettingsPersistsRoutingGroupWithoutOverwritingPricing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	previousGroups := setting.UserUsableGroups2JSONString()
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	previousModelRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios))
	})
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"seedance-2.0":7,"seedance-2.0-fast":8,"seedance-2.0-mini":9}`))

	require.NoError(t, seedRuntimeSettings(seedRunSettings{
		CostMode:                 types.CostAccountingTracking,
		MinimumExpectedMarginBPS: 0,
	}))

	var usableGroupsOption model.Option
	require.NoError(t, db.First(&usableGroupsOption, "key = ?", "UserUsableGroups").Error)
	var usableGroups map[string]string
	require.NoError(t, common.UnmarshalJsonStr(usableGroupsOption.Value, &usableGroups))
	require.Equal(t, "ARK SDK material matrix local", usableGroups[seedGroup])

	var groupRatioOption model.Option
	require.NoError(t, db.First(&groupRatioOption, "key = ?", "GroupRatio").Error)
	var groupRatios map[string]float64
	require.NoError(t, common.UnmarshalJsonStr(groupRatioOption.Value, &groupRatios))
	require.Equal(t, seedGroupRatio, groupRatios[seedGroup])

	var modelRatioOptionCount int64
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "ModelRatio").Count(&modelRatioOptionCount).Error)
	require.Zero(t, modelRatioOptionCount)
	modelRatios := ratio_setting.GetModelRatioCopy()
	require.Equal(t, 7.0, modelRatios["seedance-2.0"])
	require.Equal(t, 8.0, modelRatios["seedance-2.0-fast"])
	require.Equal(t, 9.0, modelRatios["seedance-2.0-mini"])
}

func TestLoadSeedRunSettingsReadsStrictMargin(t *testing.T) {
	t.Setenv("ARK_VIDEO_MATERIAL_SEED_COST_MODE", "strict")
	t.Setenv("ARK_VIDEO_MATERIAL_SEED_MIN_MARGIN_BPS", "5000")

	settings, err := loadSeedRunSettings()

	require.NoError(t, err)
	require.Equal(t, types.CostAccountingStrict, settings.CostMode)
	require.Equal(t, 5_000, settings.MinimumExpectedMarginBPS)
}

func TestLoadSeedRunSettingsRejectsFiveHundredPercentMargin(t *testing.T) {
	t.Setenv("ARK_VIDEO_MATERIAL_SEED_COST_MODE", "strict")
	t.Setenv("ARK_VIDEO_MATERIAL_SEED_MIN_MARGIN_BPS", "50000")

	_, err := loadSeedRunSettings()

	require.ErrorContains(t, err, "between 0 and 10000 basis points")
}

func TestExpectedStrictRoutingBlockRequiresNoUpstreamCall(t *testing.T) {
	require.True(t, expectedStrictRoutingBlock(types.CostAccountingStrict, http.StatusServiceUnavailable, 4, 4))
	require.False(t, expectedStrictRoutingBlock(types.CostAccountingStrict, http.StatusServiceUnavailable, 4, 5))
	require.False(t, expectedStrictRoutingBlock(types.CostAccountingTracking, http.StatusServiceUnavailable, 4, 4))
	require.False(t, expectedStrictRoutingBlock(types.CostAccountingStrict, http.StatusBadRequest, 4, 4))
}

func TestLoadChannelDefinitionsPreservesImportedProviderTypesAndModels(t *testing.T) {
	document, err := loadDocument(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)

	definitions, err := loadChannelDefinitions(document)
	require.NoError(t, err)
	require.Equal(t, constant.ChannelTypeFFLink, definitions["channel-fflink"].Type)
	require.Equal(t, constant.ChannelTypeZZone, definitions["channel-zzone"].Type)
	require.Equal(t, constant.ChannelTypeMegaByAI, definitions["megabyai-fast-real-person"].Type)
	require.Equal(t, constant.ChannelTypeMikoto, definitions["mikoto-sd"].Type)
	require.True(t, definitions["channel-fflink"].Enabled)
	require.True(t, definitions["channel-zzone"].Enabled)
	require.Contains(t, definitions["channel-fflink"].Models, "seedance-2.0")
	require.Contains(t, definitions["channel-fflink"].Models, "seedance-2.0-fast")
	require.Contains(t, definitions["megabyai-fast-real-person"].Models, "videos-fast")
	require.Contains(t, definitions["mikoto-sd"].Models, "seedance-fast-720p")
	require.Contains(t, definitions["channel-zzone"].Models, "video-ds-2.0")
}

func TestImportedCostRuleConfigPreservesSourcePriceAndCurrency(t *testing.T) {
	document, err := loadDocument(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)

	rule, ok := findImportedCostRule(document, "route-target/MAP-MEGABYAI-R136-480")
	require.True(t, ok)
	config, err := importedCostRuleConfig(rule)
	require.NoError(t, err)
	require.Equal(t, types.CostModePerRequest, config.mode)
	require.Equal(t, types.CostChargeTaskSucceeded, config.value.ChargeEvent)
	require.Equal(t, "CNY", config.value.Currency)
	require.Equal(t, "3.5", *config.value.UnitPrice)
	require.Equal(t, "0.136986301369863", config.value.CurrencyToUSDRate)
	require.Equal(t, "0.4794520547945205", *config.value.NormalizedUSDPrices.UnitPrice)
	require.NotEqual(t, "0.20", *config.value.UnitPrice)

	durationRule, ok := findImportedCostRule(document, "route-target/MAP-FFLINK-R222-720")
	require.True(t, ok)
	durationConfig, err := importedCostRuleConfig(durationRule)
	require.NoError(t, err)
	require.Equal(t, types.CostModePerDuration, durationConfig.mode)
	require.Equal(t, "0.25", *durationConfig.value.PricePerSecond)
	require.Equal(t, types.CostChargeTaskSucceeded, durationConfig.value.ChargeEvent)
	require.Equal(t, types.CostMeterValidatedRequest, durationConfig.value.MeterSource)

}

func TestSeedCostRulesRequiresPublishedActiveRule(t *testing.T) {
	targets, err := loadTargets(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)
	var enabled matrixTarget
	for _, target := range targets {
		document, loadErr := loadDocument(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
		require.NoError(t, loadErr)
		draft, ok := findImportedCostRule(document, target.RouteTargetRef)
		if ok && draft.Enabled != nil && *draft.Enabled {
			enabled = target
			break
		}
	}
	require.NotEmpty(t, enabled.RouteTargetRef)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelModelCostRule{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	workingDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Join("..", "..")))
	t.Cleanup(func() { require.NoError(t, os.Chdir(workingDirectory)) })

	err = seedCostRules([]matrixTarget{enabled}, map[string]int{enabled.LineRef: 7})
	require.ErrorContains(t, err, "published active cost rule")
	var count int64
	require.NoError(t, db.Model(&model.ChannelModelCostRule{}).Count(&count).Error)
	require.Zero(t, count)
}

func modelroutingCanonicalModels() string {
	return strings.Join([]string{"seedance-2.0", "seedance-2.0-fast", "seedance-2.0-mini"}, ",")
}
