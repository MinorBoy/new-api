package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDistributeCapabilityRoutingSkipsIncompatibleHigherPriorityChannel(t *testing.T) {
	prepareDistributorRoutingTest(t)
	seedDistributorRoutingChannel(t, 11, "A1", 100)
	seedDistributorRoutingChannel(t, 12, "A1_copy", 50)
	request := distributorRoutingPolicyRequest()
	request.Defaults.OutputResolution = "1080p"
	request.Targets = []service.RouteTargetWriteRequest{
		distributorRoutingTarget(11, "provider-720p", "720p"),
		distributorRoutingTarget(12, "provider-1080p", "1080p"),
	}
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)

	recorder, reached := runDistributorRoutingRequest(t, "", `{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"video"}],
		"resolution":"1080p","duration":10,"ratio":"16:9"
	}`)
	assert.True(t, reached)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "12", recorder.Header().Get("X-Selected-Channel"))
	assert.Equal(t, "provider-1080p", recorder.Header().Get("X-Routing-Upstream"))
}

func TestDistributeCapabilityRoutingRejectsIncompatibleSpecificChannel(t *testing.T) {
	prepareDistributorRoutingTest(t)
	seedDistributorRoutingChannel(t, 11, "A1", 100)
	seedDistributorRoutingChannel(t, 12, "A1_copy", 50)
	request := distributorRoutingPolicyRequest()
	request.Targets = []service.RouteTargetWriteRequest{
		distributorRoutingTarget(11, "provider-720p", "720p"),
		distributorRoutingTarget(12, "provider-1080p", "1080p"),
	}
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)

	recorder, reached := runDistributorRoutingRequest(t, "11", `{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"video"}],
		"resolution":"1080p","duration":10,"ratio":"16:9"
	}`)
	assert.False(t, reached)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"no_compatible_route"`)
	assert.NotContains(t, recorder.Body.String(), "provider-1080p")
	assert.NotContains(t, recorder.Body.String(), "target_id")
}

func TestDistributeCapabilityRoutingRetriesAnotherCompatibleChannelAfterNoKey(t *testing.T) {
	prepareDistributorRoutingTest(t)
	seedDistributorRoutingChannel(t, 11, "A1", 100)
	seedDistributorRoutingChannel(t, 12, "A1_copy", 50)
	var noKeyChannel model.Channel
	require.NoError(t, model.DB.First(&noKeyChannel, "id = ?", 11).Error)
	noKeyChannel.Key = ""
	noKeyChannel.ChannelInfo.IsMultiKey = true
	require.NoError(t, model.DB.Save(&noKeyChannel).Error)
	request := distributorRoutingPolicyRequest()
	request.Defaults.OutputResolution = "1080p"
	request.Targets = []service.RouteTargetWriteRequest{
		distributorRoutingTarget(11, "provider-1080p-a1", "1080p"),
		distributorRoutingTarget(12, "provider-1080p-copy", "1080p"),
	}
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)

	recorder, reached := runDistributorRoutingRequest(t, "", `{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"video"}],
		"resolution":"1080p","duration":10,"ratio":"16:9"
	}`)
	assert.True(t, reached)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "12", recorder.Header().Get("X-Selected-Channel"))
	assert.Equal(t, "provider-1080p-copy", recorder.Header().Get("X-Routing-Upstream"))
}

func TestDistributeCapabilityRoutingIgnoresIncompatibleAffinityChannel(t *testing.T) {
	prepareDistributorRoutingTest(t)
	seedDistributorRoutingChannel(t, 11, "A1", 100)
	seedDistributorRoutingChannel(t, 12, "A1_copy", 50)
	request := distributorRoutingPolicyRequest()
	request.Defaults.OutputResolution = "1080p"
	request.Targets = []service.RouteTargetWriteRequest{
		distributorRoutingTarget(11, "provider-720p", "720p"),
		distributorRoutingTarget(12, "provider-1080p", "1080p"),
	}
	_, err := service.SaveRoutingPolicy(0, request)
	require.NoError(t, err)
	prepareDistributorAffinity(t, 11)

	recorder, reached := runDistributorRoutingRequest(t, "", `{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"video"}],
		"resolution":"1080p","duration":10,"ratio":"16:9"
	}`)
	assert.True(t, reached)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "12", recorder.Header().Get("X-Selected-Channel"))
}

func TestDistributeTaskFetchPreservesNoChannelFlow(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/video/generations/task-public", nil)
	c.Set(common.KeySeedanceOfficialAPI, true)

	Distribute()(c)

	assert.False(t, c.IsAborted())
}

func prepareDistributorRoutingTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	model.DB = db
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	for _, table := range []string{"route_targets", "routing_policies", "abilities", "channels"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	require.NoError(t, model.InitRoutingPolicyCache())
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		for _, table := range []string{"route_targets", "routing_policies", "abilities", "channels"} {
			require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
		}
		require.NoError(t, model.InitRoutingPolicyCache())
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
}

func seedDistributorRoutingChannel(t *testing.T, id int, name string, priority int64) {
	t.Helper()
	weight := uint(100)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: id, Type: constant.ChannelTypeNewAPIVideo, Key: "secret", Status: common.ChannelStatusEnabled,
		Name: name, Models: modelrouting.Seedance20, Group: "分组A", Priority: &priority, Weight: &weight,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "分组A", Model: modelrouting.Seedance20, ChannelId: id, Enabled: true,
		Priority: &priority, Weight: weight,
	}).Error)
}

func distributorRoutingPolicyRequest() service.RoutingPolicyWriteRequest {
	return service.RoutingPolicyWriteRequest{
		GroupName: "分组A",
		Model:     modelrouting.Seedance20,
		Enabled:   true,
		Defaults: modelrouting.Defaults{
			OutputResolution: "720p",
			DurationSeconds:  10,
			AspectRatio:      "16:9",
		},
	}
}

func distributorRoutingTarget(channelID int, upstreamModel, resolution string) service.RouteTargetWriteRequest {
	supportsRealPerson := true
	minDuration, maxDuration := 4, 15
	return service.RouteTargetWriteRequest{
		ChannelID: channelID, Name: upstreamModel, UpstreamModel: upstreamModel,
		TargetPriority: 100, Enabled: true,
		Constraints: modelrouting.Constraints{
			OutputResolutions:  []string{resolution},
			Durations:          modelrouting.DurationConstraint{Min: &minDuration, Max: &maxDuration},
			AspectRatios:       []string{"16:9"},
			ReferenceLimits:    modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
			SupportsRealPerson: &supportsRealPerson,
		},
	}
}

func runDistributorRoutingRequest(t *testing.T, specificChannelID, body string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Routing-Affinity", "tenant-a")
	c.Set(common.KeySeedanceOfficialAPI, true)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "分组A")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "分组A")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "分组A")
	if specificChannelID != "" {
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, specificChannelID)
	}
	reached := false
	c.Set("routing_test_next", func() {
		reached = true
		recorder.Header().Set("X-Selected-Channel", strconv.Itoa(common.GetContextKeyInt(c, constant.ContextKeyChannelId)))
		recorder.Header().Set("X-Routing-Upstream", common.GetContextKeyString(c, constant.ContextKeyRoutingUpstreamModel))
		recorder.WriteHeader(http.StatusOK)
	})

	handler := Distribute()
	handler(c)
	if callback, ok := c.Get("routing_test_next"); ok && !c.IsAborted() {
		callback.(func())()
	}
	common.CleanupBodyStorage(c)
	return recorder, reached
}

func prepareDistributorAffinity(t *testing.T, channelID int) {
	t.Helper()
	affinitySetting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, affinitySetting)
	previousEnabled := affinitySetting.Enabled
	previousRules := affinitySetting.Rules
	affinitySetting.Enabled = true
	affinitySetting.Rules = []operation_setting.ChannelAffinityRule{{
		Name: "routing-test", ModelRegex: []string{"^doubao-seedance-2-0-260128$"},
		PathRegex:       []string{"^/v1/video/generations$"},
		KeySources:      []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Routing-Affinity"}},
		IncludeRuleName: true, IncludeModelName: true,
	}}
	service.ClearChannelAffinityCacheAll()
	t.Cleanup(func() {
		service.ClearChannelAffinityCacheAll()
		affinitySetting.Enabled = previousEnabled
		affinitySetting.Rules = previousRules
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)
	c.Request.Header.Set("X-Routing-Affinity", "tenant-a")
	_, found := service.GetPreferredChannelByAffinity(c, modelrouting.Seedance20, "分组A")
	assert.False(t, found)
	service.RecordChannelAffinity(c, channelID)
}
