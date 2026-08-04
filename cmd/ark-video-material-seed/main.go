package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	seedGroup        = "ark-sdk-material-matrix-local"
	seedUsername     = "ark_sdk_matrix_user"
	seedPassword     = "local-seed-password"
	seedToken        = "arkmatrixlocal"
	seedTokenName    = "ARK SDK material matrix local seed"
	seedChannelName  = "ark-sdk-matrix-mock-"
	seedAssetBaseURL = "http://cdn.openai.com/ark-matrix"
	seedGroupRatio   = 1.25
)

type matrixTarget struct {
	CaseID                             string
	RouteTargetRef                     string
	LineRef                            string
	Provider                           string
	RuntimeModel                       string
	UpstreamModel                      string
	Resolution                         string
	Duration                           int
	Durations                          modelrouting.DurationConstraint
	CostVariantKey                     string
	CostEnabled                        bool
	Minimums                           modelrouting.ReferenceLimits
	References                         modelrouting.ReferenceLimits
	RequestRefs                        modelrouting.ReferenceLimits
	ReferenceTotalMax                  *int
	ReferenceVideoAudioTotalMax        *int
	ReferenceVideoTotalDurationSeconds *int
	AspectRatios                       []string
	InputModes                         []modelrouting.InputMode
	ReferenceModes                     []string
	SupportsRealPerson                 *bool
	ChannelType                        int
}

type channelDefinition struct {
	Type        int
	Models      []string
	SecureGroup dto.SecureVideoGroup
	Enabled     bool
}

type importedCostConfig struct {
	mode  types.CostMode
	value types.CostRuleConfigV1
}

type seedResult struct {
	TotalTargets       int
	AcceptedTasks      int
	FailedMockTasks    int64
	ContractBlocks     int
	DisabledPricing    int
	CommonLogs         int64
	TaskRows           int64
	TerminalResults    int64
	QuotaDataRows      int64
	CostRequests       int64
	CostAttempts       int64
	CostSettled        int64
	CostConfirmedZero  int64
	CostFailed         int64
	ProfitComplete     int64
	NegativeProfit     int64
	RevenueNanoUSD     int64
	CostNanoUSD        int64
	GrossProfitNanoUSD int64
	MaterialLimits     map[string]int
	MockUpstreamCalls  int
	UserID             int
	TokenID            int
}

type mockVideoServer struct {
	mu          sync.Mutex
	nextID      int
	requests    []mockVideoRequest
	models      map[string]string
	failedTasks map[string]bool
	failNext    bool
}

type mockVideoRequest struct {
	Method        string
	Path          string
	Authorization string
	Body          []byte
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if err := initResources(); err != nil {
		return err
	}
	defer func() { _ = model.CloseDB() }()

	if os.Getenv("ARK_VIDEO_MATERIAL_SEED_AUTH_ONLY") == "1" {
		user, token, err := seedUserAndToken()
		if err != nil {
			return err
		}
		fmt.Printf("ARK SDK seed credentials repaired: user=%s (id=%d), token=%s (id=%d)\n", seedUsername, user.Id, seedToken, token.Id)
		return nil
	}

	targets, err := loadTargets(filepath.Join("e2e", "testdata", "channel-config-v1.json"))
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("no material matrix targets found")
	}

	mock := &mockVideoServer{models: make(map[string]string)}
	server := httptest.NewServer(mock)
	defer server.Close()
	assetClient, err := fixtureAssetClient(server)
	if err != nil {
		return err
	}
	metadataFetcher := videometa.NewFetcher(videometa.FetcherOptions{
		Client: assetClient, Cache: videometa.NewCache(64), TempDir: os.TempDir(),
	})
	const metadataToken = "ark-sdk-matrix-metadata"
	metadataServer := httptest.NewServer(videometa.NewServer(videometa.ServerOptions{
		Token: metadataToken, MaxConcurrency: 8, Metadata: metadataFetcher.Metadata,
	}))
	defer metadataServer.Close()
	service.SetVideoMetadataClient(service.NewHTTPVideoMetadataClient(
		metadataServer.URL, metadataToken, metadataServer.Client(), videometa.MaxVideoBytes,
	))
	defer service.SetVideoMetadataClient(nil)
	service.SetReferenceAudioDurationResolver(service.NewReferenceAudioDurationResolver(assetClient, os.TempDir()))
	defer service.SetReferenceAudioDurationResolver(nil)

	user, token, err := seedUserAndToken()
	if err != nil {
		return err
	}
	channelIDs, err := seedChannels(server.URL)
	if err != nil {
		return err
	}
	if err := cleanupSeedData(user.Id, token.Id, channelIDs); err != nil {
		return err
	}
	if err := seedRuntimeSettings(); err != nil {
		return err
	}
	if err := seedCostRules(targets, channelIDs); err != nil {
		return err
	}
	if err := model.InitRoutingPolicyCache(); err != nil {
		return err
	}
	model.InitChannelCache()
	service.InvalidateCostCoverage(0, "", "")

	engine := localRouter()
	policyIDs, err := existingPolicyIDs()
	if err != nil {
		return err
	}
	materialLimits := make(map[string]int)
	createdTasks := make([]string, 0, len(targets))
	acceptedTargets := make([]matrixTarget, 0, len(targets))
	contractBlocks := 0
	disabledPricing := 0

	for _, target := range targets {
		materialLimits[fmt.Sprintf("%d%d%d", target.References.Images, target.References.Videos, target.References.Audios)]++
		if !target.CostEnabled {
			disabledPricing++
			continue
		}
		channelID := channelIDs[target.LineRef]
		if channelID == 0 {
			return fmt.Errorf("missing mock channel for line %q", target.LineRef)
		}
		policyID, err := saveSingleTargetPolicy(policyIDs[target.RuntimeModel], target, channelID)
		if err != nil {
			var policyErr *service.RoutingPolicyServiceError
			if errors.As(err, &policyErr) && policyErr.Code == "incompatible_channel_contract" {
				contractBlocks++
				continue
			}
			return fmt.Errorf("%s: save routing policy: %w", target.CaseID, err)
		}
		policyIDs[target.RuntimeModel] = policyID

		status, body := performJSONRequest(engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer "+seedToken, requestBody(target, seedAssetBaseURL))
		if status != http.StatusOK {
			return fmt.Errorf("%s: submit failed with HTTP %d: %s", target.CaseID, status, strings.TrimSpace(string(body)))
		}
		var created struct {
			ID string `json:"id"`
		}
		if err := common.Unmarshal(body, &created); err != nil {
			return err
		}
		if !strings.HasPrefix(created.ID, "task_") {
			return fmt.Errorf("%s: unexpected task id %q", target.CaseID, created.ID)
		}
		createdTasks = append(createdTasks, created.ID)

		summary := service.RunTaskPollingOnce(context.Background(), nil)
		if summary.UnfinishedTasks < 1 {
			return fmt.Errorf("%s: polling did not process unfinished task", target.CaseID)
		}
		var task model.Task
		if err := model.DB.Where("task_id = ?", created.ID).First(&task).Error; err != nil {
			return err
		}
		if task.Status != model.TaskStatusSuccess {
			return fmt.Errorf("%s: task %s finished as %s", target.CaseID, created.ID, task.Status)
		}
		if len(task.PrivateData.UserResponseData) == 0 {
			return fmt.Errorf("%s: task %s is missing terminal user response", target.CaseID, created.ID)
		}
		if err := validateArkTerminalResponse(task.PrivateData.UserResponseData, model.TaskStatusSuccess, created.ID, target.RuntimeModel); err != nil {
			return fmt.Errorf("%s: task %s has invalid terminal user response: %w", target.CaseID, created.ID, err)
		}
		acceptedTargets = append(acceptedTargets, target)
	}

	failureTarget := selectFailureFixtureTarget(acceptedTargets)
	if failureTarget == nil {
		return errors.New("no accepted target is available for the failure fixture")
	}
	failureChannelID := channelIDs[failureTarget.LineRef]
	policyID, err := saveSingleTargetPolicy(policyIDs[failureTarget.RuntimeModel], *failureTarget, failureChannelID)
	if err != nil {
		return fmt.Errorf("save failure fixture routing policy: %w", err)
	}
	policyIDs[failureTarget.RuntimeModel] = policyID
	mock.failNextTask()
	status, body := performJSONRequest(engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer "+seedToken, requestBody(*failureTarget, seedAssetBaseURL))
	if status != http.StatusOK {
		return fmt.Errorf("failure fixture submit failed with HTTP %d: %s", status, strings.TrimSpace(string(body)))
	}
	var failedCreated struct {
		ID string `json:"id"`
	}
	if err := common.Unmarshal(body, &failedCreated); err != nil {
		return err
	}
	if !strings.HasPrefix(failedCreated.ID, "task_") {
		return fmt.Errorf("failure fixture returned unexpected task id %q", failedCreated.ID)
	}
	createdTasks = append(createdTasks, failedCreated.ID)
	if summary := service.RunTaskPollingOnce(context.Background(), nil); summary.UnfinishedTasks < 1 {
		return errors.New("failure fixture polling did not process unfinished task")
	}
	var failedTask model.Task
	if err := model.DB.Where("task_id = ?", failedCreated.ID).First(&failedTask).Error; err != nil {
		return err
	}
	if failedTask.Status != model.TaskStatusFailure {
		return fmt.Errorf("failure fixture task %s finished as %s", failedCreated.ID, failedTask.Status)
	}
	if len(failedTask.PrivateData.UserResponseData) == 0 {
		return fmt.Errorf("failure fixture task %s is missing terminal user response", failedCreated.ID)
	}
	if err := validateArkTerminalResponse(failedTask.PrivateData.UserResponseData, model.TaskStatusFailure, failedCreated.ID, failureTarget.RuntimeModel); err != nil {
		return fmt.Errorf("failure fixture task %s has invalid terminal user response: %w", failedCreated.ID, err)
	}
	if !strings.Contains(string(failedTask.PrivateData.UserResponseData), "mock content policy rejection") {
		return fmt.Errorf("failure fixture task %s is missing the public failure reason", failedCreated.ID)
	}

	model.SaveQuotaDataCache()
	result, err := summarizeResult(user.Id, token.Id, len(targets), len(createdTasks)-1, contractBlocks, disabledPricing, createdTasks, materialLimits, mock.count())
	if err != nil {
		return err
	}
	if result.CostRequests != int64(result.AcceptedTasks)+result.FailedMockTasks ||
		result.CostAttempts != result.CostRequests {
		return fmt.Errorf("incomplete cost accounting rows: requests=%d attempts=%d successful_tasks=%d failed_tasks=%d",
			result.CostRequests, result.CostAttempts, result.AcceptedTasks, result.FailedMockTasks)
	}
	if result.CostSettled != int64(result.AcceptedTasks) ||
		result.CostConfirmedZero != result.FailedMockTasks || result.CostFailed != 0 {
		return fmt.Errorf("incomplete cost settlement: settled=%d confirmed_zero=%d settlement_failed=%d",
			result.CostSettled, result.CostConfirmedZero, result.CostFailed)
	}
	if result.ProfitComplete != result.CostRequests ||
		result.RevenueNanoUSD-result.CostNanoUSD != result.GrossProfitNanoUSD {
		return fmt.Errorf("incomplete profit accounting: complete=%d requests=%d revenue=%d cost=%d gross_profit=%d",
			result.ProfitComplete, result.CostRequests, result.RevenueNanoUSD, result.CostNanoUSD, result.GrossProfitNanoUSD)
	}
	printResult(result)
	return nil
}

type arkTerminalResponse struct {
	ID          string `json:"id"`
	Model       string `json:"model"`
	Status      string `json:"status"`
	Resolution  string `json:"resolution"`
	Ratio       string `json:"ratio"`
	ServiceTier string `json:"service_tier"`
	Content     *struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Usage *struct {
		CompletionTokens *int64 `json:"completion_tokens"`
		TotalTokens      *int64 `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	CreatedAt             *json.Number `json:"created_at"`
	UpdatedAt             *json.Number `json:"updated_at"`
	Seed                  *json.Number `json:"seed"`
	Duration              *json.Number `json:"duration"`
	FramesPerSecond       *json.Number `json:"framespersecond"`
	ExecutionExpiresAfter *json.Number `json:"execution_expires_after"`
	Priority              *json.Number `json:"priority"`
	GenerateAudio         *bool        `json:"generate_audio"`
	Draft                 *bool        `json:"draft"`
}

func validateArkTerminalResponse(data json.RawMessage, wantStatus model.TaskStatus, expectedID, expectedModel string) error {
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(data, &fields); err != nil {
		return err
	}
	allowedFields := map[string]struct{}{
		"id": {}, "model": {}, "status": {}, "content": {}, "usage": {}, "error": {},
		"created_at": {}, "updated_at": {}, "seed": {}, "resolution": {}, "ratio": {},
		"duration": {}, "framespersecond": {}, "service_tier": {}, "execution_expires_after": {},
		"generate_audio": {}, "draft": {}, "priority": {},
	}
	for field := range fields {
		if _, ok := allowedFields[field]; !ok {
			return fmt.Errorf("unexpected field %s", field)
		}
	}

	var response arkTerminalResponse
	if err := common.Unmarshal(data, &response); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"id": response.ID, "model": response.Model, "status": response.Status,
		"resolution": response.Resolution, "ratio": response.Ratio, "service_tier": response.ServiceTier,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing %s", name)
		}
	}
	if expectedID != "" && response.ID != expectedID {
		return fmt.Errorf("unexpected id %q", response.ID)
	}
	if expectedModel != "" && response.Model != expectedModel {
		return fmt.Errorf("unexpected model %q", response.Model)
	}
	for _, integer := range []struct {
		name    string
		value   *json.Number
		minimum int64
		maximum int64
	}{
		{name: "created_at", value: response.CreatedAt, minimum: 0, maximum: 9223372036854775807},
		{name: "updated_at", value: response.UpdatedAt, minimum: 0, maximum: 9223372036854775807},
		{name: "seed", value: response.Seed, minimum: 0, maximum: 1<<31 - 1},
		{name: "duration", value: response.Duration, minimum: 1, maximum: int64(relaycommon.MaxTaskDurationSeconds)},
		{name: "framespersecond", value: response.FramesPerSecond, minimum: 1, maximum: 240},
		{name: "execution_expires_after", value: response.ExecutionExpiresAfter, minimum: 1, maximum: 1<<31 - 1},
		{name: "priority", value: response.Priority, minimum: 0, maximum: 1<<31 - 1},
	} {
		if err := validateArkInteger(integer.name, integer.value, integer.minimum, integer.maximum); err != nil {
			return err
		}
	}
	if response.GenerateAudio == nil {
		return errors.New("missing generate_audio")
	}
	if response.Draft == nil {
		return errors.New("missing draft")
	}
	if response.Usage == nil || response.Usage.CompletionTokens == nil || response.Usage.TotalTokens == nil {
		return errors.New("missing usage")
	}
	if *response.Usage.CompletionTokens < 0 || *response.Usage.CompletionTokens > int64(relaycommon.MaxTokensLimit) ||
		*response.Usage.TotalTokens < *response.Usage.CompletionTokens || *response.Usage.TotalTokens > int64(relaycommon.MaxTokensLimit) {
		return errors.New("invalid usage")
	}
	if err := validateArkObjectFields(fields["usage"], "usage", "completion_tokens", "total_tokens"); err != nil {
		return err
	}

	switch wantStatus {
	case model.TaskStatusSuccess:
		if response.Status != "succeeded" || response.Content == nil || strings.TrimSpace(response.Content.VideoURL) == "" {
			return errors.New("successful response is missing content.video_url")
		}
		if response.Error != nil {
			return errors.New("successful response must not contain error")
		}
		if err := validateArkObjectFields(fields["content"], "content", "video_url"); err != nil {
			return err
		}
	case model.TaskStatusFailure:
		validFailureStatus := response.Status == "failed" || response.Status == "expired" || response.Status == "cancelled"
		if !validFailureStatus || response.Error == nil ||
			strings.TrimSpace(response.Error.Code) == "" || strings.TrimSpace(response.Error.Message) == "" {
			return errors.New("failed response is missing error")
		}
		if response.Content != nil {
			return errors.New("failed response must not contain content")
		}
		if err := validateArkObjectFields(fields["error"], "error", "code", "message"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported terminal task status %s", wantStatus)
	}
	return nil
}

func validateArkInteger(name string, value *json.Number, minimum, maximum int64) error {
	if value == nil {
		return fmt.Errorf("missing %s", name)
	}
	number, err := decimal.NewFromString(value.String())
	if err != nil || !number.Equal(number.Truncate(0)) ||
		number.LessThan(decimal.NewFromInt(minimum)) || number.GreaterThan(decimal.NewFromInt(maximum)) {
		return fmt.Errorf("invalid %s", name)
	}
	return nil
}

func validateArkObjectFields(data json.RawMessage, name string, allowed ...string) error {
	var fields map[string]json.RawMessage
	if len(data) == 0 || common.Unmarshal(data, &fields) != nil {
		return fmt.Errorf("invalid %s", name)
	}
	allowedFields := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedFields[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := allowedFields[field]; !ok {
			return fmt.Errorf("unexpected %s.%s", name, field)
		}
	}
	return nil
}

func selectFailureFixtureTarget(targets []matrixTarget) *matrixTarget {
	if len(targets) == 0 {
		return nil
	}
	for i := range targets {
		if targets[i].ChannelType == constant.ChannelTypeNewAPIVideo {
			return &targets[i]
		}
	}
	return &targets[0]
}

func initResources() error {
	_ = godotenv.Load(".env")
	common.InitEnv()
	common.RedisEnabled = false
	logger.SetupLogger()
	ratio_setting.InitRatioSettings()
	service.InitHttpClient()
	service.SetRoutingRevenuePreview(func(_ context.Context, input service.RoutingRevenuePreviewInput) (int64, string, error) {
		return relayhelper.PreviewRoutingRevenueWithSeedanceInput(input.OriginModelName, input.Group, input.RequestPath, input.RelayMode, input.DurationSeconds, input.UserId, input.OutputResolution, input.HasReferenceVideo, input.InputVideoDurationMS)
	})
	service.InitTokenEncoders()
	if err := model.InitDB(); err != nil {
		return err
	}
	if err := model.InitRoutingPolicyCache(); err != nil {
		return err
	}
	model.InitOptionMap()
	if err := model.InitLogDB(); err != nil {
		return err
	}
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	service.CostCapabilityLookup = relay.CostCapabilitiesForRoute
	service.RouteTargetContractValidator = relay.ValidateVideoRouteTargetContract
	return nil
}

func seedUserAndToken() (*model.User, *model.Token, error) {
	now := common.GetTimestamp()
	passwordHash, err := seedPasswordHash()
	if err != nil {
		return nil, nil, err
	}
	user := &model.User{}
	err = model.DB.Where("username = ?", seedUsername).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = &model.User{
			Username: seedUsername, Password: passwordHash, DisplayName: "ARK SDK Matrix",
			Role: common.RoleRootUser, Status: common.UserStatusEnabled, Quota: 2_000_000_000,
			Group: seedGroup, AffCode: "ark-sdk-material-matrix-local", CreatedAt: now,
		}
		if err := model.DB.Create(user).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	} else {
		updates := map[string]any{
			"role": common.RoleRootUser, "status": common.UserStatusEnabled, "quota": 2_000_000_000, "group": seedGroup,
		}
		if !common.ValidatePasswordAndHash(seedPassword, user.Password) {
			updates["password"] = passwordHash
		}
		if err := model.DB.Model(user).Updates(updates).Error; err != nil {
			return nil, nil, err
		}
	}

	token := &model.Token{}
	err = model.DB.Where(commonKeyColumn()+" = ?", seedToken).First(token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		token = &model.Token{
			UserId: user.Id, Key: seedToken, Name: seedTokenName, Status: common.TokenStatusEnabled,
			CreatedTime: now, ExpiredTime: -1, RemainQuota: 2_000_000_000, UnlimitedQuota: true, Group: seedGroup,
		}
		if err := model.DB.Create(token).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	} else if err := model.DB.Model(token).Updates(map[string]any{
		"user_id": user.Id, "name": seedTokenName, "status": common.TokenStatusEnabled, "expired_time": -1,
		"remain_quota": 2_000_000_000, "unlimited_quota": true, "group": seedGroup,
	}).Error; err != nil {
		return nil, nil, err
	}
	return user, token, nil
}

func seedPasswordHash() (string, error) {
	return common.Password2Hash(seedPassword)
}

func cleanupSeedData(userID, tokenID int, channelIDs map[string]int) error {
	channelIDList := make([]int, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channelIDList = append(channelIDList, channelID)
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var costRequestIDs []int64
		if err := tx.Model(&model.CostAccountingRequest{}).Where("user_id = ?", userID).Pluck("id", &costRequestIDs).Error; err != nil {
			return err
		}

		var attemptIDs []int64
		if len(costRequestIDs) > 0 {
			if err := tx.Model(&model.CostAccountingAttempt{}).
				Where("cost_request_id IN ?", costRequestIDs).
				Pluck("id", &attemptIDs).Error; err != nil {
				return err
			}
		}
		if len(costRequestIDs) > 0 && len(attemptIDs) > 0 {
			if err := tx.Where("cost_request_id IN ? OR cost_attempt_id IN ?", costRequestIDs, attemptIDs).Delete(&model.CostAccountingAudit{}).Error; err != nil {
				return err
			}
		} else if len(costRequestIDs) > 0 {
			if err := tx.Where("cost_request_id IN ?", costRequestIDs).Delete(&model.CostAccountingAudit{}).Error; err != nil {
				return err
			}
		} else if len(attemptIDs) > 0 {
			if err := tx.Where("cost_attempt_id IN ?", attemptIDs).Delete(&model.CostAccountingAudit{}).Error; err != nil {
				return err
			}
		}
		if len(attemptIDs) > 0 {
			if err := tx.Where("id IN ?", attemptIDs).Delete(&model.CostAccountingAttempt{}).Error; err != nil {
				return err
			}
		}
		if len(costRequestIDs) > 0 {
			if err := tx.Where("id IN ?", costRequestIDs).Delete(&model.CostAccountingRequest{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.Task{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.QuotaData{}).Error; err != nil {
			return err
		}
		if len(channelIDList) > 0 {
			if err := tx.Where("channel_id IN ? AND source = ?", channelIDList, "local_seed").Delete(&model.ChannelModelCostRule{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{"used_quota": 0, "request_count": 0}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", 0).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return model.LOG_DB.Where("user_id = ?", userID).Delete(&model.Log{}).Error
}

func seedRuntimeSettings() error {
	usableGroups := setting.GetUserUsableGroupsCopy()
	usableGroups[seedGroup] = "ARK SDK material matrix local"
	encodedGroups, err := common.Marshal(usableGroups)
	if err != nil {
		return err
	}
	if err := setting.UpdateUserUsableGroupsByJSONString(string(encodedGroups)); err != nil {
		return err
	}

	groupRatios := ratio_setting.GetGroupRatioCopy()
	groupRatios[seedGroup] = seedGroupRatio
	encodedGroupRatios, err := common.Marshal(groupRatios)
	if err != nil {
		return err
	}
	if err := ratio_setting.UpdateGroupRatioByJSONString(string(encodedGroupRatios)); err != nil {
		return err
	}

	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	if err := config.UpdateConfigFromMap(costConfig, map[string]string{
		cost_setting.KeyMode:                     string(types.CostAccountingTracking),
		cost_setting.KeyMinimumExpectedMarginBPS: "0",
	}); err != nil {
		return err
	}
	cost_setting.UpdateAndSync()
	return model.UpdateOptionsBulk(map[string]string{
		"UserUsableGroups": string(encodedGroups),
		"GroupRatio":       string(encodedGroupRatios),
		cost_setting.ConfigName + "." + cost_setting.KeyMode:                     string(types.CostAccountingTracking),
		cost_setting.ConfigName + "." + cost_setting.KeyMinimumExpectedMarginBPS: "0",
	})
}

func seedChannels(upstreamURL string) (map[string]int, error) {
	document, err := loadDocument(filepath.Join("e2e", "testdata", "channel-config-v1.json"))
	if err != nil {
		return nil, err
	}
	definitions, err := loadChannelDefinitions(document)
	if err != nil {
		return nil, err
	}
	lineRefs := make([]string, 0, len(document.Entities.ChannelLines))
	for _, line := range document.Entities.ChannelLines {
		lineRefs = append(lineRefs, line.LineRef)
	}
	sort.Strings(lineRefs)

	channelIDs := make(map[string]int, len(lineRefs))
	currentChannelNames := make(map[string]struct{}, len(lineRefs))
	for index, lineRef := range lineRefs {
		definition, ok := definitions[lineRef]
		if !ok {
			return nil, fmt.Errorf("missing channel definition for line %q", lineRef)
		}
		name := seedChannelName + lineRef
		currentChannelNames[name] = struct{}{}
		priority := int64(100 - index)
		weight := uint(100)
		channel := &model.Channel{}
		err := model.DB.Where("name = ?", name).First(channel).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			channel = &model.Channel{CreatedTime: common.GetTimestamp()}
		} else if err != nil {
			return nil, err
		}
		channel.Type = definition.Type
		channel.Key = "matrix-local-key-" + lineRef
		channel.Status = common.ChannelStatusManuallyDisabled
		if definition.Enabled {
			channel.Status = common.ChannelStatusEnabled
		}
		channel.Name = name
		channel.BaseURL = common.GetPointer(upstreamURL)
		channel.Models = strings.Join(definition.Models, ",")
		channel.Group = seedGroup
		channel.Priority = &priority
		channel.Weight = &weight
		channel.OtherSettings = "{}"
		channel.SetOtherSettings(dto.ChannelOtherSettings{
			DisableTaskPollingSleep: true,
			SecureVideoGroup:        definition.SecureGroup,
		})
		if channel.Id == 0 {
			if err := channel.Insert(); err != nil {
				return nil, err
			}
		} else if err := channel.Update(); err != nil {
			return nil, err
		}
		channelIDs[lineRef] = channel.Id
	}
	if err := disableRemovedSeedChannels(currentChannelNames); err != nil {
		return nil, err
	}
	return channelIDs, nil
}

func disableRemovedSeedChannels(currentChannelNames map[string]struct{}) error {
	var channels []model.Channel
	if err := model.DB.Where("name LIKE ?", seedChannelName+"%").Find(&channels).Error; err != nil {
		return err
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		for index := range channels {
			channel := &channels[index]
			if _, current := currentChannelNames[channel.Name]; current {
				continue
			}
			if channel.Status != common.ChannelStatusManuallyDisabled {
				if err := tx.Model(&model.Channel{}).Where("id = ?", channel.Id).
					Update("status", common.ChannelStatusManuallyDisabled).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.ChannelModelCostRule{}).
				Where("channel_id = ? AND status = ?", channel.Id, types.CostRuleActive).
				Updates(map[string]any{
					"status": string(types.CostRuleRetired), "effective_to": now, "updated_at": now,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func seedCostRules(targets []matrixTarget, channelIDs map[string]int) error {
	document, err := loadDocument(filepath.Join("e2e", "testdata", "channel-config-v1.json"))
	if err != nil {
		return err
	}
	seen := make(map[string]struct{})
	for _, target := range targets {
		channelID := channelIDs[target.LineRef]
		ruleDraft, ok := findImportedCostRule(document, target.RouteTargetRef)
		if !ok {
			return fmt.Errorf("missing imported cost rule for %q", target.RouteTargetRef)
		}
		if ruleDraft.Enabled == nil {
			return fmt.Errorf("imported cost rule %q is missing enabled state", ruleDraft.BusinessID)
		}
		if !*ruleDraft.Enabled {
			continue
		}
		config, err := importedCostRuleConfig(ruleDraft)
		if err != nil {
			return fmt.Errorf("%s: normalize imported cost rule: %w", target.CaseID, err)
		}
		variants := []string{target.CostVariantKey}
		for _, rawVariant := range variants {
			variant, err := types.NormalizeCostVariantKey(rawVariant)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("%d\x00%s\x00%s", channelID, target.UpstreamModel, variant)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			var existing model.ChannelModelCostRule
			err = model.DB.Where(
				"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ? AND status = ?",
				channelID, target.UpstreamModel, variant, types.CostRuleActive,
			).First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%s: published active cost rule is missing", target.CaseID)
			}
			if err != nil {
				return err
			}
			if existing.Source != "config_import" {
				return fmt.Errorf("%s: active cost rule source is %q, expected config_import", target.CaseID, existing.Source)
			}
			if existing.CostMode != string(config.mode) || existing.SchemaVersion != 1 {
				return fmt.Errorf("%s: published active cost rule contract does not match import", target.CaseID)
			}
			var existingConfig types.CostRuleConfigV1
			if err := common.UnmarshalJsonStr(existing.ConfigJSON, &existingConfig); err != nil {
				return fmt.Errorf("%s: decode published active cost rule: %w", target.CaseID, err)
			}
			existingJSON, err := common.Marshal(existingConfig)
			if err != nil {
				return err
			}
			expectedJSON, err := common.Marshal(config.value)
			if err != nil {
				return err
			}
			if string(existingJSON) != string(expectedJSON) {
				return fmt.Errorf("%s: published active cost rule price does not match import", target.CaseID)
			}
		}
	}
	return nil
}

func saveSingleTargetPolicy(policyID int, target matrixTarget, channelID int) (int, error) {
	view, err := service.SaveRoutingPolicy(policyID, service.RoutingPolicyWriteRequest{
		GroupName: seedGroup,
		Model:     target.RuntimeModel,
		Enabled:   true,
		Defaults: modelrouting.Defaults{
			OutputResolution: target.Resolution,
			DurationSeconds:  target.Duration,
			AspectRatio:      "16:9",
		},
		Targets: []service.RouteTargetWriteRequest{{
			ChannelID:      channelID,
			Name:           target.UpstreamModel,
			UpstreamModel:  target.UpstreamModel,
			CostVariantKey: target.CostVariantKey,
			TargetPriority: 100,
			Enabled:        true,
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
		}},
	})
	if err != nil {
		return 0, err
	}
	return view.ID, nil
}

func existingPolicyIDs() (map[string]int, error) {
	policyIDs := make(map[string]int)
	for _, modelName := range modelrouting.CanonicalModels {
		var policy model.RoutingPolicy
		err := model.DB.Where("group_name = ? AND model = ?", seedGroup, modelName).First(&policy).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		policyIDs[modelName] = policy.ID
	}
	return policyIDs, nil
}

func localRouter() http.Handler {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.SetRelayRouter(engine)
	router.SetVideoRouter(engine)
	return engine
}

func performJSONRequest(handler http.Handler, method, path, authorization, body string) (int, []byte) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder.Code, recorder.Body.Bytes()
}

func requestBody(target matrixTarget, assetBaseURL string) string {
	content := []map[string]any{{"type": "text", "text": "ARK SDK material matrix local seed " + target.CaseID}}
	requestMode := modelrouting.InputModeOmniReference
	if len(target.InputModes) > 0 {
		requestMode = target.InputModes[0]
		for _, mode := range target.InputModes {
			if mode == modelrouting.InputModeOmniReference {
				requestMode = mode
				break
			}
		}
	}
	for index := 0; index < target.RequestRefs.Images; index++ {
		role := "reference_image"
		if requestMode == modelrouting.InputModeFirstFrame {
			role = "first_frame"
		} else if requestMode == modelrouting.InputModeFirstLastFrames {
			if index == 0 {
				role = "first_frame"
			} else {
				role = "last_frame"
			}
		}
		content = append(content, map[string]any{
			"type": "image_url", "role": role,
			"image_url": map[string]any{"url": fmt.Sprintf("https://cdn.openai.com/ark-matrix/%s/image-%02d.png", target.Provider, index+1)},
		})
	}
	for index := 0; index < target.RequestRefs.Videos; index++ {
		content = append(content, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": fmt.Sprintf("%s/sample.mp4?provider=%s&index=%d", assetBaseURL, target.Provider, index+1)},
		})
	}
	for index := 0; index < target.RequestRefs.Audios; index++ {
		content = append(content, map[string]any{
			"type": "audio_url", "role": "reference_audio",
			"audio_url": map[string]any{"url": fmt.Sprintf("%s/audio.wav?provider=%s&index=%d", assetBaseURL, target.Provider, index+1)},
		})
	}
	body, err := common.Marshal(map[string]any{
		"model": target.RuntimeModel, "content": content, "resolution": target.Resolution,
		"duration": target.Duration, "ratio": "16:9",
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func matrixRequestReferences(minimums, limits modelrouting.ReferenceLimits, totalMax, videoAudioMax *int) modelrouting.ReferenceLimits {
	result := limits
	shrinkVideoAudio := func(maximum int) {
		for result.Videos+result.Audios > maximum {
			switch {
			case result.Audios > minimums.Audios && result.Audios >= result.Videos:
				result.Audios--
			case result.Videos > minimums.Videos:
				result.Videos--
			case result.Audios > minimums.Audios:
				result.Audios--
			default:
				return
			}
		}
	}
	if videoAudioMax != nil {
		shrinkVideoAudio(*videoAudioMax)
	}
	if totalMax != nil {
		availableForVideoAudio := *totalMax - result.Images
		minimumVideoAudio := minimums.Videos + minimums.Audios
		if availableForVideoAudio < minimumVideoAudio {
			availableForVideoAudio = minimumVideoAudio
		}
		shrinkVideoAudio(availableForVideoAudio)
		for result.Images+result.Videos+result.Audios > *totalMax && result.Images > minimums.Images {
			result.Images--
		}
	}
	return result
}

func matrixRequestReferencesForInputModes(requestReferences, limits modelrouting.ReferenceLimits, inputModes []modelrouting.InputMode) modelrouting.ReferenceLimits {
	if len(inputModes) == 0 {
		return requestReferences
	}
	for _, mode := range inputModes {
		if mode == modelrouting.InputModeOmniReference {
			return requestReferences
		}
	}
	for _, mode := range inputModes {
		if mode == modelrouting.InputModeFirstLastFrames && limits.Images >= 2 {
			return modelrouting.ReferenceLimits{Images: 2}
		}
	}
	for _, mode := range inputModes {
		if mode == modelrouting.InputModeFirstFrame && limits.Images >= 1 {
			return modelrouting.ReferenceLimits{Images: 1}
		}
	}
	return modelrouting.ReferenceLimits{}
}

func loadTargets(path string) ([]matrixTarget, error) {
	document, err := loadDocument(path)
	if err != nil {
		return nil, err
	}
	definitions, err := loadChannelDefinitions(document)
	if err != nil {
		return nil, err
	}
	lines := make(map[string]types.ConfigImportChannelLine, len(document.Entities.ChannelLines))
	for _, line := range document.Entities.ChannelLines {
		lines[line.LineRef] = line
	}
	targets := make([]matrixTarget, 0)
	costRules := make(map[string]types.ConfigImportCostRuleDraft, len(document.Entities.CostRuleDrafts))
	for _, draft := range document.Entities.CostRuleDrafts {
		costRules[draft.RouteTargetRef] = draft
	}
	for _, blueprint := range document.Entities.RouteBlueprints {
		runtimeModel := runtimeModel(blueprint.CanonicalModel)
		if runtimeModel == "" {
			return nil, fmt.Errorf("unsupported canonical model %q", blueprint.CanonicalModel)
		}
		for _, target := range blueprint.Targets {
			if target.ReferenceLimits == nil || target.ReferenceLimits.Images == nil || target.ReferenceLimits.Videos == nil || target.ReferenceLimits.Audios == nil {
				return nil, fmt.Errorf("%s: reference limits are incomplete", target.RouteTargetRef)
			}
			if len(target.OutputResolutions) == 0 {
				return nil, fmt.Errorf("%s: output resolution is missing", target.RouteTargetRef)
			}
			line, ok := lines[target.LineRef]
			if !ok {
				return nil, fmt.Errorf("%s: channel line %q is missing", target.RouteTargetRef, target.LineRef)
			}
			definition, ok := definitions[target.LineRef]
			if !ok {
				return nil, fmt.Errorf("%s: channel definition for line %q is missing", target.RouteTargetRef, target.LineRef)
			}
			minimums := modelrouting.ReferenceLimits{}
			if target.ReferenceMinimums != nil {
				if target.ReferenceMinimums.Images == nil || target.ReferenceMinimums.Videos == nil || target.ReferenceMinimums.Audios == nil {
					return nil, fmt.Errorf("%s: reference minimums are incomplete", target.RouteTargetRef)
				}
				minimums = modelrouting.ReferenceLimits{
					Images: *target.ReferenceMinimums.Images,
					Videos: *target.ReferenceMinimums.Videos,
					Audios: *target.ReferenceMinimums.Audios,
				}
			}
			references := modelrouting.ReferenceLimits{
				Images: *target.ReferenceLimits.Images,
				Videos: *target.ReferenceLimits.Videos,
				Audios: *target.ReferenceLimits.Audios,
			}
			draft, ok := costRules[target.RouteTargetRef]
			if !ok || draft.Enabled == nil {
				return nil, fmt.Errorf("%s: cost rule enabled state is missing", target.RouteTargetRef)
			}
			duration := 10
			durations := modelrouting.DurationConstraint{Values: append([]int(nil), target.DurationValues...), Min: target.DurationMin, Max: target.DurationMax}
			if len(target.DurationValues) > 0 {
				duration = target.DurationValues[0]
			}
			if target.DurationMin != nil {
				duration = *target.DurationMin
			}
			if len(durations.Values) == 0 && durations.Min == nil && durations.Max == nil {
				durations.Min = common.GetPointer(duration)
				durations.Max = common.GetPointer(duration)
			}
			aspectRatios := make([]string, 0, len(target.AspectRatios))
			for _, value := range target.AspectRatios {
				if normalized := strings.ToLower(strings.TrimSpace(value)); normalized != "" {
					aspectRatios = append(aspectRatios, normalized)
				}
			}
			if len(aspectRatios) == 0 {
				aspectRatios = []string{"16:9"}
			}
			inputModes := make([]modelrouting.InputMode, 0, len(target.InputModes))
			for _, value := range target.InputModes {
				inputModes = append(inputModes, modelrouting.InputMode(strings.ToLower(strings.TrimSpace(value))))
			}
			requestReferences := matrixRequestReferences(minimums, references, target.ReferenceTotalMax, target.ReferenceVideoAudioTotalMax)
			requestReferences = matrixRequestReferencesForInputModes(requestReferences, references, inputModes)
			targets = append(targets, matrixTarget{
				CaseID: fmt.Sprintf("%s/%s/%d%d%d", target.RouteTargetRef, target.OutputResolutions[0], references.Images, references.Videos, references.Audios), RouteTargetRef: target.RouteTargetRef,
				LineRef: target.LineRef, Provider: line.ProviderTypeHint, RuntimeModel: runtimeModel,
				UpstreamModel: target.UpstreamModel, Resolution: strings.ToLower(target.OutputResolutions[0]),
				Duration: duration, Durations: durations, CostVariantKey: target.CostVariantKey, CostEnabled: *draft.Enabled,
				Minimums: minimums, References: references, RequestRefs: requestReferences,
				ReferenceTotalMax: target.ReferenceTotalMax, ReferenceVideoAudioTotalMax: target.ReferenceVideoAudioTotalMax,
				ReferenceVideoTotalDurationSeconds: target.ReferenceVideoTotalDurationSeconds,
				AspectRatios:                       aspectRatios, InputModes: inputModes, ReferenceModes: append([]string(nil), target.ReferenceModes...),
				SupportsRealPerson: target.SupportsRealPerson, ChannelType: definition.Type,
			})
		}
	}
	sort.Slice(targets, func(left, right int) bool {
		return targets[left].CaseID < targets[right].CaseID
	})
	return targets, nil
}

func loadChannelDefinitions(document types.ConfigImportDocument) (map[string]channelDefinition, error) {
	channelTypes := make(map[string]int, len(document.Entities.Channels))
	for _, channel := range document.Entities.Channels {
		if channel.ChannelType == nil {
			return nil, fmt.Errorf("channel %q has no channel type", channel.BusinessID)
		}
		channelType := *channel.ChannelType
		if channel.BusinessID == "CH-4STOKEN" && channelType == constant.ChannelTypeOpenAI {
			channelType = constant.ChannelTypeFourSToken
		} else if channel.BusinessID == "CH-8YES" && channelType == constant.ChannelTypeOpenAI {
			channelType = constant.ChannelTypeEightYes
		}
		channelTypes[channel.BusinessID] = channelType
	}

	definitions := make(map[string]channelDefinition, len(document.Entities.ChannelLines))
	for _, line := range document.Entities.ChannelLines {
		channelType, ok := channelTypes[line.ChannelRef]
		if !ok {
			return nil, fmt.Errorf("line %q references unknown channel %q", line.LineRef, line.ChannelRef)
		}
		definitions[line.LineRef] = channelDefinition{Type: channelType}
	}

	modelSets := make(map[string]map[string]struct{}, len(definitions))
	for _, mapping := range document.Entities.ModelMappings {
		if _, ok := definitions[mapping.LineRef]; !ok {
			continue
		}
		if modelSets[mapping.LineRef] == nil {
			modelSets[mapping.LineRef] = make(map[string]struct{})
		}
		canonical := runtimeModel(mapping.CanonicalModel)
		if canonical == "" || strings.TrimSpace(mapping.UpstreamModel) == "" {
			continue
		}
		modelSets[mapping.LineRef][canonical] = struct{}{}
		modelSets[mapping.LineRef][mapping.UpstreamModel] = struct{}{}
	}
	for _, blueprint := range document.Entities.RouteBlueprints {
		for _, target := range blueprint.Targets {
			definition, ok := definitions[target.LineRef]
			if !ok {
				continue
			}
			definition.Enabled = true
			definitions[target.LineRef] = definition
		}
	}

	for lineRef, definition := range definitions {
		models := make([]string, 0, len(modelSets[lineRef]))
		for modelName := range modelSets[lineRef] {
			models = append(models, modelName)
		}
		sort.Strings(models)
		if len(models) == 0 {
			return nil, fmt.Errorf("line %q has no imported model mappings", lineRef)
		}
		definition.Models = models
		if definition.Type == constant.ChannelTypeSecure {
			switch lineRef {
			case "secure-discount":
				definition.SecureGroup = dto.SecureVideoGroupDiscount
			case "secure-overseas":
				definition.SecureGroup = dto.SecureVideoGroupOverseas
			case "secure-enterprise":
				definition.SecureGroup = dto.SecureVideoGroupEnterprise
			default:
				return nil, fmt.Errorf("secure line %q has no profile mapping", lineRef)
			}
		}
		definitions[lineRef] = definition
	}
	return definitions, nil
}

func findImportedCostRule(document types.ConfigImportDocument, routeTargetRef string) (types.ConfigImportCostRuleDraft, bool) {
	for _, rule := range document.Entities.CostRuleDrafts {
		if rule.RouteTargetRef == routeTargetRef {
			return rule, true
		}
	}
	return types.ConfigImportCostRuleDraft{}, false
}

func importedCostRuleConfig(draft types.ConfigImportCostRuleDraft) (importedCostConfig, error) {
	mode := types.CostMode(strings.TrimSpace(draft.CostMode))
	if mode == "" {
		return importedCostConfig{}, errors.New("cost mode is required")
	}
	config := types.CostRuleConfigV1{
		Currency: draft.Currency, BillingMultiplier: pointerStringValue(draft.BillingMultiplier),
		PurchaseDiscountRatio: pointerStringValue(draft.PurchaseDiscountRatio), RechargeExchangeRatio: pointerStringValue(draft.RechargeExchangeRatio),
		FeeRate: pointerStringValue(draft.FeeRate), CurrencyToUSDRate: pointerStringValue(draft.CurrencyToUSDRate),
		UnitPrice: draft.UnitPrice, PricePerSecond: draft.PricePerSecond, InputPerMillion: draft.InputPerMillion,
		OutputPerMillion: draft.OutputPerMillion, CompletionPerMillion: draft.CompletionPerMillion, TotalPerMillion: draft.TotalPerMillion,
		ZeroCostReason: draft.ZeroCostReason, ChargeEvent: types.CostChargeEvent(draft.ChargeEvent), MeterSource: types.CostMeterSource(draft.MeterSource), TokenMode: types.CostTokenMode(draft.TokenMode),
	}
	if config.ChargeEvent == "" {
		switch mode {
		case types.CostModePerRequest:
			config.ChargeEvent = types.CostChargeSubmitAccepted
		default:
			config.ChargeEvent = types.CostChargeTaskSucceeded
		}
	}
	if config.MeterSource == "" && mode == types.CostModePerDuration {
		config.MeterSource = types.CostMeterValidatedRequest
	}
	if config.MeterSource == "" && mode == types.CostModePerToken {
		config.MeterSource = types.CostMeterUpstreamUsage
	}
	if mode == types.CostModePerToken && config.TokenMode == "" {
		switch {
		case config.InputPerMillion != nil || config.OutputPerMillion != nil:
			config.TokenMode = types.CostTokenModeInputOutput
		case config.CompletionPerMillion != nil:
			config.TokenMode = types.CostTokenModeCompletion
		default:
			config.TokenMode = types.CostTokenModeTotal
		}
	}
	normalized, err := service.NormalizeCostRuleConfig(mode, config)
	if err != nil {
		return importedCostConfig{}, err
	}
	return importedCostConfig{mode: mode, value: normalized}, nil
}

func pointerStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func loadDocument(path string) (types.ConfigImportDocument, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return types.ConfigImportDocument{}, err
	}
	var document types.ConfigImportDocument
	if err := common.Unmarshal(payload, &document); err != nil {
		return types.ConfigImportDocument{}, err
	}
	return document, nil
}

func runtimeModel(modelName string) string {
	switch modelName {
	case "seedance-2.0":
		return modelrouting.Seedance20
	case "seedance-2.0-fast":
		return modelrouting.Seedance20Fast
	case "seedance-2.0-mini":
		return modelrouting.Seedance20Mini
	default:
		return ""
	}
}

func (target matrixTarget) CaseIDRouteTargetRef() string {
	return target.RouteTargetRef
}

func materialDistribution(targets []matrixTarget) map[string]int {
	distribution := make(map[string]int)
	for _, target := range targets {
		distribution[fmt.Sprintf("%d%d%d", target.References.Images, target.References.Videos, target.References.Audios)]++
	}
	return distribution
}

func (m *mockVideoServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ark-matrix/sample.mp4" {
		var data []byte
		var err error
		for _, path := range []string{"e2e/testdata/sample.mp4", "../../e2e/testdata/sample.mp4"} {
			data, err = os.ReadFile(path)
			if err == nil {
				break
			}
		}
		if err != nil {
			http.Error(w, "video fixture unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
			return
		}
		if r.Method == http.MethodHead {
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/ark-matrix/audio.wav" {
		data := fixturePCM16WAV(1_000)
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
			return
		}
		if r.Method == http.MethodHead {
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockVideoRequest{
		Method: r.Method, Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), Body: append([]byte(nil), body...),
	})
	m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost && isMockVideoSubmitPath(r.URL.Path) {
		var request map[string]any
		_ = common.Unmarshal(body, &request)
		modelName, _ := request["model"].(string)
		taskID := m.nextTaskID(modelName)
		response, _ := common.Marshal(map[string]any{
			"id": taskID, "task_id": taskID, "object": "video", "model": modelName, "status": "queued", "progress": 0,
			"created_at": time.Now().Unix(),
		})
		_, _ = w.Write(response)
		return
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/video/generations/") {
		taskID := strings.TrimPrefix(r.URL.Path, "/v1/video/generations/")
		_, _ = w.Write(m.pollingResponse(taskID))
		return
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/task/") {
		taskID := strings.TrimPrefix(r.URL.Path, "/api/task/")
		_, _ = w.Write(m.pollingResponse(taskID))
		return
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/tasks/") {
		taskID := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
		_, _ = w.Write(m.pollingResponse(taskID))
		return
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/videos/tasks/") {
		taskID := strings.TrimPrefix(r.URL.Path, "/v1/videos/tasks/")
		response, _ := common.Marshal(map[string]any{
			"task_id": taskID, "status": "completed", "progress": 100,
			"result": map[string]any{"url": "https://example.com/video.mp4"},
		})
		_, _ = w.Write(response)
		return
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/videos/") {
		taskID := strings.TrimPrefix(r.URL.Path, "/v1/videos/")
		_, failed := m.taskState(taskID)
		status := "completed"
		videoURL := "https://example.com/video.mp4"
		var upstreamError map[string]any
		if failed {
			status = "failed"
			videoURL = ""
			upstreamError = map[string]any{"code": "content_policy_violation", "message": "mock content policy rejection"}
		}
		response, _ := common.Marshal(map[string]any{
			"task_id": taskID, "status": status, "progress": 100, "video_url": videoURL, "error": upstreamError,
		})
		_, _ = w.Write(response)
		return
	}
	http.NotFound(w, r)
}

func fixturePCM16WAV(durationMS int) []byte {
	const sampleRate = 8_000
	const channels = 1
	const bitsPerSample = 16
	sampleCount := sampleRate * durationMS / 1_000
	dataSize := sampleCount * channels * bitsPerSample / 8
	buffer := bytes.NewBuffer(make([]byte, 0, 44+dataSize))
	buffer.WriteString("RIFF")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(36+dataSize))
	buffer.WriteString("WAVEfmt ")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(16))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(1))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buffer, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buffer, binary.LittleEndian, uint32(sampleRate*channels*bitsPerSample/8))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(channels*bitsPerSample/8))
	_ = binary.Write(buffer, binary.LittleEndian, uint16(bitsPerSample))
	buffer.WriteString("data")
	_ = binary.Write(buffer, binary.LittleEndian, uint32(dataSize))
	buffer.Write(make([]byte, dataSize))
	return buffer.Bytes()
}

type fixtureAssetTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (t fixtureAssetTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

func fixtureAssetClient(server *httptest.Server) (*http.Client, error) {
	target, err := url.Parse(server.URL)
	if err != nil {
		return nil, err
	}
	base := server.Client().Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{Transport: fixtureAssetTransport{base: base, target: target}}, nil
}

func isMockVideoSubmitPath(path string) bool {
	switch path {
	case "/v1/video/generations", "/v1/videos/generations", "/v1/videos", "/v1/media/generate", "/api/generate-video":
		return true
	default:
		return false
	}
}

func (m *mockVideoServer) nextTaskID(modelName string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	taskID := fmt.Sprintf("upstream-task-%03d", m.nextID)
	if m.models == nil {
		m.models = make(map[string]string)
	}
	m.models[taskID] = modelName
	if m.failNext {
		if m.failedTasks == nil {
			m.failedTasks = make(map[string]bool)
		}
		m.failedTasks[taskID] = true
		m.failNext = false
	}
	return taskID
}

func (m *mockVideoServer) failNextTask() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNext = true
}

func (m *mockVideoServer) taskState(taskID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.models[taskID], m.failedTasks[taskID]
}

func (m *mockVideoServer) pollingResponse(taskID string) []byte {
	modelName, failed := m.taskState(taskID)
	if modelName == "" {
		modelName = "seedance-local-mock"
	}
	status := "SUCCESS"
	resultURL := "https://example.com/video.mp4"
	nestedStatus := "succeeded"
	failReason := ""
	var nestedError map[string]any
	if failed {
		status = "FAILURE"
		resultURL = ""
		nestedStatus = "failed"
		failReason = "mock content policy rejection"
		nestedError = map[string]any{"code": "content_policy_violation", "message": failReason}
	}
	createdAt := time.Now().Unix() - 20
	updatedAt := time.Now().Unix()
	totalTokens := 216900
	if strings.HasSuffix(modelName, "-token") {
		totalTokens = 281700
	}
	response, _ := common.Marshal(map[string]any{
		"code": "success", "message": "",
		"data": map[string]any{
			"task_id": taskID, "status": status, "result_url": resultURL, "fail_reason": failReason,
			"submit_time": createdAt, "start_time": createdAt + 5, "finish_time": updatedAt,
			"progress": "100%", "quota": 2_000_000, "platform": "54",
			"properties": map[string]any{"origin_model_name": modelrouting.Seedance20, "upstream_model_name": modelName},
			"data": map[string]any{
				"content": map[string]any{"video_url": resultURL},
				"error":   nestedError,
				"id":      taskID, "model": modelName, "status": nestedStatus, "duration": 10, "resolution": "720p", "ratio": "16:9",
				"seed": 78674, "framespersecond": 24, "service_tier": "default", "execution_expires_after": 172800,
				"generate_audio": true, "draft": false, "priority": 0,
				"usage":      map[string]any{"completion_tokens": 216900, "total_tokens": totalTokens},
				"created_at": createdAt, "updated_at": updatedAt,
			},
		},
	})
	return response
}

func (m *mockVideoServer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func summarizeResult(userID, tokenID, totalTargets, acceptedTasks, contractBlocks, disabledPricing int, taskIDs []string, materialLimits map[string]int, upstreamCalls int) (seedResult, error) {
	var commonLogs int64
	if err := model.LOG_DB.Model(&model.Log{}).Where("user_id = ?", userID).Count(&commonLogs).Error; err != nil {
		return seedResult{}, err
	}
	var taskRows int64
	var failedMockTasks int64
	var terminalResults int64
	if len(taskIDs) > 0 {
		var tasks []model.Task
		if err := model.DB.Where("task_id IN ?", taskIDs).Find(&tasks).Error; err != nil {
			return seedResult{}, err
		}
		taskRows = int64(len(tasks))
		for i := range tasks {
			if tasks[i].Status == model.TaskStatusFailure {
				failedMockTasks++
			}
			if len(tasks[i].PrivateData.UserResponseData) > 0 {
				terminalResults++
			}
		}
	}
	var quotaDataRows int64
	if err := model.DB.Model(&model.QuotaData{}).Where("user_id = ?", userID).Count(&quotaDataRows).Error; err != nil {
		return seedResult{}, err
	}
	var costRequestRows []model.CostAccountingRequest
	if err := model.DB.Where("user_id = ?", userID).Find(&costRequestRows).Error; err != nil {
		return seedResult{}, err
	}
	var costAttemptRows []model.CostAccountingAttempt
	if err := model.DB.Where("channel_name LIKE ?", seedChannelName+"%").Find(&costAttemptRows).Error; err != nil {
		return seedResult{}, err
	}
	result := seedResult{
		TotalTargets: totalTargets, AcceptedTasks: acceptedTasks, FailedMockTasks: failedMockTasks, ContractBlocks: contractBlocks, DisabledPricing: disabledPricing,
		CommonLogs: commonLogs, TaskRows: taskRows, TerminalResults: terminalResults, QuotaDataRows: quotaDataRows,
		CostRequests: int64(len(costRequestRows)), CostAttempts: int64(len(costAttemptRows)), MaterialLimits: materialLimits,
		MockUpstreamCalls: upstreamCalls, UserID: userID, TokenID: tokenID,
	}
	for i := range costAttemptRows {
		switch types.CostAttemptStatus(costAttemptRows[i].Status) {
		case types.CostAttemptSettled:
			result.CostSettled++
		case types.CostAttemptConfirmedZero:
			result.CostConfirmedZero++
		case types.CostAttemptSettlementFailed:
			result.CostFailed++
		}
	}
	for i := range costRequestRows {
		request := &costRequestRows[i]
		if types.CostProfitStatus(request.ProfitStatus) == types.CostProfitComplete {
			result.ProfitComplete++
		}
		if request.BilledGrossProfitNanoUSD != nil && *request.BilledGrossProfitNanoUSD < 0 {
			result.NegativeProfit++
		}
		var addErr error
		if request.BilledRevenueEquivalentNanoUSD != nil {
			result.RevenueNanoUSD, addErr = service.CheckedNanoAdd(result.RevenueNanoUSD, *request.BilledRevenueEquivalentNanoUSD)
			if addErr != nil {
				return seedResult{}, addErr
			}
		}
		result.CostNanoUSD, addErr = service.CheckedNanoAdd(result.CostNanoUSD, request.ConfirmedCostNanoUSD)
		if addErr != nil {
			return seedResult{}, addErr
		}
		if request.BilledGrossProfitNanoUSD != nil {
			result.GrossProfitNanoUSD, addErr = service.CheckedNanoAdd(result.GrossProfitNanoUSD, *request.BilledGrossProfitNanoUSD)
			if addErr != nil {
				return seedResult{}, addErr
			}
		}
	}
	return result, nil
}

func printResult(result seedResult) {
	fmt.Println("ARK SDK video material matrix seed completed")
	fmt.Printf("user: %s (id=%d), token: %s (id=%d), group: %s\n", seedUsername, result.UserID, seedToken, result.TokenID, seedGroup)
	fmt.Printf("targets: %d, accepted tasks: %d, contract blocks before submit: %d, disabled pricing drafts: %d\n", result.TotalTargets, result.AcceptedTasks, result.ContractBlocks, result.DisabledPricing)
	fmt.Printf("task rows: %d, failed mock tasks: %d, terminal user results: %d, usage logs for user: %d, quota_data rows: %d\n", result.TaskRows, result.FailedMockTasks, result.TerminalResults, result.CommonLogs, result.QuotaDataRows)
	fmt.Printf("cost accounting requests: %d, attempts: %d, mock upstream calls: %d\n", result.CostRequests, result.CostAttempts, result.MockUpstreamCalls)
	fmt.Printf("cost settlement: settled=%d, confirmed_zero=%d, settlement_failed=%d, profit_complete=%d\n", result.CostSettled, result.CostConfirmedZero, result.CostFailed, result.ProfitComplete)
	fmt.Printf("accounting totals: revenue=$%s, supplier_cost=$%s, gross_profit=$%s, negative_profit_requests=%d\n",
		decimal.NewFromInt(result.RevenueNanoUSD).Shift(-9).StringFixed(9),
		decimal.NewFromInt(result.CostNanoUSD).Shift(-9).StringFixed(9),
		decimal.NewFromInt(result.GrossProfitNanoUSD).Shift(-9).StringFixed(9),
		result.NegativeProfit)
	fmt.Printf("material limits: 431=%d, 900=%d, 903=%d, 933=%d\n", result.MaterialLimits["431"], result.MaterialLimits["900"], result.MaterialLimits["903"], result.MaterialLimits["933"])
	fmt.Println("view pages:")
	fmt.Println("  http://127.0.0.1:3000/dashboard/overview")
	fmt.Println("  http://127.0.0.1:3000/usage-logs/common")
	fmt.Println("  http://127.0.0.1:3000/usage-logs/task")
	fmt.Println("  http://127.0.0.1:3000/cost-accounting")
	fmt.Printf("filter by username %q or group %q\n", seedUsername, seedGroup)
}

func commonKeyColumn() string {
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return `"key"`
	}
	return "`key`"
}
