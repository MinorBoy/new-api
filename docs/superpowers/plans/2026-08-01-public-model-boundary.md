# 公共模型边界实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将三个 Doubao Seedance 标准模型定义为唯一公共模型目录，使模型广场、模型列表和公共 Relay 请求均不再暴露或接受上游渠道模型 ID。

**Architecture:** 保留 `channels.models`、`abilities`、模型映射、成本规则和路由目标的完整内部模型集合，在 `pkg/modelrouting` 增加不可变的公共目录判断与稳定投影。控制器在响应边界投影公共模型，`middleware.Distribute` 在渠道选择前拒绝非公共模型；管理员内部接口继续使用完整数据。

**Tech Stack:** Go 1.22+、Gin、GORM、testify、React 19/Rsbuild（仅构建验收）、Docker Compose。

**Design:** `docs/superpowers/specs/2026-08-01-public-model-boundary-design.md`

---

## 文件结构

- Create: `pkg/modelrouting/public.go` — 公共模型精确判断、稳定顺序过滤和公开所有者常量。
- Create: `pkg/modelrouting/public_test.go` — 公共目录精确边界与顺序回归测试。
- Create: `controller/public_models.go` — 定价响应的公共模型、供应商和端点投影。
- Create: `controller/public_models_test.go` — 定价、模型列表、单模型查询和用户模型接口回归测试。
- Modify: `controller/pricing.go` — 对 `/api/pricing` 应用公共投影。
- Modify: `controller/model.go` — 过滤兼容模型列表、固定 `owned_by`、过滤 Dashboard 模型并保护空 Anthropic 列表。
- Modify: `controller/user.go` — 过滤用户可用模型响应。
- Modify: `controller/model_list_test.go` — 将既有公开列表测试改为使用三个公共模型，同时继续断言内部计费和端点缓存行为。
- Modify: `controller/cost_routing_test.go` — 将经过公共分发器的成本路由测试改用标准公共模型，不改变内部上游映射断言。
- Modify: `middleware/distributor.go` — 在渠道选择前执行公共模型校验。
- Modify: `middleware/distributor_routing_test.go` — 覆盖上游 ID 拒绝与三个公共 ID 放行。
- Modify: `docs/api/video-generation.md` — 记录公共模型目录、兼容方式和拒绝语义。

### Task 1: 建立唯一公共模型目录

**Files:**
- Create: `pkg/modelrouting/public.go`
- Create: `pkg/modelrouting/public_test.go`

- [ ] **Step 1: 编写公共目录失败测试**

```go
package modelrouting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPublicModelRequiresExactCanonicalID(t *testing.T) {
	for _, modelName := range CanonicalModels {
		assert.True(t, IsPublicModel(modelName), modelName)
	}
	for _, modelName := range []string{
		"doubao-seedance-2-0-mini-260128",
		"mg-seedance2.0-480p-fast-gz-15s",
		" " + Seedance20,
		Seedance20 + " ",
		"",
	} {
		assert.False(t, IsPublicModel(modelName), modelName)
	}
}

func TestFilterPublicModelsUsesCatalogOrderAndDeduplicates(t *testing.T) {
	actual := FilterPublicModels([]string{
		"provider-hidden", Seedance20Mini, Seedance20, Seedance20Mini, Seedance20Fast,
	})
	require.Equal(t, []string{Seedance20, Seedance20Fast, Seedance20Mini}, actual)
}
```

- [ ] **Step 2: 运行测试并确认因公共目录 API 缺失而失败**

Run: `go test ./pkg/modelrouting -run 'Test(IsPublicModel|FilterPublicModels)' -count=1`

Expected: FAIL，提示 `IsPublicModel` 或 `FilterPublicModels` 未定义。

- [ ] **Step 3: 实现精确判断与稳定过滤**

```go
package modelrouting

const PublicModelOwner = "doubao"

func IsPublicModel(modelName string) bool {
	switch modelName {
	case Seedance20, Seedance20Fast, Seedance20Mini:
		return true
	default:
		return false
	}
}

func FilterPublicModels(modelNames []string) []string {
	available := make(map[string]struct{}, len(modelNames))
	for _, modelName := range modelNames {
		available[modelName] = struct{}{}
	}

	filtered := make([]string, 0, len(CanonicalModels))
	for _, modelName := range CanonicalModels {
		if _, ok := available[modelName]; ok {
			filtered = append(filtered, modelName)
		}
	}
	return filtered
}
```

- [ ] **Step 4: 运行公共目录测试并确认通过**

Run: `go test ./pkg/modelrouting -run 'Test(IsPublicModel|FilterPublicModels)' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交公共目录**

```powershell
git add -- pkg/modelrouting/public.go pkg/modelrouting/public_test.go
git commit -m "feat: define public model catalog"
```

### Task 2: 将模型广场投影为三个 Doubao 模型

**Files:**
- Create: `controller/public_models.go`
- Create: `controller/public_models_test.go`
- Modify: `controller/pricing.go`

- [ ] **Step 1: 编写定价投影失败测试**

在 `controller/public_models_test.go` 创建最小输入，包含乱序的三个公共模型、一个上游模型、内部供应商和多余端点：

```go
package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectPublicPricingRemovesInternalModelsAndVendors(t *testing.T) {
	pricing := []model.Pricing{
		{ModelName: "provider-hidden", VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIResponse}},
		{ModelName: modelrouting.Seedance20Mini, VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
		{ModelName: modelrouting.Seedance20, VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
		{ModelName: modelrouting.Seedance20Fast, VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
	}
	endpoints := map[string]common.EndpointInfo{
		string(constant.EndpointTypeOpenAI):         {Path: "/v1/chat/completions", Method: "POST"},
		string(constant.EndpointTypeOpenAIResponse): {Path: "/v1/responses", Method: "POST"},
	}

	projection := projectPublicPricing(pricing, endpoints)
	require.Equal(t, []string{
		modelrouting.Seedance20, modelrouting.Seedance20Fast, modelrouting.Seedance20Mini,
	}, []string{
		projection.Pricing[0].ModelName,
		projection.Pricing[1].ModelName,
		projection.Pricing[2].ModelName,
	})
	for _, item := range projection.Pricing {
		assert.Equal(t, publicDoubaoVendor.ID, item.VendorID)
		assert.Equal(t, modelrouting.PublicModelOwner, item.OwnerBy)
		assert.Equal(t, publicDoubaoVendor.Icon, item.Icon)
	}
	require.Equal(t, []model.PricingVendor{publicDoubaoVendor}, projection.Vendors)
	require.Contains(t, projection.SupportedEndpoints, string(constant.EndpointTypeOpenAI))
	require.NotContains(t, projection.SupportedEndpoints, string(constant.EndpointTypeOpenAIResponse))
}
```

- [ ] **Step 2: 运行测试并确认投影 API 缺失**

Run: `go test ./controller -run TestProjectPublicPricingRemovesInternalModelsAndVendors -count=1`

Expected: FAIL，提示 `projectPublicPricing` 或 `publicDoubaoVendor` 未定义。

- [ ] **Step 3: 实现公共定价投影**

在 `controller/public_models.go` 定义稳定的公开供应商和投影。实现必须复制 `model.Pricing` 值，不能原地修改 `model.GetPricing()` 返回的共享切片：

```go
package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
)

var publicDoubaoVendor = model.PricingVendor{
	ID: -1, Name: "Doubao", Icon: "Doubao.Color",
}

type publicPricingProjection struct {
	Pricing            []model.Pricing
	Vendors            []model.PricingVendor
	SupportedEndpoints map[string]common.EndpointInfo
}

func projectPublicPricing(pricing []model.Pricing, endpoints map[string]common.EndpointInfo) publicPricingProjection {
	pricingByName := make(map[string]model.Pricing, len(pricing))
	for _, item := range pricing {
		pricingByName[item.ModelName] = item
	}

	projection := publicPricingProjection{
		Pricing:            make([]model.Pricing, 0, len(modelrouting.CanonicalModels)),
		Vendors:            []model.PricingVendor{},
		SupportedEndpoints: make(map[string]common.EndpointInfo),
	}
	for _, modelName := range modelrouting.CanonicalModels {
		item, ok := pricingByName[modelName]
		if !ok {
			continue
		}
		item.VendorID = publicDoubaoVendor.ID
		item.Icon = publicDoubaoVendor.Icon
		item.OwnerBy = modelrouting.PublicModelOwner
		projection.Pricing = append(projection.Pricing, item)
		for _, endpointType := range item.SupportedEndpointTypes {
			key := string(endpointType)
			if endpoint, exists := endpoints[key]; exists {
				projection.SupportedEndpoints[key] = endpoint
			}
		}
	}
	if len(projection.Pricing) > 0 {
		projection.Vendors = append(projection.Vendors, publicDoubaoVendor)
	}
	return projection
}
```

- [ ] **Step 4: 在 `/api/pricing` 响应边界应用投影**

在 `controller/pricing.go:GetPricing` 完成 `filterPricingByUsableGroups` 后创建投影，并替换三个响应字段：

```go
projection := projectPublicPricing(pricing, model.GetSupportedEndpointMap())

c.JSON(http.StatusOK, gin.H{
	"success":            true,
	"data":               projection.Pricing,
	"vendors":            projection.Vendors,
	"group_ratio":        groupRatio,
	"usable_group":       usableGroup,
	"supported_endpoint": projection.SupportedEndpoints,
	"auto_groups":        service.GetUserAutoGroup(group),
	"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
})
```

- [ ] **Step 5: 运行定价投影和控制器测试**

Run: `go test ./controller -run 'TestProjectPublicPricing|TestPricing' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交模型广场边界**

```powershell
git add -- controller/public_models.go controller/public_models_test.go controller/pricing.go
git commit -m "feat: expose only public models in pricing"
```

### Task 3: 收紧所有用户模型列表

**Files:**
- Modify: `controller/public_models_test.go`
- Modify: `controller/model_list_test.go`
- Modify: `controller/model.go`
- Modify: `controller/user.go`

- [ ] **Step 1: 编写用户模型列表失败测试**

将 `controller/public_models_test.go` 的 import 更新为：

```go
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)
```

追加以下 fixture 和测试；复制 `CanonicalModels` 后再追加内部模型，禁止 `append` 直接复用公共切片的底层数组：

```go
func seedPublicModelAbilities(t *testing.T, db *gorm.DB, modelNames []string) {
	t.Helper()

	require.NoError(t, db.Create(&model.User{
		Id: 19001, Username: "public-model-user", Password: "password",
		Group: "default", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id: 19001, Type: constant.ChannelTypeOpenAI, Name: "public-model-channel",
		Models: strings.Join(modelNames, ","), Group: "default", Status: common.ChannelStatusEnabled,
	}).Error)

	abilities := make([]model.Ability, 0, len(modelNames))
	for _, modelName := range modelNames {
		abilities = append(abilities, model.Ability{
			Group: "default", Model: modelName, ChannelId: 19001, Enabled: true,
		})
	}
	require.NoError(t, db.Create(&abilities).Error)
}

func TestListModelsReturnsOnlyPublicCatalogWithDoubaoOwner(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)
	modelNames := append([]string(nil), modelrouting.CanonicalModels...)
	modelNames = append(modelNames, "provider-hidden")
	seedPublicModelAbilities(t, db, modelNames)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	ListModels(ctx, constant.ChannelTypeOpenAI)

	payload := decodeListModelsPayload(t, recorder)
	require.Len(t, payload.Data, 3)
	for index, modelName := range modelrouting.CanonicalModels {
		assert.Equal(t, modelName, payload.Data[index].Id)
		assert.Equal(t, modelrouting.PublicModelOwner, payload.Data[index].OwnedBy)
	}
}

func TestListModelsCompatibilityFormatsHideInternalModels(t *testing.T) {
	for _, modelType := range []int{
		constant.ChannelTypeAnthropic,
		constant.ChannelTypeGemini,
	} {
		t.Run(constant.GetChannelTypeName(modelType), func(t *testing.T) {
			withSelfUseModeEnabled(t)
			db := setupModelListControllerTestDB(t)
			modelNames := append([]string(nil), modelrouting.CanonicalModels...)
			modelNames = append(modelNames, "provider-hidden")
			seedPublicModelAbilities(t, db, modelNames)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			ListModels(ctx, modelType)

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "provider-hidden")
			for _, modelName := range modelrouting.CanonicalModels {
				assert.Contains(t, recorder.Body.String(), modelName)
			}
		})
	}
}

func TestListModelsTokenLimitCannotExposeInternalModel(t *testing.T) {
	withSelfUseModeEnabled(t)
	setupModelListControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		"provider-hidden": true,
	})

	ListModels(ctx, constant.ChannelTypeOpenAI)

	payload := decodeListModelsPayload(t, recorder)
	require.Empty(t, payload.Data)
}

func TestListModelsEmptyAnthropicCatalogDoesNotPanic(t *testing.T) {
	withSelfUseModeEnabled(t)
	setupModelListControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		"provider-hidden": true,
	})

	ListModels(ctx, constant.ChannelTypeAnthropic)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Data    []dto.AnthropicModel `json:"data"`
		HasMore bool                 `json:"has_more"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Empty(t, payload.Data)
	assert.False(t, payload.HasMore)
}

func TestRetrieveModelRejectsInternalModelAndReturnsPublicModel(t *testing.T) {
	hiddenRecorder := httptest.NewRecorder()
	hiddenContext, _ := gin.CreateTestContext(hiddenRecorder)
	hiddenContext.Params = gin.Params{{Key: "model", Value: "provider-hidden"}}

	RetrieveModel(hiddenContext, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, hiddenRecorder.Code)
	var hiddenPayload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(hiddenRecorder.Body.Bytes(), &hiddenPayload))
	assert.Equal(t, "model_not_found", hiddenPayload.Error.Code)

	publicRecorder := httptest.NewRecorder()
	publicContext, _ := gin.CreateTestContext(publicRecorder)
	publicContext.Params = gin.Params{{Key: "model", Value: modelrouting.Seedance20}}

	RetrieveModel(publicContext, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, publicRecorder.Code)
	var publicModel dto.OpenAIModels
	require.NoError(t, common.Unmarshal(publicRecorder.Body.Bytes(), &publicModel))
	assert.Equal(t, modelrouting.Seedance20, publicModel.Id)
	assert.Equal(t, modelrouting.PublicModelOwner, publicModel.OwnedBy)
}

func TestGetUserModelsReturnsOnlyPublicCatalog(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	modelNames := append([]string(nil), modelrouting.CanonicalModels...)
	modelNames = append(modelNames, "provider-hidden")
	seedPublicModelAbilities(t, db, modelNames)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?group=default", nil)
	ctx.Set("id", 19001)

	GetUserModels(ctx)

	require.Equal(t, modelrouting.CanonicalModels, decodeUserModelsResponse(t, recorder))
}

func TestDashboardListModelsReturnsOnlyPublicModelsPerChannel(t *testing.T) {
	original := channelId2Models
	channelId2Models = map[int][]string{
		constant.ChannelTypeOpenAI: {
			"provider-hidden", modelrouting.Seedance20Mini,
			modelrouting.Seedance20, modelrouting.Seedance20Fast,
		},
		constant.ChannelTypeAnthropic: {"provider-only"},
	}
	t.Cleanup(func() { channelId2Models = original })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	DashboardListModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool             `json:"success"`
		Data    map[int][]string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	assert.Equal(t, modelrouting.CanonicalModels, payload.Data[constant.ChannelTypeOpenAI])
	assert.Empty(t, payload.Data[constant.ChannelTypeAnthropic])
}

func TestChannelListModelsKeepsInternalCatalogForAdmins(t *testing.T) {
	original := openAIModels
	openAIModels = []dto.OpenAIModels{{Id: "provider-hidden", Object: "model"}}
	t.Cleanup(func() { openAIModels = original })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ChannelListModels(ctx)

	require.Contains(t, recorder.Body.String(), "provider-hidden")
}
```

- [ ] **Step 2: 运行测试并确认当前响应仍包含上游模型**

Run: `go test ./controller -run 'Test(ListModelsReturnsOnlyPublic|ListModelsCompatibilityFormats|ListModelsTokenLimitCannotExpose|ListModelsEmptyAnthropic|RetrieveModelRejectsInternal|GetUserModelsReturnsOnlyPublic)' -count=1`

Expected: FAIL；至少一个断言观察到 `provider-hidden`，且旧 `RetrieveModel` 无法正确返回自定义公共模型。

- [ ] **Step 3: 过滤兼容模型列表并固定所有者**

在 `controller/model.go:ListModels` 收集并完成计费过滤后加入：

```go
userModelNames = modelrouting.FilterPublicModels(userModelNames)
ownerByModel := make(map[string]string, len(userModelNames))
for _, modelName := range userModelNames {
	ownerByModel[modelName] = modelrouting.PublicModelOwner
}
```

同步添加 `github.com/QuantumNous/new-api/pkg/modelrouting` import，不再为公共响应调用 `getPreferredModelOwners`。Anthropic 分支必须保护空切片：

```go
response := gin.H{"data": userAnthropicModels, "has_more": false}
if len(userAnthropicModels) > 0 {
	response["first_id"] = userAnthropicModels[0].ID
	response["last_id"] = userAnthropicModels[len(userAnthropicModels)-1].ID
}
c.JSON(http.StatusOK, response)
```

- [ ] **Step 4: 收紧单模型和 Dashboard 模型响应**

将 `DashboardListModels` 改为新建响应 map，禁止修改共享 `channelId2Models`：

```go
func DashboardListModels(c *gin.Context) {
	publicModelsByChannel := make(map[int][]string, len(channelId2Models))
	for channelType, modelNames := range channelId2Models {
		publicModelsByChannel[channelType] = modelrouting.FilterPublicModels(modelNames)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    publicModelsByChannel,
	})
}
```

将 `RetrieveModel` 改为先执行精确公共目录检查，再使用统一公开所有者构造结果；非公共 ID 保持既有 OpenAI `model_not_found` 响应结构：

```go
func RetrieveModel(c *gin.Context, modelType int) {
	modelId := c.Param("model")
	if !modelrouting.IsPublicModel(modelId) {
		openAIError := types.OpenAIError{
			Message: fmt.Sprintf("The model '%s' does not exist", modelId),
			Type:    "invalid_request_error",
			Param:   "model",
			Code:    "model_not_found",
		}
		c.JSON(http.StatusOK, gin.H{"error": openAIError})
		return
	}

	aiModel := buildOpenAIModel(modelId, map[string]string{
		modelId: modelrouting.PublicModelOwner,
	})
	switch modelType {
	case constant.ChannelTypeAnthropic:
		c.JSON(http.StatusOK, dto.AnthropicModel{
			ID:          aiModel.Id,
			CreatedAt:   time.Unix(int64(aiModel.Created), 0).UTC().Format(time.RFC3339),
			DisplayName: aiModel.Id,
			Type:        "model",
		})
	default:
		c.JSON(http.StatusOK, aiModel)
	}
}
```

- [ ] **Step 5: 过滤用户模型接口**

在 `controller/user.go:GetUserModels` 返回前应用公共目录：

```go
userModels := service.GetGroupsEnabledModels(groupsToQuery)
c.JSON(http.StatusOK, gin.H{
	"success": true,
	"message": "",
	"data":    modelrouting.FilterPublicModels(userModels),
})
```

同步添加 `github.com/QuantumNous/new-api/pkg/modelrouting` import。

- [ ] **Step 6: 更新既有公开列表测试而不削弱内部计费契约**

在 `controller/model_list_test.go` 添加 `pkg/modelrouting` import，并作以下完整语义替换：

```go
func TestListModelsIncludesDurationBillingModel(t *testing.T) {
	withSelfUseModeDisabled(t)
	db := setupModelListControllerTestDB(t)

	const modelName = modelrouting.Seedance20
	rule := types.DurationPrice{
		Price: 0.25, Unit: types.DurationUnitMinute,
		RoundingStepSeconds: 5, MinimumDurationSeconds: 10,
	}
	withDurationBillingConfig(t,
		map[string]string{modelName: billing_setting.BillingModePerDuration},
		map[string]types.DurationPrice{modelName: rule},
	)
	require.NoError(t, db.Create(&model.User{
		Id: 1005, Username: "duration-model-user", Password: "password",
		Group: "default", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id: 1005, Type: constant.ChannelTypeOpenAI, Name: "duration-model-channel",
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: modelName, ChannelId: 1005, Enabled: true,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 1005)
	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, modelName)
	pricing, ok := pricingByModelName(model.GetPricing())[modelName]
	require.True(t, ok)
	assert.Equal(t, billing_setting.BillingModePerDuration, pricing.BillingMode)
	require.NotNil(t, pricing.DurationPrice)
	assert.Equal(t, rule, *pricing.DurationPrice)
	assert.Zero(t, pricing.ModelPrice)
	assert.Zero(t, pricing.ModelRatio)
	assert.Zero(t, pricing.CompletionRatio)
	assert.Equal(t, 1, pricing.QuotaType)
}

func TestGetUserModelsFiltersByRequestedGroup(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id: 1002, Username: "playground-model-user", Password: "password",
		Group: "default", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: modelrouting.Seedance20, ChannelId: 1, Enabled: true},
		{Group: "default", Model: modelrouting.Seedance20Fast, ChannelId: 1, Enabled: false},
	}).Error)

	defaultRecorder := httptest.NewRecorder()
	defaultContext, _ := gin.CreateTestContext(defaultRecorder)
	defaultContext.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?group=default", nil)
	defaultContext.Set("id", 1002)
	GetUserModels(defaultContext)
	require.Equal(t, []string{modelrouting.Seedance20}, decodeUserModelsResponse(t, defaultRecorder))

	vipRecorder := httptest.NewRecorder()
	vipContext, _ := gin.CreateTestContext(vipRecorder)
	vipContext.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?group=vip", nil)
	vipContext.Set("id", 1002)
	GetUserModels(vipContext)
	require.Empty(t, decodeUserModelsResponse(t, vipRecorder))
}

func TestGetUserModelsProjectsAutoGroupsInPublicCatalogOrder(t *testing.T) {
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalSpecialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
		specialGroups.Clear()
		specialGroups.AddAll(originalSpecialGroups)
	})

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip","default","unavailable"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"auto":"自动分组","default":"默认分组","unavailable":"不可用分组"}`))
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	specialGroups.Clear()
	specialGroups.Set("default", map[string]string{
		"+:vip":         "VIP 分组",
		"-:unavailable": "",
	})

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id: 1003, Username: "playground-auto-model-user", Password: "password",
		Group: "default", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "vip", Model: modelrouting.Seedance20Fast, ChannelId: 1, Enabled: true},
		{Group: "vip", Model: modelrouting.Seedance20, ChannelId: 1, Enabled: true},
		{Group: "default", Model: modelrouting.Seedance20Mini, ChannelId: 1, Enabled: true},
		{Group: "default", Model: modelrouting.Seedance20, ChannelId: 2, Enabled: true},
		{Group: "unavailable", Model: "provider-hidden", ChannelId: 1, Enabled: true},
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?group=auto", nil)
	ctx.Set("id", 1003)
	GetUserModels(ctx)
	require.Equal(t, modelrouting.CanonicalModels, decodeUserModelsResponse(t, recorder))
}

func TestListModelsIncludesTieredBillingModel(t *testing.T) {
	withSelfUseModeDisabled(t)
	withTieredBillingConfig(t, map[string]string{
		modelrouting.Seedance20:     "tiered_expr",
		modelrouting.Seedance20Fast: "tiered_expr",
		modelrouting.Seedance20Mini: "tiered_expr",
	}, map[string]string{
		modelrouting.Seedance20:     `tier("base", p * 1 + c * 2)`,
		modelrouting.Seedance20Fast: "   ",
	})

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id: 1001, Username: "model-list-user", Password: "password",
		Group: "default", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id: 1001, Type: constant.ChannelTypeOpenAI, Name: "tiered-model-channel",
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: modelrouting.Seedance20, ChannelId: 1001, Enabled: true},
		{Group: "default", Model: modelrouting.Seedance20Fast, ChannelId: 1001, Enabled: true},
		{Group: "default", Model: modelrouting.Seedance20Mini, ChannelId: 1001, Enabled: true},
		{Group: "default", Model: "zz-unpriced-model", ChannelId: 1001, Enabled: true},
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 1001)
	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, modelrouting.Seedance20)
	require.NotContains(t, ids, modelrouting.Seedance20Fast)
	require.NotContains(t, ids, modelrouting.Seedance20Mini)
	require.NotContains(t, ids, "zz-unpriced-model")

	pricingByName := pricingByModelName(model.GetPricing())
	visiblePricing, ok := pricingByName[modelrouting.Seedance20]
	require.True(t, ok)
	require.Equal(t, "tiered_expr", visiblePricing.BillingMode)
	require.NotEmpty(t, visiblePricing.BillingExpr)

	emptyExprPricing, ok := pricingByName[modelrouting.Seedance20Fast]
	require.True(t, ok)
	require.Empty(t, emptyExprPricing.BillingMode)
	require.Empty(t, emptyExprPricing.BillingExpr)

	missingExprPricing, ok := pricingByName[modelrouting.Seedance20Mini]
	require.True(t, ok)
	require.Empty(t, missingExprPricing.BillingMode)
	require.Empty(t, missingExprPricing.BillingExpr)
}

func TestListModelsUsesAdvancedCustomEndpointTypesFromPricingCache(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)

	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		model.InvalidatePricingCache()
	})

	require.NoError(t, db.Create(&model.User{
		Id: 1003, Username: "advanced-custom-model-list-user", Password: "password",
		Group: "default", Status: common.UserStatusEnabled,
	}).Error)
	channel := &model.Channel{
		Id: 701, Type: constant.ChannelTypeAdvancedCustom, Key: "advanced-custom-key",
		Status: common.ChannelStatusEnabled, Name: "advanced-custom-channel",
		Group: "default", Models: modelrouting.Seedance20,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{IncomingPath: "/v1/chat/completions", UpstreamPath: "/v1/chat/completions"},
				{
					IncomingPath: "/v1/responses",
					UpstreamPath: "/v1beta/models/{model}:generateContent",
					Converter:    "openai_responses_to_gemini_generate_content",
					Models:       []string{modelrouting.Seedance20},
				},
			},
		},
	})
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: modelrouting.Seedance20, ChannelId: 701, Enabled: true,
	}).Error)

	model.InitChannelCache()
	model.GetPricing()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 1003)
	ListModels(ctx, constant.ChannelTypeOpenAI)

	payload := decodeListModelsPayload(t, recorder)
	require.Len(t, payload.Data, 1)
	require.Equal(t, modelrouting.Seedance20, payload.Data[0].Id)
	require.Equal(t, []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
	}, payload.Data[0].SupportedEndpointTypes)
}

func TestListModelsTokenLimitIncludesTieredBillingModel(t *testing.T) {
	withSelfUseModeDisabled(t)
	withTieredBillingConfig(t, map[string]string{
		modelrouting.Seedance20:     "tiered_expr",
		modelrouting.Seedance20Fast: "tiered_expr",
		modelrouting.Seedance20Mini: "tiered_expr",
	}, map[string]string{
		modelrouting.Seedance20:     `tier("base", p * 1 + c * 2)`,
		modelrouting.Seedance20Fast: "",
	})
	setupModelListControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		modelrouting.Seedance20:     true,
		modelrouting.Seedance20Fast: true,
		modelrouting.Seedance20Mini: true,
		"zz-token-unpriced-model":  true,
	})
	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, modelrouting.Seedance20)
	require.NotContains(t, ids, modelrouting.Seedance20Fast)
	require.NotContains(t, ids, modelrouting.Seedance20Mini)
	require.NotContains(t, ids, "zz-token-unpriced-model")
}
```

保留对 `model.GetPricing()` 的断言，确保内部定价缓存仍识别按时长、分层表达式和 Advanced Custom 端点；只改变公开控制器的预期。

- [ ] **Step 7: 运行控制器回归测试**

Run: `go test ./controller -run 'Test(ListModels|RetrieveModel|GetUserModels|DashboardListModels|ProjectPublicPricing)' -count=1`

Expected: PASS；空 Anthropic 列表不 panic，所有用户列表只含公共 ID。

- [ ] **Step 8: 提交用户模型列表边界**

```powershell
git add -- controller/public_models_test.go controller/model_list_test.go controller/model.go controller/user.go
git commit -m "feat: restrict user model lists to public catalog"
```

### Task 4: 在渠道选择前拒绝上游模型 ID

**Files:**
- Modify: `controller/cost_routing_test.go`
- Modify: `middleware/distributor_routing_test.go`
- Modify: `middleware/distributor.go`

- [ ] **Step 1: 编写直接调用失败测试**

在 `middleware/distributor_routing_test.go` 添加：

```go
func TestDistributeRejectsInternalModelBeforeChannelSelection(t *testing.T) {
	prepareDistributorRoutingTest(t)
	seedDistributorRoutingChannel(t, 11, "A1", 100)

	recorder, reached := runDistributorRoutingRequest(t, "", `{
		"model":"provider-hidden",
		"content":[{"type":"text","text":"video"}],
		"resolution":"720p","duration":10,"ratio":"16:9"
	}`)

	assert.False(t, reached)
	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"model_not_found"`)
	assert.NotContains(t, recorder.Body.String(), "channel")
	assert.NotContains(t, recorder.Body.String(), "provider-hidden")
}

func TestDistributeAllowsAllPublicModels(t *testing.T) {
	for index, modelName := range modelrouting.CanonicalModels {
		t.Run(modelName, func(t *testing.T) {
			prepareDistributorRoutingTest(t)
			channelID := 30 + index
			seedDistributorRoutingChannelModel(t, channelID, "public", 100, modelName)
			request := distributorRoutingPolicyRequestForModel(modelName)
			request.Targets = []service.RouteTargetWriteRequest{
				distributorRoutingTarget(channelID, "provider-public", "720p"),
			}
			_, err := service.SaveRoutingPolicy(0, request)
			require.NoError(t, err)

			body := fmt.Sprintf(`{
				"model":%q,
				"content":[{"type":"text","text":"video"}],
				"resolution":"720p","duration":10,"ratio":"16:9"
			}`, modelName)
			recorder, reached := runDistributorRoutingRequest(t, "", body)

			assert.True(t, reached)
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, strconv.Itoa(channelID), recorder.Header().Get("X-Selected-Channel"))
		})
	}
}
```

同步为 `middleware/distributor_routing_test.go` 添加 `fmt` import，并将现有 fixture 扩展为可指定模型，保留原 helper 供既有测试使用：

```go
func seedDistributorRoutingChannel(t *testing.T, id int, name string, priority int64) {
	t.Helper()
	seedDistributorRoutingChannelModel(t, id, name, priority, modelrouting.Seedance20)
}

func seedDistributorRoutingChannelModel(t *testing.T, id int, name string, priority int64, modelName string) {
	t.Helper()
	weight := uint(100)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: id, Type: constant.ChannelTypeNewAPIVideo, Key: "secret", Status: common.ChannelStatusEnabled,
		Name: name, Models: modelName, Group: "分组A", Priority: &priority, Weight: &weight,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "分组A", Model: modelName, ChannelId: id, Enabled: true,
		Priority: &priority, Weight: weight,
	}).Error)
}

func distributorRoutingPolicyRequest() service.RoutingPolicyWriteRequest {
	return distributorRoutingPolicyRequestForModel(modelrouting.Seedance20)
}

func distributorRoutingPolicyRequestForModel(modelName string) service.RoutingPolicyWriteRequest {
	return service.RoutingPolicyWriteRequest{
		GroupName: "分组A",
		Model:     modelName,
		Enabled:   true,
		Defaults: modelrouting.Defaults{
			OutputResolution: "720p",
			DurationSeconds:  10,
			AspectRatio:      "16:9",
		},
	}
}
```

- [ ] **Step 2: 运行测试并确认内部模型尚未被边界拒绝**

Run: `go test ./middleware -run 'TestDistribute(RejectsInternalModel|AllowsPublicModels)' -count=1`

Expected: FAIL；内部模型请求进入现有渠道选择路径或返回非 `model_not_found` 错误。

- [ ] **Step 3: 在 `Distribute` 最前端增加精确校验**

在 `getModelRequest` 成功后、`extractSeedanceRoutingInput` 和 Token 模型限制之前加入：

```go
if shouldSelectChannel && modelRequest.Model != "" && !modelrouting.IsPublicModel(modelRequest.Model) {
	abortSeedanceRoutingError(
		c,
		http.StatusNotFound,
		types.ErrorCodeModelNotFound,
		"model is not available",
	)
	return
}
```

保留 `modelRequest.Model == ""` 的现有“模型名必填”错误；任务查询、内容下载和 Remix 等 `shouldSelectChannel == false` 流程不执行新校验。

- [ ] **Step 4: 让既有成本路由集成测试使用公共入口模型**

在 `controller/cost_routing_test.go` 添加 `pkg/modelrouting` import。新增稳定的测试映射构造器：

```go
func costRoutingModelMapping(t *testing.T, upstreamModel string) string {
	t.Helper()
	mapping, err := common.Marshal(map[string]string{
		modelrouting.Seedance20: upstreamModel,
	})
	require.NoError(t, err)
	return string(mapping)
}
```

将六处成本路由测试的映射参数分别改为：

```go
costRoutingModelMapping(t, "missing-model")
costRoutingModelMapping(t, "covered-model")
costRoutingModelMapping(t, "supplier-secret-model")
```

将同文件的公共入口字段统一改为 `modelrouting.Seedance20`，上游成本规则字段继续使用 `missing-model`、`covered-model` 和 `supplier-secret-model`：

```go
// seedCostRoutingChannelForGroup
model.Ability{
	Group: group, Model: modelrouting.Seedance20, ChannelId: channelID, Enabled: true,
	Priority: &priority, Weight: weight,
}

// costRoutingRelayInfo
&relaycommon.RelayInfo{
	ChannelMeta: &relaycommon.ChannelMeta{}, OriginModelName: modelrouting.Seedance20,
	TokenGroup: "default", UserGroup: "default", UsingGroup: "default",
}

// costRoutingRetryParam
&service.RetryParam{
	Ctx: c, TokenGroup: "default", ModelName: modelrouting.Seedance20,
	RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
}

// performCostRoutingDistribution
body, err := common.Marshal(map[string]string{"model": modelrouting.Seedance20})
require.NoError(t, err)
request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
```

同步用 `bytes.NewReader` 替换该 helper 中的 `strings.NewReader`；文件其他隐私断言保持不变。

- [ ] **Step 5: 运行中间件与路由测试**

Run: `go test ./middleware ./controller -run 'Test(Distribute|ExtractSeedanceRoutingInput|CostRoutingDistributor)' -count=1`

Expected: PASS；内部 ID 在渠道查询前被拒绝，三个公共 ID 继续正常路由。

- [ ] **Step 6: 提交公共请求边界**

```powershell
git add -- controller/cost_routing_test.go middleware/distributor.go middleware/distributor_routing_test.go
git commit -m "feat: reject non-public relay models"
```

### Task 5: 同步公开 API 文档并完成验收

**Files:**
- Modify: `docs/api/video-generation.md`

- [ ] **Step 1: 记录公共模型和迁移行为**

在端点概览之后新增“公共模型目录”章节，内容必须包含：

```markdown
## 公共模型目录

客户端只使用以下三个模型 ID：

- `doubao-seedance-2-0-260128`
- `doubao-seedance-2-0-fast-260128`
- `doubao-seedance-2-0-mini-260615`

切换服务时只需替换 Base URL 和 API Key。渠道模型 ID 是内部路由实现，不会出现在模型广场或 `/v1/models`；直接请求渠道模型 ID 返回 `model_not_found`。旧 Mini ID `doubao-seedance-2-0-mini-260128` 不属于公共兼容 ID。
```

- [ ] **Step 2: 格式化并运行聚焦测试**

Run:

```powershell
gofmt -w pkg/modelrouting/public.go pkg/modelrouting/public_test.go controller/public_models.go controller/public_models_test.go controller/pricing.go controller/model.go controller/model_list_test.go controller/user.go controller/cost_routing_test.go middleware/distributor.go middleware/distributor_routing_test.go
go test ./pkg/modelrouting ./controller ./middleware ./model -count=1
git diff --check
```

Expected: 所有命令退出码为 0。

- [ ] **Step 3: 构建前端并运行全量 Go 测试**

Run:

```powershell
Set-Location web
bun install --frozen-lockfile
bun run build:check
Set-Location ..
go test ./... -count=1
```

Expected: Rsbuild 生产构建成功；全量 Go 测试无失败。前端构建先生成 `web/dist/index.html`，满足 Go embed 编译要求。

- [ ] **Step 4: 提交文档与最终整理**

```powershell
git add -- docs/api/video-generation.md
git commit -m "docs: document public Seedance model catalog"
git status --short --branch
```

Expected: 功能分支没有未提交的任务改动。

- [ ] **Step 5: 合并到本地 `ysr` 并更新本地容器**

使用 `superpowers:finishing-a-development-branch` 完成分支检查，只允许将 `codex/public-model-boundary` 合并到本地 `ysr`，禁止提交或合并到 `main`。合并后在主工作区执行：

```powershell
docker compose -f docker-compose.local.yml up -d --build new-api video-metadata
docker compose -f docker-compose.local.yml ps
```

Expected: `new-api`、`video-metadata`、MySQL 和 Redis 均为 `healthy`。

- [ ] **Step 6: API 与浏览器验收**

Run:

```powershell
$pricing = Invoke-RestMethod -Uri 'http://127.0.0.1:3000/api/pricing' -Method Get
@($pricing.data).model_name
```

Expected: 只输出三个公共 ID，顺序为 2.0、2.0 Fast、2.0 Mini。使用有效 API Key 调用 `/v1/models` 时同样只返回三个 ID；以任一已知上游模型 ID 提交公共 Relay 请求时返回 `model_not_found`。

最后在 `http://127.0.0.1:3000/pricing` 验证：标题统计为 3，每张模型卡对应一个公共 ID，供应商为 Doubao，页面中不存在 `mg-`、`jimeng-`、`lec-`、`dimensio-` 等上游模型。
