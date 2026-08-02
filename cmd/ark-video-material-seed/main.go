package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	"gorm.io/gorm"
)

const (
	seedGroup       = "ark-sdk-material-matrix-local"
	seedUsername    = "ark_sdk_matrix_user"
	seedPassword    = "local-seed-password"
	seedToken       = "arkmatrixlocal"
	seedTokenName   = "ARK SDK material matrix local seed"
	seedChannelName = "ark-sdk-matrix-mock-"
)

type matrixTarget struct {
	CaseID         string
	RouteTargetRef string
	LineRef        string
	Provider       string
	RuntimeModel   string
	UpstreamModel  string
	Resolution     string
	Duration       int
	CostVariantKey string
	Minimums       modelrouting.ReferenceLimits
	References     modelrouting.ReferenceLimits
	ChannelType    int
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
	TotalTargets      int
	AcceptedTasks     int
	ContractBlocks    int
	CommonLogs        int64
	TaskRows          int64
	QuotaDataRows     int64
	CostRequests      int64
	CostAttempts      int64
	MaterialLimits    map[string]int
	MockUpstreamCalls int
	UserID            int
	TokenID           int
}

type mockVideoServer struct {
	mu       sync.Mutex
	nextID   int
	requests []mockVideoRequest
	models   map[string]string
}

type mockVideoRequest struct {
	Method        string
	Path          string
	Authorization string
	Body          []byte
}

type localMetadataClient struct{}

func (localMetadataClient) Metadata(context.Context, string) (videometa.Metadata, error) {
	return videometa.Metadata{DurationMS: 5_000}, nil
}

type localReferenceAudioResolver struct{}

func (localReferenceAudioResolver) ResolveMS(context.Context, []string) (int64, error) {
	return 5_000, nil
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
	contractBlocks := 0

	for _, target := range targets {
		materialLimits[fmt.Sprintf("%d%d%d", target.References.Images, target.References.Videos, target.References.Audios)]++
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

		status, body := performJSONRequest(engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer "+seedToken, requestBody(target))
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
	}

	model.SaveQuotaDataCache()
	result, err := summarizeResult(user.Id, token.Id, len(targets), contractBlocks, createdTasks, materialLimits, mock.count())
	if err != nil {
		return err
	}
	printResult(result)
	return nil
}

func initResources() error {
	_ = godotenv.Load(".env")
	common.InitEnv()
	common.RedisEnabled = false
	logger.SetupLogger()
	ratio_setting.InitRatioSettings()
	service.InitHttpClient()
	service.SetVideoMetadataClient(localMetadataClient{})
	service.SetReferenceAudioDurationResolver(localReferenceAudioResolver{})
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

		attemptQuery := tx.Model(&model.CostAccountingAttempt{})
		switch {
		case len(costRequestIDs) > 0 && len(channelIDList) > 0:
			attemptQuery = attemptQuery.Where("cost_request_id IN ? OR channel_id IN ?", costRequestIDs, channelIDList)
		case len(costRequestIDs) > 0:
			attemptQuery = attemptQuery.Where("cost_request_id IN ?", costRequestIDs)
		case len(channelIDList) > 0:
			attemptQuery = attemptQuery.Where("channel_id IN ?", channelIDList)
		default:
			attemptQuery = nil
		}

		var attemptIDs []int64
		if attemptQuery != nil {
			if err := attemptQuery.Pluck("id", &attemptIDs).Error; err != nil {
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
		if len(channelIDList) > 0 {
			if err := tx.Model(&model.Channel{}).Where("id IN ?", channelIDList).Update("used_quota", 0).Error; err != nil {
				return err
			}
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
	groupRatios[seedGroup] = 1
	encodedGroupRatios, err := common.Marshal(groupRatios)
	if err != nil {
		return err
	}
	if err := ratio_setting.UpdateGroupRatioByJSONString(string(encodedGroupRatios)); err != nil {
		return err
	}

	modelRatios := ratio_setting.GetModelRatioCopy()
	for _, modelName := range modelrouting.CanonicalModels {
		modelRatios[modelName] = 0.1
	}
	encodedModelRatios, err := common.Marshal(modelRatios)
	if err != nil {
		return err
	}
	if err := ratio_setting.UpdateModelRatioByJSONString(string(encodedModelRatios)); err != nil {
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
	for index, lineRef := range lineRefs {
		definition, ok := definitions[lineRef]
		if !ok {
			return nil, fmt.Errorf("missing channel definition for line %q", lineRef)
		}
		name := seedChannelName + lineRef
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
	return channelIDs, nil
}

func seedCostRules(targets []matrixTarget, channelIDs map[string]int) error {
	document, err := loadDocument(filepath.Join("e2e", "testdata", "channel-config-v1.json"))
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	seen := make(map[string]struct{})
	for _, target := range targets {
		channelID := channelIDs[target.LineRef]
		ruleDraft, ok := findImportedCostRule(document, target.RouteTargetRef)
		if !ok {
			return fmt.Errorf("missing imported cost rule for %q", target.RouteTargetRef)
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
			configJSON, err := common.Marshal(config.value)
			if err != nil {
				return err
			}
			var existing model.ChannelModelCostRule
			err = model.DB.Where(
				"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ? AND status = ?",
				channelID, target.UpstreamModel, variant, types.CostRuleActive,
			).First(&existing).Error
			if err == nil {
				if err := model.DB.Model(&existing).Updates(map[string]any{
					"cost_mode": string(config.mode), "schema_version": 1, "config_json": string(configJSON), "updated_at": now,
				}).Error; err != nil {
					return err
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := model.DB.Create(&model.ChannelModelCostRule{
				ChannelID: channelID, BillableUpstreamModel: target.UpstreamModel, CostVariantKey: variant,
				Version: 1, Status: string(types.CostRuleActive), CostMode: string(config.mode),
				SchemaVersion: 1, ConfigJSON: string(configJSON), Source: "local_seed", CreatedBy: 1, ActivatedBy: 1,
				EffectiveFrom: &now, CreatedAt: now, UpdatedAt: now,
			}).Error; err != nil {
				return err
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
				OutputResolutions: []string{target.Resolution},
				Durations: modelrouting.DurationConstraint{
					Min: common.GetPointer(target.Duration),
					Max: common.GetPointer(target.Duration),
				},
				AspectRatios:      []string{"16:9"},
				ReferenceMinimums: target.Minimums,
				ReferenceLimits:   target.References,
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

func requestBody(target matrixTarget) string {
	content := []map[string]any{{"type": "text", "text": "ARK SDK material matrix local seed " + target.CaseID}}
	for index := 0; index < target.References.Images; index++ {
		content = append(content, map[string]any{
			"type": "image_url", "role": "reference_image",
			"image_url": map[string]any{"url": fmt.Sprintf("https://cdn.openai.com/ark-matrix/%s/image-%02d.png", target.Provider, index+1)},
		})
	}
	for index := 0; index < target.References.Videos; index++ {
		content = append(content, map[string]any{
			"type": "video_url", "role": "reference_video",
			"video_url": map[string]any{"url": fmt.Sprintf("https://cdn.openai.com/ark-matrix/%s/video-%02d.mp4", target.Provider, index+1)},
		})
	}
	for index := 0; index < target.References.Audios; index++ {
		content = append(content, map[string]any{
			"type": "audio_url", "role": "reference_audio",
			"audio_url": map[string]any{"url": fmt.Sprintf("https://cdn.openai.com/ark-matrix/%s/audio-%02d.mp3", target.Provider, index+1)},
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
			duration := 10
			if target.DurationMin != nil {
				duration = *target.DurationMin
			}
			targets = append(targets, matrixTarget{
				CaseID: fmt.Sprintf("%s/%s/%d%d%d", target.RouteTargetRef, target.OutputResolutions[0], references.Images, references.Videos, references.Audios), RouteTargetRef: target.RouteTargetRef,
				LineRef: target.LineRef, Provider: line.ProviderTypeHint, RuntimeModel: runtimeModel,
				UpstreamModel: target.UpstreamModel, Resolution: strings.ToLower(target.OutputResolutions[0]),
				Duration: duration, CostVariantKey: target.CostVariantKey, Minimums: minimums, References: references, ChannelType: definition.Type,
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
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/videos/tasks/") {
		taskID := strings.TrimPrefix(r.URL.Path, "/v1/videos/tasks/")
		response, _ := common.Marshal(map[string]any{
			"task_id": taskID, "status": "succeeded", "progress": 100,
			"result": map[string]any{"url": "https://example.com/video.mp4"},
		})
		_, _ = w.Write(response)
		return
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/videos/") {
		taskID := strings.TrimPrefix(r.URL.Path, "/v1/videos/")
		response, _ := common.Marshal(map[string]any{
			"task_id": taskID, "status": "completed", "progress": 100, "video_url": "https://example.com/video.mp4",
		})
		_, _ = w.Write(response)
		return
	}
	http.NotFound(w, r)
}

func isMockVideoSubmitPath(path string) bool {
	switch path {
	case "/v1/video/generations", "/v1/videos/generations", "/v1/videos", "/api/generate-video":
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
	m.models[taskID] = modelName
	return taskID
}

func (m *mockVideoServer) pollingResponse(taskID string) []byte {
	m.mu.Lock()
	modelName := m.models[taskID]
	m.mu.Unlock()
	if modelName == "" {
		modelName = "seedance-local-mock"
	}
	response, _ := common.Marshal(map[string]any{
		"code": "success", "message": "",
		"data": map[string]any{
			"task_id": taskID, "status": "SUCCESS", "result_url": "https://example.com/video.mp4",
			"submit_time": time.Now().Unix() - 20, "start_time": time.Now().Unix() - 15, "finish_time": time.Now().Unix(),
			"progress": "100%", "quota": 2_000_000, "platform": "54",
			"properties": map[string]any{"origin_model_name": modelrouting.Seedance20, "upstream_model_name": modelName},
			"data": map[string]any{
				"content": map[string]any{"video_url": "https://example.com/video.mp4"},
				"id":      taskID, "model": modelName, "status": "succeeded", "duration": 10, "resolution": "720p", "ratio": "16:9",
				"usage": map[string]any{"completion_tokens": 216900, "total_tokens": 216900},
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

func summarizeResult(userID, tokenID, totalTargets, contractBlocks int, taskIDs []string, materialLimits map[string]int, upstreamCalls int) (seedResult, error) {
	var commonLogs int64
	if err := model.LOG_DB.Model(&model.Log{}).Where("user_id = ?", userID).Count(&commonLogs).Error; err != nil {
		return seedResult{}, err
	}
	var taskRows int64
	if len(taskIDs) > 0 {
		if err := model.DB.Model(&model.Task{}).Where("task_id IN ?", taskIDs).Count(&taskRows).Error; err != nil {
			return seedResult{}, err
		}
	}
	var quotaDataRows int64
	if err := model.DB.Model(&model.QuotaData{}).Where("user_id = ?", userID).Count(&quotaDataRows).Error; err != nil {
		return seedResult{}, err
	}
	var costRequests int64
	if err := model.DB.Model(&model.CostAccountingRequest{}).Where("user_id = ?", userID).Count(&costRequests).Error; err != nil {
		return seedResult{}, err
	}
	var costAttempts int64
	if err := model.DB.Model(&model.CostAccountingAttempt{}).Where("channel_name LIKE ?", seedChannelName+"%").Count(&costAttempts).Error; err != nil {
		return seedResult{}, err
	}
	return seedResult{
		TotalTargets: totalTargets, AcceptedTasks: len(taskIDs), ContractBlocks: contractBlocks,
		CommonLogs: commonLogs, TaskRows: taskRows, QuotaDataRows: quotaDataRows,
		CostRequests: costRequests, CostAttempts: costAttempts, MaterialLimits: materialLimits,
		MockUpstreamCalls: upstreamCalls, UserID: userID, TokenID: tokenID,
	}, nil
}

func printResult(result seedResult) {
	fmt.Println("ARK SDK video material matrix seed completed")
	fmt.Printf("user: %s (id=%d), token: %s (id=%d), group: %s\n", seedUsername, result.UserID, seedToken, result.TokenID, seedGroup)
	fmt.Printf("targets: %d, accepted tasks: %d, contract blocks before submit: %d\n", result.TotalTargets, result.AcceptedTasks, result.ContractBlocks)
	fmt.Printf("task rows: %d, usage logs for user: %d, quota_data rows: %d\n", result.TaskRows, result.CommonLogs, result.QuotaDataRows)
	fmt.Printf("cost accounting requests: %d, attempts: %d, mock upstream calls: %d\n", result.CostRequests, result.CostAttempts, result.MockUpstreamCalls)
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
