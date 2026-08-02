package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relay"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLocalMediaResolversUseAdapterBoundaryFixtures(t *testing.T) {
	metadata, err := (localMetadataClient{}).Metadata(context.Background(), "https://example.com/video.mp4")
	require.NoError(t, err)
	require.Equal(t, int64(5_000), metadata.DurationMS)

	durationMS, err := (localReferenceAudioResolver{}).ResolveMS(context.Background(), []string{"https://example.com/audio.mp3"})
	require.NoError(t, err)
	require.Equal(t, int64(5_000), durationMS)
}

func TestSeedPasswordHashAcceptsSeedPassword(t *testing.T) {
	hashed, err := seedPasswordHash()
	require.NoError(t, err)
	require.NotEqual(t, seedPassword, hashed)
	require.True(t, common.ValidatePasswordAndHash(seedPassword, hashed))
}

func TestLoadTargetsPreservesImportedMaterialMatrix(t *testing.T) {
	targets, err := loadTargets(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)
	require.Len(t, targets, 98)
	require.Equal(t, map[string]int{"431": 22, "900": 4, "903": 1, "933": 71}, materialDistribution(targets))

	targetsByLine := make(map[string][]matrixTarget)
	for _, target := range targets {
		targetsByLine[target.LineRef] = append(targetsByLine[target.LineRef], target)
	}
	require.Equal(t, constant.ChannelTypeFourSToken, targetsByLine["channel-4stoken"][0].ChannelType)
	require.Equal(t, constant.ChannelTypeMegaByAI, targetsByLine["megabyai-fast-real-person"][0].ChannelType)
	require.Equal(t, constant.ChannelTypeSecure, targetsByLine["secure-discount"][0].ChannelType)
}

func TestLoadTargetsMatchesRouteContractBlocks(t *testing.T) {
	targets, err := loadTargets(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)

	accepted := 0
	blocked := 0
	for _, target := range targets {
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
		err := relay.ValidateVideoRouteTargetContract(channel, modelrouting.Target{
			UpstreamModel: target.UpstreamModel,
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
		})
		if err != nil {
			blocked++
			continue
		}
		accepted++
	}

	require.Equal(t, 26, accepted)
	require.Equal(t, 72, blocked)
}

func TestMockVideoServerSupportsProviderSubmitAndPollingPaths(t *testing.T) {
	server := &mockVideoServer{models: make(map[string]string)}
	submitPaths := []string{"/v1/video/generations", "/v1/videos/generations", "/v1/videos", "/api/generate-video"}
	for _, path := range submitPaths {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"seedance-provider-model"}`))
		server.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code, path)
		require.Contains(t, recorder.Body.String(), "upstream-task-", path)
	}

	pollPaths := []string{
		"/v1/video/generations/upstream-task-001",
		"/api/task/upstream-task-002",
		"/v1/videos/tasks/upstream-task-003",
		"/v1/videos/upstream-task-004",
	}
	for _, path := range pollPaths {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, recorder.Code, path)
		require.Contains(t, recorder.Body.String(), "upstream-task-", path)
	}
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
	require.NoError(t, db.Create(&model.CostAccountingAudit{ID: 600, CostRequestID: seedRequest.ID}).Error)
	require.NoError(t, db.Create(&model.CostAccountingAudit{ID: 601, CostRequestID: otherRequest.ID}).Error)
	require.NoError(t, db.Create(&model.ChannelModelCostRule{ID: 700, ChannelID: seedChannel.Id, BillableUpstreamModel: "seed-model", CostVariantKey: "720p", Version: 1, Status: string(types.CostRuleActive), Source: "local_seed"}).Error)
	require.NoError(t, db.Create(&model.ChannelModelCostRule{ID: 701, ChannelID: otherChannel.Id, BillableUpstreamModel: "other-model", CostVariantKey: "720p", Version: 1, Status: string(types.CostRuleActive), Source: "manual"}).Error)

	require.NoError(t, cleanupSeedData(seedUser.Id, seedTokenRow.Id, map[string]int{"line": seedChannel.Id}))

	for _, table := range []any{&model.Task{}, &model.Log{}, &model.QuotaData{}, &model.CostAccountingRequest{}, &model.CostAccountingAttempt{}, &model.CostAccountingAudit{}} {
		var count int64
		require.NoError(t, db.Model(table).Count(&count).Error)
		require.Equal(t, int64(1), count)
	}
	var costRuleCount int64
	require.NoError(t, db.Model(&model.ChannelModelCostRule{}).Count(&costRuleCount).Error)
	require.Equal(t, int64(1), costRuleCount)
	require.NoError(t, db.First(&seedUser, seedUser.Id).Error)
	require.Zero(t, seedUser.UsedQuota)
	require.Zero(t, seedUser.RequestCount)
	require.NoError(t, db.First(&seedTokenRow, seedTokenRow.Id).Error)
	require.Zero(t, seedTokenRow.UsedQuota)
	require.NoError(t, db.First(&seedChannel, seedChannel.Id).Error)
	require.Zero(t, seedChannel.UsedQuota)
	require.NoError(t, db.First(&otherUser, otherUser.Id).Error)
	require.Equal(t, 456, otherUser.UsedQuota)
}

func TestLoadChannelDefinitionsPreservesImportedProviderTypesAndModels(t *testing.T) {
	document, err := loadDocument(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)

	definitions, err := loadChannelDefinitions(document)
	require.NoError(t, err)
	require.Equal(t, constant.ChannelTypeFourSToken, definitions["channel-4stoken"].Type)
	require.Equal(t, constant.ChannelTypeEightYes, definitions["channel-8yes"].Type)
	require.Equal(t, constant.ChannelTypeClmmMall, definitions["channel-clmm"].Type)
	require.True(t, definitions["channel-4stoken"].Enabled)
	require.False(t, definitions["channel-8yes"].Enabled)
	require.Contains(t, definitions["channel-4stoken"].Models, "4sdance_fast431")
	require.Contains(t, definitions["channel-paipu"].Models, "lec-seedance-videos-standard")
	require.NotContains(t, definitions["channel-4stoken"].Models, modelroutingCanonicalModels())
}

func TestImportedCostRuleConfigPreservesSourcePriceAndCurrency(t *testing.T) {
	document, err := loadDocument(filepath.Join("..", "..", "e2e", "testdata", "channel-config-v1.json"))
	require.NoError(t, err)

	rule, ok := findImportedCostRule(document, "route-target/MAP-4STOKEN-R116-480")
	require.True(t, ok)
	config, err := importedCostRuleConfig(rule)
	require.NoError(t, err)
	require.Equal(t, types.CostModePerRequest, config.mode)
	require.Equal(t, types.CostChargeSubmitAccepted, config.value.ChargeEvent)
	require.Equal(t, "CNY", config.value.Currency)
	require.Equal(t, "3.35", *config.value.UnitPrice)
	require.Equal(t, "0.136986301369863", config.value.CurrencyToUSDRate)
	require.Equal(t, "0.45890410958904105", *config.value.NormalizedUSDPrices.UnitPrice)
	require.NotEqual(t, "0.20", *config.value.UnitPrice)

	durationRule, ok := findImportedCostRule(document, "route-target/MAP-4STOKEN-R122-720")
	require.True(t, ok)
	durationConfig, err := importedCostRuleConfig(durationRule)
	require.NoError(t, err)
	require.Equal(t, types.CostModePerDuration, durationConfig.mode)
	require.Equal(t, "0.48", *durationConfig.value.PricePerSecond)
	require.Equal(t, types.CostChargeTaskSucceeded, durationConfig.value.ChargeEvent)
	require.Equal(t, types.CostMeterValidatedRequest, durationConfig.value.MeterSource)

	tokenRule, ok := findImportedCostRule(document, "route-target/MAP-LUCEN-R56-480")
	require.True(t, ok)
	tokenConfig, err := importedCostRuleConfig(tokenRule)
	require.NoError(t, err)
	require.Equal(t, types.CostModePerToken, tokenConfig.mode)
	require.Equal(t, types.CostChargeTaskSucceeded, tokenConfig.value.ChargeEvent)
	require.Equal(t, types.CostMeterUpstreamUsage, tokenConfig.value.MeterSource)
}

func modelroutingCanonicalModels() string {
	return strings.Join([]string{"seedance-2.0", "seedance-2.0-fast", "seedance-2.0-mini"}, ",")
}
